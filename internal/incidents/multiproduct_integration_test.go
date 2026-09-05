package incidents

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestZabbixAndKumaShareAnIncidentWithoutRequiringConsensus(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	targetA := insertCycleTarget(t, ctx, pool, "API")
	targetB := insertCycleTarget(t, ctx, pool, "Worker")
	connector := func(kind string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connectors
			(kind, name, endpoint, credential_sealed, status, encrypted_transport)
			VALUES ($1, $1, 'https://example.test', repeat('x', 32), 'connected', true)
			RETURNING id::text`, kind).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	binding := func(connectorID, targetID, externalID string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings
			(connector_id, target_id, external_id, external_name)
			VALUES ($1::uuid, $2::uuid, $3, $3) RETURNING id::text`, connectorID, targetID, externalID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	zabbix, kuma := connector("zabbix"), connector("uptime_kuma")
	zabbixA := binding(zabbix, targetA, "1")
	kumaA, kumaB := binding(kuma, targetA, "1"), binding(kuma, targetB, "2")
	at := time.Now().UTC()
	if err := store.ReconcileZabbix(ctx, ReconcileZabbixInput{
		ConnectorID: zabbix, ObservedAt: at,
		Signals: []ZabbixSignal{{TargetID: targetA, BindingID: zabbixA, ExternalEventID: "1", ExternalObjectID: "2",
			CanonicalNature: "availability", Name: "Source label", Severity: SeverityMajor, OpenedAt: at}},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, "active", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AffectedTargetCount != 1 {
		t.Fatal("a single product must suffice to open the Incident")
	}
	id := items[0].ID
	for range 2 {
		if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
			ConnectorID: kuma, ObservedAt: at.Add(10 * time.Second), ObservedBindings: []string{kumaA, kumaB},
			Signals: []UptimeKumaSignal{
				{TargetID: targetA, BindingID: kumaA, ExternalMonitor: "1", Name: "API DOWN", Severity: SeverityMajor},
				{TargetID: targetB, BindingID: kumaB, ExternalMonitor: "2", Name: "Worker DOWN", Severity: SeverityMajor},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	item, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, impact := range item.Impacts {
		count += len(impact.Evidence)
	}
	if item.AffectedTargetCount != 2 || count != 3 {
		t.Fatalf("expected 2 targets / 3 proofs, got %d / %d", item.AffectedTargetCount, count)
	}
	// A partial successful read says nothing about the other binding, nor
	// about the other product's active proof for this target.
	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: kuma, ObservedAt: at.Add(20 * time.Second), ObservedBindings: []string{kumaA},
	}); err != nil {
		t.Fatal(err)
	}
	item, err = store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "active" || item.AffectedTargetCount != 2 {
		t.Fatalf("partial recovery erased another source's evidence: %#v", item)
	}
	if item.Summary.FR.Body != "2 Cibles concernées · gravité majeure" {
		t.Fatalf("wrong common summary: %#v", item.Summary)
	}
}
