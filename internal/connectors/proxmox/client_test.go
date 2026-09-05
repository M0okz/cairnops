package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

var testCredential = Credentials{TokenID: "observer@pve!cairnops", Secret: "test-secret"}

func apiFixture(t *testing.T, mutate func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "PVEAPIToken="+testCredential.TokenID+"="+testCredential.Secret {
			t.Error("unexpected API authorization")
		}
		if mutate != nil && mutate(w, r) {
			return
		}
		var data any
		switch r.URL.Path {
		case "/api2/json/version":
			data = map[string]any{"version": "9.2.3"}
		case "/api2/json/access/permissions":
			data = map[string]any{"/": map[string]int{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1}}
		case "/api2/json/cluster/resources":
			data = []map[string]any{
				{"id": "node/alpha", "type": "node", "node": "alpha", "status": "online", "cpu": 0.2, "mem": 20, "maxmem": 100},
				{"id": "qemu/100", "type": "qemu", "vmid": 100, "name": "app", "node": "alpha", "status": "stopped", "disk": 0, "maxdisk": 100},
				{"id": "lxc/101", "type": "lxc", "vmid": 101, "name": "dns", "node": "alpha", "status": "running"},
				{"id": "network/ignored", "type": "network"},
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestInspectUsesReadOnlyAPIAndStableGuestIdentity(t *testing.T) {
	server := apiFixture(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method != "GET" {
			t.Error("runtime must only read")
		}
		return false
	})
	inspection, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL+"/api2/json/", testCredential)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Version != "9.2.3" || len(inspection.Resources) != 3 {
		t.Fatalf("unexpected inventory: %+v", inspection)
	}
	for _, r := range inspection.Resources {
		if r.ID == "qemu/100" {
			before := r.ID
			r.Node = "beta"
			if r.ID != before {
				t.Fatal("migration changed guest identity")
			}
			if outcome, _ := r.Condition(false); outcome != "unknown" {
				t.Fatal("intentional stop was treated as failure")
			}
			if outcome, _ := r.Condition(true); outcome != "unhealthy" {
				t.Fatal("expected guest stop did not fail")
			}
		}
	}
}

func TestPartialOrMalformedInventoryCannotBecomeComplete(t *testing.T) {
	for name, response := range map[string]string{
		"permission loss": `{"data":{"/":{"Sys.Audit":1,"VM.Audit":0,"Datastore.Audit":1}}}`,
		"missing data":    `{}`,
		"null data":       `{"data":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := apiFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if strings.HasSuffix(r.URL.Path, "permissions") {
					_, _ = fmt.Fprint(w, response)
					return true
				}
				return false
			})
			if _, err := NewClientWithHTTP(server.Client()).Resources(context.Background(), server.URL, testCredential); err == nil {
				t.Fatal("accepted incomplete permission proof")
			}
		})
	}
	for name, response := range map[string]string{
		"empty":        `{"data":[]}`,
		"duplicate":    `{"data":[{"id":"node/alpha","type":"node","node":"alpha"},{"id":"node/alpha","type":"node","node":"alpha"}]}`,
		"inconsistent": `{"data":[{"id":"qemu/100","type":"qemu","vmid":101,"node":"alpha"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := apiFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if strings.HasSuffix(r.URL.Path, "resources") {
					_, _ = fmt.Fprint(w, response)
					return true
				}
				return false
			})
			if _, err := NewClientWithHTTP(server.Client()).Resources(context.Background(), server.URL, testCredential); err == nil {
				t.Fatal("accepted invalid inventory")
			}
		})
	}
}

func TestCertificateRequiresExactApprovalBeforeCredentials(t *testing.T) {
	requests := 0
	server := apiFixture(t, func(_ http.ResponseWriter, _ *http.Request) bool { requests++; return false })
	client := NewClient()
	cert, err := client.ProbeCertificate(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Trusted || cert.Fingerprint == "" || requests != 0 {
		t.Fatal("probe trusted private certificate or sent credentials")
	}
	if _, err := client.Inspect(context.Background(), server.URL, testCredential); err == nil || requests != 0 {
		t.Fatal("credentials sent without approval")
	}
	pinned := testCredential
	pinned.Fingerprint = cert.Fingerprint
	if _, err := client.Inspect(context.Background(), server.URL, pinned); err != nil {
		t.Fatal(err)
	}
	before := requests
	pinned.Fingerprint = strings.Repeat("0", 64)
	if _, err := client.Inspect(context.Background(), server.URL, pinned); err == nil || requests != before {
		t.Fatal("changed certificate accepted")
	}
}

func TestRedirectDoesNotLeakCredential(t *testing.T) {
	forwarded := 0
	other := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded++ }))
	defer other.Close()
	server := apiFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		http.Redirect(w, r, other.URL, http.StatusFound)
		return true
	})
	if _, err := NewClientWithHTTP(server.Client()).Inspect(context.Background(), server.URL, testCredential); err == nil || forwarded != 0 {
		t.Fatal("redirect followed")
	}
}

