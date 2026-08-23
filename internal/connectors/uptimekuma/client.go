package uptimekuma

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
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
	maximumMetricsBytes = 8 << 20
	// Une latence supérieure à une journée n'est plus une latence : la valeur
	// est écartée plutôt que stockée.
	maximumLatencyMilliseconds = 86_400_000
)

type Monitor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	URL      string `json:"url,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Port     string `json:"port,omitempty"`
	Status   int    `json:"status"`
	// ResponseMilliseconds est le temps de réponse mesuré par Uptime Kuma.
	// Il reste absent quand le produit ne le publie pas : CairnOps ne mesure
	// alors aucune latence plutôt que d'en inventer une nulle.
	ResponseMilliseconds     *int     `json:"response_milliseconds,omitempty"`
	CertificateDaysRemaining *float64 `json:"certificate_days_remaining,omitempty"`
	CertificateValid         *bool    `json:"certificate_valid,omitempty"`
}

func (monitor Monitor) Address() string {
	if strings.TrimSpace(monitor.URL) != "" && monitor.URL != "null" {
		return monitor.URL
	}
	if strings.TrimSpace(monitor.Hostname) == "" || monitor.Hostname == "null" {
		return ""
	}
	if strings.TrimSpace(monitor.Port) == "" || monitor.Port == "null" {
		return monitor.Hostname
	}
	return net.JoinHostPort(monitor.Hostname, monitor.Port)
}

type Inspection struct {
	Endpoint           string    `json:"endpoint"`
	EncryptedTransport bool      `json:"encrypted_transport"`
	Monitors           []Monitor `json:"monitors"`
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
	if !strings.HasSuffix(cleanPath, "/metrics") && cleanPath != "metrics" {
		cleanPath = path.Join(cleanPath, "metrics")
		if !strings.HasPrefix(cleanPath, "/") {
			cleanPath = "/" + cleanPath
		}
	}
	parsed.Path = cleanPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

func (client *Client) Inspect(ctx context.Context, address, apiKey string) (Inspection, error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return Inspection{}, err
	}
	monitors, err := client.Monitors(ctx, endpoint, apiKey)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{
		Endpoint: endpoint, EncryptedTransport: strings.HasPrefix(endpoint, "https://"), Monitors: monitors,
	}, nil
}

func (client *Client) Monitors(ctx context.Context, endpoint, apiKey string) ([]Monitor, error) {
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || len(apiKey) > 4096 {
		return nil, fmt.Errorf("API key must contain between 1 and 4096 characters")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create Uptime Kuma metrics request: %w", err)
	}
	request.Header.Set("Accept", "text/plain")
	request.SetBasicAuth("", apiKey)
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read Uptime Kuma metrics: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("read Uptime Kuma metrics: API key rejected")
		}
		return nil, fmt.Errorf("read Uptime Kuma metrics: unexpected HTTP status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumMetricsBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Uptime Kuma metrics response: %w", err)
	}
	if len(body) > maximumMetricsBytes {
		return nil, fmt.Errorf("read Uptime Kuma metrics: response exceeds %d bytes", maximumMetricsBytes)
	}
	return parseMonitors(body)
}

// attachResponseTimes rattache à chaque monitor le temps de réponse mesuré par
// Uptime Kuma. Les versions publient tantôt des millisecondes, tantôt des
// secondes ; une valeur absente, négative ou non finie laisse la latence
// absente plutôt que de la supposer.
func attachResponseTimes(families map[string]*dto.MetricFamily, byID map[string]Monitor) {
	for name, scale := range map[string]float64{"monitor_response_time": 1, "monitor_response_seconds": 1000} {
		family := families[name]
		if family == nil {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric.GetGauge() == nil {
				continue
			}
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			identity := strings.TrimSpace(labels["monitor_id"])
			if identity == "" {
				identity = "name:" + strings.TrimSpace(labels["monitor_name"])
			}
			monitor, known := byID[identity]
			if !known || monitor.ResponseMilliseconds != nil {
				continue
			}
			value := metric.GetGauge().GetValue() * scale
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(maximumLatencyMilliseconds) {
				continue
			}
			milliseconds := int(math.Round(value))
			monitor.ResponseMilliseconds = &milliseconds
			byID[identity] = monitor
		}
	}
}

// attachCertificateMetrics lit uniquement les deux gauges documentées par
// l'endpoint Prometheus. Elles restent facultatives : les monitors sans TLS
// ne doivent pas fabriquer un certificat valide ni une échéance nulle.
func attachCertificateMetrics(families map[string]*dto.MetricFamily, byID map[string]Monitor) {
	if family := families["monitor_cert_days_remaining"]; family != nil {
		for _, metric := range family.GetMetric() {
			if metric.GetGauge() == nil {
				continue
			}
			identity := metricIdentity(metric)
			monitor, known := byID[identity]
			value := metric.GetGauge().GetValue()
			if !known || math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			monitor.CertificateDaysRemaining = &value
			byID[identity] = monitor
		}
	}
	if family := families["monitor_cert_is_valid"]; family != nil {
		for _, metric := range family.GetMetric() {
			if metric.GetGauge() == nil {
				continue
			}
			identity := metricIdentity(metric)
			monitor, known := byID[identity]
			value := metric.GetGauge().GetValue()
			if !known || (value != 0 && value != 1) {
				continue
			}
			valid := value == 1
			monitor.CertificateValid = &valid
			byID[identity] = monitor
		}
	}
}

func metricIdentity(metric *dto.Metric) string {
	labels := make(map[string]string, len(metric.GetLabel()))
	for _, pair := range metric.GetLabel() {
		labels[pair.GetName()] = pair.GetValue()
	}
	identity := strings.TrimSpace(labels["monitor_id"])
	if identity == "" {
		identity = "name:" + strings.TrimSpace(labels["monitor_name"])
	}
	return identity
}

func parseMonitors(body []byte) ([]Monitor, error) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse Uptime Kuma metrics: %w", err)
	}
	family := families["monitor_status"]
	if family == nil || len(family.GetMetric()) == 0 {
		return nil, fmt.Errorf("parse Uptime Kuma metrics: monitor_status is absent")
	}
	byID := make(map[string]Monitor, len(family.GetMetric()))
	for _, metric := range family.GetMetric() {
		if metric.GetGauge() == nil {
			return nil, fmt.Errorf("parse Uptime Kuma metrics: monitor_status is not a gauge")
		}
		labels := make(map[string]string, len(metric.GetLabel()))
		for _, pair := range metric.GetLabel() {
			labels[pair.GetName()] = pair.GetValue()
		}
		monitor := Monitor{
			ID: strings.TrimSpace(labels["monitor_id"]), Name: strings.TrimSpace(labels["monitor_name"]),
			Type: strings.TrimSpace(labels["monitor_type"]), URL: strings.TrimSpace(labels["monitor_url"]),
			Hostname: strings.TrimSpace(labels["monitor_hostname"]), Port: strings.TrimSpace(labels["monitor_port"]),
		}
		if monitor.Name == "" || len(monitor.Name) > 160 || len(monitor.Type) > 80 {
			return nil, fmt.Errorf("parse Uptime Kuma metrics: invalid monitor metadata")
		}
		if monitor.ID == "" {
			// Uptime Kuma 1.x n'expose pas monitor_id : le nom devient l'identité.
			monitor.ID = "name:" + monitor.Name
		} else if _, err := strconv.ParseUint(monitor.ID, 10, 64); err != nil {
			return nil, fmt.Errorf("parse Uptime Kuma metrics: invalid monitor identity")
		}
		status := metric.GetGauge().GetValue()
		if math.Trunc(status) != status || status < 0 || status > 3 {
			return nil, fmt.Errorf("parse Uptime Kuma metrics: invalid monitor status")
		}
		monitor.Status = int(status)
		if previous, duplicate := byID[monitor.ID]; duplicate && previous != monitor {
			if strings.HasPrefix(monitor.ID, "name:") {
				return nil, fmt.Errorf("parse Uptime Kuma metrics: two distinct monitors share the name %q; rename one in Uptime Kuma", monitor.Name)
			}
			return nil, fmt.Errorf("parse Uptime Kuma metrics: conflicting monitor identity %s", monitor.ID)
		}
		byID[monitor.ID] = monitor
	}
	attachResponseTimes(families, byID)
	attachCertificateMetrics(families, byID)

	monitors := make([]Monitor, 0, len(byID))
	for _, monitor := range byID {
		monitors = append(monitors, monitor)
	}
	sort.SliceStable(monitors, func(i, j int) bool {
		left, right := strings.ToLower(monitors[i].Name), strings.ToLower(monitors[j].Name)
		if left == right {
			return monitors[i].ID < monitors[j].ID
		}
		return left < right
	})
	return monitors, nil
}
