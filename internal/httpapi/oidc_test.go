package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/oidcauth"
)

type fakeOIDC struct {
	input oidcauth.ConfigurationInput
}

func (*fakeOIDC) PublicStatus(context.Context) (oidcauth.PublicStatus, error) {
	return oidcauth.PublicStatus{Enabled: true, Label: "Authentik"}, nil
}

func (*fakeOIDC) Configurations(context.Context) (oidcauth.ConfigurationSet, error) {
	return oidcauth.ConfigurationSet{Active: &oidcauth.Configuration{ID: "configuration-id", State: "active", Label: "Authentik"}}, nil
}

func (fake *fakeOIDC) SaveDraft(_ context.Context, _ string, input oidcauth.ConfigurationInput) (oidcauth.Configuration, error) {
	fake.input = input
	return oidcauth.Configuration{ID: "draft-id", State: "draft", Label: input.Label}, nil
}

func (*fakeOIDC) Activate(context.Context) (oidcauth.Configuration, error) {
	return oidcauth.Configuration{ID: "configuration-id", State: "active", Label: "Authentik"}, nil
}

func (*fakeOIDC) Begin(context.Context, string, string) (oidcauth.Authorization, error) {
	return oidcauth.Authorization{URL: "https://auth.example.net/authorize?state=state", State: "state"}, nil
}

func (*fakeOIDC) Complete(context.Context, string, string) (oidcauth.Completion, error) {
	return oidcauth.Completion{Purpose: "login", ReturnTo: "/", Session: testAuthenticatedSession()}, nil
}

func TestOIDCStatusIsPublicAndDoesNotExposeConfiguration(t *testing.T) {
	t.Parallel()
	server := NewServer(ServerOptions{Identity: &fakeIdentity{initialized: true}, OIDC: &fakeOIDC{}})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/oidc/status", nil))
	if response.Code != http.StatusOK || response.Body.String() != "{\"enabled\":true,\"label\":\"Authentik\"}\n" {
		t.Fatalf("unexpected public OIDC status: %d %s", response.Code, response.Body)
	}
}

func TestOIDCConfigurationRequiresLocalAdministrator(t *testing.T) {
	t.Parallel()
	oidc := &fakeOIDC{}
	operator := &accountIdentity{fakeIdentity: &fakeIdentity{initialized: true}, role: "operator"}
	server := NewServer(ServerOptions{Identity: operator, OIDC: oidc})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/configuration", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("operator read the OIDC configuration: %d %s", response.Code, response.Body)
	}

	externalAdministrator := &accountIdentity{fakeIdentity: &fakeIdentity{initialized: true}, role: "administrator", regime: "external"}
	server = NewServer(ServerOptions{Identity: externalAdministrator, OIDC: oidc})
	request = httptest.NewRequest(http.MethodGet, "/api/v1/oidc/configuration", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("external administrator read the authority configuration: %d %s", response.Code, response.Body)
	}
}

func TestOIDCLoginBindsTheFlowToTheBrowser(t *testing.T) {
	t.Parallel()
	server := NewServer(ServerOptions{
		PublicURL: "https://cairnops.example.net",
		Identity:  &fakeIdentity{initialized: true}, OIDC: &fakeOIDC{},
	})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/oidc/login", nil))
	cookies := response.Result().Cookies()
	if response.Code != http.StatusFound || response.Header().Get("Location") != "https://auth.example.net/authorize?state=state" || len(cookies) != 1 {
		t.Fatalf("OIDC login did not start a bound flow: status=%d location=%q cookies=%#v", response.Code, response.Header().Get("Location"), cookies)
	}
	flowCookie := cookies[0]
	if flowCookie.Name != "__Host-cairnops_oidc_state" || flowCookie.Value != "login:state" || !flowCookie.Secure || !flowCookie.HttpOnly || flowCookie.SameSite != http.SameSiteLaxMode || flowCookie.Path != "/" {
		t.Fatalf("OIDC flow cookie is not hardened: %#v", flowCookie)
	}
}

func TestOIDCCallbackSetsTheSameHardenedSessionCookie(t *testing.T) {
	t.Parallel()
	server := NewServer(ServerOptions{
		PublicURL: "https://cairnops.example.net",
		Identity:  &fakeIdentity{initialized: true}, OIDC: &fakeOIDC{},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state=state&code=code", nil)
	request.AddCookie(&http.Cookie{Name: "__Host-cairnops_oidc_state", Value: "login:state"})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if response.Code != http.StatusFound || len(cookies) != 2 {
		t.Fatalf("OIDC callback did not open a session: status=%d cookies=%#v", response.Code, cookies)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "__Host-cairnops_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("OIDC session cookie is absent: %#v", cookies)
	}
	if sessionCookie.Name != "__Host-cairnops_session" || !sessionCookie.Secure || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("OIDC session cookie is not hardened: %#v", sessionCookie)
	}
	if strings.Contains(response.Body.String(), testSessionToken) {
		t.Fatal("OIDC session token leaked in the redirect body")
	}
}

func TestOIDCCallbackRejectsAStateFromAnotherBrowser(t *testing.T) {
	t.Parallel()
	server := NewServer(ServerOptions{
		PublicURL: "https://cairnops.example.net",
		Identity:  &fakeIdentity{initialized: true}, OIDC: &fakeOIDC{},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state=attacker-state&code=code", nil)
	request.AddCookie(&http.Cookie{Name: "__Host-cairnops_oidc_state", Value: "login:victim-state"})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/?oidc_error=invalid_flow" {
		t.Fatalf("cross-browser OIDC callback was accepted: %d %s", response.Code, response.Header().Get("Location"))
	}
}

func TestOIDCTestFailureReturnsToConfiguration(t *testing.T) {
	t.Parallel()
	server := NewServer(ServerOptions{
		PublicURL: "https://cairnops.example.net",
		Identity:  &fakeIdentity{initialized: true}, OIDC: &fakeOIDC{},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/oidc/callback?state=state&error=access_denied", nil)
	request.AddCookie(&http.Cookie{Name: "__Host-cairnops_oidc_state", Value: "test:state"})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/reglages?oidc_test=failed" {
		t.Fatalf("failed OIDC test lost its configuration context: %d %s", response.Code, response.Header().Get("Location"))
	}
}

var _ OIDC = (*fakeOIDC)(nil)
