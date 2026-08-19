package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// Une passe consolide au plus deux jours pour ne jamais bloquer le worker
	// sur un rattrapage ; la passe suivante reprend où celle-ci s'est arrêtée.
	maximumHoursPerPass = 48

	// Au premier démarrage, la consolidation ne remonte pas au-delà de la plus
	// longue fenêtre affichée : rien de plus ancien n'est mesuré.
	maximumBacklog = 30 * 24 * time.Hour
)

// Rollup consolide les Observations en agrégats horaires. Il ne touche jamais
// à l'heure en cours : celle-ci se lit sur les Observations brutes, de sorte
// qu'une consolidation en retard ne rend aucune mesure fausse.
type Rollup struct {
	pool     *pgxpool.Pool
	logger   *slog.Logger
	interval time.Duration
}

func NewRollup(pool *pgxpool.Pool, logger *slog.Logger) *Rollup {
	return &Rollup{pool: pool, logger: logger, interval: 5 * time.Minute}
}

// Run consolide au démarrage puis à intervalle régulier. Une passe en échec
// n'arrête pas la boucle : la suivante reprendra les mêmes heures, l'écriture
// étant idempotente.
func (rollup *Rollup) Run(ctx context.Context) error {
	if _, err := rollup.Consolidate(ctx); err != nil {
		rollup.logger.Error("observation rollup failed", "error", err)
	}
	ticker := time.NewTicker(rollup.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			consolidated, err := rollup.Consolidate(ctx)
			if err != nil {
				rollup.logger.Error("observation rollup failed", "error", err)
				continue
			}
			if consolidated > 0 {
				rollup.logger.Info("observations consolidated", "hours", consolidated)
			}
		}
	}
}

// Consolidate écrit les heures révolues qui ne l'étaient pas encore et rend
// leur nombre.
func (rollup *Rollup) Consolidate(ctx context.Context) (int, error) {
	tx, err := rollup.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin rollup transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Un seul worker consolide à la fois ; les autres passent leur tour plutôt
	// que d'attendre, la passe suivante étant à quelques minutes.
	var acquired bool
	if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtext('cairnops-observation-rollup'))").Scan(&acquired); err != nil {
		return 0, fmt.Errorf("lock rollup: %w", err)
	}
	if !acquired {
		return 0, nil
	}

	var boundary time.Time
	if err := tx.QueryRow(ctx, `SELECT date_trunc('hour', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`).Scan(&boundary); err != nil {
		return 0, fmt.Errorf("read rollup boundary: %w", err)
	}

	from, err := rollup.startingHour(ctx, tx, boundary)
	if err != nil {
		return 0, err
	}
	if !from.Before(boundary) {
		return 0, tx.Commit(ctx)
	}
	hours := int(boundary.Sub(from) / time.Hour)
	if hours > maximumHoursPerPass {
		hours = maximumHoursPerPass
		boundary = from.Add(maximumHoursPerPass * time.Hour)
	}

	if _, err := tx.Exec(ctx, consolidateStatement, from, boundary); err != nil {
		return 0, fmt.Errorf("consolidate observation hours: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_observation_rollup_state (id, consolidated_through)
		VALUES (true, $1)
		ON CONFLICT (id) DO UPDATE SET consolidated_through = excluded.consolidated_through, updated_at = now()
	`, boundary); err != nil {
		return 0, fmt.Errorf("record rollup progress: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit rollup: %w", err)
	}
	return hours, nil
}

// startingHour reprend la consolidation là où elle s'est arrêtée. Au premier
// démarrage, elle commence à la première Observation connue, sans jamais
// remonter au-delà de la plus longue fenêtre affichée.
func (rollup *Rollup) startingHour(ctx context.Context, tx pgx.Tx, boundary time.Time) (time.Time, error) {
	var consolidatedThrough time.Time
	err := tx.QueryRow(ctx, `SELECT consolidated_through FROM cairnops_observation_rollup_state WHERE id`).Scan(&consolidatedThrough)
	if err == nil {
		return latest(consolidatedThrough, boundary.Add(-maximumBacklog)), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, fmt.Errorf("read rollup progress: %w", err)
	}

	var earliest *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT date_trunc('hour', min(observed_at) AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		FROM cairnops_observations
	`).Scan(&earliest); err != nil {
		return time.Time{}, fmt.Errorf("read earliest observation: %w", err)
	}
	if earliest == nil {
		return boundary, nil
	}
	return latest(*earliest, boundary.Add(-maximumBacklog)), nil
}

func latest(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}

// consolidateStatement écrit une ligne par Source active et par heure révolue,
// y compris lorsque l'heure n'a produit aucune Observation : sans cette ligne,
// une interruption du worker resterait invisible à la Couverture.
//
// L'écriture est idempotente, ce qui permet de rejouer une passe interrompue.
const consolidateStatement = `
INSERT INTO cairnops_observation_hours (
	source_id, target_id, hour, healthy, unhealthy, unknown, expected,
	latency_sum_milliseconds, latency_count, latency_maximum_milliseconds, consolidated_at
)
SELECT source.id, source.target_id, slot.hour,
       counted.healthy, counted.unhealthy, counted.unknown,
       -- Un Signal entrant poussé par un webhook ne promet aucune cadence :
       -- rien n'y est attendu, donc rien n'y manque.
       CASE WHEN source.kind = 'generic_webhook' THEN 0 ELSE greatest(0, floor(
           extract(epoch FROM (slot.hour + interval '1 hour' - greatest(slot.hour, source.created_at)))
           / source.interval_seconds
       ))::integer END,
       counted.latency_sum, counted.latency_count, counted.latency_maximum, now()
FROM cairnops_signal_sources source
CROSS JOIN generate_series($1::timestamptz, $2::timestamptz - interval '1 hour', interval '1 hour') AS slot(hour)
LEFT JOIN LATERAL (
	SELECT count(*) FILTER (WHERE observation.outcome = 'healthy')::integer AS healthy,
	       count(*) FILTER (WHERE observation.outcome = 'unhealthy')::integer AS unhealthy,
	       count(*) FILTER (WHERE observation.outcome = 'unknown')::integer AS unknown,
	       coalesce(sum(observation.latency_milliseconds) FILTER (WHERE observation.outcome = 'healthy'), 0)::bigint AS latency_sum,
	       count(*) FILTER (WHERE observation.outcome = 'healthy')::integer AS latency_count,
	       coalesce(max(observation.latency_milliseconds) FILTER (WHERE observation.outcome = 'healthy'), 0)::integer AS latency_maximum
	FROM cairnops_observations observation
	WHERE observation.source_id = source.id
	  AND observation.observed_at >= slot.hour
	  AND observation.observed_at < slot.hour + interval '1 hour'
) counted ON true
WHERE source.enabled AND source.created_at < slot.hour + interval '1 hour'
ON CONFLICT (source_id, hour) DO UPDATE SET
	healthy = excluded.healthy,
	unhealthy = excluded.unhealthy,
	unknown = excluded.unknown,
	expected = excluded.expected,
	latency_sum_milliseconds = excluded.latency_sum_milliseconds,
	latency_count = excluded.latency_count,
	latency_maximum_milliseconds = excluded.latency_maximum_milliseconds,
	consolidated_at = excluded.consolidated_at
`
