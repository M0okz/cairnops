package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La consolidation horaire puis la lecture d'une fenêtre doivent rendre les
// mêmes conclusions que les Observations brutes, l'heure en cours comprise.
func TestRollupAndWindowsMeasureTheSameObservations(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	targetID, sourceID := seedTarget(t, ctx, pool, 20)

	// Deux heures révolues : la première parfaite, la seconde à moitié en
	// défaut et amputée de la moitié de ses Observations attendues.
	// Une heure porte 180 Observations attendues à 20 secondes d'intervalle.
	insertObservations(t, ctx, pool, targetID, sourceID, 2, 180, 0, 0, 100)
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 45, 45, 0, 200)
	// L'heure en cours reste hors consolidation : elle se lit sur les brutes.
	insertObservations(t, ctx, pool, targetID, sourceID, 0, 3, 0, 1, 300)

	consolidated, err := NewRollup(pool, slog.New(slog.DiscardHandler)).Consolidate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if consolidated < 2 {
		t.Fatalf("expected at least the two elapsed hours to be consolidated, got %d", consolidated)
	}

	store := NewStore(pool)
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	measured := findTarget(t, listed, targetID)

	if got := len(measured.Measures); got != 1 {
		t.Fatalf("a list carries the 24 hours window alone, got %d measures", got)
	}
	day := measured.Measures[0]
	if day.Window != domain.WindowDay {
		t.Fatalf("unexpected window %q", day.Window)
	}
	// 228 Observations saines sur 273 concluantes ; l'Inconnue ne conclut rien.
	if day.ConclusiveObservations != 273 || day.UnknownObservations != 1 || day.Availability == nil {
		t.Fatalf("unexpected counts: %#v", day)
	}
	if day.Availability == nil || fmt.Sprintf("%.4f", *day.Availability) != "0.8352" {
		t.Fatalf("unexpected availability: %v", day.Availability)
	}
	// 273 Observations concluantes pour 360 attendues sur les deux heures
	// révolues, plus la part écoulée de l'heure en cours.
	if day.Coverage == nil || *day.Coverage <= 0.5 || *day.Coverage > 1 {
		t.Fatalf("unexpected coverage: %v", day.Coverage)
	}
	if day.ExpectedObservations < 360 {
		t.Fatalf("the two elapsed hours should expect at least 360 observations, got %d", day.ExpectedObservations)
	}
	if day.AverageLatencyMilliseconds == nil || *day.AverageLatencyMilliseconds < 100 || *day.AverageLatencyMilliseconds > 300 {
		t.Fatalf("unexpected average latency: %v", day.AverageLatencyMilliseconds)
	}
	if day.MaximumLatencyMilliseconds == nil || *day.MaximumLatencyMilliseconds != 300 {
		t.Fatalf("expected the worst latency of 300 ms, got %v", day.MaximumLatencyMilliseconds)
	}
	// Trois heures ont conclu quelque chose : la tendance en porte trois points.
	if len(measured.Trend) != 3 {
		t.Fatalf("expected three hourly points, got %v", measured.Trend)
	}
	if measured.Trend[0] != 1 || measured.Trend[1] != 0.5 || measured.Trend[2] != 1 {
		t.Fatalf("unexpected trend: %v", measured.Trend)
	}
	// La latence horaire suit les mêmes heures, et ne retient que les
	// Observations saines : les trois heures ont été posées à 100, 200 et
	// 300 ms.
	if len(measured.LatencyTrend) != 3 {
		t.Fatalf("expected three hourly latencies, got %v", measured.LatencyTrend)
	}
	if measured.LatencyTrend[0] != 100 || measured.LatencyTrend[1] != 200 || measured.LatencyTrend[2] != 300 {
		t.Fatalf("unexpected latency trend: %v", measured.LatencyTrend)
	}

	detail, err := store.Target(ctx, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Measures) != 3 {
		t.Fatalf("the detail opens the three windows, got %d", len(detail.Measures))
	}
	if detail.Measures[0].Window != domain.WindowDay ||
		detail.Measures[1].Window != domain.WindowWeek ||
		detail.Measures[2].Window != domain.WindowMonth {
		t.Fatalf("unexpected windows: %#v", detail.Measures)
	}
	// Les mêmes Observations tiennent dans les trois fenêtres, et une Source
	// née il y a trois heures n'attendait rien avant d'exister : élargir la
	// fenêtre n'invente donc aucune attente déçue.
	for _, measure := range detail.Measures {
		if measure.ConclusiveObservations != 273 {
			t.Fatalf("window %q lost observations: %#v", measure.Window, measure)
		}
		if *measure.Coverage != *detail.Measures[0].Coverage {
			t.Fatalf("window %q invented an expectation: %#v", measure.Window, measure)
		}
	}
	if len(detail.Sources) != 1 || detail.Sources[0].SourceID != sourceID {
		t.Fatalf("unexpected sources: %#v", detail.Sources)
	}
	if len(detail.Sources[0].Measures) != 3 {
		t.Fatalf("each source opens the three windows, got %d", len(detail.Sources[0].Measures))
	}
	if *detail.Sources[0].Measures[0].Availability != *day.Availability {
		t.Fatal("the lone source of a target must measure exactly what the target measures")
	}
}

