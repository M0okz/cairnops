package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

// accountIdentity retient ce que le transport a transmis au service, car c'est
// tout ce que cette couche décide : qui agit, sur qui, et vers quel état.
type accountIdentity struct {
	*fakeIdentity
	role       string
	regime     string
	created    identitymodel.CreateAccountInput
	updated    identitymodel.UpdateAccountInput
	actorID    string
	targetID   string
	activation *bool
	err        error
}

func (identity *accountIdentity) Authenticate(_ context.Context, token string) (identitymodel.Principal, error) {
	if token != testSessionToken {
		return identitymodel.Principal{}, identitymodel.ErrInvalidSession
	}
	principal := testAuthenticatedSession().Principal
	principal.Role = identity.role
	if identity.regime != "" {
		principal.AuthorizationRegime = identity.regime
	}
	return principal, nil
}

func (identity *accountIdentity) ListAccounts(context.Context) ([]identitymodel.Account, error) {
	deactivated := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	return []identitymodel.Account{
		{Principal: identitymodel.Principal{ID: "one", Username: "camille", Role: "operator"}},
		{Principal: identitymodel.Principal{ID: "two", Username: "sacha", Role: "observer"}, DeactivatedAt: &deactivated},
	}, nil
}

func (identity *accountIdentity) CreateAccount(_ context.Context, input identitymodel.CreateAccountInput) (identitymodel.Account, error) {
	identity.created = input
	if identity.err != nil {
		return identitymodel.Account{}, identity.err
	}
	return identitymodel.Account{Principal: identitymodel.Principal{ID: "new", Username: input.Username, Role: input.Role}}, nil
}

func (identity *accountIdentity) UpdateAccount(_ context.Context, actorID, userID string, input identitymodel.UpdateAccountInput) (identitymodel.Account, error) {
	identity.actorID, identity.targetID, identity.updated = actorID, userID, input
	if identity.err != nil {
		return identitymodel.Account{}, identity.err
	}
	return identitymodel.Account{Principal: identitymodel.Principal{ID: userID}}, nil
}

func (identity *accountIdentity) SetAccountActivation(_ context.Context, actorID, userID string, active bool) (identitymodel.Account, error) {
	identity.actorID, identity.targetID, identity.activation = actorID, userID, &active
	if identity.err != nil {
		return identitymodel.Account{}, identity.err
	}
	return identitymodel.Account{Principal: identitymodel.Principal{ID: userID}}, nil
}

func accountServer(role string) (*http.Server, *accountIdentity) {
	identity := &accountIdentity{fakeIdentity: &fakeIdentity{}, role: role}
	return NewServer(ServerOptions{Identity: identity}), identity
}

func accountRequest(method, path string, body any) *http.Request {
	var reader *bytes.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	return request
}

func TestAccountCreationRequiresAdministratorAndReportsTheNewAccount(t *testing.T) {
	t.Parallel()

	server, identity := accountServer("administrator")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodPost, "/api/v1/users", identitymodel.CreateAccountInput{
		Username: "camille", DisplayName: "Camille", Role: "operator", Password: "phrase-de-camille-2026",
	}))
	if response.Code != http.StatusCreated {
		t.Fatalf("the administrator could not create an account: status=%d body=%s", response.Code, response.Body)
	}
	if identity.created.Username != "camille" || identity.created.Password != "phrase-de-camille-2026" {
		t.Fatalf("the service did not receive the request as written: %+v", identity.created)
	}

	operatorServer, operatorIdentity := accountServer("operator")
	response = httptest.NewRecorder()
	operatorServer.Handler.ServeHTTP(response, accountRequest(http.MethodPost, "/api/v1/users", identitymodel.CreateAccountInput{
		Username: "sacha", DisplayName: "Sacha", Role: "administrator", Password: "phrase-de-sacha-2026",
	}))
	if response.Code != http.StatusForbidden || operatorIdentity.created.Username != "" {
		t.Fatalf("an operator opened an account: status=%d created=%+v", response.Code, operatorIdentity.created)
	}
}

