package latency

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// RecentHealthyObservations lit une fenêtre bornée pour évaluer le Profil sans
// modifier les verdicts du Contrôle. Le booléen signale une lecture tronquée.
func (store *Store) RecentHealthyObservations(ctx context.Context, targetID string, since time.Time, limit int) ([]domain.LatencyObservation, bool, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT observation.id, observation.source_id::text, observation.observed_at,
		       observation.latency_milliseconds
		FROM cairnops_observations observation
		JOIN cairnops_signal_sources source ON source.id = observation.source_id
		WHERE observation.target_id = $1::uuid
		  AND observation.observed_at >= $2
		  AND observation.outcome = 'healthy'
		  AND source.origin = 'native'
		  AND source.kind IN ('http', 'tcp', 'dns', 'icmp')
		ORDER BY observation.observed_at DESC, observation.id DESC
		LIMIT $3
	`, targetID, since.UTC(), limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("read recent latency observations: %w", err)
	}
	defer rows.Close()
	observations := make([]domain.LatencyObservation, 0)
	for rows.Next() {
		var id int64
		var milliseconds int64
		var observation domain.LatencyObservation
		if err := rows.Scan(&id, &observation.SourceID, &observation.ObservedAt, &milliseconds); err != nil {
			return nil, false, fmt.Errorf("scan recent latency observation: %w", err)
		}
		observation.ID = strconv.FormatInt(id, 10)
		observation.Latency = time.Duration(milliseconds) * time.Millisecond
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read recent latency observations: %w", err)
	}
	truncated := len(observations) > limit
	if truncated {
		observations = observations[:limit]
	}
	return observations, truncated, nil
}

// DueSources rend les Sources dont le Profil est absent ou a vieilli, les plus
// anciennes d'abord.
//
// Seuls les Contrôles natifs qui mesurent la réactivité d'une Cible en font
// partie. Un Heartbeat n'en mesure pas : sa latence est le temps que CairnOps
// met à évaluer son échéance. Une Source d'Intégration n'en mesure pas
// davantage : sa latence est celle que son produit a bien voulu rapporter.
func (store *Store) DueSources(ctx context.Context, staleBefore time.Time, limit int) ([]string, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT source.id::text
		FROM cairnops_signal_sources source
		LEFT JOIN cairnops_source_latency_profiles profile ON profile.source_id = source.id
		WHERE source.enabled
		  AND source.origin = 'native'
		  AND source.kind IN ('http', 'tcp', 'dns', 'icmp')
		  AND (profile.source_id IS NULL OR profile.computed_at < $1)
		ORDER BY profile.computed_at ASC NULLS FIRST, source.id
		LIMIT $2
	`, staleBefore.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("read due latency profiles: %w", err)
	}
	defer rows.Close()

	sourceIDs := []string{}
	for rows.Next() {
		var sourceID string
		if err := rows.Scan(&sourceID); err != nil {
			return nil, fmt.Errorf("scan due latency profile: %w", err)
		}
		sourceIDs = append(sourceIDs, sourceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read due latency profiles: %w", err)
	}
	return sourceIDs, nil
}

