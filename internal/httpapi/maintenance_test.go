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

type fakeMaintenances struct{ actor, action string }

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

func (fake *fakeMaintenances) CancelSeries(_ context.Context, id, actor string) (maintenance.Maintenance, error) {
	fake.actor, fake.action = actor, "series-cancellation"
	return maintenance.Maintenance{ID: id}, nil
}
func (fake *fakeMaintenances) Extend(_ context.Context, id, actor string, expected time.Time) (maintenance.Maintenance, error) {
	fake.actor, fake.action = actor, "extension"
	return maintenance.Maintenance{ID: id}, nil
}

func TestMaintenanceActionsRequireOperator(t *testing.T) {
	for _, action := range []string{"extension", "series-cancellation"} {
		for _, role := range []string{"observer", "operator", "administrator"} {
			t.Run(action+"/"+role, func(t *testing.T) {
				fake := &fakeMaintenances{}
				server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: role}, Maintenances: fake})
				request := httptest.NewRequest(http.MethodPost, "/api/v1/maintenances/30000000-0000-0000-0000-000000000003/"+action, strings.NewReader(`{"expected_ends_at":"2026-10-01T03:00:00Z"}`))
				request.Header.Set("Content-Type", "application/json")
				request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
				response := httptest.NewRecorder()
				server.Handler.ServeHTTP(response, request)
				if role == "observer" {
					if response.Code != http.StatusForbidden || fake.actor != "" {
						t.Fatalf("observer reached mutation: %d", response.Code)
					}
				} else if response.Code != http.StatusOK || fake.action != action || fake.actor != "user-id" {
					t.Fatalf("action rejected: %d %s", response.Code, response.Body.String())
				}
			})
		}
	}
}

func TestMaintenanceCreationAndExtensionRejectInvalidJSON(t *testing.T) {
	for _, path := range []string{"/api/v1/maintenances", "/api/v1/maintenances/30000000-0000-0000-0000-000000000003/extension"} {
		for _, body := range []string{`{`, `{"unexpected":true}`, `{} {}`, `{"expected_ends_at":"not-a-date"}`} {
			t.Run(path+"/"+body, func(t *testing.T) {
				fake := &fakeMaintenances{}
				server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Maintenances: fake})
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
				response := httptest.NewRecorder()
				server.Handler.ServeHTTP(response, request)
				if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"error"`) || fake.actor != "" {
					t.Fatalf("invalid request reached service or returned success: status=%d actor=%q body=%s", response.Code, fake.actor, response.Body.String())
				}
			})
		}
	}
}
