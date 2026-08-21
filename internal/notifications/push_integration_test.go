package notifications_test

import (
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
	store := notifications.NewPostgresStore(pool)
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
