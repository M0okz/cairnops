package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

// bucketsCTE assemble la fenêtre : les heures révolues viennent des agrégats
// consolidés, l'heure en cours des Observations brutes. Une consolidation en
// retard ne rend donc jamais une mesure fausse, seulement plus coûteuse.
//
// $1 porte la profondeur de la fenêtre en heures.
const bucketsCTE = `
WITH horizon AS (
	SELECT date_trunc('hour', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS current_hour
),
buckets AS (
	SELECT rollup.source_id, rollup.hour, rollup.healthy, rollup.unhealthy, rollup.unknown,
	       rollup.expected, rollup.latency_sum_milliseconds, rollup.latency_count,
	       rollup.latency_maximum_milliseconds
	FROM cairnops_observation_hours rollup
	CROSS JOIN horizon
	WHERE rollup.hour >= horizon.current_hour - make_interval(hours => $1 - 1)
	  AND rollup.hour < horizon.current_hour
	UNION ALL
	SELECT source.id, horizon.current_hour, running.healthy, running.unhealthy, running.unknown,
	       CASE WHEN source.kind = 'generic_webhook' THEN 0 ELSE greatest(0, floor(
	           extract(epoch FROM (now() - greatest(horizon.current_hour, source.created_at)))
	           / source.interval_seconds
	       ))::integer END,
	       running.latency_sum, running.latency_count, running.latency_maximum
	FROM cairnops_signal_sources source
	CROSS JOIN horizon
	LEFT JOIN LATERAL (
		SELECT count(*) FILTER (WHERE observation.outcome = 'healthy')::integer AS healthy,
		       count(*) FILTER (WHERE observation.outcome = 'unhealthy')::integer AS unhealthy,
		       count(*) FILTER (WHERE observation.outcome = 'unknown')::integer AS unknown,
		       coalesce(sum(observation.latency_milliseconds) FILTER (WHERE observation.outcome = 'healthy'), 0)::bigint AS latency_sum,
		       count(*) FILTER (WHERE observation.outcome = 'healthy')::integer AS latency_count,
		       coalesce(max(observation.latency_milliseconds) FILTER (WHERE observation.outcome = 'healthy'), 0)::integer AS latency_maximum
		FROM cairnops_observations observation
		WHERE observation.source_id = source.id
		  AND observation.observed_at >= horizon.current_hour
	) running ON true
	WHERE source.enabled AND source.measures_availability
)
`

// hourlyTarget regroupe les heures d'une Cible : le total nourrit la mesure,
// la suite chronologique nourrit la tendance.
type hourlyTarget struct {
	targetID         string
	total            domain.Counters
	hours            []hourlyBucket
	latestObservedAt *time.Time
}

type hourlyBucket struct {
	hour     time.Time
	counters domain.Counters
}

// trend rend la Disponibilité de chaque heure qui a conclu quelque chose. Une
// heure muette ne produit pas de point plutôt qu'un point à zéro : la
// Couverture dit déjà ce qui manque, la tendance n'a pas à l'inventer.
func (target hourlyTarget) trend() []float64 {
	trend := make([]float64, 0, len(target.hours))
	for _, bucket := range target.hours {
		if conclusive := bucket.counters.Conclusive(); conclusive > 0 {
			trend = append(trend, float64(bucket.counters.Healthy)/float64(conclusive))
		}
	}
	return trend
}

// latencyTrend rend la latence moyenne de chaque heure qui a mesuré quelque
// chose. Comme la tendance de Disponibilité, une heure muette ne produit pas
// de point plutôt qu'un point à zéro : une Cible qui n'a rien répondu n'a pas
// répondu vite.
func (target hourlyTarget) latencyTrend() []float64 {
	trend := make([]float64, 0, len(target.hours))
	for _, bucket := range target.hours {
		if bucket.counters.LatencyCount > 0 {
			trend = append(trend, float64(bucket.counters.LatencySumMilliseconds)/float64(bucket.counters.LatencyCount))
		}
	}
	return trend
}

