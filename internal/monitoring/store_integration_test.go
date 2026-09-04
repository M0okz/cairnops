package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/testsupport"
)

// Complete enregistre l'Observation et confronte la Source à sa Politique de
// déclenchement dans la même transaction : la preuve et l'Incident ne peuvent
// donc jamais diverger.
func TestPostgresCompleteFeedsTheIncidentCycle(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	var targetID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Cible supervisée') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, interval_seconds, timeout_milliseconds,
			config, failure_threshold, recovery_threshold, severity
		) VALUES ($1::uuid, 'Public endpoint', 'http', 20, 5000,
		          '{"url":"https://example.net"}'::jsonb, 2, 1, 'critical')
	`, targetID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	const owner = "scheduler-integration-test"
	run := func(outcome domain.Outcome) {
		t.Helper()
		// Le test est seul dans sa base : la seule Source due est la sienne.
		claimed, err := store.ClaimDue(ctx, owner, 10, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if len(claimed) != 1 {
			t.Fatalf("expected the lone source under test to be claimed, got %d", len(claimed))
		}
		source := claimed[0]
		observation := domain.Observation{
			SourceID: source.ID, TargetID: source.TargetID, ObservedAt: time.Now().UTC(),
			Outcome: outcome, Reason: "status_code_unexpected",
		}
		if err := store.Complete(ctx, owner, source, observation); err != nil {
			t.Fatal(err)
		}
		// La Source redevient immédiatement due pour l'observation suivante.
		if _, err := pool.Exec(ctx, `
			UPDATE cairnops_signal_sources SET next_run_at = now() WHERE id = $1::uuid
		`, source.ID); err != nil {
			t.Fatal(err)
		}
	}
	activeIncident := func() *incidents.Incident {
		t.Helper()
		active, err := incidents.NewPostgresStore(pool).List(ctx, "active", 200)
		if err != nil {
			t.Fatal(err)
		}
		if len(active) == 0 {
			return nil
		}
		return &active[0]
	}

	run(domain.OutcomeUnhealthy)
	if incident := activeIncident(); incident != nil {
		t.Fatalf("a single unhealthy observation must not open an incident: %#v", incident)
	}
	run(domain.OutcomeUnhealthy)
	incident := activeIncident()
	if incident == nil {
		t.Fatal("the second unhealthy observation should open an incident")
	}
	if incident.Severity != incidents.SeverityCritical || incident.NatureKey != incidents.NatureAvailability {
		t.Fatalf("unexpected incident: %#v", incident)
	}

	run(domain.OutcomeHealthy)
	if err := incidents.NewPostgresStore(pool).Advance(ctx, time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if again := activeIncident(); again != nil {
		t.Fatalf("the confirmed recovery should resolve the incident: %#v", again)
	}

	var observed int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_observations WHERE target_id = $1::uuid
	`, targetID).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	if observed != 3 {
		t.Fatalf("expected the three observations to be kept, got %d", observed)
	}
}
