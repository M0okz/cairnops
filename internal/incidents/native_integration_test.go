package incidents

import (
	"context"
	"fmt"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresNativeIncidentLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("native-test-%d", suffix)
	targetName := fmt.Sprintf("Native check target %d", suffix)
	var actorID, targetID, sourceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Native Test', 'not-used', 'operator')
		RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, targetName).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM cairnops_targets WHERE id = $1::uuid`, targetID)
		_, _ = pool.Exec(ctx, `DELETE FROM cairnops_users WHERE id = $1::uuid`, actorID)
	}()
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, interval_seconds, timeout_milliseconds,
			failure_threshold, recovery_threshold, severity
		) VALUES ($1::uuid, 'Public endpoint', 'http', 20, 5000, 3, 2, 'major')
		RETURNING id::text
	`, targetID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	clock := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	observe := func(outcome domain.Outcome) {
		t.Helper()
		clock = clock.Add(20 * time.Second)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := ApplyNativeObservation(ctx, tx, NativeObservation{
			SourceID: sourceID, TargetID: targetID, SourceName: "Public endpoint",
			Outcome: outcome, ObservedAt: clock, Reason: "status_code_unexpected",
		}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	activeIncident := func() *Incident {
		t.Helper()
		active, err := store.List(ctx, "active", 100)
		if err != nil {
			t.Fatal(err)
		}
		for index := range active {
			if active[index].TargetID == targetID {
				return &active[index]
			}
		}
		return nil
	}

	observe(domain.OutcomeUnhealthy)
	observe(domain.OutcomeUnhealthy)
	if incident := activeIncident(); incident != nil {
		t.Fatalf("two unhealthy observations must not open an incident: %#v", incident)
	}

	// Une Observation Inconnue ne rapproche ni de la dégradation ni du rétablissement.
	observe(domain.OutcomeUnknown)
	if incident := activeIncident(); incident != nil {
		t.Fatalf("an unknown observation must not open an incident: %#v", incident)
	}
	assertStreaks(t, ctx, pool, sourceID, 2, 0)

	observe(domain.OutcomeUnhealthy)
	incident := activeIncident()
	if incident == nil {
		t.Fatal("the third unhealthy observation should open an incident")
	}
	if incident.NatureKey != NatureAvailability || incident.EffectiveSeverity != SeverityMajor {
		t.Fatalf("unexpected incident nature or severity: %#v", incident)
	}
	if len(incident.Signals) != 1 || incident.Signals[0].Origin != "native" || !incident.Signals[0].Active {
		t.Fatalf("unexpected incident signals: %#v", incident.Signals)
	}
	firstIncidentID := incident.ID

	observe(domain.OutcomeUnhealthy)
	if again := activeIncident(); again == nil || again.ID != firstIncidentID || len(again.Signals) != 1 {
		t.Fatalf("a repeated trigger must feed the same incident and signal: %#v", again)
	}

	observe(domain.OutcomeHealthy)
	if again := activeIncident(); again == nil {
		t.Fatal("a single healthy observation must not resolve the incident")
	}
	observe(domain.OutcomeHealthy)
	if again := activeIncident(); again != nil {
		t.Fatalf("the confirmed recovery should resolve the incident: %#v", again)
	}
	resolved, err := store.Get(ctx, firstIncidentID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Fatalf("unexpected resolved incident: %#v", resolved)
	}

	// Un déclenchement ultérieur ouvre un nouvel Incident de la même Nature.
	observe(domain.OutcomeUnhealthy)
	observe(domain.OutcomeUnhealthy)
	observe(domain.OutcomeUnhealthy)
	incident = activeIncident()
	if incident == nil || incident.ID == firstIncidentID {
		t.Fatalf("a later trigger should open a new incident: %#v", incident)
	}

	invalidated, err := store.InvalidateSignal(ctx, incident.ID, incident.Signals[0].ID, actorID, "Native Test", "sonde manifestement défaillante")
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Status != "resolved" {
		t.Fatalf("invalidating the only evidence should resolve the incident: %#v", invalidated)
	}

	// La Source Invalidée continue ses Observations sans réalimenter l'Incident.
	observe(domain.OutcomeUnhealthy)
	observe(domain.OutcomeUnhealthy)
	observe(domain.OutcomeUnhealthy)
	if again := activeIncident(); again != nil {
		t.Fatalf("an invalidated source must not reopen an incident: %#v", again)
	}

	// Son prochain cycle sain met fin à l'Invalidation.
	observe(domain.OutcomeHealthy)
	observe(domain.OutcomeHealthy)
	observe(domain.OutcomeUnhealthy)
	observe(domain.OutcomeUnhealthy)
	observe(domain.OutcomeUnhealthy)
	rearmed := activeIncident()
	if rearmed == nil {
		t.Fatal("a rearmed source should be able to open a new incident")
	}
	if rearmed.ID == incident.ID {
		t.Fatalf("the rearmed source should open a new incident, not revive %s", incident.ID)
	}
}

func assertStreaks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sourceID string, unhealthy, healthy int) {
	t.Helper()
	var actualUnhealthy, actualHealthy int
	if err := pool.QueryRow(ctx, `
		SELECT consecutive_unhealthy, consecutive_healthy
		FROM cairnops_signal_sources WHERE id = $1::uuid
	`, sourceID).Scan(&actualUnhealthy, &actualHealthy); err != nil {
		t.Fatal(err)
	}
	if actualUnhealthy != unhealthy || actualHealthy != healthy {
		t.Fatalf("expected streaks (%d, %d), got (%d, %d)", unhealthy, healthy, actualUnhealthy, actualHealthy)
	}
}
