package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/maintenance"
)

type fakeMaintenances struct{ actor string }

func (*fakeMaintenances) List(context.Context, int) ([]maintenance.Maintenance, error) {
	return nil, nil
}
func (fake *fakeMaintenances) Create(_ context.Context, actor string, input maintenance.CreateInput) (maintenance.Maintenance, error) {
	fake.actor = actor
	return maintenance.Maintenance{ID: "30000000-0000-0000-0000-000000000003", Name: input.Name}, nil
}
func (fake *fakeMaintenances) Cancel(_ context.Context, id, actor string) (maintenance.Maintenance, error) {
	fake.actor = actor
	return maintenance.Maintenance{ID: id, State: "cancelled"}, nil
}

func TestMaintenanceCreationAllowsOperator(t *testing.T) {
	t.Parallel()
	fake := &fakeMaintenances{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Maintenances: fake})
	body := `{"name":"Maintenance réseau","reason":"Remplacement du routeur principal","target_ids":["10000000-0000-0000-0000-000000000001"],"ends_at":"` + time.Now().UTC().Add(time.Hour).Format(time.RFC3339) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/maintenances", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || fake.actor != "user-id" {
		t.Fatalf("expected operator creation, status=%d actor=%q body=%s", response.Code, fake.actor, response.Body.String())
	}
}

func TestMaintenanceCreationRejectsObserver(t *testing.T) {
	t.Parallel()
	fake := &fakeMaintenances{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Maintenances: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/maintenances", strings.NewReader(`{}`))
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || fake.actor != "" {
		t.Fatalf("observer reached maintenance creation, status=%d actor=%q", response.Code, fake.actor)
	}
}
