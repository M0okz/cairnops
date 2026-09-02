package notifications_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/testsupport"
	"golang.org/x/crypto/curve25519"
)

func TestIntegratedDeliverySchedulesOnePushPerActiveDevice(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID, targetID, incidentID, channelID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('push-user', 'Push User', 'not-used', 'operator') RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Push target') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status, source_severity,
			effective_severity, opened_at
		) VALUES ($1::uuid, 'native:http', 'Indisponibilité', 'active', 'critical', 'critical', now())
		RETURNING id::text
	`, targetID).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM cairnops_notification_channels WHERE kind = 'in_app'`).Scan(&channelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_devices (
			user_id, name, platform, encryption_public_key,
			push_recipient_sealed, token_digest
		) VALUES ($1::uuid, 'iPhone', 'ios', $2, $3, $4)
	`, userID, curve25519.Basepoint, "sealed-recipient-with-sufficient-length", make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_devices (
			user_id, name, platform, encryption_public_key, token_digest
		) VALUES ($1::uuid, 'iPad sans Push', 'ios', $2, $3)
	`, userID, curve25519.Basepoint, bytes.Repeat([]byte{1}, 32)); err != nil {
		t.Fatal(err)
	}
	store := immediateNotificationStore(pool)
	if _, err := store.Deliver(ctx, notifications.Delivery{
		IncidentID: incidentID, ChannelID: channelID, ChannelKind: notifications.KindInApp,
		EventKind: "firing", TargetName: "Push target", NatureLabel: "Indisponibilité",
		Severity: incidents.SeverityCritical, OpenedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	var pushes int
	if err := pool.QueryRow(ctx, `SELECT count(*)::integer FROM cairnops_push_outbox`).Scan(&pushes); err != nil {
		t.Fatal(err)
	}
	if pushes != 1 {
		t.Fatalf("expected one per-device push, got %d", pushes)
	}
}

func TestIntegratedResolutionUpdatesStateWithoutAlertingAgain(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID, targetID, incidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('quiet-resolution-user', 'Quiet Resolution User', 'not-used', 'operator') RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Quiet resolution target') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status, source_severity,
			effective_severity, opened_at
		) VALUES ($1::uuid, 'zabbix:disk', 'Latence disque', 'active', 'major', 'major', now())
		RETURNING id::text
	`, targetID).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_devices (
			user_id, name, platform, encryption_public_key,
			push_recipient_sealed, token_digest
		) VALUES ($1::uuid, 'iPhone', 'ios', $2, $3, $4)
	`, userID, curve25519.Basepoint, "sealed-recipient-with-sufficient-length", make([]byte, 32)); err != nil {
		t.Fatal(err)
	}

	store := immediateNotificationStore(pool)
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	opening, err := store.Claim(ctx, "quiet-resolution-worker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Deliver(ctx, opening); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, opening.ID, "quiet-resolution-worker"); err != nil {
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
	resolution, err := store.Claim(ctx, "quiet-resolution-worker")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.EventKind != "resolved" || resolution.Presentation != "silent" {
		t.Fatalf("resolution must update state without a second alert: %#v", resolution)
	}
	if _, err := store.Deliver(ctx, resolution); err != nil {
		t.Fatal(err)
	}

	var presentation string
	if err := pool.QueryRow(ctx, `
		SELECT outgoing.presentation
		FROM cairnops_push_outbox outgoing
		JOIN cairnops_notification_inbox inbox ON inbox.id = outgoing.inbox_id
		WHERE inbox.incident_id = $1::uuid AND inbox.event_kind = 'resolved'
	`, incidentID).Scan(&presentation); err != nil {
		t.Fatal(err)
	}
	if presentation != "silent" {
		t.Fatalf("resolution scheduled a visible device push: %s", presentation)
	}
}