// hourlyByTarget rend, heure par heure, les Observations de chaque Cible
// active. Un targetID vide couvre toutes les Cibles, ce qu'appelle la liste.
func (store *Store) hourlyByTarget(ctx context.Context, window domain.Window, targetID string) ([]hourlyTarget, error) {
	rows, err := store.pool.Query(ctx, bucketsCTE+`
		SELECT target.id::text, bucket.hour,
		       coalesce(sum(bucket.healthy), 0)::integer,
		       coalesce(sum(bucket.unhealthy), 0)::integer,
		       coalesce(sum(bucket.unknown), 0)::integer,
		       coalesce(sum(bucket.expected), 0)::integer,
		       coalesce(sum(bucket.latency_sum_milliseconds), 0)::bigint,
		       coalesce(sum(bucket.latency_count), 0)::integer,
		       coalesce(max(bucket.latency_maximum_milliseconds), 0)::integer,
		       max(source.last_observed_at)
		FROM cairnops_targets target
		LEFT JOIN cairnops_signal_sources source
			ON source.target_id = target.id AND source.measures_availability
		LEFT JOIN buckets bucket ON bucket.source_id = source.id
		WHERE target.archived_at IS NULL
		  AND ($2::uuid IS NULL OR target.id = $2::uuid)
		GROUP BY target.id, bucket.hour
		ORDER BY target.id, bucket.hour
	`, window.Hours(), nullableUUID(targetID))
	if err != nil {
		return nil, fmt.Errorf("read hourly measures: %w", err)
	}
	defer rows.Close()

	targets := make([]hourlyTarget, 0)
	indexes := make(map[string]int)
	for rows.Next() {
		var id string
		var hour, latestObservedAt *time.Time
		var counters domain.Counters
		if err := rows.Scan(
			&id, &hour, &counters.Healthy, &counters.Unhealthy, &counters.Unknown,
			&counters.Expected, &counters.LatencySumMilliseconds, &counters.LatencyCount, &counters.LatencyMaximum,
			&latestObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan hourly measure: %w", err)
		}
		index, known := indexes[id]
		if !known {
			index = len(targets)
			indexes[id] = index
			targets = append(targets, hourlyTarget{targetID: id})
		}
		if latestObservedAt != nil &&
			(targets[index].latestObservedAt == nil || latestObservedAt.After(*targets[index].latestObservedAt)) {
			targets[index].latestObservedAt = latestObservedAt
		}
		// Une Cible sans Source ni Observation ne rend qu'une ligne sans heure :
		// elle existe dans la réponse, sans mesure à afficher.
		if hour == nil {
			continue
		}
		targets[index].total = targets[index].total.Add(counters)
		targets[index].hours = append(targets[index].hours, hourlyBucket{hour: *hour, counters: counters})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hourly measures: %w", err)
	}
	return targets, nil
}

type sourceCounters struct {
	targetID             string
	sourceID             string
	name                 string
	kind                 string
	origin               string
	measuresAvailability bool
	latestOutcome        *domain.Outcome
	latestObservedAt     *time.Time
	counters             domain.Counters
}

