// Package proxmox translates the Proxmox VE API into inventory and observations.
// Runtime calls are read-only; account provisioning is isolated in bootstrap.go.
package proxmox

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maximumResponse = 16 << 20

type responseError struct {
	method, path string
	status       int
}

func (e *responseError) Error() string {
	return fmt.Sprintf("Proxmox VE %s %s returned HTTP %d", e.method, e.path, e.status)
}

type Credentials struct {
	TokenID     string `json:"token_id"`
	Secret      string `json:"secret"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type Certificate struct {
	Endpoint    string    `json:"endpoint"`
	Trusted     bool      `json:"trusted"`
	Fingerprint string    `json:"fingerprint"`
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Resource struct {
	HostUnavailable bool     `json:"host_unavailable,omitempty"`
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Name            string   `json:"name"`
	Node            string   `json:"node"`
	VMID            int      `json:"vmid,omitempty"`
	Storage         string   `json:"storage,omitempty"`
	Status          string   `json:"status"`
	Template        int      `json:"template"`
	Tags            string   `json:"tags,omitempty"`
	CPU             *float64 `json:"cpu,omitempty"`
	Memory          *float64 `json:"mem,omitempty"`
	MaxMemory       *float64 `json:"maxmem,omitempty"`
	Disk            *float64 `json:"disk,omitempty"`
	MaxDisk         *float64 `json:"maxdisk,omitempty"`
	Uptime          *int64   `json:"uptime,omitempty"`
}

func (r Resource) Guest() bool      { return r.Type == "qemu" || r.Type == "lxc" }
func (r Resource) Importable() bool { return r.Template == 0 }

// Condition never infers that a deliberately stopped guest has failed. Unknown
// statuses (including migration transitions) cannot resolve existing evidence.
func (r Resource) Condition(expectedRunning bool) (outcome, reason string) {
	if r.Guest() && r.HostUnavailable {
		return "unknown", "proxmox_host_unavailable"
	}
	if !r.Importable() {
		return "unknown", "proxmox_template"
	}
	switch r.Type {
	case "node":
		switch r.Status {
		case "online":
			return "healthy", ""
		case "offline":
			return "unhealthy", "proxmox_node_offline"
		}
	case "qemu", "lxc":
		switch r.Status {
		case "running":
			return "healthy", ""
		case "stopped":
			if expectedRunning {
				return "unhealthy", "proxmox_guest_stopped"
			}
			return "unknown", "proxmox_guest_stop_allowed"
		}
	case "storage":
		// A storage marked unknown can be disabled or invisible from this node;
		// it is not evidence of an outage of the shared storage system.
		if r.Status == "available" {
			return "healthy", ""
		}
	}
	return "unknown", "proxmox_status_unknown"
}

func (r Resource) Metadata() map[string]any {
	m := map[string]any{"resource_type": r.Type, "node": r.Node, "status": r.Status, "technical_name": r.Name, "tags": strings.FieldsFunc(r.Tags, func(c rune) bool { return c == ';' })}
	if r.Guest() {
		m["vmid"] = r.VMID
		m["host_external_id"] = "node/" + r.Node
		m["host_unavailable"] = r.HostUnavailable
	}
	if r.Type == "storage" {
		m["storage"] = r.Storage
	}
	return m
}

// Metrics contains contextual values only. In particular qemu.disk is not a
// guest filesystem measurement, and cumulative I/O counters are not rates.
func (r Resource) Metrics() map[string]float64 {
	result := map[string]float64{}
	if r.HostUnavailable {
		return result
	}
	if r.Status != "online" && r.Status != "running" && r.Status != "available" {
		return result
	}
	if r.CPU != nil && *r.CPU >= 0 && *r.CPU <= 1 {
		result["cpu.utilization"] = *r.CPU * 100
	}
	if r.Memory != nil && r.MaxMemory != nil && *r.MaxMemory > 0 && *r.Memory >= 0 && *r.Memory <= *r.MaxMemory {
		result["memory.utilization"] = *r.Memory / *r.MaxMemory * 100
	}
	if r.Type == "node" || r.Type == "storage" {
		if r.Disk != nil && r.MaxDisk != nil && *r.MaxDisk > 0 && *r.Disk >= 0 && *r.Disk <= *r.MaxDisk {
			result["filesystem.utilization"] = *r.Disk / *r.MaxDisk * 100
		}
	}
	return result
}

type Inspection struct {
	Endpoint  string     `json:"endpoint"`
	Version   string     `json:"version"`
	Resources []Resource `json:"resources"`
}

type Client struct{ http *http.Client }

func NewClient() *Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout, t.ResponseHeaderTimeout = 5*time.Second, 10*time.Second
	t.MaxResponseHeaderBytes = 64 << 10
	return NewClientWithHTTP(&http.Client{Transport: t, Timeout: 20 * time.Second})
}

func NewClientWithHTTP(client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &copy}
}

func NormalizeEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("Proxmox VE requires an HTTPS address without credentials, query or fragment")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid Proxmox VE port")
		}
	}
	p := strings.TrimSuffix(u.Path, "/")
	if p != "" && p != "/api2/json" {
		return "", fmt.Errorf("use the Proxmox VE server address, without an API path")
	}
	u.Path, u.RawPath = "", ""
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

func validateCredentials(c Credentials) error {
	if len(c.TokenID) > 256 || !strings.Contains(c.TokenID, "@") || strings.Count(c.TokenID, "!") != 1 || strings.ContainsAny(c.TokenID, "\r\n\t =/?#") || len(c.Secret) < 1 || len(c.Secret) > 4096 || strings.ContainsAny(c.Secret, "\r\n\t ") {
		return fmt.Errorf("provide a Proxmox VE token identity (user@realm!token) and its secret")
	}
	if c.Fingerprint != "" {
		decoded, err := hex.DecodeString(c.Fingerprint)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("invalid certificate SHA-256 fingerprint")
		}
	}
	return nil
}

func fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// ProbeCertificate performs only a TLS handshake, without sending credentials
// or HTTP requests. Untrusted identity is presented for explicit approval.
func (client *Client) ProbeCertificate(ctx context.Context, address string) (Certificate, error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return Certificate{}, err
	}
	u, _ := url.Parse(endpoint)
	port := u.Port()
	if port == "" {
		port = "443"
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), InsecureSkipVerify: true}} // no application data is sent
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return Certificate{}, fmt.Errorf("read Proxmox VE certificate: %w", err)
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	leaf := state.PeerCertificates[0]
	now := time.Now()
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return Certificate{}, fmt.Errorf("the Proxmox VE certificate is expired or not yet valid")
	}
	intermediates := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		intermediates.AddCert(c)
	}
	_, verifyErr := leaf.Verify(x509.VerifyOptions{DNSName: u.Hostname(), Intermediates: intermediates})
	return Certificate{Endpoint: endpoint, Trusted: verifyErr == nil, Fingerprint: fingerprint(leaf), Subject: leaf.Subject.String(), Issuer: leaf.Issuer.String(), ExpiresAt: leaf.NotAfter}, nil
}

func (client *Client) transport(c Credentials) (*http.Client, func(), error) {
	copy := *client.http
	if c.Fingerprint == "" {
		return &copy, func() {}, nil
	}
	base, ok := copy.Transport.(*http.Transport)
	if !ok {
		return nil, nil, fmt.Errorf("certificate pinning requires an HTTP transport")
	}
	t := base.Clone()
	t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return fmt.Errorf("Proxmox VE did not present a certificate")
		}
		cert := state.PeerCertificates[0]
		if fingerprint(cert) != strings.ToLower(c.Fingerprint) {
			return fmt.Errorf("Proxmox VE certificate changed; approve its new fingerprint")
		}
		now := time.Now()
		if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
			return fmt.Errorf("Proxmox VE certificate expired or not yet valid")
		}
		return nil
	}}
	copy.Transport = t
	return &copy, t.CloseIdleConnections, nil
}

func (client *Client) request(ctx context.Context, endpoint, method, path string, c Credentials, form url.Values, target any) error {
	if err := validateCredentials(c); err != nil {
		return err
	}
	httpClient, closeIdle, err := client.transport(c)
	if err != nil {
		return err
	}
	defer closeIdle()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint+"/api2/json"+path, body)
	if err != nil {
		return fmt.Errorf("prepare Proxmox VE request: %w", err)
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.TokenID+"="+c.Secret)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Proxmox VE request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &responseError{method: method, path: strings.Split(path, "?")[0], status: response.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponse+1))
	if err != nil {
		return fmt.Errorf("read Proxmox VE response: %w", err)
	}
	if len(raw) > maximumResponse {
		return fmt.Errorf("Proxmox VE response exceeds the size limit")
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("invalid Proxmox VE JSON response")
	}
	if len(envelope.Data) == 0 {
		return fmt.Errorf("Proxmox VE response has no data field")
	}
	if target == nil {
		return nil
	}
	if string(envelope.Data) == "null" {
		return fmt.Errorf("Proxmox VE returned incomplete data")
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return fmt.Errorf("invalid Proxmox VE data")
	}
	return nil
}

func (client *Client) Inspect(ctx context.Context, address string, c Credentials) (Inspection, error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return Inspection{}, err
	}
	var version struct {
		Version string `json:"version"`
	}
	if err := client.request(ctx, endpoint, http.MethodGet, "/version", c, nil, &version); err != nil {
		return Inspection{}, err
	}
	major, err := strconv.Atoi(strings.Split(version.Version, ".")[0])
	if err != nil || major < 8 || major > 9 {
		return Inspection{}, fmt.Errorf("Proxmox VE %s is unsupported; supported API versions are 8 and 9", version.Version)
	}
	resources, err := client.Resources(ctx, endpoint, c)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{Endpoint: endpoint, Version: version.Version, Resources: resources}, nil
}

// Resources checks effective propagated permissions on every cycle. The API
// otherwise silently filters its inventory instead of returning forbidden.
func (client *Client) Resources(ctx context.Context, address string, c Credentials) ([]Resource, error) {
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return nil, err
	}
	var permissions map[string]map[string]int
	if err := client.request(ctx, endpoint, http.MethodGet, "/access/permissions?path=/", c, nil, &permissions); err != nil {
		return nil, err
	}
	for _, privilege := range []string{"Sys.Audit", "VM.Audit", "Datastore.Audit"} {
		if permissions["/"][privilege] != 1 {
			return nil, fmt.Errorf("Proxmox VE requires propagated %s at / for the user and token (PVEAuditor)", privilege)
		}
	}
	var raw []Resource
	if err := client.request(ctx, endpoint, http.MethodGet, "/cluster/resources", c, nil, &raw); err != nil {
		return nil, err
	}
	result := make([]Resource, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		switch r.Type {
		case "node", "qemu", "lxc", "storage":
		default:
			continue
		}
		if r.Node == "" || strings.ContainsAny(r.Node, "/\r\n") {
			return nil, fmt.Errorf("invalid Proxmox VE node identity")
		}
		expected := "node/" + r.Node
		if r.Guest() {
			if r.VMID < 100 {
				return nil, fmt.Errorf("invalid Proxmox VE guest identity")
			}
			expected = r.Type + "/" + strconv.Itoa(r.VMID)
		}
		if r.Type == "storage" {
			if r.Storage == "" {
				return nil, fmt.Errorf("invalid Proxmox VE storage identity")
			}
			expected = "storage/" + r.Node + "/" + r.Storage
		}
		if r.ID != expected || seen[r.ID] {
			return nil, fmt.Errorf("duplicate or inconsistent Proxmox VE resource identity")
		}
		seen[r.ID] = true
		if r.Name == "" {
			r.Name = r.Node
			if r.Guest() {
				r.Name = r.ID
			}
			if r.Type == "storage" {
				r.Name = r.Storage + " · " + r.Node
			}
		}
		if len(r.Name) > 160 {
			return nil, fmt.Errorf("Proxmox VE resource name exceeds 160 characters")
		}
		result = append(result, r)
	}
	if len(result) == 0 {
		return nil, errors.New("Proxmox VE inventory is empty; check token permissions")
	}
	nodes := map[string]string{}
	for _, resource := range result {
		if resource.Type == "node" {
			nodes[resource.Node] = resource.Status
		}
	}
	for index := range result {
		if result[index].Guest() {
			result[index].HostUnavailable = nodes[result[index].Node] != "online"
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
