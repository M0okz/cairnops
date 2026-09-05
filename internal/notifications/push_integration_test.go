package notifications_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/push"
	"github.com/M0okz/cairnops/internal/testsupport"
	"golang.org/x/crypto/curve25519"
)

func TestIntegratedDeliverySchedulesOnePushPerActiveDevice(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID, channelID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('push-user', 'Push User', 'not-used', 'operator') RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	_, incidentID := seedActiveIncident(t, pool, "critical")
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
		IncidentID: incidentID, IncidentRevision: 1, ChannelID: channelID, ChannelKind: notifications.KindInApp,
		EventKind: "firing", TargetName: "Push target", NatureLabel: "Indisponibilité",
		Severity: incidents.SeverityCritical, ImpactCount: 1, AffectedTargets: 1,
		MaxAffected: 1, PropagationStatus: "open", OpenedAt: time.Now().UTC(),
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

func TestSilentRevisionsPreserveAnUndeliveredPushAndItsBackoff(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	user := seedAccount(t, pool, "operator")
	_, id := seedActiveIncident(t, pool, "major")
	if _, err := pool.Exec(ctx, `INSERT INTO cairnops_devices (user_id,name,platform,encryption_public_key,push_recipient_sealed,token_digest)
		VALUES ($1::uuid,'Test phone','ios',$2,'sealed-recipient-with-sufficient-length',$3)`, user, curve25519.Basepoint, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	store := immediateNotificationStore(pool)
	pushes := push.NewPostgresStore(pool)
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "firing", "alert")
	first, err := pushes.Claim(ctx, "push-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := pushes.Fail(ctx, first.ID, "push-test", "relay unavailable"); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := pool.QueryRow(ctx, `SELECT next_attempt_at FROM cairnops_push_outbox WHERE id=$1`, first.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET revision=revision+1 WHERE id=$1::uuid`, id); err != nil {
			t.Fatal(err)
		}
		if err := store.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
		claimAndDeliver(t, ctx, store, "incident_update", "silent")
		var presentation string
		var after time.Time
		var attempts int
		if err := pool.QueryRow(ctx, `SELECT presentation,next_attempt_at,attempts FROM cairnops_push_outbox WHERE status IN ('pending','failed')`).Scan(&presentation, &after, &attempts); err != nil {
			t.Fatal(err)
		}
		if presentation != "alert" || !after.Equal(before) || attempts != 1 {
			t.Fatalf("lost push intent or backoff: %s %s %d", presentation, after, attempts)
		}
	}
}

func TestInFlightPushFinishesBeforeItsSilentReplacement(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	user := seedAccount(t, pool, "operator")
	seedActiveIncident(t, pool, "major")
	if _, err := pool.Exec(ctx, `INSERT INTO cairnops_devices (user_id,name,platform,encryption_public_key,push_recipient_sealed,token_digest)
		VALUES ($1::uuid,'Test phone','ios',$2,'sealed-recipient-with-sufficient-length',$3)`, user, curve25519.Basepoint, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	store := immediateNotificationStore(pool)
	pushes := push.NewPostgresStore(pool)
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	opening := claimAndDeliver(t, ctx, store, "firing", "alert")
	first, err := pushes.Claim(ctx, "push-test")
	if err != nil {
		t.Fatal(err)
	}
	update := opening
	update.IncidentRevision++
	update.EventKind = "incident_update"
	update.Presentation = "silent"
	if _, err := store.Deliver(ctx, update); err == nil {
		t.Fatal("replaced a push still being sent")
	}
	if err := pushes.Complete(ctx, first.ID, "push-test"); err != nil {
		t.Fatalf("in-flight lease was destroyed: %v", err)
	}
	if _, err := store.Deliver(ctx, update); err != nil {
		t.Fatal(err)
	}
	next, err := pushes.Claim(ctx, "push-test")
	if err != nil {
		t.Fatal(err)
	}
	if next.PresentationMode != "silent" {
		t.Fatalf("already delivered alert was repeated: %#v", next)
	}
}

func TestIntegratedResolutionUpdatesStateWithoutAlertingAgain(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('quiet-resolution-user', 'Quiet Resolution User', 'not-used', 'operator') RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	_, incidentID := seedActiveIncident(t, pool, "major")
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
	resolveSeedIncident(t, pool, incidentID)
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	resolution, err := store.Claim(ctx, "quiet-resolution-worker")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.EventKind != "incident_update" || resolution.Presentation != "silent" {
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
		ORDER BY outgoing.revision DESC LIMIT 1
	`, incidentID).Scan(&presentation); err != nil {
		t.Fatal(err)
	}
	if presentation != "silent" {
		t.Fatalf("resolution scheduled a visible device push: %s", presentation)
	}
}