func TestAccountUpdateCarriesTheActingAdministratorAndTheTarget(t *testing.T) {
	t.Parallel()

	server, identity := accountServer("administrator")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodPatch, "/api/v1/users/other-id", map[string]string{"role": "operator"}))
	if response.Code != http.StatusOK {
		t.Fatalf("the correction was refused: status=%d body=%s", response.Code, response.Body)
	}
	if identity.actorID != "user-id" || identity.targetID != "other-id" {
		t.Fatalf("the service cannot tell who acted on whom: actor=%q target=%q", identity.actorID, identity.targetID)
	}
	if identity.updated.Role == nil || *identity.updated.Role != "operator" || identity.updated.DisplayName != nil {
		t.Fatalf("an unnamed field was not left alone: %+v", identity.updated)
	}
}

func TestAccountDeactivationAndReactivationShareOneRoute(t *testing.T) {
	t.Parallel()

	server, identity := accountServer("administrator")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodPost, "/api/v1/users/other-id/deactivation", nil))
	if response.Code != http.StatusOK || identity.activation == nil || *identity.activation {
		t.Fatalf("the account was not deactivated: status=%d activation=%v", response.Code, identity.activation)
	}

	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodDelete, "/api/v1/users/other-id/deactivation", nil))
	if response.Code != http.StatusOK || identity.activation == nil || !*identity.activation {
		t.Fatalf("the account was not reactivated: status=%d activation=%v", response.Code, identity.activation)
	}
}

func TestAccountConflictAnswersWithoutTheInternalPrefix(t *testing.T) {
	t.Parallel()

	server, identity := accountServer("administrator")
	identity.err = identitymodel.ErrConflict
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodPost, "/api/v1/users/other-id/deactivation", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("a refused state change did not answer 409: status=%d body=%s", response.Code, response.Body)
	}

	identity.err = identitymodel.ErrNotFound
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodPatch, "/api/v1/users/absent", map[string]string{"display_name": "Personne"}))
	if response.Code != http.StatusNotFound {
		t.Fatalf("an absent account did not answer 404: status=%d body=%s", response.Code, response.Body)
	}
}

func TestAccountListingReportsWhoIsDeactivated(t *testing.T) {
	t.Parallel()

	server, _ := accountServer("administrator")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodGet, "/api/v1/users", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("the listing was refused: status=%d body=%s", response.Code, response.Body)
	}
	var payload struct {
		Users []struct {
			Username      string  `json:"username"`
			DeactivatedAt *string `json:"deactivated_at"`
		} `json:"users"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Users) != 2 || payload.Users[0].DeactivatedAt != nil || payload.Users[1].DeactivatedAt == nil {
		t.Fatalf("the listing does not say who is deactivated: %+v", payload.Users)
	}
}

func TestExternalAdministratorCanAuditAccountsButCannotGovernLocalIdentity(t *testing.T) {
	t.Parallel()
	server, identity := accountServer("administrator")
	identity.regime = "external"

	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodGet, "/api/v1/users", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("the external administrator could not audit accounts: status=%d body=%s", response.Code, response.Body)
	}

	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, accountRequest(http.MethodPost, "/api/v1/users", identitymodel.CreateAccountInput{
		Username: "local-backdoor", DisplayName: "Local backdoor", Role: "administrator", Password: "a-local-password-2026",
	}))
	if response.Code != http.StatusForbidden || identity.created.Username != "" {
		t.Fatalf("the external administrator governed local identity: status=%d created=%+v", response.Code, identity.created)
	}
}

// L'écriture reste soumise à la même origine, comme tous les gestes qui
// changent l'état de l'instance.
func TestAccountWritesRequireSameOrigin(t *testing.T) {
	t.Parallel()

	server, identity := accountServer("administrator")
	request := accountRequest(http.MethodPost, "/api/v1/users/other-id/deactivation", nil)
	request.Header.Set("Origin", "https://ailleurs.example.net")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || identity.activation != nil {
		t.Fatalf("a cross-origin request reached the service: status=%d activation=%v", response.Code, identity.activation)
	}
}
