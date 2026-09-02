package notifications_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/bursts"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5"
)

func TestTransientBurstResolvesInsideStabilityWindowWithoutNotification(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	first := notificationIncident(t, ctx, pool, notificationTarget(t, ctx, pool, "Transient burst A"), now)

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	second := notificationIncident(t, ctx, pool, notificationTarget(t, ctx, pool, "Transient burst B"), now.Add(time.Second))
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if delivery, err := store.Claim(ctx, "transient-burst-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("unstable Rafale was already deliverable: %#v, %v", delivery, err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE id = ANY($1::uuid[])
	`, []string{first, second}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incident_bursts SET propagation_ends_at = now() - interval '1 second'
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if delivery, err := store.Claim(ctx, "transient-burst-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("transient Rafale produced a notification after resolving: %#v, %v", delivery, err)
	}
}

func TestBurstReusesOpeningAndSuppressesSecondIncidentNotification(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	now := time.Now().UTC().Truncate(time.Second)

	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Burst Operator', 'not-used', 'operator') RETURNING id::text
	`, fmt.Sprintf("burst-operator-%d", now.UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	firstTarget := notificationTarget(t, ctx, pool, "Burst notification A")
	secondTarget := notificationTarget(t, ctx, pool, "Burst notification B")
	firstIncident := notificationIncident(t, ctx, pool, firstTarget, now)

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	opening, err := store.Claim(ctx, "burst-worker")
	if err != nil {
		t.Fatal(err)
	}
	if opening.IncidentID != firstIncident || opening.BurstID != "" || opening.EventKind != "firing" {
		t.Fatalf("unexpected first opening: %#v", opening)
	}
	if _, err := store.Deliver(ctx, opening); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, opening.ID, "burst-worker"); err != nil {
		t.Fatal(err)
	}

	secondIncident := notificationIncident(t, ctx, pool, secondTarget, now.Add(10*time.Second))
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	update, err := store.Claim(ctx, "burst-worker")
	if err != nil {
		t.Fatal(err)
	}
	if update.BurstID == "" || update.IncidentID != firstIncident || update.EventKind != "burst_update" {
		t.Fatalf("second incident must update the first opening: %#v", update)
	}
	if update.Presentation != "silent" || update.IncidentCount != 2 || update.AffectedTargets != 2 {
		t.Fatalf("unexpected burst update policy: %#v", update)
	}
	if _, err := store.Deliver(ctx, update); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, update.ID, "burst-worker"); err != nil {
		t.Fatal(err)
	}

	if delivery, err := store.Claim(ctx, "burst-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("no notification must remain for the second incident, got %#v, %v", delivery, err)
	}
	inbox, err := store.Inbox(ctx, userID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Entries) != 1 {
		t.Fatalf("one mutable entry expected, got %#v", inbox.Entries)
	}
	entry := inbox.Entries[0]
	if entry.BurstID == "" || entry.IncidentID != firstIncident || entry.IncidentCount != 2 || entry.AffectedTargetCount != 2 {
		t.Fatalf("unexpected mutable burst entry: %#v", entry)
	}

	var redundant int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM cairnops_notification_outbox
		WHERE incident_id = $1::uuid AND status <> 'cancelled'
	`, secondIncident).Scan(&redundant); err != nil {
		t.Fatal(err)
	}
	if redundant != 0 {
		t.Fatalf("second incident has %d non-cancelled deliveries", redundant)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE id = ANY($1::uuid[])
	`, []string{firstIncident, secondIncident}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incident_bursts SET propagation_ends_at = now() - interval '1 second'
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	resolution := deliverNotification(t, ctx, store, "burst_update", "silent")
	if resolution.BurstStatus != "resolved" {
		t.Fatalf("resolved burst did not become a quiet state update: %#v", resolution)
	}
}

