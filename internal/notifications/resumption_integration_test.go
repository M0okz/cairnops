package notifications_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/push"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/curve25519"
)

// resumeSeedIncident reproduit l'effet d'une Reprise (ADR 0054) sur un
// Incident résolu : son unique Atteinte redevient active.
func resumeSeedIncident(t *testing.T, pool *pgxpool.Pool, incidentID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incident_impacts
		SET status = 'active', resolved_at = NULL, updated_at = now()
		WHERE incident_id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET status = 'active', resolved_at = NULL, active_impact_count = 1,
		    affected_target_count = 1, resumption_count = resumption_count + 1,
		    resumed_at = now(), revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
}

// drainDeliveries livre tout ce qui est dû et le rend par type d'événement.
func drainDeliveries(t *testing.T, ctx context.Context, store *notifications.PostgresStore) []notifications.Delivery {
	t.Helper()
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	var deliveries []notifications.Delivery
	for {
		delivery, err := store.Claim(ctx, "resumption-worker")
		if errors.Is(err, notifications.ErrNoDelivery) {
			return deliveries
		}
		if err != nil {
			t.Fatal(err)
		}
		if delivery.ChannelKind == notifications.KindInApp {
			if _, err := store.Deliver(ctx, delivery); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.Complete(ctx, delivery.ID, "resumption-worker"); err != nil {
			t.Fatal(err)
		}
		deliveries = append(deliveries, delivery)
	}
}

func drainPushes(t *testing.T, ctx context.Context, pushes *push.PostgresStore) []push.Delivery {
	t.Helper()
	var deliveries []push.Delivery
	for {
		delivery, err := pushes.Claim(ctx, "resumption-push")
		if errors.Is(err, push.ErrNoDelivery) {
			return deliveries
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := pushes.Complete(ctx, delivery.ID, "resumption-push"); err != nil {
			t.Fatal(err)
		}
		deliveries = append(deliveries, delivery)
	}
}

func describe(deliveries []notifications.Delivery) []string {
	described := make([]string, 0, len(deliveries))
	for _, delivery := range deliveries {
		described = append(described, delivery.EventKind+"/"+delivery.Presentation)
	}
	return described
}

func alerts(deliveries []notifications.Delivery) []notifications.Delivery {
	var visible []notifications.Delivery
	for _, delivery := range deliveries {
		if delivery.Presentation == "alert" {
			visible = append(visible, delivery)
		}
	}
	return visible
}

func pushAlerts(deliveries []push.Delivery) []string {
	var visible []string
	for _, delivery := range deliveries {
		if delivery.PresentationMode == "alert" {
			visible = append(visible, delivery.EventKind)
		}
	}
	return visible
}

func seedIOSDevice(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO cairnops_devices (user_id, name, platform, encryption_public_key, push_recipient_sealed, token_digest)
		VALUES ($1::uuid, 'iPhone', 'ios', $2, 'sealed-recipient-with-sufficient-length', $3)
	`, userID, curve25519.Basepoint, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyTheFirstResumptionAlertsAndEachVisibleCycleEndsOnce(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	user := seedAccount(t, pool, "operator")
	seedIOSDevice(t, pool, user)
	_, incidentID := seedActiveIncident(t, pool, "major")
	store := immediateNotificationStore(pool)
	pushes := push.NewPostgresStore(pool)

	// Ouverture puis Résolution : le cycle habituel.
	if visible := alerts(drainDeliveries(t, ctx, store)); len(visible) != 1 || visible[0].EventKind != "firing" {
		t.Fatalf("the opening must alert once, got %v", describe(visible))
	}
	if got := pushAlerts(drainPushes(t, ctx, pushes)); fmt.Sprint(got) != "[firing]" {
		t.Fatalf("the device must display the opening, got %v", got)
	}
	resolveSeedIncident(t, pool, incidentID)
	drainDeliveries(t, ctx, store)
	if got := pushAlerts(drainPushes(t, ctx, pushes)); fmt.Sprint(got) != "[resolved]" {
		t.Fatalf("the displayed opening must be replaced by its resolution, got %v", got)
	}

	// Première Reprise : un Fait opérationnel, annoncé comme tel.
	resumeSeedIncident(t, pool, incidentID)
	visible := alerts(drainDeliveries(t, ctx, store))
	if len(visible) != 1 || visible[0].EventKind != "firing" || visible[0].Context.Resumptions != 1 {
		t.Fatalf("the first resumption must alert once and say so, got %#v", visible)
	}
	if got := pushAlerts(drainPushes(t, ctx, pushes)); fmt.Sprint(got) != "[firing]" {
		t.Fatalf("the device must display the first resumption, got %v", got)
	}
	inbox, err := store.Inbox(ctx, user, notifications.InboxLimit)
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Unread != 1 || len(inbox.Entries) != 1 || inbox.Entries[0].EventKind != "firing" {
		t.Fatalf("the resumed entry must be unread again, got %#v", inbox)
	}

	resolveSeedIncident(t, pool, incidentID)
	drainDeliveries(t, ctx, store)
	if got := pushAlerts(drainPushes(t, ctx, pushes)); fmt.Sprint(got) != "[resolved]" {
		t.Fatalf("the displayed resumption must be replaced by its resolution, got %v", got)
	}

	// Reprises suivantes : l'état suit, sans interrompre ni annoncer de fin.
	for cycle := 2; cycle <= 3; cycle++ {
		resumeSeedIncident(t, pool, incidentID)
		if visible := alerts(drainDeliveries(t, ctx, store)); len(visible) != 0 {
			t.Fatalf("resumption %d must not alert again, got %v", cycle, describe(visible))
		}
		resolveSeedIncident(t, pool, incidentID)
		drainDeliveries(t, ctx, store)
		if got := pushAlerts(drainPushes(t, ctx, pushes)); len(got) != 0 {
			t.Fatalf("cycle %d must stay silent on the device, got %v", cycle, got)
		}
	}
}

func TestResumptionWaitsForStabilityAndIsDroppedIfResolvedMeanwhile(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	seedAccount(t, pool, "operator")
	_, incidentID := seedActiveIncident(t, pool, "major")
	immediate := immediateNotificationStore(pool)
	drainDeliveries(t, ctx, immediate)
	resolveSeedIncident(t, pool, incidentID)
	drainDeliveries(t, ctx, immediate)

	held := notifications.NewPostgresStoreWithStabilityDelay(pool, 2*time.Minute)
	resumeSeedIncident(t, pool, incidentID)
	if visible := alerts(drainDeliveries(t, ctx, held)); len(visible) != 0 {
		t.Fatalf("a resumption must wait for stability before alerting, got %v", describe(visible))
	}
	resolveSeedIncident(t, pool, incidentID)
	drainDeliveries(t, ctx, held)

	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM cairnops_notification_outbox
		WHERE incident_id = $1::uuid AND event_key = 'resumed'
	`, incidentID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("an unstable resumption must be dropped, got %s", status)
	}
}

func TestAcknowledgedIncidentResumesWithoutAlerting(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	seedAccount(t, pool, "operator")
	_, incidentID := seedActiveIncident(t, pool, "major")
	store := immediateNotificationStore(pool)
	drainDeliveries(t, ctx, store)
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET acknowledged_at = now(), revision = revision + 1 WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	resolveSeedIncident(t, pool, incidentID)
	drainDeliveries(t, ctx, store)

	resumeSeedIncident(t, pool, incidentID)
	if visible := alerts(drainDeliveries(t, ctx, store)); len(visible) != 0 {
		t.Fatalf("an acknowledged Incident must resume silently, got %v", describe(visible))
	}
}

func TestMattermostSummarizesEachVisibleCycleOnce(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	actorID := seedAccount(t, pool, "administrator")
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store := immediateNotificationStore(pool)
	if _, err := notifications.NewService(store, acceptingMattermost{}, box).CreateMattermost(ctx, actorID, notifications.CreateMattermostInput{
		Name: "Exploitation", WebhookURL: "https://mattermost.example.test/hooks/secret",
		Severities: []incidents.Severity{incidents.SeverityMajor},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_notification_channels SET enabled = false WHERE kind = 'in_app'`); err != nil {
		t.Fatal(err)
	}
	_, incidentID := seedActiveIncident(t, pool, "major")

	cycle := func(label string, want string) {
		t.Helper()
		got := describe(drainDeliveries(t, ctx, store))
		resolveSeedIncident(t, pool, incidentID)
		got = append(got, describe(drainDeliveries(t, ctx, store))...)
		if fmt.Sprint(got) != want {
			t.Fatalf("%s: got %v, want %s", label, got, want)
		}
		resumeSeedIncident(t, pool, incidentID)
	}
	cycle("opening", "[firing/alert resolved/alert]")
	cycle("first resumption", "[firing/alert resolved/alert]")
	cycle("second resumption", "[]")
}

func TestHeldResumptionStillAlertsTheDeviceAfterItsSilentRevision(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	user := seedAccount(t, pool, "operator")
	seedIOSDevice(t, pool, user)
	_, incidentID := seedActiveIncident(t, pool, "major")
	immediate := immediateNotificationStore(pool)
	pushes := push.NewPostgresStore(pool)
	drainDeliveries(t, ctx, immediate)
	resolveSeedIncident(t, pool, incidentID)
	drainDeliveries(t, ctx, immediate)
	drainPushes(t, ctx, pushes)

	// La révision silencieuse part tout de suite ; l'alerte attend son sas.
	held := notifications.NewPostgresStoreWithStabilityDelay(pool, 2*time.Minute)
	resumeSeedIncident(t, pool, incidentID)
	drainDeliveries(t, ctx, held)
	if got := pushAlerts(drainPushes(t, ctx, pushes)); len(got) != 0 {
		t.Fatalf("nothing may alert before the stability delay, got %v", got)
	}
	// Trois minutes plus tard, la Reprise est toujours là.
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET resumed_at = resumed_at - interval '3 minutes'
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if visible := alerts(drainDeliveries(t, ctx, held)); len(visible) != 1 {
		t.Fatalf("the stable resumption must alert, got %v", describe(visible))
	}
	if got := pushAlerts(drainPushes(t, ctx, pushes)); fmt.Sprint(got) != "[firing]" {
		t.Fatalf("the device must display the stable resumption, got %v", got)
	}
}
