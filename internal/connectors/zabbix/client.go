package zabbix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maximumResponseBytes = 4 << 20
	maximumHosts         = 5000
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
	EventID      string
	TriggerID    string
	Name         string
	Severity     int
	Acknowledged bool
	Suppressed   bool
	StartedAt    time.Time
	HostIDs      []string
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
	var remoteTriggers []struct {
		TriggerID string `json:"triggerid"`
		Hosts     []struct {
			HostID string `json:"hostid"`
		} `json:"hosts"`
	}
	if err := client.call(ctx, endpoint, token, "trigger.get", map[string]any{
		"output": []string{"triggerid"}, "triggerids": triggerIDs,
		"selectHosts": []string{"hostid"},
	}, &remoteTriggers); err != nil {
		return nil, fmt.Errorf("resolve Zabbix problem hosts: %w", err)
	}
	hostsByTrigger := make(map[string][]string, len(remoteTriggers))
	for _, trigger := range remoteTriggers {
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
			EventID: remote.EventID, TriggerID: remote.ObjectID, Name: name, Severity: severity,
			Acknowledged: remote.Acknowledged == "1", Suppressed: remote.Suppressed == "1",
			StartedAt: time.Unix(clock, 0).UTC(), HostIDs: problemHosts,
		})
	}
	return problems, nil
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
