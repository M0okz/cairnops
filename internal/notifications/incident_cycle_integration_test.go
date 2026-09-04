package notifications_test

import (
	"context"
	"testing"

	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestIncidentUpdatesReuseTheOpeningInboxEntry(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	userID := seedAccount(t, pool, "operator")
	_, incidentID := seedActiveIncident(t, pool, "major")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	opening := claimAndDeliver(t, ctx, store, "firing", "alert")

	secondTarget, _ := seedTarget(t, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incident_impacts (
			incident_id, target_id, status, source_severity,
			effective_severity, opened_at
		) VALUES ($1::uuid, $2::uuid, 'active', 'critical', 'critical', now())
	`, incidentID, secondTarget); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET severity = 'critical', active_impact_count = 2, impact_count = 2,
		    affected_target_count = 2, max_affected_targets = 2,
		    revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	update := claimAndDeliver(t, ctx, store, "incident_update", "alert")
	if update.IncidentID != opening.IncidentID || update.ImpactCount != 2 || update.AffectedTargets != 2 {
		t.Fatalf("unexpected incident update: %#v", update)
	}

	inbox, err := store.Inbox(ctx, userID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Entries) != 1 {
		t.Fatalf("one mutable incident entry expected, got %#v", inbox.Entries)
	}
	entry := inbox.Entries[0]
	if entry.IncidentID != incidentID || entry.ImpactCount != 2 || entry.AffectedTargetCount != 2 {
		t.Fatalf("unexpected mutable incident entry: %#v", entry)
	}
}

func TestResolvedIncidentProducesOneSilentInboxRevision(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	userID := seedAccount(t, pool, "operator")
	_, incidentID := seedActiveIncident(t, pool, "major")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "firing", "alert")
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incident_impacts
		SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE incident_id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET status = 'resolved', propagation_status = 'closed',
		    propagation_closed_at = now(), resolved_at = now(),
		    active_impact_count = 0, affected_target_count = 0,
		    revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "incident_update", "silent")

	inbox, err := store.Inbox(ctx, userID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Entries) != 1 || inbox.Entries[0].EventKind != "resolved" {
		t.Fatalf("resolution must revise the opening entry: %#v", inbox.Entries)
	}
}

func claimAndDeliver(t *testing.T, ctx context.Context, store *notifications.PostgresStore, wantKind, wantPresentation string) notifications.Delivery {
	t.Helper()
	delivery, err := store.Claim(ctx, "incident-cycle-worker")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.EventKind != wantKind || delivery.Presentation != wantPresentation {
		t.Fatalf("unexpected delivery: got %s/%s, want %s/%s (%#v)", delivery.EventKind, delivery.Presentation, wantKind, wantPresentation, delivery)
	}
	if delivery.ChannelKind == notifications.KindInApp {
		if _, err := store.Deliver(ctx, delivery); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Complete(ctx, delivery.ID, "incident-cycle-worker"); err != nil {
		t.Fatal(err)
	}
	return delivery
}
