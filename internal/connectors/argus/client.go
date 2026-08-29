// Package argus encapsule l'intégration HTTP en lecture seule de Release-Argus.
package argus

import (
	"bytes"
	"context"
	"encoding/json"
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

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const (
	minimumVersion      = "0.29.0"
	maximumJSONBytes    = 16 << 20
	maximumMetricsBytes = 8 << 20
)

type DeploymentState int

const (
	DeploymentStateUnactioned DeploymentState = iota
	DeploymentStateDeployed
	DeploymentStateApproved
	DeploymentStateSkipped
	DeploymentStateUnknown
)

const (
	IneligibilityInactive          = "inactive"
	IneligibilityNoDeployedVersion = "deployed_version_not_configured"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Service struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Active            bool            `json:"active"`
	Importable        bool            `json:"importable"`
	Ineligibility     string          `json:"ineligibility,omitempty"`
	DeployedVersion   string          `json:"deployed_version,omitempty"`
	LatestVersion     string          `json:"latest_version,omitempty"`
	LastChecked       string          `json:"last_checked,omitempty"`
	Approved          bool            `json:"approved"`
	Skipped           bool            `json:"skipped"`
	AutoApprove       bool            `json:"auto_approve"`
	DeploymentState   DeploymentState `json:"deployment_state"`
	LatestQueryOK     bool            `json:"latest_query_ok"`
	DeployedQueryOK   bool            `json:"deployed_query_ok"`
	Unknown           bool            `json:"unknown"`
	UnknownReason     string          `json:"unknown_reason,omitempty"`
	DashboardTemplate string          `json:"-"`
	VersionURL        string          `json:"version_url,omitempty"`
}

type Inspection struct {
	Endpoint           string    `json:"endpoint"`
	EncryptedTransport bool      `json:"encrypted_transport"`
	Version            string    `json:"version"`
	Compatibility      string    `json:"compatibility"`
	Services           []Service `json:"services"`
}

type configService struct {
	Name    string `json:"name"`
	Options struct {
		Active *bool `json:"active"`
	} `json:"options"`
	DeployedVersion json.RawMessage `json:"deployed_version"`
	Dashboard       struct {
		WebURL string `json:"web_url"`
	} `json:"dashboard"`
	Status struct {
		DeployedVersion string `json:"deployed_version"`
		LatestVersion   string `json:"latest_version"`
		LastQueried     string `json:"last_queried"`
	} `json:"status"`
}

type updateDetail struct {
	ServiceName     string `json:"service_name"`
	DeployedVersion string `json:"deployed_version"`
	LatestVersion   string `json:"latest_version"`
	LastChecked     string `json:"last_checked"`
	AutoApprove     bool   `json:"auto_approve"`
	Approved        bool   `json:"approved"`
	Skipped         bool   `json:"skipped"`
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
	return NewClientWithHTTP(&http.Client{Transport: transport, Timeout: 20 * time.Second})
}

func NewClientWithHTTP(client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{http: &copy}
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
	cleanPath := strings.TrimSuffix(parsed.Path, "/")
	for _, suffix := range []string{"/api/v1/config", "/api/v1/counts", "/api/v1/version", "/api/v1/template", "/metrics"} {
		if strings.HasSuffix(cleanPath, suffix) {
			cleanPath = strings.TrimSuffix(cleanPath, suffix)
			break
		}
	}
	if cleanPath == "/" {
		cleanPath = ""
	}
	parsed.Path = cleanPath
	parsed.RawPath = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func (client *Client) Inspect(ctx context.Context, address string, credentials Credentials) (Inspection, error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return Inspection{}, err
	}
	credentials.Username = strings.TrimSpace(credentials.Username)
	if (credentials.Username == "") != (credentials.Password == "") || len(credentials.Username) > 4096 || len(credentials.Password) > 4096 {
		return Inspection{}, fmt.Errorf("Basic authentication requires a username and password of at most 4096 characters")
	}

	var versionPayload struct {
		Version string `json:"version"`
	}
	if err := client.getJSON(ctx, endpoint, "/api/v1/version", credentials, &versionPayload); err != nil {
		return Inspection{}, err
	}
	versionPayload.Version = strings.TrimSpace(versionPayload.Version)
	if !versionAtLeast(versionPayload.Version, minimumVersion) {
		return Inspection{}, fmt.Errorf("Argus %s is unsupported; CairnOps requires Argus %s or newer", versionPayload.Version, minimumVersion)
	}

	var configPayload struct {
		Service map[string]configService `json:"service"`
		Order   []string                 `json:"order"`
	}
	if err := client.getJSON(ctx, endpoint, "/api/v1/config", credentials, &configPayload); err != nil {
		return Inspection{}, err
	}
	var counts struct {
		UpdateDetails []updateDetail `json:"update_details"`
	}
	if err := client.getJSON(ctx, endpoint, "/api/v1/counts", credentials, &counts); err != nil {
		return Inspection{}, err
	}
	metricsBody, err := client.get(ctx, endpoint, "/metrics", credentials, maximumMetricsBytes, "text/plain")
	if err != nil {
		return Inspection{}, err
	}
	metricState, err := parseMetricState(metricsBody)
	if err != nil {
		return Inspection{}, err
	}

	updates := make(map[string]updateDetail, len(counts.UpdateDetails))
	for _, update := range counts.UpdateDetails {
		id := strings.TrimSpace(update.ServiceName)
		if id == "" {
			return Inspection{}, fmt.Errorf("decode Argus counts: update detail has no service identity")
		}
		updates[id] = update
	}

	orderedIDs := serviceOrder(configPayload.Order, configPayload.Service)
	services := make([]Service, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		configured := configPayload.Service[id]
		active := configured.Options.Active == nil || *configured.Options.Active
		hasDeployedVersion := len(bytes.TrimSpace(configured.DeployedVersion)) > 0 && string(bytes.TrimSpace(configured.DeployedVersion)) != "null"
		service := Service{
			ID: id, Name: strings.TrimSpace(configured.Name), Active: active,
			Importable:        active && hasDeployedVersion,
			DeployedVersion:   strings.TrimSpace(configured.Status.DeployedVersion),
			LatestVersion:     strings.TrimSpace(configured.Status.LatestVersion),
			LastChecked:       strings.TrimSpace(configured.Status.LastQueried),
			DashboardTemplate: strings.TrimSpace(configured.Dashboard.WebURL),
		}
		if service.Name == "" {
			service.Name = id
		}
		if !active {
			service.Ineligibility = IneligibilityInactive
		} else if !hasDeployedVersion {
			service.Ineligibility = IneligibilityNoDeployedVersion
		}
		state := metricState[id]
		service.DeploymentState = state.deployment
		service.LatestQueryOK = state.latestSeen && state.latestOK
		service.DeployedQueryOK = state.deployedSeen && state.deployedOK
		service.Approved = state.deploymentSeen && state.deployment == DeploymentStateApproved
		service.Skipped = state.deploymentSeen && state.deployment == DeploymentStateSkipped
		if update, ok := updates[id]; ok {
			service.DeployedVersion = strings.TrimSpace(update.DeployedVersion)
			service.LatestVersion = strings.TrimSpace(update.LatestVersion)
			service.LastChecked = strings.TrimSpace(update.LastChecked)
			service.AutoApprove = update.AutoApprove
			service.Approved = update.Approved
			service.Skipped = update.Skipped
		}
		if service.Importable {
			switch {
			case !service.LatestQueryOK:
				service.Unknown, service.UnknownReason = true, "latest_version_query_failed"
			case !service.DeployedQueryOK:
				service.Unknown, service.UnknownReason = true, "deployed_version_query_failed"
			case !state.deploymentSeen || state.deployment == DeploymentStateUnknown:
				service.Unknown, service.UnknownReason = true, "deployment_state_unknown"
			}
			if !service.Unknown {
				if state.deployment == DeploymentStateDeployed && (service.DeployedVersion == "" || service.LatestVersion == "") {
					version, renderErr := client.renderTemplate(ctx, endpoint, id, "{{ latest_version }}", credentials)
					if renderErr != nil {
						service.Unknown, service.UnknownReason = true, "version_template_failed"
					} else {
						service.DeployedVersion, service.LatestVersion = version, version
					}
				} else {
					if service.DeployedVersion == "" {
						service.DeployedVersion, err = client.renderTemplate(ctx, endpoint, id, "{{ deployed_version }}", credentials)
						if err != nil {
							service.Unknown, service.UnknownReason = true, "version_template_failed"
						}
					}
					if !service.Unknown && service.LatestVersion == "" {
						service.LatestVersion, err = client.renderTemplate(ctx, endpoint, id, "{{ latest_version }}", credentials)
						if err != nil {
							service.Unknown, service.UnknownReason = true, "version_template_failed"
						}
					}
				}
			}
			if !service.Unknown && (service.DeployedVersion == "" || service.LatestVersion == "") {
				service.Unknown, service.UnknownReason = true, "version_value_missing"
			}
		}
		if service.DashboardTemplate != "" && service.Importable {
			service.VersionURL, err = client.renderWebURL(ctx, endpoint, id, credentials)
			if err != nil && !service.Unknown {
				service.Unknown, service.UnknownReason = true, "dashboard_url_failed"
			}
		}
		services = append(services, service)
	}

	return Inspection{
		Endpoint: endpoint, EncryptedTransport: strings.HasPrefix(endpoint, "https://"),
		Version: versionPayload.Version, Compatibility: "supported", Services: services,
	}, nil
}

func (client *Client) renderWebURL(ctx context.Context, endpoint, serviceID string, credentials Credentials) (string, error) {
	rendered, err := client.renderTemplate(ctx, endpoint, serviceID, "{{ web_url }}", credentials)
	if err != nil {
		return "", err
	}
	if rendered == "" {
		return "", nil
	}
	parsed, err := url.Parse(rendered)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return "", fmt.Errorf("render Argus dashboard URL for %s: only absolute HTTP(S) URLs are accepted", serviceID)
	}
	return parsed.String(), nil
}

