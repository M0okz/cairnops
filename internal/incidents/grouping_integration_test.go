package incidents

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestGroupedIncidentKeepsEvidenceOnEveryTarget(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	at := time.Now().UTC()
	facts := make([]EvidenceFact, 0, 15)
	for n := range 15 {
		target := insertCycleTarget(t, ctx, pool, fmt.Sprintf("VM %02d", n))
		facts = append(facts, webhookEvidence(fmt.Sprint(n), target, at.Add(time.Duration(n)*time.Second)))
	}
	// A replay must neither duplicate evidence nor count the same target twice.
	for range 2 {
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
			Origin: "webhook", ObservedAt: at.Add(15 * time.Second), Facts: facts,
		}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List(ctx, "active", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d incidents, want 1", len(items))
	}
	incident, err := store.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []Incident{items[0], incident} {
		if item.AffectedTargetCount != 15 || len(item.Impacts) != 15 {
			t.Fatalf("got %d affected targets and %d impacts", item.AffectedTargetCount, len(item.Impacts))
		}
		for _, impact := range item.Impacts {
			if len(impact.Evidence) != 1 || impact.Evidence[0].TargetID != impact.TargetID {
				t.Errorf("%s lost or duplicated its evidence: %#v", impact.TargetName, impact.Evidence)
			}
		}
	}
}

func TestPropagationKeepsTheLongestCadenceAfterAFasterSourceJoins(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	at := time.Now().UTC().Truncate(time.Second)
	var connectorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, sync_interval_seconds, status, encrypted_transport)
		VALUES ('zabbix', 'Slow source', 'https://example.test', repeat('x', 32), 150, 'connected', true) RETURNING id::text
	`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	first := webhookEvidence("slow", insertCycleTarget(t, ctx, pool, "Slow VM"), at)
	first.ConnectorID = connectorID
	second := webhookEvidence("fast", insertCycleTarget(t, ctx, pool, "Fast VM"), at.Add(250*time.Second))
	third := webhookEvidence("later", insertCycleTarget(t, ctx, pool, "Third VM"), at.Add(450*time.Second))
	for _, fact := range []EvidenceFact{first, second, third} {
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
			Origin: "webhook", ConnectorID: fact.ConnectorID, ObservedAt: fact.OpenedAt, Facts: []EvidenceFact{fact},
		}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List(ctx, "active", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AffectedTargetCount != 3 {
		t.Fatalf("mixed cadences split one propagation: %d incidents", len(items))
	}
	if want := at.Add(750 * time.Second); !items[0].PropagationEndsAt.Equal(want) {
		t.Fatalf("propagation ends at %s, want %s", items[0].PropagationEndsAt, want)
	}
}

func TestEvaluationWindowGroupsStaggeredEvidenceButKeepsLaterEpisodesSeparate(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	at := time.Now().UTC().Truncate(time.Second)
	first := webhookEvidence("slow-condition", insertCycleTarget(t, ctx, pool, "First VM"), at)
	first.EvaluationWindow = 15 * time.Minute
	apply := func(fact EvidenceFact, observedAt time.Time) {
		t.Helper()
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{Origin: "webhook", ObservedAt: observedAt, Facts: []EvidenceFact{fact}}); err != nil {
			t.Fatal(err)
		}
	}
	apply(first, at)
	apply(first, at.Add(3*time.Minute)) // Relecture du même fait, sans nouvelle Atteinte.
	items, err := store.List(ctx, "active", 20)
	if err != nil || len(items) != 1 || !items[0].PropagationEndsAt.Equal(at.Add(5*time.Minute)) {
		t.Fatalf("repeated evidence extended propagation or exceeded the bound: %#v (%v)", items, err)
	}
	second := webhookEvidence("second", insertCycleTarget(t, ctx, pool, "Second VM"), at.Add(4*time.Minute))
	apply(second, second.OpenedAt) // Une Source sans période conserve la plus longue fenêtre déjà établie.
	items, err = store.List(ctx, "active", 20)
	if err != nil || len(items) != 1 || items[0].AffectedTargetCount != 2 || !items[0].PropagationEndsAt.Equal(at.Add(9*time.Minute)) {
		t.Fatalf("compatible late target split the group: %#v (%v)", items, err)
	}
	if err := store.Advance(ctx, at.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	third := webhookEvidence("third", insertCycleTarget(t, ctx, pool, "Third VM"), at.Add(10*time.Minute))
	third.EvaluationWindow = 24 * time.Hour
	apply(third, third.OpenedAt)
	items, err = store.List(ctx, "active", 20)
	if err != nil || len(items) != 2 {
		t.Fatalf("a later episode was absorbed into the original incident: %#v (%v)", items, err)
	}
	for _, item := range items {
		if item.PropagationWindowSeconds != 300 {
			t.Fatalf("condition escaped the five-minute bound: %d", item.PropagationWindowSeconds)
		}
	}
}