func TestBurstAlertsOnlyForANewHistoricalSeverityHigh(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	first := notificationIncidentWithSeverity(t, ctx, pool, notificationTarget(t, ctx, pool, "Severity A"), now, "major")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	deliverNotification(t, ctx, store, "firing", "alert")

	second := notificationIncidentWithSeverity(t, ctx, pool, notificationTarget(t, ctx, pool, "Severity B"), now.Add(5*time.Second), "warning")
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	deliverNotification(t, ctx, store, "burst_update", "silent")

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET source_severity = 'critical', effective_severity = 'critical', updated_at = now()
		WHERE id = $1::uuid
	`, second); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	deliverNotification(t, ctx, store, "burst_update", "alert")

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET source_severity = 'warning', effective_severity = 'warning', updated_at = now()
		WHERE id = $1::uuid
	`, second); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	deliverNotification(t, ctx, store, "burst_update", "silent")

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET source_severity = 'critical', effective_severity = 'critical', updated_at = now()
		WHERE id = $1::uuid
	`, second); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery := deliverNotification(t, ctx, store, "burst_update", "silent")
	if delivery.IncidentID != first {
		t.Fatalf("burst update lost its original opening: %#v", delivery)
	}
}

func TestBurstAlertsForExtendedPropagationOnlyOnce(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	now := time.Now().UTC().Truncate(time.Second)

	for index := 0; index < 6; index++ {
		notificationIncident(t, ctx, pool, notificationTarget(t, ctx, pool, fmt.Sprintf("Extended %d", index+1)), now.Add(time.Duration(index)*time.Second))
		if err := store.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
		wantKind, wantPresentation := "burst_update", "silent"
		if index == 0 {
			wantKind, wantPresentation = "firing", "alert"
		} else if index == 4 {
			wantPresentation = "alert"
		}
		delivery := deliverNotification(t, ctx, store, wantKind, wantPresentation)
		if index == 4 && (!delivery.BurstExtended || delivery.AffectedTargets != 5) {
			t.Fatalf("extended threshold did not carry its impact: %#v", delivery)
		}
		if index == 5 && !delivery.BurstExtended {
			t.Fatalf("extended state must remain sticky: %#v", delivery)
		}
	}
}

func TestMattermostBurstSendsOnlyOpeningAndFinalSummary(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Burst Admin', 'not-used', 'administrator') RETURNING id::text
	`, fmt.Sprintf("burst-mattermost-%d", now.UnixNano())).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	channel, err := notifications.NewService(store, acceptingMattermost{}, box).CreateMattermost(ctx, actorID, notifications.CreateMattermostInput{
		Name: "Rafales", WebhookURL: "https://mattermost.example.test/hooks/burst",
		Severities: []incidents.Severity{incidents.SeverityMajor},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_notification_channels SET enabled = false WHERE kind = 'in_app'`); err != nil {
		t.Fatal(err)
	}

	first := notificationIncident(t, ctx, pool, notificationTarget(t, ctx, pool, "Mattermost burst A"), now)
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	opening := deliverNotification(t, ctx, store, "firing", "alert")
	if opening.ChannelID != channel.ID || opening.BurstID != "" {
		t.Fatalf("unexpected Mattermost opening: %#v", opening)
	}
	second := notificationIncident(t, ctx, pool, notificationTarget(t, ctx, pool, "Mattermost burst B"), now.Add(5*time.Second))
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if delivery, err := store.Claim(ctx, "burst-transition-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("Mattermost must not receive an intermediate update, got %#v, %v", delivery, err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE id = ANY($1::uuid[])
	`, []string{first, second}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incident_bursts SET propagation_ends_at = now() - interval '1 second'
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	resolution := deliverNotification(t, ctx, store, "resolved", "alert")
	if resolution.ChannelID != channel.ID || resolution.BurstID == "" || resolution.MaxAffected != 2 {
		t.Fatalf("unexpected Mattermost final summary: %#v", resolution)
	}
	if delivery, err := store.Claim(ctx, "burst-transition-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("only one final summary is allowed, got %#v, %v", delivery, err)
	}
}

func TestBurstOpeningDoesNotDependOnAnchorMaintenance(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	anchorTarget := notificationTarget(t, ctx, pool, "Maintained anchor")
	first := notificationIncident(t, ctx, pool, anchorTarget, now)
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	second := notificationIncident(t, ctx, pool, notificationTarget(t, ctx, pool, "Visible member"), now.Add(time.Second))
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		WITH maintenance AS (
			INSERT INTO cairnops_maintenances (name, reason, starts_at, ends_at)
			VALUES ('Maintenance ancre', 'Maintenance de la seule Cible ancre', now() - interval '1 minute', now() + interval '1 hour')
			RETURNING id
		)
		INSERT INTO cairnops_maintenance_targets (maintenance_id, target_id)
		SELECT maintenance.id, $1::uuid FROM maintenance
	`, anchorTarget); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery := deliverNotification(t, ctx, store, "firing", "alert")
	if delivery.BurstID == "" || delivery.IncidentID != first || delivery.AffectedTargets != 1 {
		t.Fatalf("the visible member must keep the burst opening deliverable: %#v (second=%s)", delivery, second)
	}
}

func TestAcknowledgedBurstStillAlertsForANewSeverityHigh(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	now := time.Now().UTC().Truncate(time.Second)
	first := notificationIncident(t, ctx, pool, notificationTarget(t, ctx, pool, "Acknowledged A"), now)
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	deliverNotification(t, ctx, store, "firing", "alert")
	second := notificationIncidentWithSeverity(t, ctx, pool, notificationTarget(t, ctx, pool, "Acknowledged B"), now.Add(time.Second), "warning")
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	deliverNotification(t, ctx, store, "burst_update", "silent")

	var actorID, burstID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Burst Acknowledger', 'not-used', 'operator') RETURNING id::text
	`, fmt.Sprintf("burst-new-high-%d", now.UnixNano())).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM cairnops_incident_bursts`).Scan(&burstID); err != nil {
		t.Fatal(err)
	}
	incidentService := incidents.NewService(incidents.NewPostgresStore(pool), nil)
	if _, err := bursts.NewService(bursts.NewPostgresStore(pool), incidentService).Acknowledge(ctx, burstID, actorID, "Burst Acknowledger"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET source_severity = 'critical', effective_severity = 'critical', updated_at = now()
		WHERE id = $1::uuid
	`, second); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery := deliverNotification(t, ctx, store, "burst_update", "alert")
	if delivery.BurstID != burstID || delivery.IncidentID != first || delivery.Severity != incidents.SeverityCritical {
		t.Fatalf("new high after acknowledgement was not preserved: %#v", delivery)
	}
}