// Rebuild recalcule le Profil d'une Source et le réécrit entièrement. Il rend
// false lorsqu'un autre worker le recalcule déjà : la passe suivante repassera.
//
// La réécriture est complète et non additive. Un seau que la fenêtre glissante
// a quitté doit disparaître, sans quoi le Profil décrirait indéfiniment une
// heure qu'il n'observe plus.
func (store *Store) Rebuild(ctx context.Context, sourceID string, now time.Time) (bool, error) {
	windowEnd := now.UTC().Add(-domain.LatencyEvaluationWindow)
	windowStart := windowEnd.Add(-domain.LatencyProfileWindow)

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin latency profile transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var acquired bool
	if err := tx.QueryRow(ctx, `
		SELECT pg_try_advisory_xact_lock(hashtext('cairnops-latency-profile'), hashtext($1))
	`, sourceID).Scan(&acquired); err != nil {
		return false, fmt.Errorf("lock latency profile: %w", err)
	}
	if !acquired {
		return false, nil
	}

	buckets, err := quantiles(ctx, tx, sourceID, windowStart, windowEnd)
	if err != nil {
		return false, err
	}
	samples := 0
	for _, bucket := range buckets {
		if bucket.Hour == domain.AllHours {
			samples = bucket.Samples
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_source_latency_profiles (source_id, window_start, window_end, samples, computed_at)
		VALUES ($1::uuid, $2, $3, $4, now())
		ON CONFLICT (source_id) DO UPDATE SET
			window_start = excluded.window_start,
			window_end = excluded.window_end,
			samples = excluded.samples,
			computed_at = excluded.computed_at
	`, sourceID, windowStart, windowEnd, samples); err != nil {
		return false, fmt.Errorf("write latency profile: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM cairnops_source_latency_hours WHERE source_id = $1::uuid
	`, sourceID); err != nil {
		return false, fmt.Errorf("clear latency profile hours: %w", err)
	}
	for _, bucket := range buckets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_source_latency_hours (
				source_id, hour, samples, median_milliseconds, p95_milliseconds, p99_milliseconds
			) VALUES ($1::uuid, $2, $3, $4, $5, $6)
		`, sourceID, bucket.Hour, bucket.Samples,
			bucket.Median.Milliseconds(), bucket.P95.Milliseconds(), bucket.P99.Milliseconds(),
		); err != nil {
			return false, fmt.Errorf("write latency profile hour: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit latency profile: %w", err)
	}
	return true, nil
}

// quantiles lit la latence habituelle des Observations saines de la fenêtre,
// par heure UTC puis toutes heures confondues.
//
// Seules les Observations saines comptent. Une Observation défavorable porte le
// délai écoulé avant son échec, souvent le délai d'attente entier : l'inclure
// apprendrait qu'un service indisponible est un service lent. Une Observation
// Inconnue, elle, ne mesure rien.
//
// Une heure sans Observation saine ne produit pas de seau : le Profil ne
// prétend rien sur ce qu'il n'a pas observé.
func quantiles(ctx context.Context, tx pgx.Tx, sourceID string, windowStart, windowEnd time.Time) ([]domain.LatencyBucket, error) {
	rows, err := tx.Query(ctx, `
		WITH observed AS (
			SELECT extract(hour FROM observation.observed_at AT TIME ZONE 'UTC')::integer AS hour,
			       observation.latency_milliseconds AS latency
			FROM cairnops_observations observation
			WHERE observation.source_id = $1::uuid
			  AND observation.observed_at >= $2
			  AND observation.observed_at < $3
			  AND observation.outcome = 'healthy'
		)
		SELECT hour, count(*)::integer,
		       round(percentile_cont(0.5) WITHIN GROUP (ORDER BY latency))::integer,
		       round(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency))::integer,
		       round(percentile_cont(0.99) WITHIN GROUP (ORDER BY latency))::integer
		FROM observed GROUP BY hour HAVING count(*) > 0
		UNION ALL
		SELECT $4::integer, count(*)::integer,
		       round(percentile_cont(0.5) WITHIN GROUP (ORDER BY latency))::integer,
		       round(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency))::integer,
		       round(percentile_cont(0.99) WITHIN GROUP (ORDER BY latency))::integer
		FROM observed HAVING count(*) > 0
		ORDER BY 1
	`, sourceID, windowStart, windowEnd, domain.AllHours)
	if err != nil {
		return nil, fmt.Errorf("read latency quantiles: %w", err)
	}
	defer rows.Close()

	buckets := []domain.LatencyBucket{}
	for rows.Next() {
		var hour, samples, median, p95, p99 int
		if err := rows.Scan(&hour, &samples, &median, &p95, &p99); err != nil {
			return nil, fmt.Errorf("scan latency quantile: %w", err)
		}
		buckets = append(buckets, domain.LatencyBucket{
			Hour:    hour,
			Samples: samples,
			Median:  time.Duration(median) * time.Millisecond,
			P95:     time.Duration(p95) * time.Millisecond,
			P99:     time.Duration(p99) * time.Millisecond,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read latency quantiles: %w", err)
	}
	return buckets, nil
}

// Profile lit le Profil d'une Source. Une Source sans Profil calculé rend
// ErrNotFound : rien n'est inventé en attendant la première passe.
func (store *Store) Profile(ctx context.Context, sourceID string) (domain.LatencyProfile, error) {
	profile := domain.LatencyProfile{SourceID: sourceID, Buckets: []domain.LatencyBucket{}}
	err := store.pool.QueryRow(ctx, `
		SELECT window_start, window_end, samples, computed_at
		FROM cairnops_source_latency_profiles WHERE source_id = $1::uuid
	`, sourceID).Scan(&profile.WindowStart, &profile.WindowEnd, &profile.Samples, &profile.ComputedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LatencyProfile{}, ErrNotFound
	}
	if err != nil {
		return domain.LatencyProfile{}, fmt.Errorf("read latency profile: %w", err)
	}

	rows, err := store.pool.Query(ctx, `
		SELECT hour, samples, median_milliseconds, p95_milliseconds, p99_milliseconds
		FROM cairnops_source_latency_hours WHERE source_id = $1::uuid ORDER BY hour
	`, sourceID)
	if err != nil {
		return domain.LatencyProfile{}, fmt.Errorf("read latency profile hours: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		bucket, err := scanBucket(rows)
		if err != nil {
			return domain.LatencyProfile{}, err
		}
		profile.Buckets = append(profile.Buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return domain.LatencyProfile{}, fmt.Errorf("read latency profile hours: %w", err)
	}
	return profile, nil
}

// Profiles lit les Profils des Sources d'une Cible, indexés par Source. Une
// Source sans Profil n'y figure pas.
func (store *Store) Profiles(ctx context.Context, targetID string) (map[string]domain.LatencyProfile, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT profile.source_id::text, profile.window_start, profile.window_end,
		       profile.samples, profile.computed_at
		FROM cairnops_source_latency_profiles profile
		JOIN cairnops_signal_sources source ON source.id = profile.source_id
		WHERE source.target_id = $1::uuid
	`, targetID)
	if err != nil {
		return nil, fmt.Errorf("read target latency profiles: %w", err)
	}
	defer rows.Close()

	profiles := map[string]domain.LatencyProfile{}
	for rows.Next() {
		profile := domain.LatencyProfile{Buckets: []domain.LatencyBucket{}}
		if err := rows.Scan(
			&profile.SourceID, &profile.WindowStart, &profile.WindowEnd,
			&profile.Samples, &profile.ComputedAt,
		); err != nil {
			return nil, fmt.Errorf("scan target latency profile: %w", err)
		}
		profiles[profile.SourceID] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read target latency profiles: %w", err)
	}
	if len(profiles) == 0 {
		return profiles, nil
	}

	bucketRows, err := store.pool.Query(ctx, `
		SELECT hours.source_id::text, hours.hour, hours.samples,
		       hours.median_milliseconds, hours.p95_milliseconds, hours.p99_milliseconds
		FROM cairnops_source_latency_hours hours
		JOIN cairnops_signal_sources source ON source.id = hours.source_id
		WHERE source.target_id = $1::uuid
		ORDER BY hours.source_id, hours.hour
	`, targetID)
	if err != nil {
		return nil, fmt.Errorf("read target latency profile hours: %w", err)
	}
	defer bucketRows.Close()
	for bucketRows.Next() {
		var sourceID string
		var hour, samples, median, p95, p99 int
		if err := bucketRows.Scan(&sourceID, &hour, &samples, &median, &p95, &p99); err != nil {
			return nil, fmt.Errorf("scan target latency profile hour: %w", err)
		}
		profile, found := profiles[sourceID]
		if !found {
			continue
		}
		profile.Buckets = append(profile.Buckets, domain.LatencyBucket{
			Hour:    hour,
			Samples: samples,
			Median:  time.Duration(median) * time.Millisecond,
			P95:     time.Duration(p95) * time.Millisecond,
			P99:     time.Duration(p99) * time.Millisecond,
		})
		profiles[sourceID] = profile
	}
	if err := bucketRows.Err(); err != nil {
		return nil, fmt.Errorf("read target latency profile hours: %w", err)
	}
	return profiles, nil
}

func scanBucket(rows pgx.Rows) (domain.LatencyBucket, error) {
	var hour, samples, median, p95, p99 int
	if err := rows.Scan(&hour, &samples, &median, &p95, &p99); err != nil {
		return domain.LatencyBucket{}, fmt.Errorf("scan latency profile hour: %w", err)
	}
	return domain.LatencyBucket{
		Hour:    hour,
		Samples: samples,
		Median:  time.Duration(median) * time.Millisecond,
		P95:     time.Duration(p95) * time.Millisecond,
		P99:     time.Duration(p99) * time.Millisecond,
	}, nil
}
