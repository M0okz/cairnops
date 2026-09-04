package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (store *Store) ListSuggestions(ctx context.Context, status string) ([]Suggestion, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "pending"
	}
	allowed := map[string]bool{"pending": true, "rejected": true, "snoozed": true, "accepted": true, "superseded": true}
	if !allowed[status] {
		return nil, fmt.Errorf("%w: invalid suggestion status", ErrInvalidInput)
	}
	summaries, err := store.targetSummaries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := store.pool.Query(ctx, `
		SELECT suggestion.id::text, suggestion.kind,
		       suggestion.left_target_id::text, suggestion.right_target_id::text,
		       coalesce(suggestion.source_id::text, ''),
		       coalesce(source.target_id::text, ''), coalesce(source.name, ''),
		       coalesce(source.kind, ''), coalesce(source.origin, ''),
		       suggestion.confidence, suggestion.score, suggestion.evidence,
		       suggestion.contradictions, suggestion.status, suggestion.snoozed_until,
		       suggestion.decision_reason, suggestion.last_detected_at,
		       suggestion.created_at, suggestion.updated_at
		FROM cairnops_target_reconciliation_suggestions suggestion
		LEFT JOIN cairnops_signal_sources source ON source.id = suggestion.source_id
		WHERE suggestion.status = $1
		  AND (suggestion.status <> 'snoozed' OR suggestion.snoozed_until > now())
		ORDER BY CASE suggestion.confidence WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
		         suggestion.score DESC, suggestion.updated_at DESC
	`, status)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation suggestions: %w", err)
	}
	defer rows.Close()
	items := make([]Suggestion, 0)
	for rows.Next() {
		var item Suggestion
		var leftID, rightID, sourceID, sourceTargetID, sourceName, sourceKind, sourceOrigin string
		var evidenceJSON, contradictionsJSON []byte
		if err := rows.Scan(
			&item.ID, &item.Kind, &leftID, &rightID, &sourceID,
			&sourceTargetID, &sourceName, &sourceKind, &sourceOrigin,
			&item.Confidence, &item.Score, &evidenceJSON, &contradictionsJSON,
			&item.Status, &item.SnoozedUntil, &item.DecisionReason,
			&item.LastDetectedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan reconciliation suggestion: %w", err)
		}
		item.Left = summaries[leftID]
		item.Right = summaries[rightID]
		if sourceID != "" {
			item.Source = &SourceSummary{ID: sourceID, TargetID: sourceTargetID, Name: sourceName, Kind: sourceKind, Origin: sourceOrigin}
		}
		if err := json.Unmarshal(evidenceJSON, &item.Evidence); err != nil {
			return nil, fmt.Errorf("decode reconciliation evidence: %w", err)
		}
		if err := json.Unmarshal(contradictionsJSON, &item.Contradictions); err != nil {
			return nil, fmt.Errorf("decode reconciliation contradictions: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation suggestions: %w", err)
	}
	return items, nil
}

func (store *Store) PreviewTargets(ctx context.Context, primaryID, secondaryID string) (Preview, error) {
	if primaryID == secondaryID || primaryID == "" || secondaryID == "" {
		return Preview{}, fmt.Errorf("%w: choose two different targets", ErrInvalidInput)
	}
	primary, err := store.targetSummary(ctx, primaryID)
	if err != nil {
		return Preview{}, err
	}
	secondary, err := store.targetSummary(ctx, secondaryID)
	if err != nil {
		return Preview{}, err
	}
	conflicts, err := store.incidentConflicts(ctx, primaryID, secondaryID)
	if err != nil {
		return Preview{}, err
	}
	suggested := primary.ID
	if richer(secondary, primary) {
		suggested = secondary.ID
	}
	warnings := make([]string, 0)
	combined := primary.SourceCount + secondary.SourceCount
	if combined > 5 {
		warnings = append(warnings, fmt.Sprintf("La Cible réunira %d Sources, au-delà du gabarit historique de 5.", combined))
	}
	if len(conflicts) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d Incident(s) actif(s) de même Nature seront réunis.", len(conflicts)))
	}
	duplicates, err := store.targetSourceDuplicates(ctx, primaryID, secondaryID)
	if err != nil {
		return Preview{}, err
	}
	if duplicates > 0 {
		warnings = append(warnings, fmt.Sprintf("%d groupe(s) de Sources ressemblantes resteront distincts et devront être vérifiés.", duplicates))
	}
	return Preview{
		Kind: "target_merge", Primary: primary, Secondary: secondary,
		SuggestedPrimaryID: suggested, Conflicts: conflicts,
		CombinedSourceCount: combined, Warnings: warnings,
	}, nil
}