func deliverNotification(t *testing.T, ctx context.Context, store *notifications.PostgresStore, wantKind, wantPresentation string) notifications.Delivery {
	t.Helper()
	delivery, err := store.Claim(ctx, "burst-transition-worker")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.EventKind != wantKind || delivery.Presentation != wantPresentation {
		t.Fatalf("unexpected delivery transition: got kind=%s presentation=%s, want %s/%s (%#v)", delivery.EventKind, delivery.Presentation, wantKind, wantPresentation, delivery)
	}
	if delivery.ChannelKind == "in_app" {
		if _, err := store.Deliver(ctx, delivery); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Complete(ctx, delivery.ID, "burst-transition-worker"); err != nil {
		t.Fatal(err)
	}
	return delivery
}

type notificationQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func notificationTarget(t *testing.T, ctx context.Context, pool notificationQueryer, prefix string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text
	`, fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func notificationIncident(t *testing.T, ctx context.Context, pool notificationQueryer, targetID string, openedAt time.Time) string {
	return notificationIncidentWithSeverity(t, ctx, pool, targetID, openedAt, "major")
}

func notificationIncidentWithSeverity(t *testing.T, ctx context.Context, pool notificationQueryer, targetID string, openedAt time.Time, severity string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status,
			source_severity, effective_severity, opened_at,
			nature_scope, nature_namespace, nature_fingerprint, burst_eligible
		) VALUES (
			$1::uuid, 'zabbix:connector-a:disk-prototype', 'Latence disque élevée',
			'active', $3, $3, $2,
			'connector', 'connector-a', 'disk-prototype', true
		) RETURNING id::text
	`, targetID, openedAt, severity).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
