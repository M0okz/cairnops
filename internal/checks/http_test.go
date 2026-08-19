package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

func TestHTTPCheckAcceptsStatusAndContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("cairnops ready"))
	}))
	defer server.Close()

	config, _ := json.Marshal(HTTPConfig{URL: server.URL, AcceptedStatuses: []int{202}, Contains: "ready"})
	source := testSource(domain.SourceHTTP, config)
	result := (HTTP{}).Check(context.Background(), source)
	if result.Outcome != domain.OutcomeHealthy {
		t.Fatalf("expected healthy observation, got %#v", result)
	}
}

func TestHTTPCheckRejectsUnexpectedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	config, _ := json.Marshal(HTTPConfig{URL: server.URL})
	result := (HTTP{}).Check(context.Background(), testSource(domain.SourceHTTP, config))
	if result.Outcome != domain.OutcomeUnhealthy || result.Reason != "unexpected_status" {
		t.Fatalf("expected unexpected status, got %#v", result)
	}
}

func testSource(kind domain.SourceKind, config json.RawMessage) domain.Source {
	return domain.Source{
		ID: "source", TargetID: "target", Name: "test", Kind: kind,
		Interval: time.Minute, Timeout: 2 * time.Second, Config: config,
	}
}
