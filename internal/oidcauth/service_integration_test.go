package oidcauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/testsupport"
)

type recordingSessionIssuer struct {
	userID string
}

func (issuer *recordingSessionIssuer) NewSession(_ context.Context, userID string) (identity.AuthenticatedSession, error) {
	issuer.userID = userID
	return identity.AuthenticatedSession{
		Principal: identity.Principal{ID: userID, AuthorizationRegime: "external"},
		Token:     "session-token", ExpiresAt: time.Now().Add(time.Hour),
	}, nil
}

func TestPostgresOIDCFlowAndSynchronizationEndToEnd(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var providerURL string
	var expectedNonce atomic.Value
	var currentGroup atomic.Value
	currentGroup.Store("ops")

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeProviderJSON(t, w, map[string]any{
				"issuer": providerURL, "authorization_endpoint": providerURL + "/authorize",
				"token_endpoint": providerURL + "/token", "userinfo_endpoint": providerURL + "/userinfo",
				"jwks_uri": providerURL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
			})
		case "/jwks":
			writeProviderJSON(t, w, map[string]any{"keys": []any{rsaJWK(&privateKey.PublicKey)}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			if r.FormValue("client_id") != "cairnops" || r.FormValue("client_secret") != "client-secret" {
				http.Error(w, "invalid client", http.StatusUnauthorized)
				return
			}
			response := map[string]any{
				"access_token": "access-token", "refresh_token": "rotated-refresh-token",
				"token_type": "Bearer", "expires_in": 300,
			}
			switch r.FormValue("grant_type") {
			case "authorization_code":
				if r.FormValue("code_verifier") == "" {
					http.Error(w, "PKCE verifier missing", http.StatusBadRequest)
					return
				}
				nonce, _ := expectedNonce.Load().(string)
				response["id_token"] = signedIDToken(t, privateKey, providerURL, "external-subject", "cairnops", nonce)
			case "refresh_token":
				if r.FormValue("refresh_token") == "" {
					http.Error(w, "refresh token missing", http.StatusBadRequest)
					return
				}
			default:
				http.Error(w, "unsupported grant", http.StatusBadRequest)
				return
			}
			writeProviderJSON(t, w, response)
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer access-token" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			writeProviderJSON(t, w, map[string]any{
				"sub": "external-subject", "name": "External User", "groups": []string{currentGroup.Load().(string)},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	providerURL = provider.URL

	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('flow-admin', 'Flow admin', 'sealed-local-hash', 'administrator')
		RETURNING id::text
	`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sessions := &recordingSessionIssuer{}
	service := NewService(pool, box, sessions, "https://cairnops.example.net", provider.Client())
	if _, err := service.SaveDraft(ctx, actorID, ConfigurationInput{
		Label: "Test provider", Issuer: providerURL, ClientID: "cairnops", ClientSecret: "client-secret",
		GroupsClaim: "groups", Groups: GroupMappings{Operator: []string{"ops"}, Observer: []string{"read"}},
	}); err != nil {
		t.Fatal(err)
	}

	testAuthorization, err := service.Begin(ctx, "test", "/reglages")
	if err != nil {
		t.Fatal(err)
	}
	expectedNonce.Store(assertAuthorizationRequest(t, testAuthorization))
	completion, err := service.Complete(ctx, testAuthorization.State, "test-code")
	if err != nil || completion.Purpose != "test" {
		t.Fatalf("interactive configuration test failed: %#v, %v", completion, err)
	}
	var externalUsers int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_users WHERE authorization_regime = 'external'`).Scan(&externalUsers); err != nil || externalUsers != 0 {
		t.Fatalf("configuration test created an external User: count=%d err=%v", externalUsers, err)
	}
	if _, err := service.Activate(ctx); err != nil {
		t.Fatal(err)
	}

	loginAuthorization, err := service.Begin(ctx, "login", "/incidents")
	if err != nil {
		t.Fatal(err)
	}
	expectedNonce.Store(assertAuthorizationRequest(t, loginAuthorization))
	completion, err = service.Complete(ctx, loginAuthorization.State, "login-code")
	if err != nil || completion.Purpose != "login" || completion.ReturnTo != "/incidents" || sessions.userID == "" {
		t.Fatalf("OIDC login failed: %#v sessions=%#v err=%v", completion, sessions, err)
	}

	currentGroup.Store("read")
	if _, err := pool.Exec(ctx, `UPDATE cairnops_oidc_identities SET sync_due_at = now() WHERE user_id = $1::uuid`, sessions.userID); err != nil {
		t.Fatal(err)
	}
	syncer := NewSynchronizer(service, "integration-test", nil)
	if err := syncer.drain(ctx); err != nil {
		t.Fatal(err)
	}
	var role string
	if err := pool.QueryRow(ctx, `SELECT role FROM cairnops_users WHERE id = $1::uuid`, sessions.userID).Scan(&role); err != nil || role != "observer" {
		t.Fatalf("group change did not synchronize the role: role=%q err=%v", role, err)
	}

	currentGroup.Store("unmapped")
	if _, err := pool.Exec(ctx, `UPDATE cairnops_oidc_identities SET sync_due_at = now() WHERE user_id = $1::uuid`, sessions.userID); err != nil {
		t.Fatal(err)
	}
	if err := syncer.drain(ctx); err != nil {
		t.Fatal(err)
	}
	var suspendedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT external_suspended_at FROM cairnops_users WHERE id = $1::uuid`, sessions.userID).Scan(&suspendedAt); err != nil || suspendedAt == nil {
		t.Fatalf("loss of every mapped group did not suspend access: suspended=%v err=%v", suspendedAt, err)
	}
}

func assertAuthorizationRequest(t *testing.T, authorization Authorization) string {
	t.Helper()
	parsed, err := url.Parse(authorization.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if authorization.State == "" || query.Get("state") != authorization.State || query.Get("nonce") == "" || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" || query.Get("prompt") != "consent" || !strings.Contains(query.Get("scope"), "offline_access") {
		t.Fatalf("authorization request lacks an OIDC safeguard: %s", authorization.URL)
	}
	return query.Get("nonce")
}

func writeProviderJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}

func rsaJWK(publicKey *rsa.PublicKey) map[string]string {
	exponent := big.NewInt(int64(publicKey.E)).Bytes()
	return map[string]string{
		"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func signedIDToken(t *testing.T, privateKey *rsa.PrivateKey, issuer, subject, audience, nonce string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test-key","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"iss": issuer, "sub": subject, "aud": audience, "nonce": nonce,
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s.%s", unsigned, base64.RawURLEncoding.EncodeToString(signature))
}

func TestPostgresDraftActivationAndIssuerLock(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('break-glass', 'Break glass', 'sealed-local-hash', 'administrator')
		RETURNING id::text
	`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(pool, box, identity.NewStore(pool), "https://cairnops.example.net", nil)
	input := ConfigurationInput{
		Label: "Authentik", Issuer: "https://auth.example.net", ClientID: "cairnops",
		ClientSecret: "client-secret", GroupsClaim: "groups",
		Groups: GroupMappings{Administrator: []string{"cairnops-admin"}},
	}
	draft, err := service.SaveDraft(ctx, actorID, input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(draft)
	if strings.Contains(string(encoded), input.ClientSecret) {
		t.Fatal("the client secret escaped through the configuration projection")
	}
	input.Label = "Authentik production"
	input.ClientSecret = ""
	replaced, err := service.SaveDraft(ctx, actorID, input)
	if err != nil || replaced.ID == draft.ID || replaced.Label != input.Label {
		t.Fatalf("draft replacement did not reuse its sealed secret: %#v, %v", replaced, err)
	}
	reusedSecret, err := box.Open(replaced.clientSecretSealed, clientSecretPurpose)
	if err != nil || string(reusedSecret) != "client-secret" {
		t.Fatalf("replacement lost the sealed client secret: %q, %v", reusedSecret, err)
	}
	draft = replaced
	if _, err := pool.Exec(ctx, `UPDATE cairnops_oidc_configurations SET tested_at = now() WHERE id = $1::uuid`, draft.ID); err != nil {
		t.Fatal(err)
	}
	active, err := service.Activate(ctx)
	if err != nil || active.State != "active" || active.ActivatedAt == nil {
		t.Fatalf("draft did not activate: %#v, %v", active, err)
	}

	refresh, _ := box.Seal([]byte("refresh"), refreshTokenPurpose)
	var externalID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role, authorization_regime)
		VALUES ('oidc-test-user', 'OIDC Test', NULL, 'observer', 'external')
		RETURNING id::text
	`).Scan(&externalID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_oidc_identities (
			user_id, issuer, subject, refresh_token_sealed, last_verified_at, sync_due_at
		) VALUES ($1::uuid, $2, 'subject', $3, now(), $4)
	`, externalID, active.Issuer, refresh, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	identityStore := identity.NewStore(pool)
	newRole := "operator"
	if _, err := identityStore.UpdateAccount(ctx, actorID, externalID, identity.UpdateAccountInput{Role: &newRole}); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("a local administrator replaced an OIDC-governed role: %v", err)
	}
	if _, err := identityStore.SetPassword(ctx, externalID, "a-local-password-2026"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("an external User received a local password: %v", err)
	}
	input.Issuer = "https://other.example.net"
	if _, err := service.SaveDraft(ctx, actorID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("issuer changed after an external User existed: %v", err)
	}
}

func TestPostgresAuthorizationRegimeConstrainsPasswords(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role, authorization_regime)
		VALUES ('bad-external', 'Bad external', 'password-hash', 'observer', 'external')
	`); err == nil {
		t.Fatal("an external User kept a local password")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role, authorization_regime)
		VALUES ('bad-local', 'Bad local', NULL, 'observer', 'local')
	`); err == nil {
		t.Fatal("a local User was created without a password")
	}
	var localID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role, authorization_regime)
		VALUES ('local-identity', 'Local identity', 'password-hash', 'observer', 'local')
		RETURNING id::text
	`).Scan(&localID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_oidc_identities (
			user_id, issuer, subject, refresh_token_sealed, last_verified_at, sync_due_at
		) VALUES ($1::uuid, 'https://auth.example.net', 'local-subject', 'sealed', now(), now())
	`, localID); err == nil {
		t.Fatal("a local User received an OIDC identity")
	}
}
