package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/realtime"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/curve25519"
)

// testPurger supprime par lots de deux : chaque scénario traverse ainsi
// plusieurs transactions, comme une purge réelle sur une grosse table.
func testPurger(pool *pgxpool.Pool) *Purger {
	purger := NewPurger(pool, DefaultPolicy(), slog.New(slog.DiscardHandler))
	purger.batchSize = 2
	return purger
}

// Une Observation ancienne ne s'efface qu'une fois son heure consolidée, et
// la dernière Observation d'une Source survit toujours : une Source muette
// depuis des semaines garde son dernier verdict.
func TestPurgeKeepsUnconsolidatedAndLatestObservations(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	purger := testPurger(pool)
	now := time.Now().UTC()
	targetID := seedTarget(t, pool)
	active := seedSource(t, pool, targetID, "Endpoint actif")
	silent := seedSource(t, pool, targetID, "Endpoint muet")

	for _, age := range []time.Duration{40 * day, 39 * day, 38 * day, 36 * day, 33 * day, time.Hour} {
		insertObservation(t, pool, active, targetID, now.Add(-age))
	}
	insertObservation(t, pool, silent, targetID, now.Add(-45*day))
	insertObservation(t, pool, silent, targetID, now.Add(-40*day))

	// Sans consolidation, rien n'est effacé : l'agrégat n'existe pas encore.
	report, err := purger.Purge(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Observations != 0 || countObservations(t, pool, active) != 6 {
		t.Fatalf("observations were deleted before any consolidation: %#v", report)
	}

	// La consolidation est en retard : seules les heures qu'elle a conclues
	// perdent leurs Observations brutes, même au-delà de 30 jours.
	setConsolidatedThrough(t, pool, now.Add(-35*day))
	report, err = purger.Purge(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := observationAges(t, pool, active, now); fmt.Sprint(got) != "[33 0]" {
		t.Fatalf("unexpected surviving observations of the active source (days): %v", got)
	}
	if got := observationAges(t, pool, silent, now); fmt.Sprint(got) != "[40]" {
		t.Fatalf("the latest observation of a silent source must survive, got %v", got)
	}
	if report.Observations != 5 {
		t.Fatalf("expected five observations to expire across several batches, got %#v", report)
	}

	// Une fois l'heure consolidée, la rétention de 30 jours s'applique.
	setConsolidatedThrough(t, pool, now.Truncate(time.Hour))
	if _, err := purger.Purge(ctx, now); err != nil {
		t.Fatal(err)
	}
	if got := observationAges(t, pool, active, now); fmt.Sprint(got) != "[0]" {
		t.Fatalf("observations older than 30 days must expire once consolidated, got %v", got)
	}
}

func TestPurgeExpiresHoursAfterThirteenMonths(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC()
	targetID := seedTarget(t, pool)
	sourceID := seedSource(t, pool, targetID, "Endpoint historique")
	hours := []time.Time{
		now.AddDate(0, -14, 0), now.AddDate(0, -13, -1), now.AddDate(0, -13, -2),
		now.AddDate(0, -12, 0), now.AddDate(0, 0, -1),
	}
	for _, hour := range hours {
		if _, err := pool.Exec(ctx, `
			INSERT INTO cairnops_observation_hours (source_id, target_id, hour, healthy, expected)
			VALUES ($1::uuid, $2::uuid, date_trunc('hour', $3::timestamptz), 120, 120)
		`, sourceID, targetID, hour); err != nil {
			t.Fatal(err)
		}
	}

	report, err := testPurger(pool).Purge(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_observation_hours WHERE source_id = $1::uuid`, sourceID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if report.ObservationHours != 3 || remaining != 2 {
		t.Fatalf("expected the three hours older than 13 months to expire, report %#v, remaining %d", report, remaining)
	}
}

// Les événements au-delà de la fenêtre de reprise s'effacent, sauf le plus
// récent : il porte la version courante, même sur une instance restée calme.
func TestPurgeExpiresEventsButKeepsTheCurrentVersion(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	events := realtime.NewStore(pool)
	for range 5 {
		if _, err := pool.Exec(ctx, `SELECT cairnops_append_event('component.heartbeat', 'component', 'worker')`); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := events.LatestVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_events SET occurred_at = now() - interval '2 days'`); err != nil {
		t.Fatal(err)
	}

	report, err := testPurger(pool).Purge(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Events == 0 {
		t.Fatalf("expected old events to expire: %#v", report)
	}
	oldest, err := events.OldestVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stillLatest, err := events.LatestVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if oldest != latest || stillLatest != latest {
		t.Fatalf("only the current version must survive: oldest %d, latest %d, want %d", oldest, stillLatest, latest)
	}

	// Un événement récent reste dans la fenêtre de reprise.
	if _, err := pool.Exec(ctx, `SELECT cairnops_append_event('component.heartbeat', 'component', 'worker')`); err != nil {
		t.Fatal(err)
	}
	report, err = testPurger(pool).Purge(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Events != 1 {
		t.Fatalf("expected only the previous, now superseded, old event to expire: %#v", report)
	}
}

// Les livraisons d'un Incident résolu depuis longtemps s'effacent ; celles
// d'un Incident encore actif, ou résolu récemment, restent sa mémoire de
// notification. Après la purge, aucune ouverture ni Résolution n'est renvoyée.
func TestPurgeExpiresDeliveriesOfLongResolvedIncidentsOnly(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStoreWithStabilityDelay(pool, 0)
	userID := seedAccount(t, pool)
	seedIOSDevice(t, pool, userID)

	longResolved := seedActiveIncident(t, pool)
	recentlyResolved := seedActiveIncident(t, pool)
	stillActive := seedActiveIncident(t, pool)
	if got := len(drainDeliveries(t, ctx, store)); got != 3 {
		t.Fatalf("expected three openings, got %d", got)
	}
	resolveIncident(t, pool, longResolved)
	resolveIncident(t, pool, recentlyResolved)
	if got := len(drainDeliveries(t, ctx, store)); got != 2 {
		t.Fatalf("expected two resolutions, got %d", got)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_push_outbox SET status = 'delivered', delivered_at = now()`); err != nil {
		t.Fatal(err)
	}

	// Toutes les livraisons ont 40 jours ; seul le premier Incident est
	// résolu depuis aussi longtemps.
	for _, statement := range []string{
		`UPDATE cairnops_notification_outbox SET updated_at = now() - interval '40 days'`,
		`UPDATE cairnops_push_outbox SET updated_at = now() - interval '40 days'`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET resolved_at = now() - interval '40 days' WHERE id = $1::uuid
	`, longResolved); err != nil {
		t.Fatal(err)
	}
	before := map[string][2]int{}
	for _, incidentID := range []string{longResolved, recentlyResolved, stillActive} {
		before[incidentID] = countDeliveries(t, pool, incidentID)
	}

	report, err := testPurger(pool).Purge(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := countDeliveries(t, pool, longResolved); got != [2]int{0, 0} {
		t.Fatalf("deliveries of a long resolved incident must expire, %v remain", got)
	}
	for _, incidentID := range []string{recentlyResolved, stillActive} {
		if got := countDeliveries(t, pool, incidentID); got != before[incidentID] || got[0] == 0 || got[1] == 0 {
			t.Fatalf("deliveries of incident %s must remain: before %v, after %v", incidentID, before[incidentID], got)
		}
	}
	if report.NotificationDeliveries != int64(before[longResolved][0]) || report.PushDeliveries != int64(before[longResolved][1]) {
		t.Fatalf("unexpected report %#v for %v", report, before[longResolved])
	}

	if again := drainDeliveries(t, ctx, store); len(again) != 0 {
		t.Fatalf("purging deliveries must not send anything again, got %#v", again)
	}
}

func seedTarget(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var targetID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`,
		fmt.Sprintf("Cible conservée %d", time.Now().UnixNano()),
	).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	return targetID
}

func seedSource(t *testing.T, pool *pgxpool.Pool, targetID, name string) string {
	t.Helper()
	var sourceID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, interval_seconds, timeout_milliseconds, config, created_at
		) VALUES ($1::uuid, $2, 'http', 30, 5000, '{"url":"https://example.net"}'::jsonb, now() - interval '60 days')
		RETURNING id::text
	`, targetID, name).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	return sourceID
}

func insertObservation(t *testing.T, pool *pgxpool.Pool, sourceID, targetID string, observedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO cairnops_observations (source_id, target_id, observed_at, outcome, latency_milliseconds)
		VALUES ($1::uuid, $2::uuid, $3, 'healthy', 40)
	`, sourceID, targetID, observedAt); err != nil {
		t.Fatal(err)
	}
}

func setConsolidatedThrough(t *testing.T, pool *pgxpool.Pool, through time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO cairnops_observation_rollup_state (id, consolidated_through) VALUES (true, $1)
		ON CONFLICT (id) DO UPDATE SET consolidated_through = excluded.consolidated_through
	`, through); err != nil {
		t.Fatal(err)
	}
}

func countObservations(t *testing.T, pool *pgxpool.Pool, sourceID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM cairnops_observations WHERE source_id = $1::uuid`, sourceID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// observationAges rend l'âge en jours entiers des Observations d'une Source,
// de la plus ancienne à la plus récente.
func observationAges(t *testing.T, pool *pgxpool.Pool, sourceID string, now time.Time) []int {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT observed_at FROM cairnops_observations WHERE source_id = $1::uuid ORDER BY observed_at`, sourceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ages []int
	for rows.Next() {
		var observedAt time.Time
		if err := rows.Scan(&observedAt); err != nil {
			t.Fatal(err)
		}
		ages = append(ages, int(now.Sub(observedAt)/day))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ages
}

func seedAccount(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('operateur-retention', 'Compte de test', 'not-used', 'operator') RETURNING id::text
	`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
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

func seedActiveIncident(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	targetID := seedTarget(t, pool)
	var incidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			nature_key, nature_label, nature_scope, nature_namespace,
			nature_fingerprint, propagation_eligible, status,
			propagation_status, severity, opened_at, last_impact_at,
			propagation_window_seconds, propagation_ends_at,
			active_impact_count, impact_count, affected_target_count,
			max_affected_targets
		) VALUES (
			'native:http', 'Indisponibilité', 'canonical', 'cairnops',
			'native:http', true, 'active', 'open', 'major', now(), now(),
			60, now() + interval '1 minute', 1, 1, 1, 1
		)
		RETURNING id::text
	`).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incident_impacts (
			incident_id, target_id, status, source_severity, effective_severity, opened_at
		) VALUES ($1::uuid, $2::uuid, 'active', 'major', 'major', now())
	`, incidentID, targetID); err != nil {
		t.Fatal(err)
	}
	return incidentID
}

func resolveIncident(t *testing.T, pool *pgxpool.Pool, incidentID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incident_impacts SET status = 'resolved', resolved_at = now(), updated_at = now()
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
}

func drainDeliveries(t *testing.T, ctx context.Context, store *notifications.PostgresStore) []notifications.Delivery {
	t.Helper()
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	var deliveries []notifications.Delivery
	for {
		delivery, err := store.Claim(ctx, "retention-worker")
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
		if err := store.Complete(ctx, delivery.ID, "retention-worker"); err != nil {
			t.Fatal(err)
		}
		deliveries = append(deliveries, delivery)
	}
}

// countDeliveries compte les livraisons d'un Incident : boîte d'envoi des
// Canaux, puis envois Push.
func countDeliveries(t *testing.T, pool *pgxpool.Pool, incidentID string) [2]int {
	t.Helper()
	var counts [2]int
	if err := pool.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM cairnops_notification_outbox WHERE incident_id = $1::uuid),
		       (SELECT count(*) FROM cairnops_push_outbox outgoing
		        JOIN cairnops_notification_inbox inbox ON inbox.id = outgoing.inbox_id
		        WHERE inbox.incident_id = $1::uuid)
	`, incidentID).Scan(&counts[0], &counts[1]); err != nil {
		t.Fatal(err)
	}
	return counts
}