func (store *Store) PreviewSourceMove(ctx context.Context, sourceID, destinationID string) (Preview, error) {
	var source SourceSummary
	err := store.pool.QueryRow(ctx, `
		SELECT source.id::text, source.target_id::text, source.name, source.kind, source.origin
		FROM cairnops_signal_sources source
		JOIN cairnops_targets target ON target.id = source.target_id
		WHERE source.id = $1::uuid AND target.archived_at IS NULL
	`, sourceID).Scan(&source.ID, &source.TargetID, &source.Name, &source.Kind, &source.Origin)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrNotFound
	}
	if err != nil {
		return Preview{}, fmt.Errorf("read source for reassignment: %w", err)
	}
	if source.TargetID == destinationID {
		return Preview{}, fmt.Errorf("%w: source already belongs to that target", ErrInvalidInput)
	}
	destination, err := store.targetSummary(ctx, destinationID)
	if err != nil {
		return Preview{}, err
	}
	origin, err := store.targetSummary(ctx, source.TargetID)
	if err != nil {
		return Preview{}, err
	}
	warnings := []string{"La correction est rétroactive : Observations, agrégats et preuves attribuables à la Source seront déplacés."}
	if origin.SourceCount == 1 {
		warnings = append(warnings, "La Cible d’origine n’aura plus aucune Source.")
	}
	var duplicate bool
	if err := store.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM cairnops_signal_sources moving
			JOIN cairnops_signal_sources candidate
			  ON candidate.target_id = $2::uuid
			 AND candidate.kind = moving.kind
			 AND lower(btrim(candidate.name)) = lower(btrim(moving.name))
			WHERE moving.id = $1::uuid
		)
	`, sourceID, destinationID).Scan(&duplicate); err != nil {
		return Preview{}, fmt.Errorf("check duplicate Source after reassignment: %w", err)
	}
	if duplicate {
		warnings = append(warnings, "Une Source ressemblante existe déjà sur la Cible de destination ; les deux resteront distinctes.")
	}
	return Preview{
		Kind: "source_move", Primary: destination, Secondary: origin,
		SuggestedPrimaryID: destination.ID, CombinedSourceCount: destination.SourceCount + 1,
		Warnings: warnings, Source: &source,
	}, nil
}

func (store *Store) targetSourceDuplicates(ctx context.Context, leftID, rightID string) (int, error) {
	var duplicates int
	err := store.pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM (
			SELECT source.kind, lower(btrim(source.name))
			FROM cairnops_signal_sources source
			WHERE source.target_id IN ($1::uuid, $2::uuid)
			GROUP BY source.kind, lower(btrim(source.name))
			HAVING count(*) > 1
		) duplicate
	`, leftID, rightID).Scan(&duplicates)
	if err != nil {
		return 0, fmt.Errorf("check duplicate Sources after reconciliation: %w", err)
	}
	return duplicates, nil
}