// Une consolidation rejouée écrit les mêmes heures sans les compter deux fois,
// et l'heure restée muette conserve ses Observations attendues.
func TestConsolidateIsIdempotentAndKeepsSilentHours(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	targetID, sourceID := seedTarget(t, ctx, pool, 60)
	// Une seule heure produit des Observations ; la suivante reste muette.
	insertObservations(t, ctx, pool, targetID, sourceID, 2, 30, 0, 0, 50)

	rollup := NewRollup(pool, slog.New(slog.DiscardHandler))
	if _, err := rollup.Consolidate(ctx); err != nil {
		t.Fatal(err)
	}
	// La deuxième passe n'a plus d'heure révolue à traiter.
	again, err := rollup.Consolidate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("expected nothing left to consolidate, got %d hours", again)
	}

	var rows, healthy int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer, coalesce(sum(healthy), 0)::integer
		FROM cairnops_observation_hours WHERE source_id = $1::uuid
	`, sourceID).Scan(&rows, &healthy); err != nil {
		t.Fatal(err)
	}
	if healthy != 30 {
		t.Fatalf("expected the thirty observations to be counted once, got %d", healthy)
	}
	// L'heure muette porte sa ligne elle aussi : sans elle, une interruption du
	// worker resterait invisible.
	if rows != 2 {
		t.Fatalf("expected a row per elapsed hour since the first observation, got %d", rows)
	}

	// 30 Observations concluantes pour 120 attendues sur deux heures : la
	// Couverture chute au lieu de rester silencieuse, tandis que la
	// Disponibilité reste entière — rien n'a jamais conclu à une défaillance.
	measured := findTarget(t, mustList(t, ctx, NewStore(pool)), targetID)
	if measured.Measures[0].Coverage == nil || *measured.Measures[0].Coverage > 0.4 {
		t.Fatalf("a silent hour must lower coverage, got %v", measured.Measures[0].Coverage)
	}
	if measured.Measures[0].Availability == nil || *measured.Measures[0].Availability != 1 {
		t.Fatalf("expected a full availability, got %v", measured.Measures[0].Availability)
	}
}

// Une Cible sans Source figure dans la liste, sans mesure inventée.
func TestListKeepsTargetsWithoutObservations(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	targetID := createTarget(t, ctx, pool)

	measured := findTarget(t, mustList(t, ctx, NewStore(pool)), targetID)
	if measured.Measures[0].Availability != nil || measured.Measures[0].Coverage != nil {
		t.Fatalf("nothing observed can establish nothing: %#v", measured.Measures[0])
	}
	if len(measured.Trend) != 0 {
		t.Fatalf("expected an empty trend, got %v", measured.Trend)
	}
}

func TestTargetRejectsUnknownIdentifier(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	if _, err := NewStore(pool).Target(ctx, "00000000-0000-4000-8000-000000000000"); err == nil {
		t.Fatal("an unknown target must not measure anything")
	}
}

/* ── Fixtures ─────────────────────────────────────────────────────────────── */

func openTestDatabase(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	return context.Background(), testsupport.Pool(t)
}

func createTarget(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var targetID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO cairnops_targets (name) VALUES ('Cible mesurée') RETURNING id::text`,
	).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	return targetID
}

