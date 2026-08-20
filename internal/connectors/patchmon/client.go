// Package patchmon encapsule l'API d'intégration en lecture seule de PatchMon.
package patchmon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

const maximumResponseBytes = 16 << 20

type HostGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Host est l'instantané synthétique publié par l'API d'intégration.
// Il suffit à la découverte et à la posture du MVP, sans lire l'inventaire
// complet de chaque paquet.
type Host struct {
	ID                   string      `json:"id"`
	MachineID            string      `json:"machine_id,omitempty"`
	FriendlyName         string      `json:"friendly_name"`
	Hostname             string      `json:"hostname"`
	IP                   string      `json:"ip"`
	HostGroups           []HostGroup `json:"host_groups"`
	OSType               string      `json:"os_type"`
	OSVersion            string      `json:"os_version"`
	LastUpdate           *time.Time  `json:"last_update,omitempty"`
	Status               string      `json:"status"`
	EffectiveStatus      string      `json:"effective_status"`
	ReportingState       string      `json:"reporting_state"`
	UpdateState          string      `json:"update_state"`
	NeedsReboot          bool        `json:"needs_reboot"`
	UpdatesCount         int         `json:"updates_count"`
	SecurityUpdatesCount int         `json:"security_updates_count"`
	TotalPackages        int         `json:"total_packages"`
}

func (host Host) Name() string {
	if strings.TrimSpace(host.FriendlyName) != "" {
		return strings.TrimSpace(host.FriendlyName)
	}
	return strings.TrimSpace(host.Hostname)
}

type Inspection struct {
	Endpoint           string `json:"endpoint"`
	EncryptedTransport bool   `json:"encrypted_transport"`
	Hosts              []Host `json:"hosts"`
}

type Credentials struct {
	Key    string `json:"key"`
	Secret string `json:"secret"`
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
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func NewClientWithHTTP(client *http.Client) *Client { return &Client{http: client} }

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
	if !strings.HasSuffix(cleanPath, "/api/v1/api/hosts") {
		cleanPath = path.Join(cleanPath, "api/v1/api/hosts")
		if !strings.HasPrefix(cleanPath, "/") {
			cleanPath = "/" + cleanPath
		}
	}
	parsed.Path = cleanPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

func (client *Client) Inspect(ctx context.Context, address string, credentials Credentials) (Inspection, error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return Inspection{}, err
	}
	hosts, err := client.Hosts(ctx, endpoint, credentials)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{
		Endpoint: endpoint, EncryptedTransport: strings.HasPrefix(endpoint, "https://"), Hosts: hosts,
	}, nil
}

func (client *Client) Hosts(ctx context.Context, endpoint string, credentials Credentials) ([]Host, error) {
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	credentials.Key = strings.TrimSpace(credentials.Key)
	credentials.Secret = strings.TrimSpace(credentials.Secret)
	if credentials.Key == "" || len(credentials.Key) > 4096 || credentials.Secret == "" || len(credentials.Secret) > 4096 {
		return nil, fmt.Errorf("API key and secret must each contain between 1 and 4096 characters")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?include=stats", nil)
	if err != nil {
		return nil, fmt.Errorf("create PatchMon hosts request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(credentials.Key, credentials.Secret)
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read PatchMon hosts: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("read PatchMon hosts: API credential rejected or missing host:get scope")
		}
		return nil, fmt.Errorf("read PatchMon hosts: unexpected HTTP status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read PatchMon hosts response: %w", err)
	}
	if len(body) > maximumResponseBytes {
		return nil, fmt.Errorf("read PatchMon hosts: response exceeds %d bytes", maximumResponseBytes)
	}
	var payload struct {
		Hosts []Host `json:"hosts"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode PatchMon hosts: %w", err)
	}
	seen := make(map[string]struct{}, len(payload.Hosts))
	for index := range payload.Hosts {
		host := &payload.Hosts[index]
		host.ID = strings.TrimSpace(host.ID)
		if host.ID == "" || len(host.ID) > 255 || host.Name() == "" || len([]rune(host.Name())) > 160 {
			return nil, fmt.Errorf("decode PatchMon hosts: invalid host identity or name")
		}
		if _, duplicate := seen[host.ID]; duplicate {
			return nil, fmt.Errorf("decode PatchMon hosts: duplicate host identity %s", host.ID)
		}
		seen[host.ID] = struct{}{}
		if host.UpdatesCount < 0 || host.SecurityUpdatesCount < 0 || host.TotalPackages < 0 {
			return nil, fmt.Errorf("decode PatchMon hosts: invalid package counts for host %s", host.ID)
		}
	}
	sort.SliceStable(payload.Hosts, func(i, j int) bool {
		left, right := strings.ToLower(payload.Hosts[i].Name()), strings.ToLower(payload.Hosts[j].Name())
		if left == right {
			return payload.Hosts[i].ID < payload.Hosts[j].ID
		}
		return left < right
	})
	return payload.Hosts, nil
}
