package incidents

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEvidenceCycleGroupsTargetImpactsAndWaitsForPropagationBeforeResolution(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	firstTarget := insertCycleTarget(t, ctx, pool, "Passerelle")
	secondTarget := insertCycleTarget(t, ctx, pool, "Base de données")
	startedAt := time.Date(2026, time.September, 4, 10, 0, 0, 0, time.UTC)

	for _, fact := range []EvidenceFact{
		webhookEvidence("first", firstTarget, startedAt),
		webhookEvidence("second", secondTarget, startedAt.Add(20*time.Second)),
	} {
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
			Origin: "webhook", ObservedAt: fact.OpenedAt, Facts: []EvidenceFact{fact},
		}); err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.List(ctx, "active", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Impacts) != 2 || items[0].ActiveImpactCount != 2 {
		t.Fatalf("expected one Incident with two active Atteintes, got %#v", items)
	}
	incidentID := items[0].ID

	if err := store.ResolveEvidence(ctx, "webhook", firstTarget, "first", startedAt.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveEvidence(ctx, "webhook", secondTarget, "second", startedAt.Add(40*time.Second)); err != nil {
		t.Fatal(err)
	}
	incident, err := store.Get(ctx, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	if incident.Status != "active" || incident.ActiveImpactCount != 0 || incident.PropagationStatus != "open" {
		t.Fatalf("zero active Atteinte must wait for propagation closure, got %#v", incident)
	}

	if err := store.Advance(ctx, startedAt.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	incident, err = store.Get(ctx, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	if incident.Status != "resolved" || incident.PropagationStatus != "closed" || incident.ResolvedAt == nil {
		t.Fatalf("expected resolution after propagation closure, got %#v", incident)
	}
}

func TestEvidenceCycleUsesEvidenceTimeForHistoricalBatches(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	firstTarget := insertCycleTarget(t, ctx, pool, "Historique API")
	secondTarget := insertCycleTarget(t, ctx, pool, "Historique base")
	startedAt := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC)

	if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "webhook", ObservedAt: startedAt.Add(time.Hour),
		Facts: []EvidenceFact{
			webhookEvidence("second", secondTarget, startedAt.Add(20*time.Second)),
			webhookEvidence("first", firstTarget, startedAt),
		},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, "all", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Impacts) != 2 {
		t.Fatalf("historical Evidence from one propagation must share an Incident: %#v", items)
	}
	if items[0].PropagationStatus != "closed" {
		t.Fatalf("a historical batch must not extend propagation until receipt time: %#v", items[0])
	}
}

func TestEvidenceCycleSerializesConcurrentNatureArrivals(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	firstTarget := insertCycleTarget(t, ctx, pool, "Concurrent API")
	secondTarget := insertCycleTarget(t, ctx, pool, "Concurrent worker")
	startedAt := time.Now().UTC()
	ready := make(chan struct{})
	errorsByArrival := make(chan error, 2)
	var workers sync.WaitGroup

	for _, fact := range []EvidenceFact{
		webhookEvidence("concurrent-first", firstTarget, startedAt),
		webhookEvidence("concurrent-second", secondTarget, startedAt),
	} {
		workers.Add(1)
		go func(fact EvidenceFact) {
			defer workers.Done()
			<-ready
			errorsByArrival <- store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
				Origin: "webhook", ObservedAt: startedAt, Facts: []EvidenceFact{fact},
			})
		}(fact)
	}
	close(ready)
	workers.Wait()
	close(errorsByArrival)
	for err := range errorsByArrival {
		if err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.List(ctx, "active", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Impacts) != 2 {
		t.Fatalf("concurrent arrivals split one Nature into multiple Incidents: %#v", items)
	}
}

func TestEvidenceCycleStartsANewIncidentAfterPropagationCloses(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	firstTarget := insertCycleTarget(t, ctx, pool, "API")
	secondTarget := insertCycleTarget(t, ctx, pool, "Worker")
	startedAt := time.Date(2026, time.September, 4, 11, 0, 0, 0, time.UTC)

	for _, fact := range []EvidenceFact{
		webhookEvidence("api-first", firstTarget, startedAt),
		webhookEvidence("worker", secondTarget, startedAt.Add(10*time.Second)),
	} {
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
			Origin: "webhook", ObservedAt: fact.OpenedAt, Facts: []EvidenceFact{fact},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ResolveEvidence(ctx, "webhook", firstTarget, "api-first", startedAt.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.Advance(ctx, startedAt.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}

	relapse := webhookEvidence("api-second", firstTarget, startedAt.Add(4*time.Minute))
	if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "webhook", ObservedAt: relapse.OpenedAt, Facts: []EvidenceFact{relapse},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListForTarget(ctx, "active", firstTarget, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected the old active Incident and a new Incident after relapse, got %#v", items)
	}
}

func webhookEvidence(key, targetID string, openedAt time.Time) EvidenceFact {
	return EvidenceFact{
		Origin: "webhook", IdentityScope: targetID, IdentityKey: key,
		TargetID: targetID, ExternalEventID: key, ExternalObjectID: key,
		Nature: CanonicalNature(NatureAvailability, NatureAvailabilityLabel),
		Name:   "Indisponibilité détectée", Severity: SeverityMajor, OpenedAt: openedAt,
	}
}

func insertCycleTarget(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text
	`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