func (client *Client) renderTemplate(ctx context.Context, endpoint, serviceID, template string, credentials Credentials) (string, error) {
	query := url.Values{"service_id": {serviceID}, "template": {template}}
	var payload struct {
		Parsed string `json:"parsed"`
	}
	if err := client.getJSON(ctx, endpoint, "/api/v1/template?"+query.Encode(), credentials, &payload); err != nil {
		return "", fmt.Errorf("render Argus template for %s: %w", serviceID, err)
	}
	return strings.TrimSpace(payload.Parsed), nil
}

func (client *Client) getJSON(ctx context.Context, endpoint, route string, credentials Credentials, target any) error {
	body, err := client.get(ctx, endpoint, route, credentials, maximumJSONBytes, "application/json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Argus %s response: %w", route, err)
	}
	return nil
}

func (client *Client) get(ctx context.Context, endpoint, route string, credentials Credentials, limit int64, accept string) ([]byte, error) {
	requestURL, err := endpointURL(endpoint, route)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create Argus request: %w", err)
	}
	request.Header.Set("Accept", accept)
	if credentials.Username != "" {
		request.SetBasicAuth(credentials.Username, credentials.Password)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read Argus %s: %w", route, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("read Argus %s: Basic authentication was rejected", route)
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			return nil, fmt.Errorf("read Argus %s: redirects are not accepted", route)
		}
		return nil, fmt.Errorf("read Argus %s: unexpected HTTP status %d", route, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read Argus %s response: %w", route, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("read Argus %s: response exceeds %d bytes", route, limit)
	}
	return body, nil
}

