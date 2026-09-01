package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	conflicts, err := activeIncidentPairs(ctx, tx, work.PrimaryTargetID, work.SecondaryTargetID)
	if err != nil {
		return nil, err
	}
	for _, conflict := range conflicts {
		if err := mergeActiveIncidents(ctx, tx, work.PrimaryTargetID, conflict[0], conflict[1]); err != nil {
			return nil, err
		}
	}

	statements := []struct {
		label string
		sql   string
	}{
		{"move Sources", `UPDATE cairnops_signal_sources SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
		{"move connector bindings", `UPDATE cairnops_connector_bindings SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
		{"move Observations", `UPDATE cairnops_observations SET target_id = $1::uuid WHERE target_id = $2::uuid`},
		{"move observation rollups", `UPDATE cairnops_observation_hours SET target_id = $1::uuid WHERE target_id = $2::uuid`},
		{"move Incident signals", `UPDATE cairnops_incident_signals SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
		{"move remaining Incidents", `UPDATE cairnops_incidents SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
		{"move contextual indicators", `UPDATE cairnops_context_indicators SET target_id = $1::uuid, updated_at = now() WHERE target_id = $2::uuid`},
		{"move inbox references", `UPDATE cairnops_notification_inbox SET target_id = $1::uuid WHERE target_id = $2::uuid`},
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
		"absorbed_target_id": work.SecondaryTargetID, "incident_conflicts": len(conflicts),
	}, nil
}

func activeIncidentPairs(ctx context.Context, tx pgx.Tx, leftID, rightID string) ([][2]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT left_incident.id::text, right_incident.id::text
		FROM cairnops_incidents left_incident
		JOIN cairnops_incidents right_incident
		  ON right_incident.target_id = $2::uuid AND right_incident.status = 'active'
		 AND right_incident.nature_key = left_incident.nature_key
		WHERE left_incident.target_id = $1::uuid AND left_incident.status = 'active'
		ORDER BY left_incident.nature_key
		FOR UPDATE OF left_incident, right_incident
	`, leftID, rightID)
	if err != nil {
		return nil, fmt.Errorf("lock active incident conflicts: %w", err)
	}
	defer rows.Close()
	items := make([][2]string, 0)
	for rows.Next() {
		var pair [2]string
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			return nil, fmt.Errorf("scan active incident conflict: %w", err)
		}
		items = append(items, pair)
	}
	return items, rows.Err()
}

type incidentMergeState struct {
	ID                string
	OpenedAt          time.Time
	AcknowledgedAt    *time.Time
	SourceSeverity    string
	EffectiveSeverity string
}

func mergeActiveIncidents(ctx context.Context, tx pgx.Tx, targetID, leftID, rightID string) error {
	states := make([]incidentMergeState, 0, 2)
	rows, err := tx.Query(ctx, `
		SELECT id::text, opened_at, acknowledged_at, source_severity, effective_severity
		FROM cairnops_incidents WHERE id = ANY($1::uuid[]) ORDER BY opened_at, id FOR UPDATE
	`, []string{leftID, rightID})
	if err != nil {
		return fmt.Errorf("read active incident merge state: %w", err)
	}
	for rows.Next() {
		var state incidentMergeState
		if err := rows.Scan(&state.ID, &state.OpenedAt, &state.AcknowledgedAt, &state.SourceSeverity, &state.EffectiveSeverity); err != nil {
			rows.Close()
			return fmt.Errorf("scan active incident merge state: %w", err)
		}
		states = append(states, state)
	}
	rows.Close()
	if len(states) != 2 {
		return fmt.Errorf("%w: active incident changed during reconciliation", ErrConflict)
	}
	keep, absorbed := states[0], states[1]
	bothAcknowledged := keep.AcknowledgedAt != nil && absorbed.AcknowledgedAt != nil
	if err := execBatch(ctx, tx, `
		INSERT INTO cairnops_incident_indicator_snapshots (
			incident_id, indicator_id, semantic_key, label, unit, value, observed_at, created_at
		)
		SELECT $1::uuid, indicator_id, semantic_key, label, unit, value, observed_at, created_at
		FROM cairnops_incident_indicator_snapshots WHERE incident_id = $2::uuid
		ON CONFLICT DO NOTHING;
		DELETE FROM cairnops_incident_indicator_snapshots WHERE incident_id = $2::uuid;
		UPDATE cairnops_incident_signals
		SET incident_id = $1::uuid, target_id = $3::uuid, updated_at = now()
		WHERE incident_id = $2::uuid;
		UPDATE cairnops_incident_activity SET incident_id = $1::uuid WHERE incident_id = $2::uuid;
		UPDATE cairnops_notification_outbox
		SET status = 'cancelled', last_error = 'Incident réuni lors d’un rapprochement de Cibles',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE incident_id = $2::uuid AND status IN ('pending', 'failed');
		INSERT INTO cairnops_notification_outbox (
			incident_id, channel_id, event_kind, event_key, status, target_name, nature_label,
			severity, opened_at, resolved_at, last_error
		)
		SELECT $2::uuid, opening.channel_id, 'resolved', 'resolved', 'cancelled', opening.target_name,
		       opening.nature_label, opening.severity, opening.opened_at, now(),
		       'Résolution technique supprimée lors d’un rapprochement de Cibles'
		FROM cairnops_notification_outbox opening
		WHERE opening.incident_id = $2::uuid
		  AND opening.event_kind = 'firing' AND opening.status = 'delivered'
		ON CONFLICT DO NOTHING;
		UPDATE cairnops_incidents
		SET status = 'resolved', resolved_at = now(), target_id = $3::uuid,
		    updated_at = now()
		WHERE id = $2::uuid;
		UPDATE cairnops_incident_burst_members
		SET target_id = $3::uuid WHERE incident_id IN ($1::uuid, $2::uuid);
	`, keep.ID, absorbed.ID, targetID); err != nil {
		return fmt.Errorf("consolidate active incident evidence: %w", err)
	}
	if err := execBatch(ctx, tx, `
		UPDATE cairnops_incidents
		SET target_id = $2::uuid,
		    opened_at = least(opened_at, $3),
		    source_severity = CASE
		      WHEN array_position(ARRAY['information','warning','major','critical'], source_severity)
		         >= array_position(ARRAY['information','warning','major','critical'], $4) THEN source_severity ELSE $4 END,
		    effective_severity = CASE
		      WHEN array_position(ARRAY['information','warning','major','critical'], effective_severity)
		         >= array_position(ARRAY['information','warning','major','critical'], $5) THEN effective_severity ELSE $5 END,
		    acknowledged_at = CASE WHEN $6 THEN least(acknowledged_at, $7) ELSE NULL END,
		    acknowledged_by = CASE WHEN $6 THEN acknowledged_by ELSE NULL END,
		    acknowledgement_origin = CASE WHEN $6 THEN acknowledgement_origin ELSE NULL END,
		    acknowledgement_sync_status = CASE WHEN $6 THEN acknowledgement_sync_status ELSE 'not_applicable' END,
		    acknowledgement_sync_error = CASE WHEN $6 THEN acknowledgement_sync_error ELSE '' END,
		    updated_at = now()
		WHERE id = $1::uuid;
		INSERT INTO cairnops_incident_activity (incident_id, kind, origin, message, data)
		VALUES ($1::uuid, 'target_reconciled', 'cairnops',
		        'Preuves réunies lors du rapprochement de deux Cibles',
		        jsonb_build_object('absorbed_incident_id', $8))
	`, keep.ID, targetID, absorbed.OpenedAt, absorbed.SourceSeverity,
		absorbed.EffectiveSeverity, bothAcknowledged, absorbed.AcknowledgedAt, absorbed.ID); err != nil {
		return fmt.Errorf("finalize active incident merge: %w", err)
	}
	return nil
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
	incidentRows, err := tx.Query(ctx, `
		SELECT DISTINCT signal.incident_id::text
		FROM cairnops_incident_signals signal
		WHERE signal.source_id = $1::uuid
		   OR signal.connector_binding_id = nullif($2, '')::uuid
		ORDER BY signal.incident_id::text
	`, work.SourceID, bindingID)
	if err != nil {
		return nil, fmt.Errorf("list Source Incident history: %w", err)
	}
	incidentIDs := make([]string, 0)
	for incidentRows.Next() {
		var incidentID string
		if err := incidentRows.Scan(&incidentID); err != nil {
			incidentRows.Close()
			return nil, fmt.Errorf("scan Source Incident history: %w", err)
		}
		incidentIDs = append(incidentIDs, incidentID)
	}
	incidentRows.Close()
	for _, incidentID := range incidentIDs {
		if err := reassignIncidentEvidence(ctx, tx, incidentID, work.PrimaryTargetID, work.SourceID, bindingID); err != nil {
			return nil, err
		}
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

func reassignIncidentEvidence(ctx context.Context, tx pgx.Tx, incidentID, destinationID, sourceID, bindingID string) error {
	var natureKey, natureLabel, status, sourceSeverity, effectiveSeverity string
	var openedAt time.Time
	var resolvedAt, acknowledgedAt *time.Time
	var acknowledgedBy *string
	err := tx.QueryRow(ctx, `
		SELECT nature_key, nature_label, status, source_severity, effective_severity,
		       opened_at, resolved_at, acknowledged_at, acknowledged_by::text
		FROM cairnops_incidents WHERE id = $1::uuid FOR UPDATE
	`, incidentID).Scan(&natureKey, &natureLabel, &status, &sourceSeverity, &effectiveSeverity,
		&openedAt, &resolvedAt, &acknowledgedAt, &acknowledgedBy)
	if err != nil {
		return fmt.Errorf("lock Incident for Source reassignment: %w", err)
	}
	var moving, total int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (
		         WHERE signal.source_id = $2::uuid
		            OR signal.connector_binding_id = nullif($3, '')::uuid
		       )::integer,
		       count(*)::integer
		FROM cairnops_incident_signals signal
		WHERE signal.incident_id = $1::uuid
	`, incidentID, sourceID, bindingID).Scan(&moving, &total); err != nil {
		return fmt.Errorf("count Incident evidence for Source reassignment: %w", err)
	}
	if moving == 0 {
		return nil
	}
	var destinationIncidentID string
	if status == "active" {
		err = tx.QueryRow(ctx, `
			SELECT id::text FROM cairnops_incidents
			WHERE target_id = $1::uuid AND nature_key = $2 AND status = 'active'
			FOR UPDATE
		`, destinationID, natureKey).Scan(&destinationIncidentID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find destination Incident for Source reassignment: %w", err)
		}
	}
	if moving == total && destinationIncidentID == "" {
		if _, err := tx.Exec(ctx, `UPDATE cairnops_incidents SET target_id = $2::uuid, updated_at = now() WHERE id = $1::uuid`, incidentID, destinationID); err != nil {
			return fmt.Errorf("move Source-only Incident: %w", err)
		}
		destinationIncidentID = incidentID
	} else if destinationIncidentID == "" {
		err = tx.QueryRow(ctx, `
			INSERT INTO cairnops_incidents (
				target_id, nature_key, nature_label, status, source_severity,
				effective_severity, opened_at, resolved_at
			) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id::text
		`, destinationID, natureKey, natureLabel, status, sourceSeverity,
			effectiveSeverity, openedAt, resolvedAt).Scan(&destinationIncidentID)
		if err != nil {
			return fmt.Errorf("reconstruct Source Incident history: %w", err)
		}
	}
	if err := execBatch(ctx, tx, `
		UPDATE cairnops_incident_signals signal
		SET incident_id = $1::uuid, target_id = $2::uuid, updated_at = now()
		WHERE signal.incident_id = $3::uuid
		  AND (signal.source_id = $4::uuid
		       OR signal.connector_binding_id = nullif($5, '')::uuid)
	`, destinationIncidentID, destinationID, incidentID, sourceID, bindingID); err != nil {
		return fmt.Errorf("move Source Incident evidence: %w", err)
	}
	if status == "active" && destinationIncidentID != incidentID {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incidents
			SET opened_at = least(opened_at, $2),
			    source_severity = CASE
			      WHEN array_position(ARRAY['information','warning','major','critical'], source_severity)
			         >= array_position(ARRAY['information','warning','major','critical'], $3) THEN source_severity ELSE $3 END,
			    effective_severity = CASE
			      WHEN array_position(ARRAY['information','warning','major','critical'], effective_severity)
			         >= array_position(ARRAY['information','warning','major','critical'], $4) THEN effective_severity ELSE $4 END,
			    acknowledged_at = CASE WHEN $5 THEN acknowledged_at ELSE NULL END,
			    acknowledged_by = CASE WHEN $5 THEN acknowledged_by ELSE NULL END,
			    acknowledgement_origin = CASE WHEN $5 THEN acknowledgement_origin ELSE NULL END,
			    acknowledgement_sync_status = CASE WHEN $5 THEN acknowledgement_sync_status ELSE 'not_applicable' END,
			    acknowledgement_sync_error = CASE WHEN $5 THEN acknowledgement_sync_error ELSE '' END,
			    updated_at = now()
			WHERE id = $1::uuid
		`, destinationIncidentID, openedAt, sourceSeverity, effectiveSeverity, acknowledgedAt != nil); err != nil {
			return fmt.Errorf("merge active Incident after Source reassignment: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_activity (incident_id, kind, origin, message, data)
		VALUES ($1::uuid, 'source_reassigned', 'cairnops',
		        'Preuves reconstruites après correction du rattachement d’une Source',
		        jsonb_build_object('previous_incident_id', $2::uuid))
	`, destinationIncidentID, incidentID); err != nil {
		return fmt.Errorf("record Source Incident reconstruction: %w", err)
	}
	if destinationIncidentID != incidentID {
		if err := execBatch(ctx, tx, `
			INSERT INTO cairnops_incident_activity (incident_id, kind, origin, message, data)
			VALUES ($1::uuid, 'source_reassigned', 'cairnops',
			        'Une Source a été rattachée rétroactivement à une autre Cible',
			        jsonb_build_object('destination_incident_id', $2::uuid))
		`, incidentID, destinationIncidentID); err != nil {
			return fmt.Errorf("record original Source Incident correction: %w", err)
		}
	}
	if moving < total || destinationIncidentID != incidentID {
		if err := recomputeOriginIncident(ctx, tx, incidentID); err != nil {
			return err
		}
	}
	return nil
}

