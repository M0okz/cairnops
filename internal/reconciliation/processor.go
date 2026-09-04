package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const processorInterval = time.Second

type Processor struct {
	pool     *pgxpool.Pool
	workerID string
	logger   *slog.Logger
	interval time.Duration
}

// execBatch réserve le protocole PostgreSQL simple aux lots de mutations qui
// doivent rester dans la même transaction. pgx refuse plusieurs instructions
// dans une requête préparée ; les valeurs restent néanmoins passées comme
// paramètres et encodées par pgx, jamais concaténées au SQL.
func execBatch(ctx context.Context, tx pgx.Tx, sql string, arguments ...any) error {
	arguments = append([]any{pgx.QueryExecModeSimpleProtocol}, arguments...)
	_, err := tx.Exec(ctx, sql, arguments...)
	return err
}

func NewProcessor(pool *pgxpool.Pool, workerID string, logger *slog.Logger) *Processor {
	return &Processor{pool: pool, workerID: strings.TrimSpace(workerID), logger: logger, interval: processorInterval}
}

type operationWork struct {
	ID                string
	Kind              string
	PrimaryTargetID   string
	SecondaryTargetID string
	SourceID          string
	ArchiveOrigin     bool
	Reason            string
	ActorID           string
	Attempts          int
}

func (processor *Processor) Run(ctx context.Context) error {
	ticker := time.NewTicker(processor.interval)
	defer ticker.Stop()
	for {
		if err := processor.processOne(ctx); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			processor.logger.Error("process target reconciliation", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (processor *Processor) processOne(ctx context.Context) error {
	work, err := processor.claim(ctx)
	if err != nil {
		return err
	}
	if err := processor.setStage(ctx, work.ID, "consolidating"); err != nil {
		return processor.fail(ctx, work, err)
	}
	if err := processor.setStage(ctx, work.ID, "reconciling_incidents"); err != nil {
		return processor.fail(ctx, work, err)
	}
	var result map[string]any
	switch work.Kind {
	case "target_merge":
		result, err = processor.mergeTargets(ctx, work)
	case "source_move":
		result, err = processor.moveSource(ctx, work)
	default:
		err = fmt.Errorf("unsupported reconciliation operation %q", work.Kind)
	}
	if err != nil {
		return processor.fail(ctx, work, err)
	}
	if err := processor.setStage(ctx, work.ID, "recalculating_metrics"); err != nil {
		return processor.fail(ctx, work, err)
	}
	if err := processor.verifyMetricAttribution(ctx, work); err != nil {
		return processor.fail(ctx, work, err)
	}
	if err := processor.setStage(ctx, work.ID, "finalizing"); err != nil {
		return processor.fail(ctx, work, err)
	}
	encoded, _ := json.Marshal(result)
	if _, err := processor.pool.Exec(ctx, `
		UPDATE cairnops_target_reconciliation_operations
		SET status = 'succeeded', stage = 'completed', result = $2::jsonb,
		    last_error = '', completed_at = now(), lease_owner = NULL,
		    lease_until = NULL, updated_at = now()
		WHERE id = $1::uuid AND lease_owner = $3
	`, work.ID, encoded, processor.workerID); err != nil {
		return fmt.Errorf("complete reconciliation operation: %w", err)
	}
	return nil
}

func (processor *Processor) verifyMetricAttribution(ctx context.Context, work operationWork) error {
	var inconsistent int
	if work.Kind == "target_merge" {
		if err := processor.pool.QueryRow(ctx, `
			SELECT (
			  (SELECT count(*) FROM cairnops_observations WHERE target_id = $1::uuid) +
			  (SELECT count(*) FROM cairnops_observation_hours WHERE target_id = $1::uuid)
			)::integer
		`, work.SecondaryTargetID).Scan(&inconsistent); err != nil {
			return fmt.Errorf("verify merged metric attribution: %w", err)
		}
	} else {
		if err := processor.pool.QueryRow(ctx, `
			SELECT (
			  (SELECT count(*) FROM cairnops_signal_sources WHERE id = $1::uuid AND target_id <> $2::uuid) +
			  (SELECT count(*) FROM cairnops_observations WHERE source_id = $1::uuid AND target_id <> $2::uuid) +
			  (SELECT count(*) FROM cairnops_observation_hours WHERE source_id = $1::uuid AND target_id <> $2::uuid)
			)::integer
		`, work.SourceID, work.PrimaryTargetID).Scan(&inconsistent); err != nil {
			return fmt.Errorf("verify reassigned Source metric attribution: %w", err)
		}
	}
	if inconsistent != 0 {
		return fmt.Errorf("%w: %d metric rows remain attributed to the previous identity", ErrConflict, inconsistent)
	}
	return nil
}

func (processor *Processor) claim(ctx context.Context) (operationWork, error) {
	var work operationWork
	err := processor.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM cairnops_target_reconciliation_operations
			WHERE status IN ('queued', 'running') AND attempts < 3
			  AND next_attempt_at <= now()
			  AND (lease_until IS NULL OR lease_until < now())
			ORDER BY next_attempt_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		), claimed AS (
			UPDATE cairnops_target_reconciliation_operations operation
			SET status = 'running', stage = 'preparing', attempts = attempts + 1,
			    started_at = coalesce(started_at, now()), lease_owner = $1,
			    lease_until = now() + interval '2 minutes', updated_at = now()
			FROM candidate WHERE operation.id = candidate.id
			RETURNING operation.*
		)
		SELECT id::text, kind, primary_target_id::text, secondary_target_id::text,
		       coalesce(source_id::text, ''), archive_origin, reason,
		       coalesce(requested_by::text, ''), attempts
		FROM claimed
	`, processor.workerID).Scan(
		&work.ID, &work.Kind, &work.PrimaryTargetID, &work.SecondaryTargetID,
		&work.SourceID, &work.ArchiveOrigin, &work.Reason, &work.ActorID, &work.Attempts,
	)
	if err != nil {
		return operationWork{}, err
	}
	return work, nil
}

func (processor *Processor) setStage(ctx context.Context, operationID, stage string) error {
	result, err := processor.pool.Exec(ctx, `
		UPDATE cairnops_target_reconciliation_operations
		SET stage = $2, lease_until = now() + interval '2 minutes', updated_at = now()
		WHERE id = $1::uuid AND status = 'running' AND lease_owner = $3
	`, operationID, stage, processor.workerID)
	if err != nil {
		return fmt.Errorf("set reconciliation stage: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("reconciliation operation lease lost")
	}
	return nil
}

func (processor *Processor) fail(ctx context.Context, work operationWork, cause error) error {
	message := cause.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	permanent := errors.Is(cause, ErrConflict) || errors.Is(cause, ErrNotFound) ||
		strings.Contains(message, "unsupported reconciliation operation")
	if work.Attempts < 3 && !permanent {
		_, err := processor.pool.Exec(ctx, `
			UPDATE cairnops_target_reconciliation_operations
			SET status = 'queued', stage = 'preparing', last_error = $2,
			    next_attempt_at = now() + make_interval(secs => $3),
			    lease_owner = NULL, lease_until = NULL, updated_at = now()
			WHERE id = $1::uuid AND lease_owner = $4
		`, work.ID, message, work.Attempts*5, processor.workerID)
		if err != nil {
			return fmt.Errorf("retry reconciliation operation after %v: %w", cause, err)
		}
		return cause
	}
	_, err := processor.pool.Exec(ctx, `
		UPDATE cairnops_target_reconciliation_operations
		SET status = 'failed', stage = 'failed', last_error = $2,
		    completed_at = now(), lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE id = $1::uuid AND lease_owner = $3
	`, work.ID, message, processor.workerID)
	if err != nil {
		return fmt.Errorf("fail reconciliation operation after %v: %w", cause, err)
	}
	return cause
}

type lockedTarget struct {
	ID             string
	Name           string
	ArchivedAt     *time.Time
	ReconciledInto string
}

func lockTargets(ctx context.Context, tx pgx.Tx, targetIDs ...string) (map[string]lockedTarget, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, name, archived_at, coalesce(reconciled_into_target_id::text, '')
		FROM cairnops_targets
		WHERE id = ANY($1::uuid[])
		ORDER BY id
		FOR UPDATE
	`, targetIDs)
	if err != nil {
		return nil, fmt.Errorf("lock reconciliation targets: %w", err)
	}
	defer rows.Close()
	items := make(map[string]lockedTarget)
	for rows.Next() {
		var item lockedTarget
		if err := rows.Scan(&item.ID, &item.Name, &item.ArchivedAt, &item.ReconciledInto); err != nil {
			return nil, fmt.Errorf("scan locked reconciliation target: %w", err)
		}
		items[item.ID] = item
	}
	if len(items) != len(targetIDs) {
		return nil, ErrNotFound
	}
	for _, item := range items {
		if item.ArchivedAt != nil || item.ReconciledInto != "" {
			return nil, fmt.Errorf("%w: target %s is no longer active", ErrConflict, item.Name)
		}
	}
	return items, rows.Err()
}

func (processor *Processor) mergeTargets(ctx context.Context, work operationWork) (map[string]any, error) {
	tx, err := processor.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin target reconciliation: %w", err)
	}
	defer tx.Rollback(ctx)
	targets, err := lockTargets(ctx, tx, work.PrimaryTargetID, work.SecondaryTargetID)
	if err != nil {
		return nil, err
	}
	conflicts, err := incidents.MergeTargets(ctx, tx, work.PrimaryTargetID, work.SecondaryTargetID)
	if err != nil {
		return nil, err
	}

	statements := []struct {
		label string
		sql   string
	}{
		{"move Sources", `UPDATE cairnops_signal_sources SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
		{"move connector bindings", `UPDATE cairnops_connector_bindings SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
		{"move Observations", `UPDATE cairnops_observations SET target_id = $1::uuid WHERE target_id = $2::uuid`},
		{"move observation rollups", `UPDATE cairnops_observation_hours SET target_id = $1::uuid WHERE target_id = $2::uuid`},
		{"move contextual indicators", `UPDATE cairnops_context_indicators SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.sql, work.PrimaryTargetID, work.SecondaryTargetID); err != nil {
			return nil, fmt.Errorf("%s: %w", statement.label, err)
		}
	}
	if err := execBatch(ctx, tx, `
		INSERT INTO cairnops_maintenance_targets (maintenance_id, target_id)
		SELECT maintenance_id, $1::uuid FROM cairnops_maintenance_targets WHERE target_id = $2::uuid
		ON CONFLICT DO NOTHING;
		DELETE FROM cairnops_maintenance_targets WHERE target_id = $2::uuid
	`, work.PrimaryTargetID, work.SecondaryTargetID); err != nil {
		return nil, fmt.Errorf("merge target maintenances: %w", err)
	}
	if err := execBatch(ctx, tx, `
		INSERT INTO cairnops_target_aliases (target_id, absorbed_target_id, alias, origin, created_by)
		VALUES ($1::uuid, $2::uuid, $3, 'reconciliation', nullif($4, '')::uuid)
		ON CONFLICT DO NOTHING
	`, work.PrimaryTargetID, work.SecondaryTargetID, targets[work.SecondaryTargetID].Name, work.ActorID); err != nil {
		return nil, fmt.Errorf("record absorbed target alias: %w", err)
	}
	if err := execBatch(ctx, tx, `
		DELETE FROM cairnops_target_aliases old_alias
		USING cairnops_target_aliases surviving_alias
		WHERE old_alias.target_id = $2::uuid
		  AND surviving_alias.target_id = $1::uuid
		  AND surviving_alias.alias = old_alias.alias;
		UPDATE cairnops_target_aliases
		SET target_id = $1::uuid
		WHERE target_id = $2::uuid
	`, work.PrimaryTargetID, work.SecondaryTargetID); err != nil {
		return nil, fmt.Errorf("move target aliases: %w", err)
	}
	activityData, _ := json.Marshal(map[string]any{
		"operation_id": work.ID, "absorbed_target_id": work.SecondaryTargetID,
		"absorbed_target_name": targets[work.SecondaryTargetID].Name,
	})
	if err := execBatch(ctx, tx, `
		UPDATE cairnops_target_activity SET target_id = $1::uuid WHERE target_id = $2::uuid;
		INSERT INTO cairnops_target_activity (target_id, kind, actor_id, message, data)
		VALUES ($1::uuid, 'reconciled', nullif($3, '')::uuid, $4, $5::jsonb);
		UPDATE cairnops_targets
		SET archived_at = now(), reconciled_into_target_id = $1::uuid,
		    reconciled_at = now(), reconciled_by = nullif($3, '')::uuid,
		    reconciliation_reason = $4, updated_at = now()
		WHERE id = $2::uuid;
		UPDATE cairnops_targets SET updated_at = now() WHERE id = $1::uuid;
		UPDATE cairnops_target_reconciliation_suggestions
		SET status = CASE WHEN status = 'accepted' THEN status ELSE 'superseded' END,
		    updated_at = now()
		WHERE left_target_id = $2::uuid OR right_target_id = $2::uuid
	`, work.PrimaryTargetID, work.SecondaryTargetID, work.ActorID, work.Reason, string(activityData)); err != nil {
		return nil, fmt.Errorf("finalize target reconciliation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit target reconciliation: %w", err)
	}
	return map[string]any{
		"target_id": work.PrimaryTargetID, "target_name": targets[work.PrimaryTargetID].Name,
		"absorbed_target_id": work.SecondaryTargetID, "incident_conflicts": conflicts,
	}, nil
}

func (processor *Processor) moveSource(ctx context.Context, work operationWork) (map[string]any, error) {
	tx, err := processor.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin source reassignment: %w", err)
	}
	defer tx.Rollback(ctx)
	targets, err := lockTargets(ctx, tx, work.PrimaryTargetID, work.SecondaryTargetID)
	if err != nil {
		return nil, err
	}
	var sourceName, sourceOrigin, bindingID string
	err = tx.QueryRow(ctx, `
		SELECT name, origin, coalesce(connector_binding_id::text, '')
		FROM cairnops_signal_sources
		WHERE id = $1::uuid AND target_id = $2::uuid
		FOR UPDATE
	`, work.SourceID, work.SecondaryTargetID).Scan(&sourceName, &sourceOrigin, &bindingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: source origin changed", ErrConflict)
	}
	if err != nil {
		return nil, fmt.Errorf("lock source for reassignment: %w", err)
	}
	if _, err := incidents.ReassignSource(ctx, tx, work.PrimaryTargetID, work.SourceID, bindingID); err != nil {
		return nil, err
	}
	statements := []struct{ label, sql string }{
		{"move Source", `UPDATE cairnops_signal_sources SET target_id = $2::uuid, updated_at = now() WHERE id = $1::uuid`},
		{"move Source Observations", `UPDATE cairnops_observations SET target_id = $2::uuid WHERE source_id = $1::uuid`},
		{"move Source rollups", `UPDATE cairnops_observation_hours SET target_id = $2::uuid WHERE source_id = $1::uuid`},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.sql, work.SourceID, work.PrimaryTargetID); err != nil {
			return nil, fmt.Errorf("%s: %w", statement.label, err)
		}
	}
	if bindingID != "" {
		if err := execBatch(ctx, tx, `
			UPDATE cairnops_connector_bindings SET target_id = $2::uuid, updated_at = now() WHERE id = $1::uuid;
			UPDATE cairnops_context_indicators SET target_id = $2::uuid, updated_at = now() WHERE connector_binding_id = $1::uuid
		`, bindingID, work.PrimaryTargetID); err != nil {
			return nil, fmt.Errorf("move Source integration binding: %w", err)
		}
	}
	activityData, _ := json.Marshal(map[string]any{
		"operation_id": work.ID, "source_id": work.SourceID, "source_name": sourceName,
		"from_target_id": work.SecondaryTargetID, "to_target_id": work.PrimaryTargetID,
	})
	if err := execBatch(ctx, tx, `
		INSERT INTO cairnops_target_activity (target_id, kind, actor_id, message, data)
		VALUES ($1::uuid, 'source_moved', nullif($3, '')::uuid, $4, $5::jsonb),
		       ($2::uuid, 'source_moved', nullif($3, '')::uuid, $4, $5::jsonb);
		UPDATE cairnops_targets SET updated_at = now() WHERE id IN ($1::uuid, $2::uuid)
	`, work.PrimaryTargetID, work.SecondaryTargetID, work.ActorID, work.Reason, string(activityData)); err != nil {
		return nil, fmt.Errorf("record Source reassignment: %w", err)
	}
	if work.ArchiveOrigin {
		var remaining int
		if err := tx.QueryRow(ctx, `SELECT count(*)::integer FROM cairnops_signal_sources WHERE target_id = $1::uuid`, work.SecondaryTargetID).Scan(&remaining); err != nil {
			return nil, fmt.Errorf("count remaining Sources: %w", err)
		}
		if remaining != 0 {
			return nil, fmt.Errorf("%w: origin target still has Sources", ErrConflict)
		}
		if _, err := tx.Exec(ctx, `UPDATE cairnops_targets SET archived_at = now(), updated_at = now() WHERE id = $1::uuid`, work.SecondaryTargetID); err != nil {
			return nil, fmt.Errorf("archive empty Source origin: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit Source reassignment: %w", err)
	}
	return map[string]any{
		"target_id": work.PrimaryTargetID, "target_name": targets[work.PrimaryTargetID].Name,
		"source_id": work.SourceID, "source_name": sourceName, "source_origin": sourceOrigin,
		"previous_target_id": work.SecondaryTargetID,
	}, nil
}