func (store *Store) Enqueue(ctx context.Context, actorID string, input EnqueueInput) (Operation, error) {
	input.Kind = strings.TrimSpace(input.Kind)
	input.Reason = strings.TrimSpace(input.Reason)
	if utf8.RuneCountInString(input.Reason) < 3 || utf8.RuneCountInString(input.Reason) > 1000 {
		return Operation{}, fmt.Errorf("%w: reason must contain between 3 and 1000 characters", ErrInvalidInput)
	}
	var preview Preview
	var err error
	switch input.Kind {
	case "target_merge":
		preview, err = store.PreviewTargets(ctx, input.PrimaryTargetID, input.SecondaryTargetID)
	case "source_move":
		preview, err = store.PreviewSourceMove(ctx, input.SourceID, input.PrimaryTargetID)
		if err == nil && preview.Secondary.ID != input.SecondaryTargetID {
			return Operation{}, fmt.Errorf("%w: source origin changed", ErrConflict)
		}
	default:
		return Operation{}, fmt.Errorf("%w: unknown reconciliation kind", ErrInvalidInput)
	}
	if err != nil {
		return Operation{}, err
	}
	if input.Confirmation != preview.Primary.Name {
		return Operation{}, fmt.Errorf("%w: confirmation must exactly match the surviving target name", ErrInvalidInput)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Operation{}, fmt.Errorf("begin reconciliation enqueue: %w", err)
	}
	defer tx.Rollback(ctx)
	lockIDs := []string{input.PrimaryTargetID, input.SecondaryTargetID}
	if lockIDs[0] > lockIDs[1] {
		lockIDs[0], lockIDs[1] = lockIDs[1], lockIDs[0]
	}
	for _, targetID := range lockIDs {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, targetID); err != nil {
			return Operation{}, fmt.Errorf("lock reconciliation enqueue: %w", err)
		}
	}
	var busy bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM cairnops_target_reconciliation_operations operation
			WHERE operation.status IN ('queued', 'running')
			  AND ($1::uuid IN (operation.primary_target_id, operation.secondary_target_id)
			       OR $2::uuid IN (operation.primary_target_id, operation.secondary_target_id))
		)
	`, input.PrimaryTargetID, input.SecondaryTargetID).Scan(&busy)
	if err != nil {
		return Operation{}, fmt.Errorf("check active reconciliation operation: %w", err)
	}
	if busy {
		return Operation{}, fmt.Errorf("%w: one of the targets already has a reconciliation in progress", ErrConflict)
	}
	encodedPreview, err := json.Marshal(preview)
	if err != nil {
		return Operation{}, fmt.Errorf("encode reconciliation preview: %w", err)
	}
	var operationID string
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO cairnops_target_reconciliation_operations (
			kind, primary_target_id, secondary_target_id, source_id,
			suggestion_id, archive_origin, reason, preview, requested_by
		) VALUES ($1, $2::uuid, $3::uuid, nullif($4, '')::uuid,
		          nullif($5, '')::uuid, $6, $7, $8::jsonb, $9::uuid)
		RETURNING id::text, created_at, updated_at
	`, input.Kind, input.PrimaryTargetID, input.SecondaryTargetID, input.SourceID,
		input.SuggestionID, input.ArchiveOrigin, input.Reason, encodedPreview, actorID,
	).Scan(&operationID, &createdAt, &updatedAt)
	if err != nil {
		return Operation{}, fmt.Errorf("enqueue reconciliation operation: %w", err)
	}
	if input.SuggestionID != "" {
		result, err := tx.Exec(ctx, `
			UPDATE cairnops_target_reconciliation_suggestions
			SET status = 'accepted', decided_by = $2::uuid, decided_at = now(),
			    decision_reason = $3, updated_at = now()
			WHERE id = $1::uuid AND kind = $4
			  AND status IN ('pending', 'snoozed')
			  AND left_target_id IN ($5::uuid, $6::uuid)
			  AND right_target_id IN ($5::uuid, $6::uuid)
			  AND coalesce(source_id::text, '') = $7
		`, input.SuggestionID, actorID, input.Reason, input.Kind,
			input.PrimaryTargetID, input.SecondaryTargetID, input.SourceID)
		if err != nil {
			return Operation{}, fmt.Errorf("accept reconciliation suggestion: %w", err)
		}
		if result.RowsAffected() != 1 {
			return Operation{}, fmt.Errorf("%w: suggestion no longer matches this operation", ErrConflict)
		}
	}
	activityData, _ := json.Marshal(map[string]any{
		"operation_id": operationID, "kind": input.Kind, "source_id": input.SourceID,
		"primary_target_id": input.PrimaryTargetID, "secondary_target_id": input.SecondaryTargetID,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_target_activity (target_id, kind, actor_id, message, data)
		VALUES ($1::uuid, 'reconciliation_started', $3::uuid, $4, $5::jsonb),
		       ($2::uuid, 'reconciliation_started', $3::uuid, $4, $5::jsonb)
	`, input.PrimaryTargetID, input.SecondaryTargetID, actorID, input.Reason, activityData); err != nil {
		return Operation{}, fmt.Errorf("record reconciliation start: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, fmt.Errorf("commit reconciliation enqueue: %w", err)
	}
	return Operation{
		ID: operationID, Kind: input.Kind,
		PrimaryTargetID: input.PrimaryTargetID, PrimaryTargetName: preview.Primary.Name,
		SecondaryTargetID: input.SecondaryTargetID, SecondaryTargetName: preview.Secondary.Name,
		SourceID: input.SourceID, SuggestionID: input.SuggestionID,
		ArchiveOrigin: input.ArchiveOrigin, Reason: input.Reason,
		Status: "queued", Stage: "preparing", Preview: anyMap(encodedPreview), Result: map[string]any{},
		RequestedBy: actorID, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}

func (store *Store) ListOperations(ctx context.Context, limit int) ([]Operation, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := store.pool.Query(ctx, `
		SELECT operation.id::text, operation.kind,
		       operation.primary_target_id::text, primary_target.name,
		       operation.secondary_target_id::text, secondary_target.name,
		       coalesce(operation.source_id::text, ''), coalesce(operation.suggestion_id::text, ''),
		       operation.archive_origin, operation.reason, operation.status, operation.stage,
		       operation.preview, operation.result, operation.last_error, operation.attempts,
		       coalesce(operation.requested_by::text, ''), operation.started_at,
		       operation.completed_at, operation.created_at, operation.updated_at
		FROM cairnops_target_reconciliation_operations operation
		JOIN cairnops_targets primary_target ON primary_target.id = operation.primary_target_id
		JOIN cairnops_targets secondary_target ON secondary_target.id = operation.secondary_target_id
		ORDER BY operation.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation operations: %w", err)
	}
	defer rows.Close()
	items := make([]Operation, 0)
	for rows.Next() {
		var item Operation
		var previewJSON, resultJSON []byte
		if err := rows.Scan(
			&item.ID, &item.Kind, &item.PrimaryTargetID, &item.PrimaryTargetName,
			&item.SecondaryTargetID, &item.SecondaryTargetName, &item.SourceID,
			&item.SuggestionID, &item.ArchiveOrigin, &item.Reason, &item.Status,
			&item.Stage, &previewJSON, &resultJSON, &item.LastError, &item.Attempts,
			&item.RequestedBy, &item.StartedAt, &item.CompletedAt,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan reconciliation operation: %w", err)
		}
		item.Preview, item.Result = anyMap(previewJSON), anyMap(resultJSON)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation operations: %w", err)
	}
	return items, nil
}