func recomputeOriginIncident(ctx context.Context, tx pgx.Tx, incidentID string) error {
	var active int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE active AND invalidated_at IS NULL)::integer
		FROM cairnops_incident_signals WHERE incident_id = $1::uuid
	`, incidentID).Scan(&active); err != nil {
		return fmt.Errorf("count remaining active Incident evidence: %w", err)
	}
	if active == 0 {
		if err := execBatch(ctx, tx, `
			UPDATE cairnops_notification_outbox
			SET status = 'cancelled', last_error = 'Preuve déplacée lors d’une correction de Source',
			    lease_owner = NULL, lease_until = NULL, updated_at = now()
			WHERE incident_id = $1::uuid AND status IN ('pending', 'failed');
			INSERT INTO cairnops_notification_outbox (
				incident_id, channel_id, event_kind, event_key, status, target_name, nature_label,
				severity, opened_at, resolved_at, last_error
			)
			SELECT $1::uuid, opening.channel_id, 'resolved', 'resolved', 'cancelled', opening.target_name,
			       opening.nature_label, opening.severity, opening.opened_at, now(),
			       'Résolution technique supprimée après correction du rattachement d’une Source'
			FROM cairnops_notification_outbox opening
			WHERE opening.incident_id = $1::uuid
			  AND opening.event_kind = 'firing' AND opening.status = 'delivered'
			ON CONFLICT DO NOTHING;
			UPDATE cairnops_incidents
			SET status = 'resolved', resolved_at = coalesce(resolved_at, now()), updated_at = now()
			WHERE id = $1::uuid AND status = 'active'
		`, incidentID); err != nil {
			return fmt.Errorf("resolve Incident emptied by Source reassignment: %w", err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents incident
		SET source_severity = evidence.maximum_severity,
		    effective_severity = CASE
		      WHEN array_position(ARRAY['information','warning','major','critical'], incident.effective_severity)
		         >= array_position(ARRAY['information','warning','major','critical'], evidence.maximum_severity)
		      THEN incident.effective_severity ELSE evidence.maximum_severity END,
		    updated_at = now()
		FROM (
			SELECT (ARRAY['information','warning','major','critical'])[max(array_position(
			       ARRAY['information','warning','major','critical'], severity))] AS maximum_severity
			FROM cairnops_incident_signals WHERE incident_id = $1::uuid AND active AND invalidated_at IS NULL
		) evidence
		WHERE incident.id = $1::uuid
	`, incidentID); err != nil {
		return fmt.Errorf("recompute Incident after Source reassignment: %w", err)
	}
	return nil
}
