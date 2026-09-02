package notifications_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/testsupport"
)

type acceptingMattermost struct{}

func (acceptingMattermost) Test(context.Context, string) error { return nil }

func TestNonCriticalOpeningWaitsForStabilityAndDropsTransientIncident(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)
	_, incidentID := seedActiveIncident(t, pool, "warning")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if delivery, err := store.Claim(ctx, "stability-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("unstable Incident was already deliverable: %#v, %v", delivery, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if delivery, err := store.Claim(ctx, "stability-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("transient Incident produced a notification after resolving: %#v, %v", delivery, err)
	}

	var activeDeliveries, resolutions int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status <> 'cancelled'),
		       count(*) FILTER (WHERE event_kind = 'resolved')
		FROM cairnops_notification_outbox WHERE incident_id = $1::uuid
	`, incidentID).Scan(&activeDeliveries, &resolutions); err != nil {
		t.Fatal(err)
	}
	if activeDeliveries != 0 || resolutions != 0 {
		t.Fatalf("transient Incident left deliverable noise: active=%d resolutions=%d", activeDeliveries, resolutions)
	}
}

func TestCriticalOpeningBypassesStabilityDelay(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)
	_, incidentID := seedActiveIncident(t, pool, "critical")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery, err := store.Claim(ctx, "critical-worker")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.IncidentID != incidentID || delivery.EventKind != "firing" {
		t.Fatalf("critical Incident did not bypass the stability delay: %#v", delivery)
	}
}

func TestMatureNonCriticalOpeningBecomesDeliverable(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)
	_, incidentID := seedActiveIncident(t, pool, "major")
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET created_at = now() - interval '3 minutes'
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery, err := store.Claim(ctx, "mature-worker")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.IncidentID != incidentID || delivery.EventKind != "firing" {
		t.Fatalf("stable Incident remained held: %#v", delivery)
	}
}

func TestNotificationRoutingStopsAtAcknowledgementAndResolvesToOpeningChannel(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	var actorID, targetID, incidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Notification Admin', 'not-used', 'administrator') RETURNING id::text
	`, fmt.Sprintf("notification-%d", suffix)).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, fmt.Sprintf("Mattermost target %d", suffix)).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store := immediateNotificationStore(pool)
	channel, err := notifications.NewService(store, acceptingMattermost{}, box).CreateMattermost(ctx, actorID, notifications.CreateMattermostInput{
		Name: "Exploitation", WebhookURL: "https://mattermost.example.test/hooks/secret",
		Severities: []incidents.Severity{incidents.SeverityMajor},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status, source_severity, effective_severity, opened_at
		) VALUES ($1::uuid, 'native:tcp', 'Port indisponible', 'active', 'major', 'major', now())
		RETURNING id::text
	`, targetID).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	// Le Canal intégré est posé par la migration et routerait le même Incident.
	// Ce scénario porte sur Mattermost : on l'écarte pour que la boîte d'envoi
	// ne contienne que la livraison qu'il examine.
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_notification_channels SET enabled = false WHERE kind = 'in_app'
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	firing, err := store.Claim(ctx, "worker-test")
	if err != nil {
		t.Fatal(err)
	}
	if firing.EventKind != "firing" || firing.ChannelID != channel.ID || firing.IncidentID != incidentID {
		t.Fatalf("unexpected firing delivery: %#v", firing)
	}
	if err := store.Complete(ctx, firing.ID, "worker-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	resolution, err := store.Claim(ctx, "worker-test")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.EventKind != "resolved" || resolution.ChannelID != firing.ChannelID {
		t.Fatalf("resolution must use the opening channel: %#v", resolution)
	}
	if err := store.Complete(ctx, resolution.ID, "worker-test"); err != nil {
		t.Fatal(err)
	}

	var acknowledgedIncidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status, source_severity, effective_severity, opened_at
		) VALUES ($1::uuid, 'native:dns', 'Réponse DNS invalide', 'active', 'major', 'major',
		          now())
		RETURNING id::text
	`, targetID).Scan(&acknowledgedIncidentID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET acknowledged_at = now(), acknowledged_by = $2::uuid, acknowledgement_origin = 'user', updated_at = now()
		WHERE id = $1::uuid
	`, acknowledgedIncidentID, actorID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if delivery, err := store.Claim(ctx, "worker-test"); err != notifications.ErrNoDelivery {
		t.Fatalf("acknowledged incident must not be delivered, got %#v, %v", delivery, err)
	}
}