func (store *Store) RejectSuggestion(ctx context.Context, actorID, suggestionID, reason string) (Suggestion, error) {
	return store.decideSuggestion(ctx, actorID, suggestionID, "rejected", nil, strings.TrimSpace(reason))
}

func (store *Store) SnoozeSuggestion(ctx context.Context, actorID, suggestionID string, input SnoozeInput) (Suggestion, error) {
	until := input.Until.UTC()
	if until.Before(time.Now().UTC().Add(time.Hour)) || until.After(time.Now().UTC().Add(366*24*time.Hour)) {
		return Suggestion{}, fmt.Errorf("%w: snooze date must be between one hour and one year", ErrInvalidInput)
	}
	return store.decideSuggestion(ctx, actorID, suggestionID, "snoozed", &until, strings.TrimSpace(input.Reason))
}

func (store *Store) decideSuggestion(ctx context.Context, actorID, suggestionID, status string, until *time.Time, reason string) (Suggestion, error) {
	var targetID string
	err := store.pool.QueryRow(ctx, `
		UPDATE cairnops_target_reconciliation_suggestions
		SET status = $2, snoozed_until = $3, decision_reason = $4,
		    decided_by = $5::uuid, decided_at = now(), updated_at = now()
		WHERE id = $1::uuid AND status IN ('pending', 'snoozed')
		RETURNING left_target_id::text
	`, suggestionID, status, until, reason, actorID).Scan(&targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Suggestion{}, ErrNotFound
	}
	if err != nil {
		return Suggestion{}, fmt.Errorf("decide reconciliation suggestion: %w", err)
	}
	data, _ := json.Marshal(map[string]any{"suggestion_id": suggestionID, "status": status, "until": until})
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO cairnops_target_activity (target_id, kind, actor_id, message, data)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5::jsonb)
	`, targetID, "suggestion_"+status, actorID, reason, data); err != nil {
		return Suggestion{}, fmt.Errorf("record suggestion decision: %w", err)
	}
	items, err := store.ListSuggestions(ctx, status)
	if err != nil {
		return Suggestion{}, err
	}
	for _, item := range items {
		if item.ID == suggestionID {
			return item, nil
		}
	}
	return Suggestion{}, ErrNotFound
}

func (store *Store) ResolveTarget(ctx context.Context, targetID string) (string, error) {
	var resolved string
	err := store.pool.QueryRow(ctx, `
		WITH RECURSIVE resolution AS (
			SELECT id, reconciled_into_target_id, 0 AS depth
			FROM cairnops_targets WHERE id = $1::uuid
			UNION ALL
			SELECT target.id, target.reconciled_into_target_id, resolution.depth + 1
			FROM resolution
			JOIN cairnops_targets target ON target.id = resolution.reconciled_into_target_id
			WHERE resolution.depth < 20
		)
		SELECT id::text FROM resolution ORDER BY depth DESC LIMIT 1
	`, targetID).Scan(&resolved)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve reconciled target: %w", err)
	}
	return resolved, nil
}

func (store *Store) ListTargetActivity(ctx context.Context, targetID string, limit int) ([]TargetActivity, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("%w: activity limit must be between 1 and 100", ErrInvalidInput)
	}
	rows, err := store.pool.Query(ctx, `
		SELECT activity.id, activity.target_id::text, activity.kind,
		       coalesce(actor.display_name, ''), activity.message, activity.data, activity.occurred_at
		FROM cairnops_target_activity activity
		LEFT JOIN cairnops_users actor ON actor.id = activity.actor_id
		WHERE activity.target_id = $1::uuid
		ORDER BY activity.occurred_at DESC, activity.id DESC
		LIMIT $2
	`, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("list Target reconciliation activity: %w", err)
	}
	defer rows.Close()
	items := make([]TargetActivity, 0)
	for rows.Next() {
		var item TargetActivity
		if err := rows.Scan(&item.ID, &item.TargetID, &item.Kind, &item.ActorName, &item.Message, &item.Data, &item.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan Target reconciliation activity: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (store *Store) targetSummaries(ctx context.Context) (map[string]TargetSummary, error) {
	rows, err := store.pool.Query(ctx, summarySelect+` ORDER BY lower(target.name), target.id`)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation target summaries: %w", err)
	}
	defer rows.Close()
	items := make(map[string]TargetSummary)
	for rows.Next() {
		item, err := scanTargetSummary(rows)
		if err != nil {
			return nil, err
		}
		items[item.ID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation target summaries: %w", err)
	}
	return items, nil
}

func (store *Store) targetSummary(ctx context.Context, targetID string) (TargetSummary, error) {
	item, err := scanTargetSummary(store.pool.QueryRow(ctx, summarySelect+` AND target.id = $1::uuid`, targetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return TargetSummary{}, ErrNotFound
	}
	return item, err
}

const summarySelect = `
	SELECT target.id::text, target.name, target.description, target.created_at,
	       target.identity_managed_at IS NOT NULL,
	       (SELECT count(*)::integer FROM cairnops_signal_sources source WHERE source.target_id = target.id),
	       (SELECT count(*)::integer FROM cairnops_incident_impacts impact WHERE impact.target_id = target.id),
	       (SELECT count(*)::integer FROM cairnops_incident_impacts impact
	        JOIN cairnops_incidents incident ON incident.id = impact.incident_id
	        WHERE impact.target_id = target.id AND incident.status = 'active'),
	       coalesce((
	         SELECT sum(hour.healthy + hour.unhealthy + hour.unknown)::bigint
	         FROM cairnops_observation_hours hour WHERE hour.target_id = target.id
	       ), 0) + (
	         SELECT count(*)::bigint
	         FROM cairnops_observations observation
	         WHERE observation.target_id = target.id
	           AND observation.observed_at > coalesce(
	             (SELECT consolidated_through FROM cairnops_observation_rollup_state WHERE id),
	             '-infinity'::timestamptz
	           )
	       ),
	       (SELECT count(*)::integer FROM cairnops_maintenance_targets maintenance WHERE maintenance.target_id = target.id),
	       (SELECT count(*)::integer FROM cairnops_context_indicators indicator WHERE indicator.target_id = target.id)
	FROM cairnops_targets target
	WHERE target.archived_at IS NULL AND target.reconciled_into_target_id IS NULL`

type rowScanner interface{ Scan(...any) error }

func scanTargetSummary(row rowScanner) (TargetSummary, error) {
	var item TargetSummary
	err := row.Scan(
		&item.ID, &item.Name, &item.Description, &item.CreatedAt, &item.HumanManaged,
		&item.SourceCount, &item.IncidentCount, &item.ActiveIncidentCount,
		&item.ObservationCount, &item.MaintenanceCount, &item.IndicatorCount,
	)
	if err != nil {
		return TargetSummary{}, err
	}
	item.RichnessScore = item.SourceCount*100 + item.IncidentCount*25 + item.MaintenanceCount*20 + item.IndicatorCount*10
	if item.Description != "" {
		item.RichnessScore += 250
	}
	if item.HumanManaged {
		item.RichnessScore += 10_000
	}
	return item, nil
}

func richer(left, right TargetSummary) bool {
	if left.RichnessScore != right.RichnessScore {
		return left.RichnessScore > right.RichnessScore
	}
	return left.CreatedAt.Before(right.CreatedAt)
}

func (store *Store) incidentConflicts(ctx context.Context, leftID, rightID string) ([]IncidentConflict, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT left_incident.nature_key, left_incident.nature_label,
		       left_incident.id::text, right_incident.id::text
		FROM cairnops_incidents left_incident
		JOIN cairnops_incident_impacts left_impact ON left_impact.incident_id = left_incident.id
		JOIN cairnops_incidents right_incident
		  ON right_incident.status = 'active'
		 AND right_incident.nature_key = left_incident.nature_key
		JOIN cairnops_incident_impacts right_impact ON right_impact.incident_id = right_incident.id
		WHERE left_impact.target_id = $1::uuid AND right_impact.target_id = $2::uuid
		  AND left_incident.status = 'active' AND left_incident.id <> right_incident.id
		ORDER BY left_incident.opened_at, left_incident.id
	`, leftID, rightID)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation incident conflicts: %w", err)
	}
	defer rows.Close()
	items := make([]IncidentConflict, 0)
	for rows.Next() {
		var item IncidentConflict
		if err := rows.Scan(&item.NatureKey, &item.NatureLabel, &item.LeftIncident, &item.RightIncident); err != nil {
			return nil, fmt.Errorf("scan reconciliation incident conflict: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func anyMap(encoded []byte) map[string]any {
	value := make(map[string]any)
	_ = json.Unmarshal(encoded, &value)
	return value
}
