package zabbix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maximumResponseBytes = 4 << 20
	maximumHosts         = 5000
	maximumItems         = 20000
)

type Interface struct {
	Type    string `json:"type"`
	Address string `json:"address"`
	Port    string `json:"port,omitempty"`
	Main    bool   `json:"main"`
}

type Host struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Technical  string      `json:"technical_name"`
	Interfaces []Interface `json:"interfaces"`
}

type Inspection struct {
	Endpoint           string `json:"endpoint"`
	Version            string `json:"version"`
	Compatibility      string `json:"compatibility"`
	CompatibilityLabel string `json:"compatibility_label"`
	EncryptedTransport bool   `json:"encrypted_transport"`
	Hosts              []Host `json:"hosts"`
}

type Problem struct {
	EventID           string
	TriggerID         string
	NatureFingerprint string
	CanonicalNature   string
	Name              string
	Severity          int
	Acknowledged      bool
	Suppressed        bool
	StartedAt         time.Time
	HostIDs           []string
}

type remoteTrigger struct {
	TriggerID          string          `json:"triggerid"`
	TemplateID         string          `json:"templateid"`
	UUID               string          `json:"uuid"`
	Description        string          `json:"description"`
	Expression         string          `json:"expression"`
	RecoveryExpression string          `json:"recovery_expression"`
	CorrelationTag     string          `json:"correlation_tag"`
	Flags              string          `json:"flags"`
	DiscoveryData      json.RawMessage `json:"discoveryData"`
	Hosts              []struct {
		HostID string `json:"hostid"`
	} `json:"hosts"`
	Items []struct {
		ItemID string `json:"itemid"`
		Key    string `json:"key_"`
	} `json:"items"`
	Functions []struct {
		ItemID    string `json:"itemid"`
		Function  string `json:"function"`
		Parameter string `json:"parameter"`
	} `json:"functions"`
	Tags []struct {
		Tag   string `json:"tag"`
		Value string `json:"value"`
	} `json:"tags"`
}

