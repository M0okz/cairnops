package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/proxmox"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/testsupport"
)

type proxmoxInventory struct {
	resources []proxmox.Resource
	err       error
}

func (c *proxmoxInventory) Resources(context.Context, string, proxmox.Credentials) ([]proxmox.Resource, error) {
	return c.resources, c.err
}

func TestProxmoxStormRecoveryAndIncompleteReads(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var actor string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users (username,display_name,password_hash,role) VALUES ('pve-test','Test','unused','administrator') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32))
	raw, _ := json.Marshal(proxmox.Credentials{TokenID: "user@pve!test", Secret: "test"})
	sealed, _ := box.Seal(raw, "connector:proxmox:https://proxmox.example.net")
	resources := []proxmox.Resource{{ID: "node/alpha", Type: "node", Name: "alpha", Node: "alpha", Status: "online"}}
	expected := map[string]bool{}
	for id := 100; id < 115; id++ {
		resource := proxmox.Resource{ID: fmt.Sprintf("qemu/%d", id), Type: "qemu", VMID: id, Name: fmt.Sprintf("vm-%d", id), Node: "alpha", Status: "stopped"}
		resources = append(resources, resource)
		expected[resource.ID] = true
	}
	resources = append(resources, proxmox.Resource{ID: "qemu/200", Type: "qemu", VMID: 200, Name: "deliberately-stopped", Node: "alpha", Status: "stopped"})
	store := NewPostgresStore(pool)
	imported, err := store.ImportProxmox(ctx, PersistProxmoxInput{ActorID: actor, Name: "PVE", Endpoint: "https://proxmox.example.net", Version: "9.2.3", CredentialSealed: sealed, Resources: resources, Inventory: resources, ExpectedRunning: expected})
	if err != nil {
		t.Fatal(err)
	}
	client := &proxmoxInventory{resources: resources}
	incidentStore := incidents.NewPostgresStore(pool)
	syncer := NewProxmoxSynchronizer(store, incidentStore, client, box, "test-worker", nil)
	tick := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE cairnops_connectors SET next_sync_at = now() WHERE id = $1::uuid`, imported.Connector.ID); err != nil {
			t.Fatal(err)
		}
		if err := syncer.tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	tick()
	tick()
	var count, affected, evidence int
	if err := pool.QueryRow(ctx, `SELECT count(*), coalesce(max(affected_target_count),0) FROM cairnops_incidents WHERE status = 'active'`).Scan(&count, &affected); err != nil {
		t.Fatal(err)
	}
	if count != 1 || affected != 15 {
		t.Fatalf("expected one incident affecting 15 guests, got %d / %d", count, affected)
	}
	assertEvidence := func(want int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_incident_evidence WHERE active`).Scan(&evidence); err != nil {
			t.Fatal(err)
		}
		if evidence != want {
			t.Fatalf("active evidence: got %d, want %d", evidence, want)
		}
	}
	assertEvidence(15)
	if _, err := store.SetStatus(ctx, imported.Connector.ID, "disabled"); err != nil {
		t.Fatal(err)
	}
	if err := incidentStore.ApplyEvidenceSnapshot(ctx, incidents.EvidenceSnapshot{Origin: "proxmox", ConnectorID: imported.Connector.ID, ObservedAt: time.Now(), LeaseOwner: "test-worker", CompleteConnector: true}); err == nil {
		t.Fatal("a cycle invalidated by suspension could still change evidence")
	}
	assertEvidence(15)
	if _, err := store.SetStatus(ctx, imported.Connector.ID, "connected"); err != nil {
		t.Fatal(err)
	}
	client.err = errors.New("permissions lost")
	tick()
	assertEvidence(15)
	client.err = nil
	client.resources = resources[:1]
	tick()
	assertEvidence(15)
	// One machine migrates and resumes; the others still require attention.
	resources[1].Node = "beta"
	resources[1].Status = "running"
	client.resources = resources
	tick()
	assertEvidence(14)
	var node string
	if err := pool.QueryRow(ctx, `SELECT metadata->>'node' FROM cairnops_connector_bindings WHERE connector_id=$1::uuid AND external_id='qemu/100'`, imported.Connector.ID).Scan(&node); err != nil || node != "beta" {
		t.Fatalf("migration metadata: %q, %v", node, err)
	}
	for i := 1; i <= 15; i++ {
		resources[i].Status = "running"
	}
	tick()
	assertEvidence(0)
	// Propagation closes in the common incident cycle, without new source events.
	if err := incidentStore.Advance(ctx, time.Now().Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_incidents WHERE status='active'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("incident did not resolve: %d, %v", count, err)
	}
}

