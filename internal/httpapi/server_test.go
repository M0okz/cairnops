package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type pingerFunc func(context.Context) error

func (fn pingerFunc) Ping(ctx context.Context) error { return fn(ctx) }

func TestLiveness(t *testing.T) {
	server := NewServer(ServerOptions{Pinger: pingerFunc(func(context.Context) error { return nil }), Service: "test"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("expected security headers")
	}
}

func TestRequestLoggerRedactsHeartbeatToken(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := requestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat/super-secret-token", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if bytes.Contains(output.Bytes(), []byte("super-secret-token")) {
		t.Fatalf("heartbeat token leaked into request log: %s", output.String())
	}
	if !bytes.Contains(output.Bytes(), []byte("[redacted]")) {
		t.Fatalf("expected redacted path, got %s", output.String())
	}
}

func TestReadinessFailsClosed(t *testing.T) {
	server := NewServer(ServerOptions{
		Pinger:  pingerFunc(func(context.Context) error { return errors.New("offline") }),
		Service: "test",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}
