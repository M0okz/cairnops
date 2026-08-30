package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

const testSessionToken = "test-session-token"

type fakeIdentity struct {
	initialized      bool
	initializedInput identitymodel.InitializeInput
	instanceName     string
	loggedOut        bool
}

func (fake *fakeIdentity) SetupStatus(context.Context) (identitymodel.Status, error) {
	return identitymodel.Status{Initialized: fake.initialized, Name: fake.instanceName}, nil
}

func (fake *fakeIdentity) RenameInstance(_ context.Context, name string) (identitymodel.Status, error) {
	fake.instanceName = name
	return identitymodel.Status{Initialized: fake.initialized, Name: name}, nil
}

func (fake *fakeIdentity) Initialize(_ context.Context, input identitymodel.InitializeInput) (identitymodel.AuthenticatedSession, error) {
	if fake.initialized {
		return identitymodel.AuthenticatedSession{}, identitymodel.ErrAlreadyInitialized
	}
	fake.initialized = true
	fake.initializedInput = input
	return testAuthenticatedSession(), nil
}

func (fake *fakeIdentity) Login(_ context.Context, input identitymodel.LoginInput) (identitymodel.AuthenticatedSession, error) {
	if input.Username != "gregory" || input.Password != "correct-password" {
		return identitymodel.AuthenticatedSession{}, identitymodel.ErrInvalidCredentials
	}
	return testAuthenticatedSession(), nil
}

func (*fakeIdentity) Authenticate(_ context.Context, token string) (identitymodel.Principal, error) {
	if token != testSessionToken {
		return identitymodel.Principal{}, identitymodel.ErrInvalidSession
	}
	return testAuthenticatedSession().Principal, nil
}

func (fake *fakeIdentity) Logout(_ context.Context, token string) error {
	if token != testSessionToken {
		return identitymodel.ErrInvalidSession
	}
	fake.loggedOut = true
	return nil
}

func testAuthenticatedSession() identitymodel.AuthenticatedSession {
	return identitymodel.AuthenticatedSession{
		Principal: identitymodel.Principal{ID: "user-id", Username: "gregory", DisplayName: "Gregory", Role: "administrator", AuthorizationRegime: "local"},
		ExpiresAt: time.Now().Add(time.Hour), Token: testSessionToken,
	}
}

func TestSetupRequiresExactBootstrapTokenAndSetsSession(t *testing.T) {
	t.Parallel()

	const bootstrap = "bootstrap-token-with-at-least-32-characters"
	fake := &fakeIdentity{}
	server := NewServer(ServerOptions{BootstrapToken: bootstrap, Identity: fake})
	body, _ := json.Marshal(identitymodel.InitializeInput{InstanceName: "Astreinte Nord", Username: "gregory", DisplayName: "Gregory", Password: "a-long-password"})

	unauthenticated := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	unauthenticated.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, unauthenticated)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bootstrap token, got %d", response.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+bootstrap)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if fake.initializedInput.Username != "gregory" || fake.initializedInput.InstanceName != "Astreinte Nord" {
		t.Fatalf("unexpected setup input: %#v", fake.initializedInput)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "cairnops_session" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	if strings.Contains(response.Body.String(), testSessionToken) {
		t.Fatal("session token leaked into JSON response")
	}
}

func TestSecurePublicURLUsesHostPrefixedCookie(t *testing.T) {
	t.Parallel()

	const bootstrap = "bootstrap-token-with-at-least-32-characters"
	server := NewServer(ServerOptions{PublicURL: "https://cairnops.example.com", BootstrapToken: bootstrap, Identity: &fakeIdentity{}})
	body := `{"username":"gregory","display_name":"Gregory","password":"a-long-password"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+bootstrap)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://cairnops.example.com")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	cookies := response.Result().Cookies()
	if response.Code != http.StatusCreated || len(cookies) != 1 || cookies[0].Name != "__Host-cairnops_session" || !cookies[0].Secure {
		t.Fatalf("expected secure host cookie, status=%d cookies=%#v", response.Code, cookies)
	}
}

func TestSessionEndpointsAuthenticateAndLogout(t *testing.T) {
	t.Parallel()

	fake := &fakeIdentity{initialized: true}
	server := NewServer(ServerOptions{Identity: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "gregory") {
		t.Fatalf("expected authenticated session, got %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !fake.loggedOut {
		t.Fatalf("expected session logout, got %d loggedOut=%v", response.Code, fake.loggedOut)
	}
}

func TestSessionLoginDoesNotDiscloseWhichCredentialFailed(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{initialized: true}})
	for _, body := range []string{
		`{"username":"missing","password":"correct-password"}`,
		`{"username":"gregory","password":"wrong-password"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"invalid credentials\"}\n" {
			t.Fatalf("unexpected login response: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestMutatingSessionRouteRejectsForeignOrigin(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{PublicURL: "https://cairnops.example.com", Identity: &fakeIdentity{initialized: true}})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	request.Header.Set("Origin", "https://hostile.example.net")
	request.AddCookie(&http.Cookie{Name: "__Host-cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

var _ Identity = (*fakeIdentity)(nil)

func (*fakeIdentity) ChangePassword(context.Context, string, string, string) error { return nil }

func (*fakeIdentity) SetPassword(context.Context, string, string) (identitymodel.Principal, error) {
	return identitymodel.Principal{}, nil
}

func (*fakeIdentity) RecoverPassword(context.Context, string, string) (identitymodel.Principal, error) {
	return identitymodel.Principal{}, nil
}

func (*fakeIdentity) ListAccounts(context.Context) ([]identitymodel.Account, error) { return nil, nil }

func (*fakeIdentity) CountActiveSessions(context.Context, string) (int, error) { return 1, nil }

func (*fakeIdentity) CreateAccount(context.Context, identitymodel.CreateAccountInput) (identitymodel.Account, error) {
	return identitymodel.Account{}, nil
}

func (*fakeIdentity) UpdateAccount(context.Context, string, string, identitymodel.UpdateAccountInput) (identitymodel.Account, error) {
	return identitymodel.Account{}, nil
}

func (*fakeIdentity) SetAccountActivation(context.Context, string, string, bool) (identitymodel.Account, error) {
	return identitymodel.Account{}, nil
}

// Le nom de l'instance se lit sans session — la porte d'entrée doit dire où
// l'on frappe — mais ne se change que par un Administrateur.
func TestInstanceNameIsPublicToReadAndReservedToRename(t *testing.T) {
	t.Parallel()

	server, identity := accountServer("administrator")
	identity.instanceName = "Astreinte Nord"

	status := httptest.NewRecorder()
	server.Handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"name":"Astreinte Nord"`) {
		t.Fatalf("the gate cannot read the instance name: status=%d body=%s", status.Code, status.Body)
	}

	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodPatch, "/api/v1/instance", map[string]string{"name": "Production Sud"}))
	if response.Code != http.StatusOK || identity.instanceName != "Production Sud" {
		t.Fatalf("the administrator could not rename the instance: status=%d name=%q", response.Code, identity.instanceName)
	}

	operatorServer, operatorIdentity := accountServer("operator")
	operatorIdentity.instanceName = "Astreinte Nord"
	refused := httptest.NewRecorder()
	operatorServer.Handler.ServeHTTP(refused, accountRequest(http.MethodPatch, "/api/v1/instance", map[string]string{"name": "Production Sud"}))
	if refused.Code != http.StatusForbidden || operatorIdentity.instanceName != "Astreinte Nord" {
		t.Fatalf("an operator renamed the instance: status=%d name=%q", refused.Code, operatorIdentity.instanceName)
	}
}
