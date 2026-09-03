package patchmon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInspectReadsScopedHostSnapshot(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/patchmon/api/v1/api/hosts" || r.URL.Query().Get("include") != "stats" {
			t.Fatalf("unexpected PatchMon request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("patchmon_key:secret"))
		if r.Header.Get("Authorization") != expected {
			t.Fatal("missing PatchMon Basic authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hosts":[{"id":"host-2","friendly_name":"Web","hostname":"web.internal","ip":"192.0.2.8","reporting_state":"reporting","update_state":"security_required","updates_count":7,"security_updates_count":2,"needs_reboot":true},{"id":"host-1","friendly_name":"Database","hostname":"db.internal","reporting_state":"reporting","update_state":"up_to_date"}],"total":2}`))
	}))
	defer server.Close()

	inspection, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL+"/patchmon", Credentials{Key: "patchmon_key", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Endpoint != server.URL+"/patchmon/api/v1/api/hosts" || !inspection.EncryptedTransport || len(inspection.Hosts) != 2 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if inspection.Hosts[0].Name() != "Database" || inspection.Hosts[1].SecurityUpdatesCount != 2 || !inspection.Hosts[1].NeedsReboot {
		t.Fatalf("unexpected PatchMon hosts: %#v", inspection.Hosts)
	}
}

func TestBootstrapCreatesScopedCredentialAndRemovesPreviewCredential(t *testing.T) {
	t.Parallel()
	created, deleted := 0, 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/patchmon/api/v1/auth/login":
			var input map[string]string
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["username"] != "installer" || input["password"] != "temporary-password" {
				t.Fatal("unexpected PatchMon installer credentials")
			}
			_, _ = w.Write([]byte(`{"token":"installer-jwt"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/patchmon/api/v1/auto-enrollment/tokens":
			if r.Header.Get("Authorization") != "Bearer installer-jwt" {
				t.Fatal("missing PatchMon installer session")
			}
			created++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":{"id":"token-` + string(rune('0'+created)) + `","token_key":"managed-key","token_secret":"managed-secret"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/patchmon/api/v1/api/hosts":
			_, _ = w.Write([]byte(`{"hosts":[{"id":"host-1","friendly_name":"Database","hostname":"db"}]}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/patchmon/api/v1/auto-enrollment/tokens/"):
			deleted++
			_, _ = w.Write([]byte(`{"message":"deleted"}`))
		default:
			t.Fatalf("unexpected PatchMon request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	remote := NewClientWithHTTP(server.Client())
	inspection, session, err := remote.PrepareBootstrap(context.Background(), server.URL+"/patchmon", "installer", "temporary-password", "")
	if err != nil || len(inspection.Hosts) != 1 || created != 1 || deleted != 1 {
		t.Fatalf("unexpected bootstrap preview: inspection=%#v created=%d deleted=%d err=%v", inspection, created, deleted, err)
	}
	credential, err := remote.Provision(context.Background(), session)
	if err != nil || credential.Credentials.Key != "managed-key" || credential.ID != "token-2" {
		t.Fatalf("unexpected durable credential: %#v err=%v", credential, err)
	}
}

func TestHostsRejectsRedirectAndEmbeddedCredentials(t *testing.T) {
	t.Parallel()
	redirect := httptest.NewServer(http.RedirectHandler("https://example.net", http.StatusFound))
	defer redirect.Close()
	if _, err := NewClientWithHTTP(redirect.Client()).Hosts(context.Background(), redirect.URL, Credentials{Key: "key", Secret: "secret"}); err == nil {
		t.Fatal("expected redirect rejection")
	}
	if _, err := NormalizeEndpoint("https://user:secret@patchmon.example.net"); err == nil {
		t.Fatal("expected embedded credentials rejection")
	}
}
