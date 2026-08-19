package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/systemhealth"
)

type fakeSystemHealth struct {
	snapshot systemhealth.Snapshot
}

func (fake fakeSystemHealth) Snapshot(context.Context) (systemhealth.Snapshot, error) {
	return fake.snapshot, nil
}

func TestSystemHealthReturnsAuthenticatedProjection(t *testing.T) {
	t.Parallel()

	snapshot := systemhealth.Snapshot{
		Status:    "operational",
		CheckedAt: time.Now().UTC(),
		Components: []systemhealth.Component{
			{Name: "worker", Status: systemhealth.StatusOperational, Instances: 2},
		},
	}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, SystemHealth: fakeSystemHealth{snapshot: snapshot}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"instances":2`) {
		t.Fatalf("expected system health snapshot, got %d: %s", response.Code, response.Body.String())
	}
}

func TestSystemHealthRequiresAuthentication(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, SystemHealth: fakeSystemHealth{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}