func TestMetricsDoNotInventGuestDiskUsageOrMissingValues(t *testing.T) {
	zero, hundred, quarter := 0.0, 100.0, 0.25
	r := Resource{Type: "qemu", Status: "running", CPU: &quarter, Disk: &zero, MaxDisk: &hundred}
	values := r.Metrics()
	if len(values) != 1 || values["cpu.utilization"] != 25 {
		t.Fatalf("unexpected metrics: %v", values)
	}
	r.Status = "stopped"
	if len(r.Metrics()) != 0 {
		t.Fatal("stopped guest has invented metrics")
	}
	r = Resource{Type: "storage", Status: "available", Disk: &zero, MaxDisk: &hundred}
	if value, present := r.Metrics()["filesystem.utilization"]; !present || value != 0 {
		t.Fatal("real zero storage usage was discarded")
	}
	for _, status := range []string{"unknown", "paused", "migrating", ""} {
		if outcome, _ := (Resource{Type: "qemu", Status: status}).Condition(true); outcome != "unknown" {
			t.Fatalf("%s can resolve a failure", status)
		}
	}
	if (Resource{Type: "qemu", Template: 1}).Importable() {
		t.Fatal("template importable")
	}
}

func TestOfflineHostMakesCachedGuestStatusInconclusive(t *testing.T) {
	server := apiFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "resources") {
			_, _ = fmt.Fprint(w, `{"data":[{"id":"node/alpha","type":"node","node":"alpha","status":"offline"},{"id":"qemu/100","type":"qemu","node":"alpha","vmid":100,"status":"running","cpu":0.1}]}`)
			return true
		}
		return false
	})
	resources, err := NewClientWithHTTP(server.Client()).Resources(context.Background(), server.URL, testCredential)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resources {
		if r.Guest() {
			outcome, reason := r.Condition(true)
			if outcome != "unknown" || reason != "proxmox_host_unavailable" || len(r.Metrics()) != 0 {
				t.Fatal("offline host supplied a fresh healthy guest or metric")
			}
		}
	}
}

func TestLiveProxmoxReadOnly(t *testing.T) {
	endpoint := os.Getenv("CAIRNOPS_TEST_PVE_ENDPOINT")
	if endpoint == "" {
		t.Skip("live Proxmox VE access not configured")
	}
	credential := Credentials{TokenID: os.Getenv("CAIRNOPS_TEST_PVE_TOKEN_ID"), Secret: os.Getenv("CAIRNOPS_TEST_PVE_SECRET"), Fingerprint: os.Getenv("CAIRNOPS_TEST_PVE_FINGERPRINT")}
	inspection, err := NewClient().Inspect(context.Background(), endpoint, credential)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, r := range inspection.Resources {
		counts[r.Type]++
	}
	t.Logf("Proxmox VE %s: %v", inspection.Version, counts)
}

func TestLiveProxmoxManagedAccessLifecycle(t *testing.T) {
	if os.Getenv("CAIRNOPS_TEST_PVE_PROVISION") != "1" {
		t.Skip("live managed-account test not enabled")
	}
	endpoint := os.Getenv("CAIRNOPS_TEST_PVE_ENDPOINT")
	installer := Credentials{TokenID: os.Getenv("CAIRNOPS_TEST_PVE_TOKEN_ID"), Secret: os.Getenv("CAIRNOPS_TEST_PVE_SECRET"), Fingerprint: os.Getenv("CAIRNOPS_TEST_PVE_FINGERPRINT")}
	client := NewClient()
	ctx := context.Background()
	if err := client.CheckProvisioning(ctx, endpoint, installer); err != nil {
		t.Fatal(err)
	}
	managed, err := client.Provision(ctx, endpoint, installer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.RemoveManaged(ctx, endpoint, installer, managed.UserID); err != nil {
			t.Errorf("remove test account %s: %v", managed.UserID, err)
		}
	})
	if _, err := client.Inspect(ctx, endpoint, managed.Credentials); err != nil {
		t.Fatal(err)
	}
	var permissions map[string]map[string]int
	if err := client.request(ctx, endpoint, http.MethodGet, "/access/permissions?path=/", managed.Credentials, nil, &permissions); err != nil {
		t.Fatal(err)
	}
	for privilege := range permissions["/"] {
		if !strings.HasSuffix(privilege, ".Audit") && privilege != "SDN.Audit" {
			t.Fatalf("runtime credential has unexpected privilege %s", privilege)
		}
	}
	if err := client.RemoveManaged(ctx, endpoint, installer, managed.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Inspect(ctx, endpoint, managed.Credentials); err == nil {
		t.Fatal("removed token still authenticates")
	}
	t.Log("Dedicated user and separated auditor token verified, removed, and confirmed unusable")
}
