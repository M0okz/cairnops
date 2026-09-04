package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (store *PostgresStore) List(ctx context.Context, status string, limit int) ([]Incident, error) {
	return store.list(ctx, status, "", limit)
}

func (store *PostgresStore) ListForTarget(ctx context.Context, status, targetID string, limit int) ([]Incident, error) {
	return store.list(ctx, status, targetID, limit)
}

func (store *PostgresStore) list(ctx context.Context, status, targetID string, limit int) ([]Incident, error) {
	if status == "all" {
		status = ""
	}
	rows, err := store.pool.Query(ctx, `
		SELECT incident.id::text, incident.nature_key, incident.nature_label,
		       incident.nature_scope, incident.nature_namespace,
		       incident.nature_fingerprint, incident.propagation_eligible,
		       incident.status, incident.propagation_status, incident.severity,
		       incident.opened_at, incident.last_impact_at,
		       incident.propagation_window_seconds, incident.propagation_ends_at,
		       incident.propagation_closed_at, incident.resolved_at,
		       incident.acknowledged_at, coalesce(account.display_name, ''),
		       coalesce(incident.acknowledgement_origin, ''),
		       incident.acknowledgement_sync_status,
		       incident.acknowledgement_sync_error, incident.extended,
		       incident.active_impact_count, incident.impact_count,
		       incident.affected_target_count, incident.max_affected_targets,
		       incident.revision, incident.created_at, incident.updated_at
		FROM cairnops_incidents incident
		LEFT JOIN cairnops_users account ON account.id = incident.acknowledged_by
		WHERE ($1 = '' OR incident.status = $1)
		  AND ($2 = '' OR EXISTS (
		      SELECT 1 FROM cairnops_incident_impacts impact
		      WHERE impact.incident_id = incident.id AND impact.target_id = $2::uuid
		  ))
		ORDER BY
		  CASE WHEN incident.status = 'active' THEN 0 ELSE 1 END,
		  CASE WHEN incident.status = 'active' THEN incident.opened_at END DESC,
		  incident.resolved_at DESC NULLS LAST,
		  incident.opened_at DESC, incident.id
		LIMIT $3
	`, status, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	items := make([]Incident, 0)
	for rows.Next() {
		item, err := scanIncident(rows)
		if err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	if err := store.loadChildren(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (store *PostgresStore) Get(ctx context.Context, incidentID string) (Incident, error) {
	item, err := scanIncident(store.pool.QueryRow(ctx, `
		SELECT incident.id::text, incident.nature_key, incident.nature_label,
		       incident.nature_scope, incident.nature_namespace,
		       incident.nature_fingerprint, incident.propagation_eligible,
		       incident.status, incident.propagation_status, incident.severity,
		       incident.opened_at, incident.last_impact_at,
		       incident.propagation_window_seconds, incident.propagation_ends_at,
		       incident.propagation_closed_at, incident.resolved_at,
		       incident.acknowledged_at, coalesce(account.display_name, ''),
		       coalesce(incident.acknowledgement_origin, ''),
		       incident.acknowledgement_sync_status,
		       incident.acknowledgement_sync_error, incident.extended,
		       incident.active_impact_count, incident.impact_count,
		       incident.affected_target_count, incident.max_affected_targets,
		       incident.revision, incident.created_at, incident.updated_at
		FROM cairnops_incidents incident
		LEFT JOIN cairnops_users account ON account.id = incident.acknowledged_by
		WHERE incident.id = $1::uuid
	`, incidentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	if err != nil {
		return Incident{}, fmt.Errorf("get incident: %w", err)
	}
	items := []Incident{item}
	if err := store.loadChildren(ctx, items); err != nil {
		return Incident{}, err
	}
	return items[0], nil
}

func (store *PostgresStore) OpenedByDay(ctx context.Context, days int) ([]OpenedDay, error) {
	rows, err := store.pool.Query(ctx, `
		WITH requested_days AS (
			SELECT generate_series(
				date_trunc('day', now() AT TIME ZONE 'UTC') - (($1::integer - 1) * interval '1 day'),
				date_trunc('day', now() AT TIME ZONE 'UTC'), interval '1 day'
			)::date AS day
		), opened AS (
			SELECT (opened_at AT TIME ZONE 'UTC')::date AS day, count(*)::integer AS count
			FROM cairnops_incidents
			WHERE opened_at >= date_trunc('day', now() AT TIME ZONE 'UTC') - (($1::integer - 1) * interval '1 day')
			GROUP BY 1
		)
		SELECT requested_days.day, coalesce(opened.count, 0)
		FROM requested_days LEFT JOIN opened USING (day)
		ORDER BY requested_days.day
	`, days)
	if err != nil {
		return nil, fmt.Errorf("list incident history: %w", err)
	}
	defer rows.Close()
	result := make([]OpenedDay, 0, days)
	for rows.Next() {
		var item OpenedDay
		if err := rows.Scan(&item.Day, &item.Opened); err != nil {
			return nil, fmt.Errorf("scan incident history: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *PostgresStore) loadChildren(ctx context.Context, incidents []Incident) error {
	if len(incidents) == 0 {
		return nil
	}
	ids := make([]string, 0, len(incidents))
	byID := make(map[string]*Incident, len(incidents))
	for index := range incidents {
		ids = append(ids, incidents[index].ID)
		byID[incidents[index].ID] = &incidents[index]
	}

	rows, err := store.pool.Query(ctx, `
		SELECT impact.id::text, impact.incident_id::text, impact.target_id::text,
		       target.name, impact.status, impact.source_severity,
		       impact.effective_severity, impact.opened_at, impact.resolved_at,
		       EXISTS (
		           SELECT 1 FROM cairnops_maintenances maintenance
		           JOIN cairnops_maintenance_targets membership ON membership.maintenance_id = maintenance.id
		           WHERE membership.target_id = impact.target_id
		             AND maintenance.cancelled_at IS NULL
		             AND maintenance.starts_at <= now() AND maintenance.ends_at > now()
		       ), (
		           SELECT min(maintenance.ends_at) FROM cairnops_maintenances maintenance
		           JOIN cairnops_maintenance_targets membership ON membership.maintenance_id = maintenance.id
		           WHERE membership.target_id = impact.target_id
		             AND maintenance.cancelled_at IS NULL
		             AND maintenance.starts_at <= now() AND maintenance.ends_at > now()
		       ), impact.created_at, impact.updated_at
		FROM cairnops_incident_impacts impact
		JOIN cairnops_targets target ON target.id = impact.target_id
		WHERE impact.incident_id = ANY($1::uuid[])
		ORDER BY impact.opened_at, impact.id
	`, ids)
	if err != nil {
		return fmt.Errorf("load incident impacts: %w", err)
	}
	impacts := make(map[string]*Impact)
	for rows.Next() {
		var incidentID string
		var impact Impact
		if err := rows.Scan(
			&impact.ID, &incidentID, &impact.TargetID, &impact.TargetName,
			&impact.Status, &impact.SourceSeverity, &impact.EffectiveSeverity,
			&impact.OpenedAt, &impact.ResolvedAt, &impact.MaintenanceActive,
			&impact.MaintenanceEndsAt, &impact.CreatedAt, &impact.UpdatedAt,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan incident impact: %w", err)
		}
		impact.Evidence = []Evidence{}
		parent := byID[incidentID]
		parent.Impacts = append(parent.Impacts, impact)
		impacts[impact.ID] = &parent.Impacts[len(parent.Impacts)-1]
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate incident impacts: %w", err)
	}
	rows.Close()

	rows, err = store.pool.Query(ctx, `
		SELECT evidence.id::text, evidence.impact_id::text, evidence.target_id::text,
		       evidence.origin, coalesce(evidence.connector_id::text, ''),
		       coalesce(connector.name, ''), evidence.external_event_id,
		       evidence.external_object_id, evidence.name, evidence.active,
		       evidence.severity, evidence.opened_at, evidence.resolved_at,
		       evidence.upstream_acknowledged,
		       evidence.acknowledgement_sync_status,
		       evidence.acknowledgement_sync_error,
		       evidence.acknowledgement_synced_at, evidence.invalidated_at,
		       coalesce(account.display_name, ''), evidence.invalidation_reason,
		       evidence.rearmed_at
		FROM cairnops_incident_evidence evidence
		LEFT JOIN cairnops_connectors connector ON connector.id = evidence.connector_id
		LEFT JOIN cairnops_users account ON account.id = evidence.invalidated_by
		WHERE evidence.incident_id = ANY($1::uuid[])
		ORDER BY evidence.opened_at, evidence.id
	`, ids)
	if err != nil {
		return fmt.Errorf("load incident evidence: %w", err)
	}
	for rows.Next() {
		var impactID string
		var evidence Evidence
		if err := rows.Scan(
			&evidence.ID, &impactID, &evidence.TargetID, &evidence.Origin,
			&evidence.ConnectorID, &evidence.ConnectorName,
			&evidence.ExternalEventID, &evidence.ExternalObjectID,
			&evidence.Name, &evidence.Active, &evidence.Severity,
			&evidence.OpenedAt, &evidence.ResolvedAt,
			&evidence.UpstreamAcknowledged,
			&evidence.AcknowledgementSyncStatus,
			&evidence.AcknowledgementSyncError,
			&evidence.AcknowledgementSyncedAt, &evidence.InvalidatedAt,
			&evidence.InvalidatedBy, &evidence.InvalidationReason,
			&evidence.RearmedAt,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan incident evidence: %w", err)
		}
		evidence.ImpactID = impactID
		if impact := impacts[impactID]; impact != nil {
			impact.Evidence = append(impact.Evidence, evidence)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate incident evidence: %w", err)
	}
	rows.Close()

	rows, err = store.pool.Query(ctx, `
		SELECT activity.id, activity.incident_id::text,
		       coalesce(activity.impact_id::text, ''),
		       coalesce(activity.evidence_id::text, ''), activity.kind,
		       activity.origin, coalesce(account.display_name, ''),
		       activity.message, activity.data, activity.occurred_at
		FROM cairnops_incident_activity activity
		LEFT JOIN cairnops_users account ON account.id = activity.actor_id
		WHERE activity.incident_id = ANY($1::uuid[])
		ORDER BY activity.occurred_at, activity.id
	`, ids)
	if err != nil {
		return fmt.Errorf("load incident activity: %w", err)
	}
	for rows.Next() {
		var incidentID string
		var activity Activity
		var raw []byte
		if err := rows.Scan(
			&activity.ID, &incidentID, &activity.ImpactID, &activity.EvidenceID,
			&activity.Kind, &activity.Origin, &activity.ActorName,
			&activity.Message, &raw, &activity.OccurredAt,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan incident activity: %w", err)
		}
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &activity.Data); err != nil {
				rows.Close()
				return fmt.Errorf("decode incident activity: %w", err)
			}
		}
		if activity.Data == nil {
			activity.Data = map[string]any{}
		}
		if parent := byID[incidentID]; parent != nil {
			parent.Activity = append(parent.Activity, activity)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate incident activity: %w", err)
	}
	rows.Close()
	return nil
}

func (store *PostgresStore) InvalidateEvidence(ctx context.Context, incidentID, evidenceID, actorID, actorName, reason string) (Incident, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Incident{}, err
	}
	defer tx.Rollback(ctx)

	var impactID, origin, name string
	err = tx.QueryRow(ctx, `
		SELECT impact_id::text, origin, name
		FROM cairnops_incident_evidence
		WHERE id = $1::uuid AND incident_id = $2::uuid AND active
		FOR UPDATE
	`, evidenceID, incidentID).Scan(&impactID, &origin, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	if err != nil {
		return Incident{}, fmt.Errorf("lock incident evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_evidence
		SET active = false, resolved_at = now(), invalidated_at = now(),
		    invalidated_by = $2::uuid, invalidation_reason = $3, updated_at = now()
		WHERE id = $1::uuid
	`, evidenceID, actorID, reason); err != nil {
		return Incident{}, fmt.Errorf("invalidate incident evidence: %w", err)
	}
	if err := appendActivity(ctx, tx, incidentID, impactID, evidenceID, "invalidated", "user", actorID,
		name, map[string]any{"reason": reason, "actor_name": actorName, "origin": origin}); err != nil {
		return Incident{}, err
	}
	if err := recomputeImpact(ctx, tx, impactID, time.Now().UTC()); err != nil {
		return Incident{}, err
	}
	if err := recomputeIncident(ctx, tx, incidentID, time.Now().UTC()); err != nil {
		return Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Incident{}, err
	}
	return store.Get(ctx, incidentID)
}

func (store *PostgresStore) AcknowledgeLocal(ctx context.Context, incidentID, actorID, actorName string) (AcknowledgementPlan, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return AcknowledgementPlan{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	var alreadyAcknowledged bool
	if err := tx.QueryRow(ctx, `
		SELECT status, acknowledged_at IS NOT NULL FROM cairnops_incidents
		WHERE id = $1::uuid FOR UPDATE
	`, incidentID).Scan(&status, &alreadyAcknowledged); errors.Is(err, pgx.ErrNoRows) {
		return AcknowledgementPlan{}, ErrNotFound
	} else if err != nil {
		return AcknowledgementPlan{}, fmt.Errorf("lock incident: %w", err)
	}
	if status == "resolved" {
		return AcknowledgementPlan{}, fmt.Errorf("%w: resolved incident cannot be acknowledged", ErrConflict)
	}
	if !alreadyAcknowledged {
		var hasExternal bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM cairnops_incident_evidence
				WHERE incident_id = $1::uuid AND active AND origin = 'zabbix'
			)
		`, incidentID).Scan(&hasExternal); err != nil {
			return AcknowledgementPlan{}, fmt.Errorf("inspect acknowledgement targets: %w", err)
		}
		syncStatus := "not_applicable"
		if hasExternal {
			syncStatus = "pending"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incidents
			SET acknowledged_at = now(), acknowledged_by = $2::uuid,
			    acknowledgement_origin = 'user', acknowledgement_sync_status = $3,
			    acknowledgement_sync_error = '', revision = revision + 1, updated_at = now()
			WHERE id = $1::uuid
		`, incidentID, actorID, syncStatus); err != nil {
			return AcknowledgementPlan{}, fmt.Errorf("acknowledge incident: %w", err)
		}
		if err := appendActivity(ctx, tx, incidentID, "", "", "acknowledged", "user", actorID,
			"Incident acquitté", map[string]any{"actor_name": actorName}); err != nil {
			return AcknowledgementPlan{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET acknowledgement_sync_status = CASE
			        WHEN upstream_acknowledged THEN 'synchronized' ELSE 'pending'
			    END,
			    acknowledgement_sync_error = '',
			    acknowledgement_synced_at = CASE
			        WHEN upstream_acknowledged THEN coalesce(acknowledgement_synced_at, now())
			        ELSE NULL
			    END,
			    updated_at = now()
			WHERE incident_id = $1::uuid AND active AND origin = 'zabbix'
		`, incidentID); err != nil {
			return AcknowledgementPlan{}, fmt.Errorf("queue acknowledgement evidence: %w", err)
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, origin, coalesce(connector_id::text, ''), external_event_id
		FROM cairnops_incident_evidence
		WHERE incident_id = $1::uuid AND active AND origin = 'zabbix'
		  AND acknowledgement_sync_status IN ('pending', 'failed')
		ORDER BY connector_id, external_event_id
	`, incidentID)
	if err != nil {
		return AcknowledgementPlan{}, fmt.Errorf("load acknowledgement targets: %w", err)
	}
	targets := make([]AcknowledgementTarget, 0)
	for rows.Next() {
		var target AcknowledgementTarget
		if err := rows.Scan(&target.EvidenceID, &target.Origin, &target.ConnectorID, &target.ExternalEventID); err != nil {
			rows.Close()
			return AcknowledgementPlan{}, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AcknowledgementPlan{}, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return AcknowledgementPlan{}, err
	}
	incident, err := store.Get(ctx, incidentID)
	return AcknowledgementPlan{Incident: incident, Targets: targets}, err
}

func (store *PostgresStore) CompleteAcknowledgement(ctx context.Context, incidentID string, results []AcknowledgementResult) (Incident, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Incident{}, err
	}
	defer tx.Rollback(ctx)
	for _, result := range results {
		syncError := strings.TrimSpace(result.Error)
		if len(syncError) > 500 {
			syncError = syncError[:500]
		}
		status, kind := "synchronized", "ack_sync_succeeded"
		if syncError != "" {
			status, kind = "failed", "ack_sync_failed"
		}
		command, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET acknowledgement_sync_status = $3,
			    acknowledgement_sync_error = $4,
			    acknowledgement_synced_at = CASE WHEN $3 = 'synchronized' THEN now() ELSE NULL END,
			    upstream_acknowledged = upstream_acknowledged OR $3 = 'synchronized',
			    updated_at = now()
			WHERE id = $1::uuid AND incident_id = $2::uuid
			  AND active AND origin = 'zabbix'
		`, result.EvidenceID, incidentID, status, syncError)
		if err != nil {
			return Incident{}, fmt.Errorf("complete evidence acknowledgement: %w", err)
		}
		if command.RowsAffected() == 0 {
			continue
		}
		if err := appendActivity(ctx, tx, incidentID, "", result.EvidenceID,
			kind, "cairnops", "", syncError, nil); err != nil {
			return Incident{}, err
		}
	}

	if err := recomputeAcknowledgement(ctx, tx, incidentID); err != nil {
		return Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Incident{}, err
	}
	return store.Get(ctx, incidentID)
}

func appendActivity(ctx context.Context, tx pgx.Tx, incidentID, impactID, evidenceID, kind, origin, actorID, message string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode incident activity: %w", err)
	}
	var impactValue, evidenceValue, actorValue any
	if strings.TrimSpace(impactID) != "" {
		impactValue = impactID
	}
	if strings.TrimSpace(evidenceID) != "" {
		evidenceValue = evidenceID
	}
	if strings.TrimSpace(actorID) != "" {
		actorValue = actorID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_activity (
			incident_id, impact_id, evidence_id, kind, origin,
			actor_id, message, data
		) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6::uuid, $7, $8::jsonb)
	`, incidentID, impactValue, evidenceValue, kind, origin, actorValue, message, payload); err != nil {
		return fmt.Errorf("append incident activity: %w", err)
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scanIncident(row scanner) (Incident, error) {
	var item Incident
	err := row.Scan(
		&item.ID, &item.NatureKey, &item.NatureLabel, &item.NatureScope,
		&item.NatureNamespace, &item.NatureFingerprint,
		&item.PropagationEligible, &item.Status, &item.PropagationStatus,
		&item.Severity, &item.OpenedAt, &item.LastImpactAt,
		&item.PropagationWindowSeconds, &item.PropagationEndsAt,
		&item.PropagationClosedAt, &item.ResolvedAt, &item.AcknowledgedAt,
		&item.AcknowledgedBy, &item.AcknowledgementOrigin,
		&item.AcknowledgementSyncStatus, &item.AcknowledgementSyncError,
		&item.Extended, &item.ActiveImpactCount, &item.ImpactCount,
		&item.AffectedTargetCount, &item.MaxAffectedTargets, &item.Revision,
		&item.CreatedAt, &item.UpdatedAt,
	)
	item.Impacts = []Impact{}
	item.Activity = []Activity{}
	return item, err
}