// seedTarget crée une Cible et une Source née trois heures plus tôt, afin que
// les heures révolues attendent réellement des Observations.
func seedTarget(t *testing.T, ctx context.Context, pool *pgxpool.Pool, intervalSeconds int) (string, string) {
	t.Helper()
	targetID := createTarget(t, ctx, pool)
	var sourceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, interval_seconds, timeout_milliseconds, config, created_at
		) VALUES ($1::uuid, 'Endpoint public', 'http', $2, 5000, '{"url":"https://example.net"}'::jsonb,
		          now() - interval '3 hours')
		RETURNING id::text
	`, targetID, intervalSeconds).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	return targetID, sourceID
}

// insertObservations pose des Observations dans l'heure indiquée, comptée en
// heures révolues avant l'heure en cours.
func insertObservations(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID, sourceID string, hoursAgo, healthy, unhealthy, unknown, latency int) {
	t.Helper()
	insert := func(outcome domain.Outcome, count int) {
		for index := range count {
			if _, err := pool.Exec(ctx, `
				INSERT INTO cairnops_observations (source_id, target_id, observed_at, outcome, latency_milliseconds)
				VALUES ($1::uuid, $2::uuid,
				        date_trunc('hour', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
				          - make_interval(hours => $3) + make_interval(secs => $4),
				        $5, $6)
			`, sourceID, targetID, hoursAgo, index, outcome, latency); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert(domain.OutcomeHealthy, healthy)
	insert(domain.OutcomeUnhealthy, unhealthy)
	insert(domain.OutcomeUnknown, unknown)
}

func mustList(t *testing.T, ctx context.Context, store *Store) []TargetMetrics {
	t.Helper()
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return listed
}

func findTarget(t *testing.T, listed []TargetMetrics, targetID string) TargetMetrics {
	t.Helper()
	for _, measured := range listed {
		if measured.TargetID == targetID {
			return measured
		}
	}
	t.Fatalf("target %s is missing from the measured list", targetID)
	return TargetMetrics{}
}

// La série horaire de l'instance couvre les mêmes heures que les mesures, et
// somme toutes les Cibles : c'est elle que la page Santé met en micro-graphes.
func TestInstanceHoursSummariseTheWholeInstance(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	targetID, sourceID := seedTarget(t, ctx, pool, 20)

	insertObservations(t, ctx, pool, targetID, sourceID, 2, 180, 0, 0, 100)
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 45, 45, 0, 200)
	insertObservations(t, ctx, pool, targetID, sourceID, 0, 3, 0, 1, 300)

	if _, err := NewRollup(pool, slog.New(slog.DiscardHandler)).Consolidate(ctx); err != nil {
		t.Fatal(err)
	}

	hours, err := NewStore(pool).InstanceHours(ctx, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(hours) != 3 {
		t.Fatalf("expected the three hours that saw observations, got %d", len(hours))
	}
	if hours[0].Hour.After(hours[2].Hour) {
		t.Fatalf("expected the oldest hour first, got %v", hours)
	}
	// L'heure du milieu : 45 saines et 45 en défaut, toutes concluantes.
	middle := hours[1]
	if middle.HealthyObservations != 45 || middle.ConclusiveObservations != 90 {
		t.Fatalf("unexpected middle hour: %#v", middle)
	}
	if middle.AverageLatencyMilliseconds == nil || *middle.AverageLatencyMilliseconds != 200 {
		t.Fatalf("unexpected middle latency: %v", middle.AverageLatencyMilliseconds)
	}
	// L'Inconnue de l'heure en cours ne conclut rien.
	if last := hours[2]; last.ConclusiveObservations != 3 || last.HealthyObservations != 3 {
		t.Fatalf("unexpected current hour: %#v", last)
	}
	if hours[0].ExpectedObservations < 180 {
		t.Fatalf("an elapsed hour expects 180 observations at 20 seconds, got %d", hours[0].ExpectedObservations)
	}
}
