package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	ClaimDue(context.Context, string, int, time.Duration) ([]domain.Source, error)
	Complete(context.Context, string, domain.Source, domain.Observation) error
}

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (store *PostgresStore) ClaimDue(ctx context.Context, owner string, limit int, lease time.Duration) ([]domain.Source, error) {
	leaseUntil := time.Now().UTC().Add(lease)
	rows, err := store.pool.Query(ctx, `
		WITH due AS (
			SELECT source.id
			FROM cairnops_signal_sources source
			JOIN cairnops_targets target ON target.id = source.target_id
			WHERE source.enabled
			  AND source.origin = 'native'
			  AND target.archived_at IS NULL
			  AND source.next_run_at <= now()
			  AND (source.lease_until IS NULL OR source.lease_until < now())
			ORDER BY source.next_run_at, source.id
			LIMIT $1
			FOR UPDATE OF source SKIP LOCKED
		)
		UPDATE cairnops_signal_sources AS source
		SET lease_owner = $2, lease_until = $3, updated_at = now()
		FROM due
		WHERE source.id = due.id
		RETURNING source.id::text, source.target_id::text, source.name, source.kind,
		          source.interval_seconds, source.timeout_milliseconds, source.config,
		          source.last_signal_at, source.last_signal_outcome, source.last_observed_at
	`, limit, owner, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("claim due sources: %w", err)
	}
	defer rows.Close()

	sources := make([]domain.Source, 0, limit)
	for rows.Next() {
		var source domain.Source
		var kind string
		var lastSignalOutcome *string
		var intervalSeconds, timeoutMilliseconds int
		var config []byte
		if err := rows.Scan(
			&source.ID, &source.TargetID, &source.Name, &kind,
			&intervalSeconds, &timeoutMilliseconds, &config,
			&source.LastSignalAt, &lastSignalOutcome, &source.LastObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan claimed source: %w", err)
		}
		source.Kind = domain.SourceKind(kind)
		source.Interval = time.Duration(intervalSeconds) * time.Second
		source.Timeout = time.Duration(timeoutMilliseconds) * time.Millisecond
		source.Config = json.RawMessage(config)
		if lastSignalOutcome != nil {
			outcome := domain.Outcome(*lastSignalOutcome)
			source.LastSignalOutcome = &outcome
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed sources: %w", err)
	}
	return sources, nil
}

func (store *PostgresStore) Complete(ctx context.Context, owner string, source domain.Source, observation domain.Observation) error {
	if observation.Details == nil {
		observation.Details = make(map[string]any)
	}
	details, err := json.Marshal(observation.Details)
	if err != nil {
		return fmt.Errorf("encode observation details: %w", err)
	}
	latencyMilliseconds := max(0, int(observation.Latency.Milliseconds()))

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin observation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE cairnops_signal_sources
		SET next_run_at = now() + make_interval(secs => interval_seconds),
		    lease_owner = NULL,
		    lease_until = NULL,
		    last_observed_at = $3,
		    updated_at = now()
		WHERE id = $1::uuid AND lease_owner = $2
	`, source.ID, owner, observation.ObservedAt)
	if err != nil {
		return fmt.Errorf("complete source lease: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("source %s is no longer leased by %s", source.ID, owner)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_observations (
			source_id, target_id, observed_at, outcome,
			latency_milliseconds, reason, message, details
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8::jsonb)
	`, source.ID, source.TargetID, observation.ObservedAt, observation.Outcome,
		latencyMilliseconds, observation.Reason, observation.Message, details); err != nil {
		return fmt.Errorf("insert observation: %w", err)
	}
	if err := incidents.ApplyNativeObservation(ctx, tx, incidents.NativeObservation{
		SourceID: source.ID, TargetID: source.TargetID, SourceName: source.Name,
		Outcome: observation.Outcome, ObservedAt: observation.ObservedAt,
		Reason: observation.Reason, Message: observation.Message,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
