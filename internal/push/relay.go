package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type DeliveryRequest struct {
	Recipient   string    `json:"recipient"`
	Envelope    Envelope  `json:"envelope"`
	CollapseKey string    `json:"collapse_key"`
	Priority    string    `json:"priority"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Relay interface {
	Ping(context.Context) error
	Deliver(context.Context, DeliveryRequest) error
}

type HTTPError struct{ StatusCode int }

func (err *HTTPError) Error() string {
	return fmt.Sprintf("push relay returned HTTP %d", err.StatusCode)
}

func RecipientExpired(err error) bool {
	var httpError *HTTPError
	return errors.As(err, &httpError) && (httpError.StatusCode == http.StatusNotFound || httpError.StatusCode == http.StatusGone)
}

type RelayClient struct {
	baseURL string
	client  *http.Client
}

func NewRelayClient(baseURL string, client *http.Client) *RelayClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &RelayClient{baseURL: strings.TrimSuffix(baseURL, "/"), client: client}
}

func (client *RelayClient) Ping(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/v1/health", nil)
	if err != nil {
		return fmt.Errorf("build push relay health request: %w", err)
	}
	return client.do(request)
}

func (client *RelayClient) Deliver(ctx context.Context, delivery DeliveryRequest) error {
	body, err := json.Marshal(delivery)
	if err != nil {
		return fmt.Errorf("encode push relay delivery: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/v1/deliveries", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push relay delivery request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "CairnOps-Push/1")
	return client.do(request)
}

func (client *RelayClient) do(request *http.Request) error {
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("contact push relay: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	return nil
}
