package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/metrics"
)

type fakeMetrics struct {
	listed []metrics.TargetMetrics
	detail metrics.TargetDetail
	err    error
}

func (fake fakeMetrics) List(context.Context) ([]metrics.TargetMetrics, error) {
	return fake.listed, fake.err
}

func (fake fakeMetrics) Target(context.Context, string) (metrics.TargetDetail, error) {
	if fake.err != nil {
		return metrics.TargetDetail{}, fake.err
	}
	return fake.detail, nil
}

func TestMetricsListCarriesTheDayWindow(t *testing.T) {
	t.Parallel()

	availability := 0.995
	fake := fakeMetrics{listed: []metrics.TargetMetrics{{
		TargetID: "4f2a1c58-6f1f-4a1c-9a41-9d2f0b1c8e77",
		Measures: []domain.Measure{{Window: domain.WindowDay, Availability: &availability, ConclusiveObservations: 200}},
		Trend:    []float64{1, 0.5},
		Sources: []metrics.SourceMetrics{{
			SourceID: "9a1d0f28-6d3b-4f52-8f0e-2c1a5d7b3e94", Name: "Hôte Zabbix", Kind: "zabbix",
		}},
	}}}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Metrics: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/targets", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"window":"24h"`) || !strings.Contains(body, `"availability":0.995`) {
		t.Fatalf("unexpected measures: %s", body)
	}
	if !strings.Contains(body, `"name":"Hôte Zabbix"`) || !strings.Contains(body, `"kind":"zabbix"`) {
		t.Fatalf("the list must carry its source identities: %s", body)
	}
	// Une mesure absente reste explicitement nulle : le client ne doit jamais
	// avoir à distinguer « zéro » de « rien à dire ».
	if !strings.Contains(body, `"coverage":null`) {
		t.Fatalf("an absent measure must be null: %s", body)
	}
}

func TestMetricsListRequiresAuthentication(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Metrics: fakeMetrics{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/targets", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestTargetMetricsRejectsMalformedIdentifier(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Metrics: fakeMetrics{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/targets/not-a-uuid/metrics", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestTargetMetricsReportsUnknownTarget(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Metrics: fakeMetrics{err: metrics.ErrNotFound}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/targets/4f2a1c58-6f1f-4a1c-9a41-9d2f0b1c8e77/metrics", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestTargetMetricsOpensTheThreeWindows(t *testing.T) {
	t.Parallel()

	detail := metrics.TargetDetail{
		TargetID: "4f2a1c58-6f1f-4a1c-9a41-9d2f0b1c8e77",
		Measures: []domain.Measure{
			{Window: domain.WindowDay}, {Window: domain.WindowWeek}, {Window: domain.WindowMonth},
		},
		Sources: []metrics.SourceMetrics{{SourceID: "9a1d0f28-6d3b-4f52-8f0e-2c1a5d7b3e94", Name: "Endpoint public"}},
	}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Metrics: fakeMetrics{detail: detail}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/targets/4f2a1c58-6f1f-4a1c-9a41-9d2f0b1c8e77/metrics", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, body)
	}
	for _, window := range []string{`"window":"24h"`, `"window":"7d"`, `"window":"30d"`} {
		if !strings.Contains(body, window) {
			t.Fatalf("missing %s in %s", window, body)
		}
	}
	if !strings.Contains(body, `"name":"Endpoint public"`) {
		t.Fatalf("the detail must name each source: %s", body)
	}
}
