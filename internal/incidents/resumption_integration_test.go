package incidents

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
)

// resolveSingleTargetCycle ouvre puis résout un Incident d'une seule Atteinte.
func resolveSingleTargetCycle(t *testing.T, ctx context.Context, store *PostgresStore, targetID, key string, startedAt time.Time) string {
	t.Helper()
	fact := webhookEvidence(key, targetID, startedAt)
	if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "webhook", ObservedAt: startedAt, Facts: []EvidenceFact{fact},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListForTarget(ctx, "active", targetID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one active Incident, got %#v", items)
	}
	if err := store.ResolveEvidence(ctx, "webhook", targetID, key, startedAt.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Advance(ctx, startedAt.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	incident, err := store.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if incident.Status != "resolved" {
		t.Fatalf("expected the first cycle to be resolved, got %#v", incident)
	}
	return incident.ID
}

func applyRelapse(t *testing.T, ctx context.Context, store *PostgresStore, targetID, key string, at time.Time) {
	t.Helper()
	fact := webhookEvidence(key, targetID, at)
	if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "webhook", ObservedAt: at, Facts: []EvidenceFact{fact},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRelapseShortlyAfterResolutionResumesTheIncident(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	target := insertCycleTarget(t, ctx, pool, "trust-zabbix-01")
	startedAt := time.Date(2026, time.October, 3, 4, 40, 0, 0, time.UTC)

	incidentID := resolveSingleTargetCycle(t, ctx, store, target, "housekeeper-1", startedAt)
	applyRelapse(t, ctx, store, target, "housekeeper-2", startedAt.Add(time.Hour))

	items, err := store.ListForTarget(ctx, "all", target, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != incidentID {
		t.Fatalf("a relapse within six hours must resume the resolved Incident, got %#v", items)
	}
	resumed := items[0]
	if resumed.Status != "active" || resumed.ResolvedAt != nil || resumed.ActiveImpactCount != 1 {
		t.Fatalf("the resumed Incident must be active again, got %#v", resumed)
	}
	if resumed.ResumptionCount != 1 || resumed.ResumedAt == nil || !resumed.ResumedAt.Equal(startedAt.Add(time.Hour)) {
		t.Fatalf("the resumed Incident must count its Reprise, got %#v", resumed)
	}
	if !resumed.OpenedAt.Equal(startedAt) {
		t.Fatalf("a Reprise keeps the original opening, got %s", resumed.OpenedAt)
	}
	if !hasActivity(resumed, "resumed") {
		t.Fatalf("a Reprise must be recorded in the activity, got %#v", resumed.Activity)
	}

	// Un problème qui continue d'osciller reste le même Incident.
	if err := store.ResolveEvidence(ctx, "webhook", target, "housekeeper-2", startedAt.Add(70*time.Minute)); err != nil {
		t.Fatal(err)
	}
	applyRelapse(t, ctx, store, target, "housekeeper-3", startedAt.Add(2*time.Hour))
	again, err := store.Get(ctx, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "active" || again.ResumptionCount != 2 {
		t.Fatalf("a second relapse must resume the same Incident again, got %#v", again)
	}
}

func TestRelapseAfterTheResumptionWindowOpensANewIncident(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	target := insertCycleTarget(t, ctx, pool, "storage-pbs-01")
	startedAt := time.Date(2026, time.October, 3, 4, 40, 0, 0, time.UTC)

	incidentID := resolveSingleTargetCycle(t, ctx, store, target, "disk-1", startedAt)
	// Résolu à +5 min : la Reprise reste possible jusqu'à +6 h 05.
	applyRelapse(t, ctx, store, target, "disk-2", startedAt.Add(6*time.Hour+6*time.Minute))

	items, err := store.ListForTarget(ctx, "active", target, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID == incidentID || items[0].ResumptionCount != 0 {
		t.Fatalf("a relapse after six hours must open a new Incident, got %#v", items)
	}
}

func TestRelapseOfAPropagatedIncidentOpensANewIncident(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	first := insertCycleTarget(t, ctx, pool, "Nextcloud")
	second := insertCycleTarget(t, ctx, pool, "trust-monitoring-01")
	startedAt := time.Date(2026, time.October, 3, 4, 40, 0, 0, time.UTC)

	for _, fact := range []EvidenceFact{
		webhookEvidence("latency-1", first, startedAt),
		webhookEvidence("latency-2", second, startedAt.Add(10*time.Second)),
	} {
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
			Origin: "webhook", ObservedAt: fact.OpenedAt, Facts: []EvidenceFact{fact},
		}); err != nil {
			t.Fatal(err)
		}
	}
	propagated, err := store.ListForTarget(ctx, "active", first, 10)
	if err != nil || len(propagated) != 1 {
		t.Fatalf("expected one propagated Incident, got %#v (%v)", propagated, err)
	}
	for _, item := range []struct{ target, key string }{{first, "latency-1"}, {second, "latency-2"}} {
		if err := store.ResolveEvidence(ctx, "webhook", item.target, item.key, startedAt.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Advance(ctx, startedAt.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	applyRelapse(t, ctx, store, first, "latency-3", startedAt.Add(time.Hour))
	items, err := store.ListForTarget(ctx, "active", first, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID == propagated[0].ID {
		t.Fatalf("a propagated Incident must not be resumed for one of its Ressources, got %#v", items)
	}
}

func TestResumedIncidentKeepsItsAcknowledgement(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	target := insertCycleTarget(t, ctx, pool, "trust-wazuh-01")
	startedAt := time.Date(2026, time.October, 3, 4, 40, 0, 0, time.UTC)

	fact := webhookEvidence("packages-1", target, startedAt)
	if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "webhook", ObservedAt: startedAt, Facts: []EvidenceFact{fact},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListForTarget(ctx, "active", target, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("expected one active Incident, got %#v (%v)", items, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET acknowledged_at = $2,
		       acknowledgement_origin = 'user'
		WHERE id = $1::uuid
	`, items[0].ID, startedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveEvidence(ctx, "webhook", target, "packages-1", startedAt.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Advance(ctx, startedAt.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	applyRelapse(t, ctx, store, target, "packages-2", startedAt.Add(time.Hour))
	resumed, err := store.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != "active" || resumed.AcknowledgedAt == nil {
		t.Fatalf("a resumed Incident keeps its Acknowledgement, got %#v", resumed)
	}
}

func hasActivity(incident Incident, kind string) bool {
	for _, activity := range incident.Activity {
		if activity.Kind == kind {
			return true
		}
	}
	return false
}
