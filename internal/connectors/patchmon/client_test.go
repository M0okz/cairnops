package patchmon

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
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