// Item est la projection numérique minimale utilisée par les Indicateurs.
// L'identifiant Zabbix reste l'identité durable : la clé et le nom servent à
// proposer une sémantique, jamais à remplacer silencieusement un item disparu.
type Item struct {
	ID        string     `json:"id"`
	HostID    string     `json:"host_id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	Units     string     `json:"units,omitempty"`
	ValueType int        `json:"value_type"`
	LastValue *float64   `json:"last_value,omitempty"`
	LastClock *time.Time `json:"last_observed_at,omitempty"`
}

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 8 * time.Second
	transport.MaxResponseHeaderBytes = 64 * 1024
	return &Client{http: &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func NewClientWithHTTP(client *http.Client) *Client {
	return &Client{http: client}
}

func NormalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", fmt.Errorf("address must contain between 1 and 2048 characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("address must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("address must not contain credentials, query parameters, or a fragment")
	}
	if strings.ContainsAny(parsed.Host, "\r\n\t ") {
		return "", fmt.Errorf("address contains an invalid host")
	}
	cleanPath := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if !strings.HasSuffix(cleanPath, "/api_jsonrpc.php") && cleanPath != "api_jsonrpc.php" {
		cleanPath = path.Join(cleanPath, "api_jsonrpc.php")
		if !strings.HasPrefix(cleanPath, "/") {
			cleanPath = "/" + cleanPath
		}
	}
	parsed.Path = cleanPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

func (client *Client) Inspect(ctx context.Context, address, token string) (Inspection, error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return Inspection{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 4096 {
		return Inspection{}, fmt.Errorf("API token must contain between 1 and 4096 characters")
	}

	var version string
	if err := client.call(ctx, endpoint, "", "apiinfo.version", []any{}, &version); err != nil {
		return Inspection{}, fmt.Errorf("read Zabbix API version: %w", err)
	}
	if strings.TrimSpace(version) == "" || len(version) > 64 {
		return Inspection{}, fmt.Errorf("read Zabbix API version: invalid response")
	}

	var remoteHosts []struct {
		HostID     string `json:"hostid"`
		Host       string `json:"host"`
		Name       string `json:"name"`
		Interfaces []struct {
			Type  string `json:"type"`
			UseIP string `json:"useip"`
			IP    string `json:"ip"`
			DNS   string `json:"dns"`
			Port  string `json:"port"`
			Main  string `json:"main"`
		} `json:"interfaces"`
	}
	params := map[string]any{
		"output":           []string{"hostid", "host", "name"},
		"selectInterfaces": []string{"type", "useip", "ip", "dns", "port", "main"},
		"monitored_hosts":  true,
		"sortfield":        "name",
		"limit":            maximumHosts + 1,
	}
	if err := client.call(ctx, endpoint, token, "host.get", params, &remoteHosts); err != nil {
		return Inspection{}, fmt.Errorf("discover monitored Zabbix hosts: %w", err)
	}
	if len(remoteHosts) > maximumHosts {
		return Inspection{}, fmt.Errorf("discover monitored Zabbix hosts: more than %d hosts are visible to this token", maximumHosts)
	}

	hosts := make([]Host, 0, len(remoteHosts))
	hostIDs := make([]string, 0, len(remoteHosts))
	for _, remote := range remoteHosts {
		if _, err := strconv.ParseUint(remote.HostID, 10, 64); err != nil {
			return Inspection{}, fmt.Errorf("discover monitored Zabbix hosts: invalid host identity")
		}
		displayName := strings.TrimSpace(remote.Name)
		if displayName == "" {
			displayName = strings.TrimSpace(remote.Host)
		}
		if displayName == "" || len(displayName) > 160 {
			return Inspection{}, fmt.Errorf("discover monitored Zabbix hosts: invalid host name")
		}
		host := Host{ID: remote.HostID, Name: displayName, Technical: strings.TrimSpace(remote.Host), Interfaces: make([]Interface, 0, len(remote.Interfaces))}
		hostIDs = append(hostIDs, remote.HostID)
		for _, remoteInterface := range remote.Interfaces {
			address := strings.TrimSpace(remoteInterface.IP)
			if remoteInterface.UseIP == "0" {
				address = strings.TrimSpace(remoteInterface.DNS)
			}
			if address == "" {
				continue
			}
			host.Interfaces = append(host.Interfaces, Interface{
				Type: interfaceType(remoteInterface.Type), Address: address,
				Port: strings.TrimSpace(remoteInterface.Port), Main: remoteInterface.Main == "1",
			})
		}
		hosts = append(hosts, host)
	}
	sort.SliceStable(hosts, func(i, j int) bool {
		return strings.ToLower(hosts[i].Name) < strings.ToLower(hosts[j].Name)
	})
	var problemProbe []struct {
		EventID string `json:"eventid"`
	}
	if err := client.call(ctx, endpoint, token, "problem.get", map[string]any{
		"output": []string{"eventid"}, "hostids": hostIDs, "recent": false, "limit": 1,
	}, &problemProbe); err != nil {
		return Inspection{}, fmt.Errorf("verify Zabbix problem access: %w", err)
	}
	var natureProbe []remoteTrigger
	if err := client.call(ctx, endpoint, token, "trigger.get", map[string]any{
		"output": []string{
			"triggerid", "templateid", "uuid", "description", "expression",
			"recovery_expression", "correlation_tag", "flags",
		},
		"hostids":             hostIDs,
		"monitored":           true,
		"selectDiscoveryData": []string{"parent_triggerid"},
		"selectItems":         []string{"itemid", "key_"},
		"selectFunctions":     []string{"itemid", "function", "parameter"},
		"selectTags":          []string{"tag", "value"},
		"limit":               1,
	}, &natureProbe); err != nil {
		return Inspection{}, fmt.Errorf("verify stable Zabbix Nature metadata: %w", err)
	}

	compatibility, label := compatibility(version)
	return Inspection{
		Endpoint: endpoint, Version: version, Compatibility: compatibility,
		CompatibilityLabel: label, EncryptedTransport: strings.HasPrefix(endpoint, "https://"), Hosts: hosts,
	}, nil
}

func (client *Client) Problems(ctx context.Context, endpoint, token string, hostIDs []string) ([]Problem, error) {
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 4096 {
		return nil, fmt.Errorf("API token must contain between 1 and 4096 characters")
	}
	if len(hostIDs) == 0 {
		return []Problem{}, nil
	}
	if len(hostIDs) > maximumHosts {
		return nil, fmt.Errorf("problem scope exceeds %d hosts", maximumHosts)
	}
	allowedHosts := make(map[string]struct{}, len(hostIDs))
	cleanHostIDs := make([]string, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		if _, err := strconv.ParseUint(hostID, 10, 64); err != nil {
			return nil, fmt.Errorf("problem scope contains an invalid host identity")
		}
		if _, duplicate := allowedHosts[hostID]; duplicate {
			continue
		}
		allowedHosts[hostID] = struct{}{}
		cleanHostIDs = append(cleanHostIDs, hostID)
	}

	var remoteProblems []struct {
		EventID      string `json:"eventid"`
		ObjectID     string `json:"objectid"`
		Clock        string `json:"clock"`
		Name         string `json:"name"`
		Acknowledged string `json:"acknowledged"`
		Severity     string `json:"severity"`
		Suppressed   string `json:"suppressed"`
	}
	if err := client.call(ctx, endpoint, token, "problem.get", map[string]any{
		"output":  []string{"eventid", "objectid", "clock", "name", "acknowledged", "severity", "suppressed"},
		"hostids": cleanHostIDs, "recent": false, "sortfield": "eventid", "sortorder": "ASC",
		"limit": maximumHosts + 1,
	}, &remoteProblems); err != nil {
		return nil, fmt.Errorf("retrieve active Zabbix problems: %w", err)
	}
	if len(remoteProblems) > maximumHosts {
		return nil, fmt.Errorf("more than %d active Zabbix problems are visible", maximumHosts)
	}
	if len(remoteProblems) == 0 {
		return []Problem{}, nil
	}

	triggerIDs := make([]string, 0, len(remoteProblems))
	triggerSeen := make(map[string]struct{}, len(remoteProblems))
	for _, problem := range remoteProblems {
		if _, err := strconv.ParseUint(problem.EventID, 10, 64); err != nil {
			return nil, fmt.Errorf("retrieve active Zabbix problems: invalid event identity")
		}
		if _, err := strconv.ParseUint(problem.ObjectID, 10, 64); err != nil {
			return nil, fmt.Errorf("retrieve active Zabbix problems: invalid trigger identity")
		}
		if _, duplicate := triggerSeen[problem.ObjectID]; !duplicate {
			triggerSeen[problem.ObjectID] = struct{}{}
			triggerIDs = append(triggerIDs, problem.ObjectID)
		}
	}
	remoteTriggers, detailed, err := client.triggers(ctx, endpoint, token, triggerIDs, true)
	if err != nil {
		return nil, fmt.Errorf("resolve Zabbix problem hosts: %w", err)
	}
	hostsByTrigger := make(map[string][]string, len(remoteTriggers))
	triggerByID := make(map[string]remoteTrigger, len(remoteTriggers))
	for _, trigger := range remoteTriggers {
		triggerByID[trigger.TriggerID] = trigger
		if _, expected := triggerSeen[trigger.TriggerID]; !expected {
			continue
		}
		for _, host := range trigger.Hosts {
			if _, allowed := allowedHosts[host.HostID]; allowed {
				hostsByTrigger[trigger.TriggerID] = append(hostsByTrigger[trigger.TriggerID], host.HostID)
			}
		}
		sort.Strings(hostsByTrigger[trigger.TriggerID])
	}
	if detailed {
		if err := client.loadTriggerAncestors(ctx, endpoint, token, triggerByID); err != nil {
			// Une version Zabbix qui ne sait pas exposer les ancêtres continue de
			// superviser. Sa Nature reste locale au trigger plutôt que déduite du
			// libellé, donc sûre mais moins regroupante.
			detailed = false
		}
	}

	problems := make([]Problem, 0, len(remoteProblems))
	for _, remote := range remoteProblems {
		name := strings.TrimSpace(remote.Name)
		if name == "" || len(name) > 512 {
			return nil, fmt.Errorf("retrieve active Zabbix problems: invalid problem name")
		}
		clock, err := strconv.ParseInt(remote.Clock, 10, 64)
		if err != nil || clock <= 0 {
			return nil, fmt.Errorf("retrieve active Zabbix problems: invalid problem timestamp")
		}
		severity, err := strconv.Atoi(remote.Severity)
		if err != nil || severity < 0 || severity > 5 {
			return nil, fmt.Errorf("retrieve active Zabbix problems: invalid severity")
		}
		problemHosts := hostsByTrigger[remote.ObjectID]
		if len(problemHosts) == 0 {
			continue
		}
		problems = append(problems, Problem{
			EventID: remote.EventID, TriggerID: remote.ObjectID,
			NatureFingerprint: triggerFingerprint(remote.ObjectID, triggerByID, detailed),
			CanonicalNature:   triggerCanonicalNature(remote.ObjectID, triggerByID, detailed),
			Name:              name, Severity: severity,
			Acknowledged: remote.Acknowledged == "1", Suppressed: remote.Suppressed == "1",
			StartedAt: time.Unix(clock, 0).UTC(), HostIDs: problemHosts,
		})
	}
	return problems, nil
}

func (client *Client) triggers(ctx context.Context, endpoint, token string, triggerIDs []string, hosts bool) ([]remoteTrigger, bool, error) {
	params := map[string]any{
		"output": []string{
			"triggerid", "templateid", "uuid", "description", "expression",
			"recovery_expression", "correlation_tag", "flags",
		},
		"triggerids":          triggerIDs,
		"selectDiscoveryData": []string{"parent_triggerid"},
		"selectItems":         []string{"itemid", "key_"},
		"selectFunctions":     []string{"itemid", "function", "parameter"},
		"selectTags":          []string{"tag", "value"},
	}
	if hosts {
		params["selectHosts"] = []string{"hostid"}
	}
	var result []remoteTrigger
	if err := client.call(ctx, endpoint, token, "trigger.get", params, &result); err == nil {
		return result, true, nil
	}
	// La V1 reste compatible avec les versions Zabbix qui ne connaissent pas
	// encore tous les champs de description. L'identité se replie alors sur le
	// trigger, toujours dans la portée stricte du Connecteur.
	fallback := map[string]any{
		"output": []string{"triggerid"}, "triggerids": triggerIDs,
	}
	if hosts {
		fallback["selectHosts"] = []string{"hostid"}
	}
	result = nil
	if err := client.call(ctx, endpoint, token, "trigger.get", fallback, &result); err != nil {
		return nil, false, err
	}
	return result, false, nil
}

func (client *Client) loadTriggerAncestors(ctx context.Context, endpoint, token string, known map[string]remoteTrigger) error {
	for depth := 0; depth < 32; depth++ {
		missing := make([]string, 0)
		seen := make(map[string]struct{})
		for _, trigger := range known {
			for _, parentID := range []string{strings.TrimSpace(trigger.TemplateID), discoveryParent(trigger.DiscoveryData)} {
				if parentID == "" || parentID == "0" {
					continue
				}
				if _, exists := known[parentID]; exists {
					continue
				}
				if _, duplicate := seen[parentID]; !duplicate {
					seen[parentID] = struct{}{}
					missing = append(missing, parentID)
				}
			}
		}
		if len(missing) == 0 {
			return nil
		}
		sort.Strings(missing)
		ancestors, detailed, err := client.triggers(ctx, endpoint, token, missing, false)
		if err != nil {
			return err
		}
		if !detailed || len(ancestors) == 0 {
			return fmt.Errorf("trigger ancestry is unavailable")
		}
		for _, ancestor := range ancestors {
			known[ancestor.TriggerID] = ancestor
		}
	}
	return fmt.Errorf("trigger ancestry exceeds 32 levels")
}

func discoveryParent(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var object struct {
		ParentTriggerID string `json:"parent_triggerid"`
	}
	if json.Unmarshal(raw, &object) == nil && object.ParentTriggerID != "" {
		return strings.TrimSpace(object.ParentTriggerID)
	}
	var list []struct {
		ParentTriggerID string `json:"parent_triggerid"`
	}
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return strings.TrimSpace(list[0].ParentTriggerID)
	}
	return ""
}

func triggerFingerprint(triggerID string, known map[string]remoteTrigger, detailed bool) string {
	if !detailed {
		return "trigger:" + triggerID
	}
	root, exists := known[triggerID]
	if !exists {
		return "trigger:" + triggerID
	}
	visited := make(map[string]struct{})
	for {
		if _, loop := visited[root.TriggerID]; loop {
			break
		}
		visited[root.TriggerID] = struct{}{}
		parentID := discoveryParent(root.DiscoveryData)
		if parentID == "" || parentID == "0" {
			parentID = strings.TrimSpace(root.TemplateID)
		}
		parent, ok := known[parentID]
		if parentID == "" || parentID == "0" || !ok {
			break
		}
		root = parent
	}
	if uuid := strings.TrimSpace(root.UUID); uuid != "" {
		return "uuid:" + strings.ToLower(uuid)
	}
	if root.TriggerID != "" && root.TriggerID != triggerID {
		return "root:" + root.TriggerID
	}
	if strings.TrimSpace(root.Description) == "" && strings.TrimSpace(root.Expression) == "" &&
		len(root.Items) == 0 && len(root.Functions) == 0 && len(root.Tags) == 0 {
		return "trigger:" + triggerID
	}

	itemKeys := make(map[string]string, len(root.Items))
	keys := make([]string, 0, len(root.Items))
	for _, item := range root.Items {
		key := strings.TrimSpace(item.Key)
		itemKeys[item.ItemID] = key
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	functions := make([]string, 0, len(root.Functions))
	for _, function := range root.Functions {
		functions = append(functions, strings.Join([]string{
			itemKeys[function.ItemID], strings.TrimSpace(function.Function),
			strings.TrimSpace(function.Parameter),
		}, "\x00"))
	}
	sort.Strings(functions)
	tags := make([]string, 0, len(root.Tags))
	for _, tag := range root.Tags {
		tags = append(tags, strings.TrimSpace(tag.Tag)+"\x00"+strings.TrimSpace(tag.Value))
	}
	sort.Strings(tags)
	canonical, _ := json.Marshal(struct {
		Description        string   `json:"description"`
		Expression         string   `json:"expression"`
		RecoveryExpression string   `json:"recovery_expression"`
		CorrelationTag     string   `json:"correlation_tag"`
		Flags              string   `json:"flags"`
		ItemKeys           []string `json:"item_keys"`
		Functions          []string `json:"functions"`
		Tags               []string `json:"tags"`
	}{
		strings.TrimSpace(root.Description), canonicalTriggerExpression(root.Expression),
		canonicalTriggerExpression(root.RecoveryExpression), strings.TrimSpace(root.CorrelationTag),
		strings.TrimSpace(root.Flags), keys, functions, tags,
	})
	digest := sha256.Sum256(canonical)
	return fmt.Sprintf("rule:%x", digest[:16])
}

func triggerCanonicalNature(triggerID string, known map[string]remoteTrigger, detailed bool) string {
	if !detailed {
		return ""
	}
	root, exists := known[triggerID]
	if !exists {
		return ""
	}
	visited := make(map[string]struct{})
	for {
		if _, loop := visited[root.TriggerID]; loop {
			return ""
		}
		visited[root.TriggerID] = struct{}{}
		parentID := discoveryParent(root.DiscoveryData)
		if parentID == "" || parentID == "0" {
			parentID = strings.TrimSpace(root.TemplateID)
		}
		parent, ok := known[parentID]
		if parentID == "" || parentID == "0" || !ok {
			break
		}
		root = parent
	}
	for _, tag := range root.Tags {
		if strings.EqualFold(strings.TrimSpace(tag.Tag), "cairnops.nature") &&
			strings.EqualFold(strings.TrimSpace(tag.Value), "availability") {
			return "availability"
		}
	}
	return ""
}

var (
	modernTriggerHost = regexp.MustCompile(`([[:alpha:]_][[:alnum:]_]*\s*\(\s*/)[^/(),\s]+(/)`)
	legacyTriggerHost = regexp.MustCompile(`\{[^{}:]+:`)
	functionIdentity  = regexp.MustCompile(`\{[0-9]+\}`)
)

func canonicalTriggerExpression(value string) string {
	value = strings.TrimSpace(value)
	value = modernTriggerHost.ReplaceAllString(value, `${1}*${2}`)
	value = legacyTriggerHost.ReplaceAllString(value, `{*:`)
	return functionIdentity.ReplaceAllString(value, `{function}`)
}

// Items découvre les items numériques actifs d'un périmètre d'hôtes et peut
// également relire une sélection exacte par itemids. Les valeurs non finies
// ou non numériques restent absentes au lieu d'être transformées en zéro.
func (client *Client) Items(ctx context.Context, endpoint, token string, hostIDs, itemIDs []string) ([]Item, error) {
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 4096 {
		return nil, fmt.Errorf("API token must contain between 1 and 4096 characters")
	}
	if len(hostIDs) == 0 && len(itemIDs) == 0 {
		return []Item{}, nil
	}
	if len(hostIDs) > maximumHosts || len(itemIDs) > maximumItems {
		return nil, fmt.Errorf("item scope is too large")
	}
	params := map[string]any{
		"output":    []string{"itemid", "hostid", "name", "key_", "units", "value_type", "lastvalue", "lastclock"},
		"monitored": true,
		"sortfield": []string{"name", "itemid"},
		"limit":     maximumItems + 1,
	}
	if len(hostIDs) > 0 {
		params["hostids"] = hostIDs
	}
	if len(itemIDs) > 0 {
		params["itemids"] = itemIDs
	}
	var remote []struct {
		ItemID    string `json:"itemid"`
		HostID    string `json:"hostid"`
		Name      string `json:"name"`
		Key       string `json:"key_"`
		Units     string `json:"units"`
		ValueType string `json:"value_type"`
		LastValue string `json:"lastvalue"`
		LastClock string `json:"lastclock"`
	}
	if err := client.call(ctx, endpoint, token, "item.get", params, &remote); err != nil {
		return nil, fmt.Errorf("retrieve Zabbix items: %w", err)
	}
	if len(remote) > maximumItems {
		return nil, fmt.Errorf("more than %d Zabbix items are visible", maximumItems)
	}
	items := make([]Item, 0, len(remote))
	for _, candidate := range remote {
		valueType, parseErr := strconv.Atoi(candidate.ValueType)
		if parseErr != nil || (valueType != 0 && valueType != 3) {
			continue
		}
		if _, parseErr := strconv.ParseUint(candidate.ItemID, 10, 64); parseErr != nil {
			return nil, fmt.Errorf("retrieve Zabbix items: invalid item identity")
		}
		item := Item{ID: candidate.ItemID, HostID: candidate.HostID, Name: strings.TrimSpace(candidate.Name), Key: strings.TrimSpace(candidate.Key), Units: strings.TrimSpace(candidate.Units), ValueType: valueType}
		if value, parseErr := strconv.ParseFloat(strings.TrimSpace(candidate.LastValue), 64); parseErr == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
			item.LastValue = &value
		}
		if seconds, parseErr := strconv.ParseInt(strings.TrimSpace(candidate.LastClock), 10, 64); parseErr == nil && seconds > 0 {
			observed := time.Unix(seconds, 0).UTC()
			item.LastClock = &observed
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].HostID != items[right].HostID {
			return items[left].HostID < items[right].HostID
		}
		leftName, rightName := strings.ToLower(items[left].Name), strings.ToLower(items[right].Name)
		if leftName != rightName {
			return leftName < rightName
		}
		return items[left].ID < items[right].ID
	})
	return items, nil
}

func (client *Client) Acknowledge(ctx context.Context, endpoint, token, eventID, message string) error {
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 4096 {
		return fmt.Errorf("API token must contain between 1 and 4096 characters")
	}
	if _, err := strconv.ParseUint(eventID, 10, 64); err != nil {
		return fmt.Errorf("event identity is invalid")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Acknowledged from CairnOps"
	}
	if len(message) > 512 {
		message = message[:512]
	}
	var result struct {
		EventIDs []string `json:"eventids"`
	}
	if err := client.call(ctx, endpoint, token, "event.acknowledge", map[string]any{
		"eventids": []string{eventID}, "action": 6, "message": message,
	}, &result); err != nil {
		return fmt.Errorf("acknowledge Zabbix event: %w", err)
	}
	for _, returnedID := range result.EventIDs {
		if returnedID == eventID {
			return nil
		}
	}
	return fmt.Errorf("acknowledge Zabbix event: remote response omitted the event")
}

func (client *Client) call(ctx context.Context, endpoint, token, method string, params any, target any) error {
	if client == nil || client.http == nil {
		return fmt.Errorf("HTTP client is not configured")
	}
	payload, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
		ID      int    `json:"id"`
	}{JSONRPC: "2.0", Method: method, Params: params, ID: 1})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json-rpc")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.http.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("remote server returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(body) > maximumResponseBytes {
		return fmt.Errorf("response exceeds %d bytes", maximumResponseBytes)
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.JSONRPC != "2.0" {
		return fmt.Errorf("remote server returned an invalid JSON-RPC response")
	}
	if envelope.Error != nil {
		message := strings.TrimSpace(envelope.Error.Message)
		if message == "" {
			message = "request rejected"
		}
		if len(message) > 180 {
			message = message[:180]
		}
		return fmt.Errorf("Zabbix API error %d: %s", envelope.Error.Code, message)
	}
	if len(envelope.Result) == 0 {
		return fmt.Errorf("remote server returned no result")
	}
	if err := json.Unmarshal(envelope.Result, target); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}
	return nil
}

func interfaceType(value string) string {
	switch value {
	case "1":
		return "agent"
	case "2":
		return "snmp"
	case "3":
		return "ipmi"
	case "4":
		return "jmx"
	default:
		return "other"
	}
}

func compatibility(version string) (string, string) {
	parts := strings.Split(version, ".")
	major, majorErr := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if majorErr == nil && ((major == 5 && minor >= 4) || major == 6 || major == 7) {
		return "supported", "Version prise en charge"
	}
	return "warning", "Version détectée, compatibilité à confirmer"
}
