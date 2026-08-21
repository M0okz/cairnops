package push

import (
	"context"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
	"golang.org/x/crypto/curve25519"
)

func TestExpiredRelayRecipientDisablesOnlyItsDevice(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID, targetID, incidentID, inboxID, deviceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('expired-push-user', 'Expired Push User', 'not-used', 'operator')
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Expired push target') RETURNING id::text`).Scan(&targetID); err != nil {
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
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_notification_inbox (
			user_id, incident_id, target_id, event_kind, target_name,
			nature_label, severity, occurred_at
		) VALUES ($1::uuid, $2::uuid, $3::uuid, 'firing', 'Expired push target',
		          'Indisponibilité', 'critical', now())
		RETURNING id::text
	`, userID, incidentID, targetID).Scan(&inboxID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_devices (
			user_id, name, platform, encryption_public_key,
			push_recipient_sealed, token_digest
		) VALUES ($1::uuid, 'Expired iPhone', 'ios', $2, $3, $4)
		RETURNING id::text
	`, userID, curve25519.Basepoint, "sealed-recipient-with-sufficient-length", make([]byte, 32)).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_push_outbox (device_id, inbox_id)
		VALUES ($1::uuid, $2::bigint)
	`, deviceID, inboxID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	delivery, err := store.Claim(ctx, "push-worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DisableDevice(ctx, delivery.ID, "push-worker", "push relay returned HTTP 410"); err != nil {
		t.Fatal(err)
	}
	var disabled bool
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT device.push_disabled_at IS NOT NULL, outgoing.status
		FROM cairnops_devices device
		JOIN cairnops_push_outbox outgoing ON outgoing.device_id = device.id
		WHERE device.id = $1::uuid
	`, deviceID).Scan(&disabled, &status); err != nil {
		t.Fatal(err)
	}
	if !disabled || status != "cancelled" {
		t.Fatalf("expired recipient was not disabled: disabled=%v status=%s", disabled, status)
	}
}
