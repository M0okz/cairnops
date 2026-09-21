package argus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"time"
)

// resolveAddress probes without credentials. An explicit scheme is authoritative;
// certificate errors and HTTP error responses never cause a downgrade.
func (client *Client) resolveAddress(ctx context.Context, address string) (string, error) {
	address = strings.TrimSpace(address)
	if strings.Contains(address, "://") {
		return NormalizeEndpoint(address)
	}
	if address == "" || len(address) > 2048 || strings.HasPrefix(address, "/") {
		return "", fmt.Errorf("address must contain a host, optionally preceded by http:// or https://")
	}
	endpoint, err := NormalizeEndpoint("https://" + address)
	if err != nil {
		return "", err
	}
	probe := *client.http
	probe.Jar = nil
	probe.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint+"/api/v1/version", nil)
	if err != nil {
		return "", err
	}
	response, err := probe.Do(request)
	if err == nil {
		response.Body.Close()
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			return "", fmt.Errorf("detect Argus protocol: redirects are not allowed")
		}
		return endpoint, nil
	}
	if errors.Is(err, http.ErrSchemeMismatch) || errors.Is(err, syscall.ECONNREFUSED) {
		endpoint = "http://" + strings.TrimPrefix(endpoint, "https://")
		request, err = http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint+"/api/v1/version", nil)
		if err != nil {
			return "", err
		}
		response, err = probe.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode >= 300 && response.StatusCode < 400 {
				return "", fmt.Errorf("detect Argus protocol: redirects are not allowed")
			}
			return endpoint, nil
		}
	}
	return "", fmt.Errorf("detect Argus protocol: %w", err)
}