func TestProxmoxImportIsAtomicAndNeverSilentlyMatchesNames(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	var actor, existing string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users (username,display_name,password_hash,role) VALUES ('pve','PVE','unused','administrator') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets(name) VALUES ('same-name') RETURNING id::text`).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	input := PersistProxmoxInput{ActorID: actor, Name: "PVE", Endpoint: "https://pve.example.net", Version: "9.2.3", CredentialSealed: "sealed-token-with-sufficient-length", Resources: []proxmox.Resource{{ID: "qemu/100", Type: "qemu", VMID: 100, Name: "same-name", Node: "alpha", Status: "running"}, {ID: "qemu/101", Type: "qemu", VMID: 101, Name: "second", Node: "alpha", Status: "running"}}, TargetAssignments: map[string]string{"qemu/101": "11111111-1111-4111-8111-111111111111"}}
	if _, err := store.ImportProxmox(ctx, input); err == nil {
		t.Fatal("invalid assignment succeeded")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_connectors`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial connector persisted: %d %v", count, err)
	}
	input.TargetAssignments = nil
	input.Inventory = input.Resources
	input.ManagedUserID = "cairnops-0123456789abcdef@pve"
	result, err := store.ImportProxmox(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Targets[0].TargetID == existing || result.Targets[0].Disposition != "created" {
		t.Fatal("same name silently attached")
	}
	// Reopening the same connector updates the explicit stop policy idempotently.
	input.ExistingID = result.Connector.ID
	input.ExpectedRunning = map[string]bool{"qemu/100": true}
	second, err := store.ImportProxmox(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Connector.BindingCount != 2 || second.Targets[0].Disposition != "already_imported" {
		t.Fatal("inventory update duplicated bindings")
	}
	settings, err := store.ProxmoxSettings(ctx, input.Endpoint)
	if err != nil || !settings["qemu/100"] {
		t.Fatalf("stop policy was not saved: %v", err)
	}
	// New unambiguous objects auto-import; a plausible target match stays pending.
	if _, err := pool.Exec(ctx, `UPDATE cairnops_connectors SET lease_owner='discovery', lease_until=now()+interval '1 minute' WHERE id=$1::uuid`, result.Connector.ID); err != nil {
		t.Fatal(err)
	}
	discovered := append(append([]proxmox.Resource{}, input.Resources...), proxmox.Resource{ID: "qemu/102", Type: "qemu", VMID: 102, Name: "brand-new", Node: "alpha", Status: "running"}, proxmox.Resource{ID: "qemu/103", Type: "qemu", VMID: 103, Name: "same-name", Node: "alpha", Status: "running"})
	bindings, err := store.RefreshProxmox(ctx, RuntimeConnector{ID: result.Connector.ID, Endpoint: input.Endpoint}, "discovery", discovered)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 3 {
		t.Fatalf("expected three bindings, got %d", len(bindings))
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_proxmox_inventory WHERE pending`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("ambiguous discovery was not pending: %d %v", count, err)
	}
	// A status response replaces the browser's connector object. It must keep
	// the managed cleanup requirements and the actionable discovery count.
	for _, status := range []string{"disabled", "connected"} {
		updated, err := store.SetStatus(ctx, result.Connector.ID, status)
		if err != nil {
			t.Fatal(err)
		}
		if updated.CredentialManagement != "managed" || updated.QuarantineCount != 1 || updated.BindingCount != 3 {
			t.Fatalf("status response lost connector context: %+v", updated)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_connectors SET lease_owner='discovery', lease_until=now()+interval '1 minute' WHERE id=$1::uuid`, result.Connector.ID); err != nil {
		t.Fatal(err)
	}
	// An absent candidate cannot be selected in the preview. Preserve its
	// decision for a later reappearance without showing an impossible action.
	for _, snapshot := range []struct {
		resources []proxmox.Resource
		pending   int
	}{{discovered[:3], 0}, {discovered, 1}} {
		if _, err := store.RefreshProxmox(ctx, RuntimeConnector{ID: result.Connector.ID, Endpoint: input.Endpoint}, "discovery", snapshot.resources); err != nil {
			t.Fatal(err)
		}
		listed, err := store.List(ctx)
		if err != nil || len(listed) != 1 {
			t.Fatalf("list connector: %v", err)
		}
		if listed[0].QuarantineCount != snapshot.pending || listed[0].BindingCount != 3 {
			t.Fatalf("discovery decision changed across disappearance: %+v", listed[0])
		}
	}
}
