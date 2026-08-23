package uptimekuma

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInspectReadsAuthenticatedMonitorStatusMetrics(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/kuma/metrics" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte(":uk2-secret")) {
			t.Fatal("missing API key basic authentication")
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(`# HELP monitor_status Monitor Status (1 = UP, 0= DOWN, 2= PENDING, 3= MAINTENANCE)
# TYPE monitor_status gauge
monitor_status{monitor_id="12",monitor_name="Database",monitor_type="tcp",monitor_url="null",monitor_hostname="db.internal",monitor_port="5432"} 0
monitor_status{monitor_id="4",monitor_name="API",monitor_type="http",monitor_url="https://api.example.net",monitor_hostname="null",monitor_port="null"} 1
`))
	}))
	defer server.Close()

	inspection, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL+"/kuma", "uk2-secret")
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Endpoint != server.URL+"/kuma/metrics" || !inspection.EncryptedTransport || len(inspection.Monitors) != 2 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if inspection.Monitors[0].Name != "API" || inspection.Monitors[1].Address() != "db.internal:5432" || inspection.Monitors[1].Status != 0 {
		t.Fatalf("unexpected monitors: %#v", inspection.Monitors)
	}
}

func TestParseMonitorsFallsBackToTheNameWhenUptimeKumaOmitsTheIdentity(t *testing.T) {
	t.Parallel()
	// Uptime Kuma 1.23.x n'expose pas monitor_id.
	monitors, err := parseMonitors([]byte(`# TYPE monitor_status gauge
monitor_status{monitor_name="Database",monitor_type="tcp",monitor_url="null",monitor_hostname="db.internal",monitor_port="5432"} 0
monitor_status{monitor_name="API",monitor_type="http",monitor_url="https://api.example.net",monitor_hostname="null",monitor_port="null"} 1
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(monitors) != 2 || monitors[0].ID != "name:API" || monitors[1].ID != "name:Database" {
		t.Fatalf("unexpected monitors: %#v", monitors)
	}

	if _, err := parseMonitors([]byte(`# TYPE monitor_status gauge
monitor_status{monitor_name="API",monitor_type="http",monitor_url="https://one.example.net"} 1
monitor_status{monitor_name="API",monitor_type="http",monitor_url="https://two.example.net"} 1
`)); err == nil {
		t.Fatal("expected duplicate monitor name rejection")
	}
}

func TestParseMonitorsReadsCertificateAndResponseGauges(t *testing.T) {
	monitors, err := parseMonitors([]byte(`# TYPE monitor_status gauge
monitor_status{monitor_id="4",monitor_name="API",monitor_type="http",monitor_url="https://api.example.net"} 1
# TYPE monitor_response_time gauge
monitor_response_time{monitor_id="4",monitor_name="API"} 37
# TYPE monitor_cert_days_remaining gauge
monitor_cert_days_remaining{monitor_id="4",monitor_name="API"} 21.5
# TYPE monitor_cert_is_valid gauge
monitor_cert_is_valid{monitor_id="4",monitor_name="API"} 1
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(monitors) != 1 || monitors[0].ResponseMilliseconds == nil || *monitors[0].ResponseMilliseconds != 37 || monitors[0].CertificateDaysRemaining == nil || *monitors[0].CertificateDaysRemaining != 21.5 || monitors[0].CertificateValid == nil || !*monitors[0].CertificateValid {
		t.Fatalf("unexpected contextual metrics: %#v", monitors)
	}
}

func TestMonitorsRejectsRedirectsAndInvalidPayloads(t *testing.T) {
	t.Parallel()
	redirect := httptest.NewServer(http.RedirectHandler("https://example.net/metrics", http.StatusFound))
	defer redirect.Close()
	if _, err := NewClientWithHTTP(redirect.Client()).Monitors(context.Background(), redirect.URL, "key"); err == nil {
		t.Fatal("expected redirect rejection")
	}

	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("monitor_status{monitor_id=\"not-an-id\",monitor_name=\"API\",monitor_type=\"http\"} 1\n"))
	}))
	defer malformed.Close()
	if _, err := NewClientWithHTTP(malformed.Client()).Monitors(context.Background(), malformed.URL, "key"); err == nil {
		t.Fatal("expected invalid monitor identity rejection")
	}
}

func TestNormalizeEndpointRejectsEmbeddedCredentials(t *testing.T) {
	t.Parallel()
	if _, err := NormalizeEndpoint("https://user:password@kuma.example.net"); err == nil {
		t.Fatal("expected embedded credentials rejection")
	}
}
