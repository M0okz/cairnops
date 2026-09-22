package connectors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestPostgresConnectionReplacementPreservesOperationalHistoryAndOwnership(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var actor string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users(username,display_name,password_hash,role) VALUES('connection-test','Connection Test','unused','administrator') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	imported, err := store.ImportZabbix(ctx, PersistZabbixInput{
		ActorID: actor, Name: "Existing", Endpoint: "https://old.example.net/api_jsonrpc.php",
		CredentialSealed: "previous-sealed-credential-long-enough", Version: "7.4", Compatibility: "supported", EncryptedTransport: true,
		CredentialManagement: "managed", ManagedCredentialID: "original-token-id",
		Hosts: []zabbix.Host{{ID: "42", Name: "Database"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := imported.Connector.ID
	var bindingID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM cairnops_connector_bindings WHERE connector_id=$1::uuid`, id).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordIntegrationObservations(ctx, time.Now().UTC(), []IntegrationObservation{{BindingID: bindingID, Outcome: "unhealthy", Message: "Existing proof"}}); err != nil {
		t.Fatal(err)
	}
	if err := incidents.NewPostgresStore(pool).ReconcileZabbix(ctx, incidents.ReconcileZabbixInput{
		ConnectorID: id, ObservedAt: time.Now().UTC(), Signals: []incidents.ZabbixSignal{{TargetID: imported.Targets[0].TargetID, BindingID: bindingID, ExternalEventID: "91", ExternalObjectID: "81", Name: "Existing problem", Severity: incidents.SeverityMajor, OpenedAt: time.Now().UTC().Add(-time.Minute)}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cairnops_connector_inventory(connector_id, external_id, excluded) VALUES($1::uuid, 'excluded-host', true)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(ctx, id, "disabled"); err != nil {
		t.Fatal(err)
	}
	// Snapshot every operational table touched by an import, discovery or removal.
	tables := []string{"cairnops_targets", "cairnops_connector_bindings", "cairnops_connector_inventory", "cairnops_signal_sources", "cairnops_observations", "cairnops_incidents", "cairnops_incident_evidence", "cairnops_incident_activity"}
	snapshot := func() map[string]string {
		t.Helper()
		rows := map[string]string{}
		for _, table := range tables {
			var contents string
			if err := pool.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(row))::text, '[]') FROM `+table+` row`).Scan(&contents); err != nil {
				t.Fatal(err)
			}
			rows[table] = contents
		}
		return rows
	}
	before := snapshot()
	replacement := connectionReplacement{ConnectorID: id, Kind: "zabbix", PreviousEndpoint: imported.Connector.Endpoint, PreviousCredential: "previous-sealed-credential-long-enough", Name: "Renamed", Endpoint: "https://new.example.net/api_jsonrpc.php", CredentialSealed: "replacement-sealed-credential-long-enough", Version: "7.4.1", Compatibility: "supported", EncryptedTransport: true}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_connectors SET lease_owner='worker-before-edit', lease_until=now()+interval '1 minute' WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceConnection(ctx, replacement); !errors.Is(err, ErrConnectionBusy) {
		t.Fatalf("in-flight synchronization was interrupted: %v", err)
	}
	unchanged, err := store.RemovalCredential(ctx, id)
	if err != nil || unchanged.CredentialSealed != replacement.PreviousCredential || unchanged.Endpoint != replacement.PreviousEndpoint {
		t.Fatal("busy response changed working access")
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_connectors SET lease_owner=NULL, lease_until=NULL WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	result, err := store.ReplaceConnection(ctx, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != id || result.Status != "disabled" || result.BindingCount != 1 || result.CredentialManagement != "managed" {
		t.Fatalf("lost identity/suspension: %#v", result)
	}
	credential, err := store.RemovalCredential(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if credential.ManagedCleanupEndpoint != replacement.PreviousEndpoint || credential.ManagedCleanupCredentialSealed != replacement.PreviousCredential {
		t.Fatal("cleanup origin was redirected by endpoint change")
	}
	if credential.ManagedCredentialID != "original-token-id" || credential.CredentialManagement != "managed" {
		t.Fatal("lost managed account ownership")
	}
	for table, after := range snapshot() {
		if before[table] != after {
			t.Fatalf("connection update changed %s", table)
		}
	}
	if _, err := store.ReplaceConnection(ctx, replacement); !errors.Is(err, ErrConnectionChanged) {
		t.Fatalf("stale verification accepted: %v", err)
	}
	// A competing connector may occupy the address after the remote test.
	if _, err := pool.Exec(ctx, `INSERT INTO cairnops_connectors(kind,name,endpoint,credential_sealed,status,remote_version,compatibility,encrypted_transport,created_by) VALUES('zabbix','Other','https://occupied.example.net/api_jsonrpc.php','another-sealed-credential-long-enough','connected','7.4','supported',true,$1::uuid)`, actor); err != nil {
		t.Fatal(err)
	}
	replacement.PreviousCredential, replacement.PreviousEndpoint = replacement.CredentialSealed, replacement.Endpoint
	replacement.Endpoint = "https://occupied.example.net/api_jsonrpc.php"
	if _, err := store.ReplaceConnection(ctx, replacement); !errors.Is(err, ErrEndpointConflict) {
		t.Fatalf("duplicate address not reported: %v", err)
	}
	preserved, err := store.RemovalCredential(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if preserved != credential {
		t.Fatal("duplicate address changed working access")
	}
}
