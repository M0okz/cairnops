package incidents

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Un cycle qui revoit la même Preuve ne fait qu'avancer son last_seen_at :
// rien de visible ne change, aucun client n'a donc à recharger les Incidents.
func TestIdenticalEvidenceCyclesAppendNoIncidentEvent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	targetID := insertCycleTarget(t, ctx, pool, "Passerelle")
	startedAt := time.Date(2026, time.October, 15, 10, 0, 0, 0, time.UTC)
	fact := webhookEvidence("passerelle", targetID, startedAt)

	apply := func(fact EvidenceFact, observedAt time.Time) {
		t.Helper()
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
			Origin: "webhook", ObservedAt: observedAt, Facts: []EvidenceFact{fact},
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply(fact, startedAt)
	if countIncidentEvents(t, ctx, pool) == 0 {
		t.Fatal("opening an Incident must announce it")
	}
	// La clôture de la propagation est un vrai changement : elle est annoncée
	// avant que la mesure commence.
	settledAt := startedAt.Add(5 * time.Minute)
	if err := store.Advance(ctx, settledAt); err != nil {
		t.Fatal(err)
	}
	incident, err := store.Get(ctx, currentIncidentID(t, ctx, pool))
	if err != nil {
		t.Fatal(err)
	}
	if incident.Status != "active" || incident.PropagationStatus != "closed" {
		t.Fatalf("expected an active Incident with closed propagation, got %#v", incident)
	}
	settled := countIncidentEvents(t, ctx, pool)

	for cycle := 1; cycle <= 10; cycle++ {
		apply(fact, settledAt.Add(time.Duration(cycle)*30*time.Second))
	}
	if got := countIncidentEvents(t, ctx, pool) - settled; got != 0 {
		t.Fatalf("10 identical cycles appended %d incident.changed, want 0", got)
	}
	var lastSeenAt time.Time
	if err := pool.QueryRow(ctx, `SELECT last_seen_at FROM cairnops_incident_evidence WHERE identity_key = 'passerelle'`).Scan(&lastSeenAt); err != nil {
		t.Fatal(err)
	}
	if want := settledAt.Add(5 * time.Minute); !lastSeenAt.Equal(want) {
		t.Fatalf("identical cycles must still refresh last_seen_at, got %s want %s", lastSeenAt, want)
	}

	fact.Name = "Passerelle injoignable"
	apply(fact, settledAt.Add(6*time.Minute))
	if got := countIncidentEvents(t, ctx, pool) - settled; got != 1 {
		t.Fatalf("a renamed Preuve appended %d incident.changed, want 1", got)
	}
}

func currentIncidentID(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM cairnops_incidents`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func countIncidentEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_events WHERE kind = 'incident.changed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
