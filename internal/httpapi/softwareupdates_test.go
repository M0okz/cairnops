package httpapi

import (
	"context"
	"github.com/M0okz/cairnops/internal/softwareupdates"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeSoftware struct {
	actor string
	saved bool
}

func (f *fakeSoftware) List(context.Context, string) ([]softwareupdates.Service, error) {
	return []softwareupdates.Service{}, nil
}
func (f *fakeSoftware) Get(context.Context, string) (softwareupdates.Service, error) {
	return softwareupdates.Service{}, nil
}
func (f *fakeSoftware) Confirm(_ context.Context, _ string, actor string, _ softwareupdates.Source) error {
	f.actor = actor
	return nil
}
func (f *fakeSoftware) Config(context.Context) (softwareupdates.AIConfig, error) {
	return softwareupdates.AIConfig{}, nil
}
func (f *fakeSoftware) SaveConfig(context.Context, softwareupdates.AIConfig) error {
	f.saved = true
	return nil
}
func TestSoftwareSettingsRequireAdministratorAndSameOrigin(t *testing.T) {
	for _, tt := range []struct {
		role, origin string
		want         int
	}{{"observer", "", 403}, {"operator", "", 403}, {"administrator", "https://evil.example", 403}, {"administrator", "http://localhost:8080", 200}} {
		f := &fakeSoftware{}
		s := NewServer(ServerOptions{PublicURL: "http://localhost:8080", Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: tt.role}, SoftwareUpdates: f})
		req := httptest.NewRequest("PUT", "http://localhost:8080/api/v1/software-update-settings", strings.NewReader(`{"enabled":true,"endpoint":"https://provider.example/v1","model":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		if tt.origin != "" {
			req.Header.Set("Origin", tt.origin)
		}
		req.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
		rec := httptest.NewRecorder()
		s.Handler.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Fatalf("%s %s: %d %s", tt.role, tt.origin, rec.Code, rec.Body.String())
		}
		if f.saved != (tt.want == 200) {
			t.Fatal("unauthorized mutation")
		}
	}
}
func TestSoftwareReadNeedsSessionAndRejectsInvalidTarget(t *testing.T) {
	f := &fakeSoftware{}
	s := NewServer(ServerOptions{Identity: &fakeIdentity{}, SoftwareUpdates: f})
	for _, tt := range []struct {
		path string
		auth bool
		want int
	}{{"/api/v1/software-updates", false, 401}, {"/api/v1/software-updates", true, 200}, {"/api/v1/software-updates?target_id=bad", true, 400}} {
		req := httptest.NewRequest("GET", tt.path, nil)
		if tt.auth {
			req.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
		}
		rec := httptest.NewRecorder()
		s.Handler.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Fatalf("%s: %d", tt.path, rec.Code)
		}
	}
}