func endpointURL(endpoint, route string) (string, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse normalized Argus endpoint: %w", err)
	}
	parts := strings.SplitN(route, "?", 2)
	base.Path = path.Join(base.Path, parts[0])
	if !strings.HasPrefix(base.Path, "/") {
		base.Path = "/" + base.Path
	}
	base.RawPath = ""
	if len(parts) == 2 {
		base.RawQuery = parts[1]
	}
	return base.String(), nil
}

type serviceMetricState struct {
	latestSeen, latestOK     bool
	deployedSeen, deployedOK bool
	deploymentSeen           bool
	deployment               DeploymentState
}

func parseMetricState(body []byte) (map[string]serviceMetricState, error) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse Argus metrics: %w", err)
	}
	states := make(map[string]serviceMetricState)
	applyResultMetric(families["latest_version_query_result_last"], states, true)
	applyResultMetric(families["deployed_version_query_result_last"], states, false)
	if family := families["latest_version_is_deployed"]; family != nil {
		for _, metric := range family.GetMetric() {
			id := metricLabel(metric, "id")
			if id == "" || metric.GetGauge() == nil {
				continue
			}
			value := int(metric.GetGauge().GetValue())
			state := states[id]
			state.deploymentSeen = true
			if value < int(DeploymentStateUnactioned) || value > int(DeploymentStateUnknown) {
				state.deployment = DeploymentStateUnknown
			} else {
				state.deployment = DeploymentState(value)
			}
			states[id] = state
		}
	}
	return states, nil
}

func applyResultMetric(family *dto.MetricFamily, states map[string]serviceMetricState, latest bool) {
	if family == nil {
		return
	}
	for _, metric := range family.GetMetric() {
		id := metricLabel(metric, "id")
		if id == "" || metric.GetGauge() == nil {
			continue
		}
		success := metric.GetGauge().GetValue() == 1
		state := states[id]
		if latest {
			state.latestOK = success && (!state.latestSeen || state.latestOK)
			state.latestSeen = true
		} else {
			state.deployedOK = success && (!state.deployedSeen || state.deployedOK)
			state.deployedSeen = true
		}
		states[id] = state
	}
}

func metricLabel(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return strings.TrimSpace(label.GetValue())
		}
	}
	return ""
}

func serviceOrder(order []string, services map[string]configService) []string {
	result := make([]string, 0, len(services))
	seen := make(map[string]struct{}, len(services))
	for _, id := range order {
		if _, exists := services[id]; !exists {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	rest := make([]string, 0, len(services)-len(result))
	for id := range services {
		if _, exists := seen[id]; !exists {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	return append(result, rest...)
}

func versionAtLeast(version, minimum string) bool {
	left, leftPrerelease, okLeft := parseVersion(version)
	right, rightPrerelease, okRight := parseVersion(minimum)
	if !okLeft || !okRight {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return left[index] > right[index]
		}
	}
	return !leftPrerelease || rightPrerelease
}

func parseVersion(value string) ([3]int, bool, bool) {
	var result [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value, _, _ = strings.Cut(value, "+")
	var prerelease string
	value, prerelease, _ = strings.Cut(value, "-")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return result, false, false
	}
	for index, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return result, false, false
		}
		result[index] = parsed
	}
	return result, prerelease != "", true
}
