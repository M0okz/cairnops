package checks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

const maximumHTTPBody = 1024 * 1024

type HTTP struct{}

type HTTPConfig struct {
	URL              string            `json:"url"`
	Method           string            `json:"method,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Body             string            `json:"body,omitempty"`
	AcceptedStatuses []int             `json:"accepted_statuses,omitempty"`
	Contains         string            `json:"contains,omitempty"`
	FollowRedirects  *bool             `json:"follow_redirects,omitempty"`
}

func (HTTP) Check(ctx context.Context, source domain.Source) domain.Observation {
	startedAt := time.Now()
	var config HTTPConfig
	if err := decodeConfig(source.Config, &config); err != nil {
		return unknown(source, startedAt, "invalid_config", err)
	}
	if config.URL == "" {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("url is required"))
	}
	method := strings.ToUpper(config.Method)
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("method must be GET, HEAD, or POST"))
	}

	request, err := http.NewRequestWithContext(ctx, method, config.URL, bytes.NewBufferString(config.Body))
	if err != nil {
		return unknown(source, startedAt, "invalid_request", err)
	}
	for name, value := range config.Headers {
		request.Header.Set(name, value)
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: source.Timeout}).DialContext,
		TLSHandshakeTimeout:   source.Timeout,
		ResponseHeaderTimeout: source.Timeout,
		ForceAttemptHTTP2:     true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: source.Timeout}
	if config.FollowRedirects != nil && !*config.FollowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	}

	response, err := client.Do(request)
	if err != nil {
		return unhealthy(source, startedAt, "http_request_failed", err)
	}
	defer response.Body.Close()

	observation := healthy(source, startedAt)
	observation.Details["status_code"] = response.StatusCode
	if !acceptedStatus(response.StatusCode, config.AcceptedStatuses) {
		observation.Outcome = domain.OutcomeUnhealthy
		observation.Reason = "unexpected_status"
		observation.Message = fmt.Sprintf("received HTTP status %d", response.StatusCode)
		return observation
	}
	if config.Contains == "" || method == http.MethodHead {
		return observation
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maximumHTTPBody+1))
	if err != nil {
		return unhealthy(source, startedAt, "response_read_failed", err)
	}
	if len(body) > maximumHTTPBody {
		return unhealthy(source, startedAt, "response_too_large", fmt.Errorf("response exceeds %d bytes", maximumHTTPBody))
	}
	observation.Details["response_bytes"] = len(body)
	if !bytes.Contains(body, []byte(config.Contains)) {
		observation.Outcome = domain.OutcomeUnhealthy
		observation.Reason = "content_missing"
		observation.Message = "expected response content was not found"
	}
	return observation
}

func acceptedStatus(status int, accepted []int) bool {
	if len(accepted) == 0 {
		return status >= 200 && status < 400
	}
	for _, candidate := range accepted {
		if status == candidate {
			return true
		}
	}
	return false
}
