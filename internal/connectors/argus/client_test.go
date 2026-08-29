package argus

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInspectReadsEligibleServicesAndCorroboratesTheirState(t *testing.T) {
	t.Parallel()

	wantedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("reader: secret "))
	var templateCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("Argus connector emitted %s instead of GET", r.Method)
		}
		if r.Header.Get("Authorization") != wantedAuth {
			t.Fatalf("missing Argus Basic authentication on %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/argus/api/v1/version":
			_, _ = w.Write([]byte(`{"version":"0.35.0","buildDate":"2026-08-01"}`))
		case "/argus/api/v1/config":
			_, _ = w.Write([]byte(`{
				"service": {
					"api": {"name":"Public API","options":{"active":true},"deployed_version":{"type":"url"},"dashboard":{"web_url":"https://releases.example/{{ version }}"},"status":{"deployed_version":"1.2.2","latest_version":"1.2.3"}},
					"worker": {"name":"Worker","deployed_version":{"type":"url"}},
					"disabled": {"name":"Disabled","options":{"active":false},"deployed_version":{"type":"url"}},
					"untracked": {"name":"Untracked"}
				},
				"order":["api","worker","disabled","untracked"]
			}`))
		case "/argus/api/v1/counts":
			_, _ = w.Write([]byte(`{"service_count":4,"service_count_active":3,"service_count_inactive":1,"updates_available":1,"updates_skipped":0,"update_details":[{"service_name":"api","deployed_version":"1.2.2","latest_version":"1.2.3","last_checked":"2026-08-29T08:00:00Z","approved":true,"skipped":false}]}`))
		case "/argus/metrics":
			_, _ = w.Write([]byte(`# HELP latest_version_is_deployed Whether deployed.
# TYPE latest_version_is_deployed gauge
latest_version_is_deployed{id="api"} 2
latest_version_is_deployed{id="worker"} 1
latest_version_is_deployed{id="disabled"} 1
latest_version_is_deployed{id="untracked"} 1
# TYPE latest_version_query_result_last gauge
latest_version_query_result_last{id="api",type="github"} 1
latest_version_query_result_last{id="worker",type="url"} 1
latest_version_query_result_last{id="disabled",type="url"} 1
latest_version_query_result_last{id="untracked",type="url"} 1
# TYPE deployed_version_query_result_last gauge
deployed_version_query_result_last{id="api",type="url"} 1
deployed_version_query_result_last{id="worker",type="url"} 1
deployed_version_query_result_last{id="disabled",type="url"} 1
`))
		case "/argus/api/v1/template":
			templateCalls.Add(1)
			switch {
			case r.URL.Query().Get("service_id") == "api" && r.URL.Query().Get("template") == "{{ web_url }}":
				_, _ = w.Write([]byte(`{"parsed":"https://releases.example/1.2.3"}`))
			case r.URL.Query().Get("service_id") == "worker" && r.URL.Query().Get("template") == "{{ latest_version }}":
				_, _ = w.Write([]byte(`{"parsed":"2.0.0"}`))
			default:
				t.Fatalf("unexpected template query: %s", r.URL.RawQuery)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	inspection, err := NewClientWithHTTP(server.Client()).Inspect(
		context.Background(), server.URL+"/argus", Credentials{Username: "reader", Password: " secret "},
	)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Endpoint != server.URL+"/argus" || !inspection.EncryptedTransport || inspection.Version != "0.35.0" {
		t.Fatalf("unexpected inspection identity: %#v", inspection)
	}
	if len(inspection.Services) != 4 || templateCalls.Load() != 2 {
		t.Fatalf("unexpected discovery: %#v", inspection.Services)
	}
	api := inspection.Services[0]
	if api.ID != "api" || api.Name != "Public API" || !api.Importable || api.DeployedVersion != "1.2.2" || api.LatestVersion != "1.2.3" || !api.Approved || api.Skipped || api.Unknown {
		t.Fatalf("unexpected API service: %#v", api)
	}
	if api.VersionURL != "https://releases.example/1.2.3" {
		t.Fatalf("unexpected rendered version URL %q", api.VersionURL)
	}
	worker := inspection.Services[1]
	if !worker.Importable || worker.DeploymentState != DeploymentStateDeployed || worker.DeployedVersion != "2.0.0" || worker.LatestVersion != "2.0.0" {
		t.Fatalf("unexpected worker service: %#v", worker)
	}
	if inspection.Services[2].Importable || inspection.Services[2].Ineligibility != IneligibilityInactive {
		t.Fatalf("inactive service should not be importable: %#v", inspection.Services[2])
	}
	if inspection.Services[3].Importable || inspection.Services[3].Ineligibility != IneligibilityNoDeployedVersion {
		t.Fatalf("untracked service should not be importable: %#v", inspection.Services[3])
	}
}

func TestInspectIsolatesTemplateFailuresToTheAffectedService(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"version":"0.35.0"}`))
		case "/api/v1/config":
			_, _ = w.Write([]byte(`{
				"service": {
					"good": {"name":"Good","deployed_version":{"type":"url"}},
					"bad": {"name":"Bad","deployed_version":{"type":"url"},"dashboard":{"web_url":"{{ web_url }}"}}
				},
				"order":["good","bad"]
			}`))
		case "/api/v1/counts":
			_, _ = w.Write([]byte(`{"update_details":[]}`))
		case "/metrics":
			_, _ = w.Write([]byte(`# TYPE latest_version_is_deployed gauge
latest_version_is_deployed{id="good"} 1
latest_version_is_deployed{id="bad"} 1
# TYPE latest_version_query_result_last gauge
latest_version_query_result_last{id="good",type="github"} 1
latest_version_query_result_last{id="bad",type="github"} 1
# TYPE deployed_version_query_result_last gauge
deployed_version_query_result_last{id="good",type="url"} 1
deployed_version_query_result_last{id="bad",type="url"} 1
`))
		case "/api/v1/template":
			if r.URL.Query().Get("service_id") == "bad" && r.URL.Query().Get("template") == "{{ web_url }}" {
				http.Error(w, "broken dashboard", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"parsed":"1.0.0"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	inspection, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Services) != 2 || inspection.Services[0].Unknown || inspection.Services[0].DeployedVersion != "1.0.0" {
		t.Fatalf("valid services must survive a neighbouring template failure: %#v", inspection.Services)
	}
	if !inspection.Services[1].Unknown || inspection.Services[1].UnknownReason != "dashboard_url_failed" {
		t.Fatalf("the affected service must become unknown: %#v", inspection.Services[1])
	}
}

func TestInspectMarksQueryFailuresUnknown(t *testing.T) {
	t.Parallel()
	server := newArgusTestServer(t, "0.29.0", `latest_version_is_deployed{id="api"} 0
latest_version_query_result_last{id="api",type="github"} 0
deployed_version_query_result_last{id="api",type="url"} 1
`)
	defer server.Close()

	inspection, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Services) != 1 || !inspection.Services[0].Unknown || inspection.Services[0].UnknownReason != "latest_version_query_failed" {
		t.Fatalf("query failure must produce an unknown service: %#v", inspection.Services)
	}
}

func TestInspectMarksMissingVersionValuesUnknown(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"version":"0.29.0"}`))
		case "/api/v1/config":
			_, _ = w.Write([]byte(`{"service":{"api":{"name":"API","deployed_version":{"type":"url"}}},"order":["api"]}`))
		case "/api/v1/counts":
			_, _ = w.Write([]byte(`{"update_details":[]}`))
		case "/api/v1/template":
			_, _ = w.Write([]byte(`{"parsed":""}`))
		case "/metrics":
			_, _ = w.Write([]byte(`# TYPE latest_version_is_deployed gauge
latest_version_is_deployed{id="api"} 0
# TYPE latest_version_query_result_last gauge
latest_version_query_result_last{id="api",type="github"} 1
# TYPE deployed_version_query_result_last gauge
deployed_version_query_result_last{id="api",type="url"} 1
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	inspection, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Services) != 1 || !inspection.Services[0].Unknown || inspection.Services[0].UnknownReason != "version_value_missing" {
		t.Fatalf("missing version values must not open or resolve an incident: %#v", inspection.Services)
	}
}

func TestInspectRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()
	server := newArgusTestServer(t, "0.28.3", "")
	defer server.Close()

	_, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL, Credentials{})
	if err == nil || !strings.Contains(err.Error(), "0.29.0") {
		t.Fatalf("expected minimum version rejection, got %v", err)
	}
}

func TestInspectRejectsMinimumVersionPrerelease(t *testing.T) {
	t.Parallel()
	server := newArgusTestServer(t, "0.29.0-rc1", "")
	defer server.Close()

	_, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL, Credentials{})
	if err == nil || !strings.Contains(err.Error(), "0.29.0") {
		t.Fatalf("expected minimum stable version rejection, got %v", err)
	}
}

func TestInspectRejectsRedirectBeforeForwardingAuthorization(t *testing.T) {
	t.Parallel()
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("Authorization") != "")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer redirect.Close()

	_, err := NewClientWithHTTP(redirect.Client()).Inspect(context.Background(), redirect.URL, Credentials{Username: "reader", Password: "secret"})
	if err == nil {
		t.Fatal("expected Argus redirect rejection")
	}
	if leaked.Load() {
		t.Fatal("Argus credentials were forwarded to a redirect target")
	}
}

func TestNormalizeEndpointRejectsEmbeddedCredentialsAndQuery(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"https://user:secret@argus.example.net", "https://argus.example.net?token=secret", "file:///tmp/argus"} {
		if _, err := NormalizeEndpoint(address); err == nil {
			t.Fatalf("expected unsafe endpoint %q to be rejected", address)
		}
	}
}

func newArgusTestServer(t *testing.T, version, metrics string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"version":"` + version + `"}`))
		case "/api/v1/config":
			_, _ = w.Write([]byte(`{"service":{"api":{"name":"API","deployed_version":{"type":"url"},"status":{"deployed_version":"1.0.0","latest_version":"1.1.0"}}},"order":["api"]}`))
		case "/api/v1/counts":
			_, _ = w.Write([]byte(`{"service_count":1,"service_count_active":1,"updates_available":1,"update_details":[{"service_name":"api","deployed_version":"1.0.0","latest_version":"1.1.0","last_checked":"2026-08-29T08:00:00Z"}]}`))
		case "/metrics":
			_, _ = w.Write([]byte(metrics))
		default:
			http.NotFound(w, r)
		}
	}))
}
