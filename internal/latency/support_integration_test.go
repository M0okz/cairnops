package latency

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestDatabase(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	return context.Background(), testsupport.Pool(t)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// seedNativeSource crée une Cible et son Contrôle natif. La Source est plus
// ancienne que les Observations qu'on lui pose.
func seedNativeSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, kind string) (string, string) {
	t.Helper()
	var targetID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO cairnops_targets (name) VALUES ('Cible profilée') RETURNING id::text`,
	).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	var sourceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, origin, interval_seconds, timeout_milliseconds, config, created_at
		) VALUES ($1::uuid, 'Contrôle natif', $2, 'native', 60, 5000, '{}'::jsonb,
		          now() - interval '40 days')
		RETURNING id::text
	`, targetID, kind).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	return targetID, sourceID
}

// seedIntegrationSource rattache à la Cible une Source apportée par une
// Intégration, avec le Connecteur et la liaison que son origine exige.
func seedIntegrationSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID string) string {
	t.Helper()
	var connectorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status, encrypted_transport
		) VALUES ('zabbix', 'Zabbix de test', 'https://zabbix.example.net',
		          repeat('x', 64), 'connected', true)
		RETURNING id::text
	`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	var bindingID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, 'host-1', 'Hôte importé')
		RETURNING id::text
	`, connectorID, targetID).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	var sourceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, origin, connector_binding_id,
			interval_seconds, timeout_milliseconds, config, created_at
		) VALUES ($1::uuid, 'Source importée', 'zabbix', 'integration', $2::uuid,
		          60, 5000, '{}'::jsonb, now() - interval '40 days')
		RETURNING id::text
	`, targetID, bindingID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	return sourceID
}

// insertObservations pose des Observations d'entraînement au moins deux jours
// avant l'heure en cours. hoursAgo conserve le choix de l'heure UTC, tandis
// que la dernière journée reste réservée à l'évaluation.
//
// C'est PostgreSQL qui la calcule et la rend : un test ne doit pas déduire
// d'une horloge Go le seau qu'une horloge PostgreSQL a choisi.
func insertObservations(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	targetID, sourceID string, hoursAgo, count, latency int, outcome domain.Outcome,
) int {
	t.Helper()
	rows, err := pool.Query(ctx, `
		INSERT INTO cairnops_observations (source_id, target_id, observed_at, outcome, latency_milliseconds)
		SELECT $1::uuid, $2::uuid,
		       date_trunc('hour', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		         - make_interval(hours => $3 + 48) + make_interval(secs => step),
		       $4, $5
		FROM generate_series(0, $6 - 1) AS step
		RETURNING extract(hour FROM observed_at AT TIME ZONE 'UTC')::integer
	`, sourceID, targetID, hoursAgo, outcome, latency, count)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	hour := -1
	for rows.Next() {
		var observed int
		if err := rows.Scan(&observed); err != nil {
			t.Fatal(err)
		}
		hour = observed
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if hour < 0 {
		t.Fatalf("expected %d observations to be written", count)
	}
	return hour
}

// insertObservationsDaysAgo pose des Observations saines hors de la fenêtre du
// Profil.
func insertObservationsDaysAgo(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	targetID, sourceID string, daysAgo, count, latency int,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_observations (source_id, target_id, observed_at, outcome, latency_milliseconds)
		SELECT $1::uuid, $2::uuid,
		       date_trunc('hour', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		         - make_interval(days => $3) + make_interval(secs => step),
		       'healthy', $4
		FROM generate_series(0, $5 - 1) AS step
	`, sourceID, targetID, daysAgo, latency, count); err != nil {
		t.Fatal(err)
	}
}

func mustRebuild(t *testing.T, ctx context.Context, store *Store, sourceID string) {
	t.Helper()
	rebuilt, err := store.Rebuild(ctx, sourceID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !rebuilt {
		t.Fatal("expected the profile to be rebuilt")
	}
}

func mustProfile(t *testing.T, ctx context.Context, store *Store, sourceID string) domain.LatencyProfile {
	t.Helper()
	profile, err := store.Profile(ctx, sourceID)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