// bySource rend la part de chaque Source d'une Cible sur la fenêtre.
func (store *Store) bySource(ctx context.Context, window domain.Window, targetID string) ([]sourceCounters, error) {
	rows, err := store.pool.Query(ctx, bucketsCTE+`
		SELECT source.target_id::text, source.id::text, source.name, source.kind, source.origin,
		       source.measures_availability,
		       latest.outcome, latest.observed_at,
		       coalesce(sum(bucket.healthy), 0)::integer,
		       coalesce(sum(bucket.unhealthy), 0)::integer,
		       coalesce(sum(bucket.unknown), 0)::integer,
		       coalesce(sum(bucket.expected), 0)::integer,
		       coalesce(sum(bucket.latency_sum_milliseconds), 0)::bigint,
		       coalesce(sum(bucket.latency_count), 0)::integer,
		       coalesce(max(bucket.latency_maximum_milliseconds), 0)::integer
		FROM cairnops_signal_sources source
		JOIN cairnops_targets target ON target.id = source.target_id AND target.archived_at IS NULL
		LEFT JOIN buckets bucket ON bucket.source_id = source.id
		LEFT JOIN LATERAL (
			SELECT observation.outcome, observation.observed_at
			FROM cairnops_observations observation
			WHERE observation.source_id = source.id
			ORDER BY observation.observed_at DESC, observation.id DESC
			LIMIT 1
		) latest ON true
		WHERE ($2::uuid IS NULL OR source.target_id = $2::uuid)
		GROUP BY source.target_id, source.id, source.name, source.kind, source.origin,
		         source.measures_availability, latest.outcome, latest.observed_at
		ORDER BY source.target_id, source.origin, lower(source.name), source.id
	`, window.Hours(), nullableUUID(targetID))
	if err != nil {
		return nil, fmt.Errorf("read source measures: %w", err)
	}
	defer rows.Close()

	sources := make([]sourceCounters, 0)
	for rows.Next() {
		var source sourceCounters
		if err := rows.Scan(
			&source.targetID, &source.sourceID, &source.name, &source.kind, &source.origin,
			&source.measuresAvailability,
			&source.latestOutcome, &source.latestObservedAt,
			&source.counters.Healthy, &source.counters.Unhealthy,
			&source.counters.Unknown, &source.counters.Expected, &source.counters.LatencySumMilliseconds,
			&source.counters.LatencyCount, &source.counters.LatencyMaximum,
		); err != nil {
			return nil, fmt.Errorf("scan source measure: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source measures: %w", err)
	}
	return sources, nil
}

func nullableUUID(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// Hour porte l'activité de toute l'instance sur une heure : ce que les
// Contrôles devaient exécuter, ce qu'ils ont conclu, et à quelle vitesse.
// C'est la matière des micro-graphes de la page Santé, où chaque cellule doit
// pouvoir montrer son propre passé sur la même fenêtre que son chiffre.
type Hour struct {
	Hour                       time.Time `json:"hour"`
	ExpectedObservations       int       `json:"expected_observations"`
	ConclusiveObservations     int       `json:"conclusive_observations"`
	HealthyObservations        int       `json:"healthy_observations"`
	AverageLatencyMilliseconds *int      `json:"average_latency_milliseconds"`
}

// InstanceHours rend les dernières heures de l'instance, de la plus ancienne
// à la plus récente. Les Cibles archivées en sont exclues : elles ne sont plus
// supervisées, leur passé n'a pas à peser sur la Santé du jour.
func (store *Store) InstanceHours(ctx context.Context, hours int) ([]Hour, error) {
	rows, err := store.pool.Query(ctx, bucketsCTE+`
		SELECT bucket.hour,
		       coalesce(sum(bucket.expected), 0)::integer,
		       coalesce(sum(bucket.healthy), 0)::integer,
		       coalesce(sum(bucket.unhealthy), 0)::integer,
		       coalesce(sum(bucket.latency_sum_milliseconds), 0)::bigint,
		       coalesce(sum(bucket.latency_count), 0)::integer
		FROM buckets bucket
		JOIN cairnops_signal_sources source ON source.id = bucket.source_id
		JOIN cairnops_targets target ON target.id = source.target_id AND target.archived_at IS NULL
		GROUP BY bucket.hour
		ORDER BY bucket.hour
	`, hours)
	if err != nil {
		return nil, fmt.Errorf("read instance hours: %w", err)
	}
	defer rows.Close()

	series := make([]Hour, 0, hours)
	for rows.Next() {
		var hour Hour
		var healthy, unhealthy, latencyCount int
		var latencySum int64
		if err := rows.Scan(&hour.Hour, &hour.ExpectedObservations, &healthy, &unhealthy, &latencySum, &latencyCount); err != nil {
			return nil, fmt.Errorf("scan instance hour: %w", err)
		}
		hour.HealthyObservations = healthy
		hour.ConclusiveObservations = healthy + unhealthy
		if latencyCount > 0 {
			average := int((latencySum + int64(latencyCount)/2) / int64(latencyCount))
			hour.AverageLatencyMilliseconds = &average
		}
		series = append(series, hour)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instance hours: %w", err)
	}
	return series, nil
}
