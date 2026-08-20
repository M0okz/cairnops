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

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (store *PostgresStore) List(ctx context.Context, status string, limit int) ([]Incident, error) {
	return store.list(ctx, status, "", limit)
}

func (store *PostgresStore) ListForTarget(ctx context.Context, status, targetID string, limit int) ([]Incident, error) {
	return store.list(ctx, status, targetID, limit)
}

func (store *PostgresStore) list(ctx context.Context, status, targetID string, limit int) ([]Incident, error) {
	var filteredTargetID any
	if targetID != "" {
		filteredTargetID = targetID
	}
	rows, err := store.pool.Query(ctx, incidentSelect+`
		WHERE ($1 = 'all' OR incident.status = $1)
		  AND ($3::uuid IS NULL OR incident.target_id = $3::uuid)
		ORDER BY
			CASE incident.status WHEN 'active' THEN 0 ELSE 1 END,
			CASE incident.effective_severity WHEN 'critical' THEN 0 WHEN 'major' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END,
			incident.opened_at DESC, incident.id
		LIMIT $2
	`, status, limit, filteredTargetID)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	items := make([]Incident, 0)
	for rows.Next() {
		incident, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, incident)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	for index := range items {
		if err := store.loadChildren(ctx, &items[index]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// OpenedByDay compte les Incidents ouverts jour par jour sur la fenêtre
// demandée. Les jours sans Incident figurent avec un zéro : une série creuse
// dessinerait un passé plus calme qu'il ne fut, en resserrant les jours
// chargés les uns contre les autres.
//
// Les Cibles archivées en sont exclues, comme elles le sont de la Santé :
// elles ne sont plus supervisées, leur passé n'a pas à peser sur le jour
// présent.
func (store *PostgresStore) OpenedByDay(ctx context.Context, days int) ([]OpenedDay, error) {
	rows, err := store.pool.Query(ctx, `
		WITH horizon AS (
			SELECT date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS today
		),
		calendar AS (
			SELECT generate_series(
				horizon.today - make_interval(days => $1 - 1),
				horizon.today,
				interval '1 day'
			) AS day
			FROM horizon
		)
		SELECT calendar.day,
		       count(incident.id) FILTER (WHERE target.id IS NOT NULL)::integer
		FROM calendar
		LEFT JOIN cairnops_incidents incident
			ON incident.opened_at >= calendar.day
			AND incident.opened_at < calendar.day + interval '1 day'
		LEFT JOIN cairnops_targets target
			ON target.id = incident.target_id AND target.archived_at IS NULL
		GROUP BY calendar.day
		ORDER BY calendar.day
	`, days)
	if err != nil {
		return nil, fmt.Errorf("count incidents by day: %w", err)
	}
	defer rows.Close()

	series := make([]OpenedDay, 0, days)
	for rows.Next() {
		var day OpenedDay
		if err := rows.Scan(&day.Day, &day.Opened); err != nil {
			return nil, fmt.Errorf("scan incident day: %w", err)
		}
		series = append(series, day)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident days: %w", err)
	}
	return series, nil
}

func (store *PostgresStore) Get(ctx context.Context, incidentID string) (Incident, error) {
	incident, err := scanIncident(store.pool.QueryRow(ctx, incidentSelect+` WHERE incident.id = $1::uuid`, incidentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	if err != nil {
		return Incident{}, err
	}
	if err := store.loadChildren(ctx, &incident); err != nil {
		return Incident{}, err
	}
	return incident, nil
}

func (store *PostgresStore) loadChildren(ctx context.Context, incident *Incident) error {
	rows, err := store.pool.Query(ctx, `
		SELECT signal.id::text, signal.origin, coalesce(signal.connector_id::text, ''),
		       coalesce(connector.name, ''), signal.external_event_id, signal.external_object_id,
		       signal.name, signal.active, signal.severity, signal.opened_at,
		       signal.resolved_at, signal.upstream_acknowledged, signal.invalidated_at,
		       coalesce(invalidator.display_name, ''), signal.invalidation_reason, signal.rearmed_at
		FROM cairnops_incident_signals signal
		LEFT JOIN cairnops_connectors connector ON connector.id = signal.connector_id
		LEFT JOIN cairnops_users invalidator ON invalidator.id = signal.invalidated_by
		WHERE signal.incident_id = $1::uuid
		ORDER BY signal.active DESC, signal.opened_at, signal.id
	`, incident.ID)
	if err != nil {
		return fmt.Errorf("list incident signals: %w", err)
	}
	incident.Signals = make([]Signal, 0)
	for rows.Next() {
		var signal Signal
		if err := rows.Scan(
			&signal.ID, &signal.Origin, &signal.ConnectorID, &signal.ConnectorName,
			&signal.ExternalEventID, &signal.ExternalObjectID, &signal.Name, &signal.Active,
			&signal.Severity, &signal.OpenedAt, &signal.ResolvedAt, &signal.UpstreamAcknowledged,
			&signal.InvalidatedAt, &signal.InvalidatedBy, &signal.InvalidationReason, &signal.RearmedAt,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan incident signal: %w", err)
		}
		incident.Signals = append(incident.Signals, signal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate incident signals: %w", err)
	}
	rows.Close()

	activityRows, err := store.pool.Query(ctx, `
		SELECT activity.id, activity.kind, activity.origin,
		       coalesce(actor.display_name, ''), activity.message, activity.data, activity.occurred_at
		FROM cairnops_incident_activity activity
		LEFT JOIN cairnops_users actor ON actor.id = activity.actor_id
		WHERE activity.incident_id = $1::uuid
		ORDER BY activity.occurred_at DESC, activity.id DESC
		LIMIT 30
	`, incident.ID)
	if err != nil {
		return fmt.Errorf("list incident activity: %w", err)
	}
	defer activityRows.Close()
	incident.Activity = make([]Activity, 0)
	for activityRows.Next() {
		var activity Activity
		var rawData []byte
		if err := activityRows.Scan(
			&activity.ID, &activity.Kind, &activity.Origin, &activity.ActorName,
			&activity.Message, &rawData, &activity.OccurredAt,
		); err != nil {
			return fmt.Errorf("scan incident activity: %w", err)
		}
		if err := json.Unmarshal(rawData, &activity.Data); err != nil {
			return fmt.Errorf("decode incident activity: %w", err)
		}
		incident.Activity = append(incident.Activity, activity)
	}
	if err := activityRows.Err(); err != nil {
		return fmt.Errorf("iterate incident activity: %w", err)
	}
	return nil
}

func (store *PostgresStore) InvalidateSignal(ctx context.Context, incidentID, signalID, actorID, actorName, reason string) (Incident, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Incident{}, fmt.Errorf("begin signal invalidation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var name, origin string
	var active bool
	err = tx.QueryRow(ctx, `
		SELECT signal.name, signal.origin, signal.active
		FROM cairnops_incident_signals signal
		JOIN cairnops_incidents incident ON incident.id = signal.incident_id
		WHERE signal.id = $2::uuid AND signal.incident_id = $1::uuid
		FOR UPDATE OF signal
	`, incidentID, signalID).Scan(&name, &origin, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	if err != nil {
		return Incident{}, fmt.Errorf("lock signal for invalidation: %w", err)
	}
	if !active {
		return Incident{}, fmt.Errorf("%w: cette preuve n’est plus active", ErrConflict)
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_signals
		SET active = false, resolved_at = $3, last_seen_at = $3,
		    invalidated_at = $3, invalidated_by = $4::uuid,
		    invalidation_reason = $5, updated_at = now()
		WHERE incident_id = $1::uuid AND id = $2::uuid AND active
	`, incidentID, signalID, now, actorID, reason); err != nil {
		return Incident{}, fmt.Errorf("invalidate incident signal: %w", err)
	}
	message := "Preuve invalidée"
	if strings.TrimSpace(actorName) != "" {
		message += " par " + strings.TrimSpace(actorName)
	}
	if err := insertActivity(ctx, tx, incidentID, "invalidated", "user", actorID, message, map[string]any{
		"signal_id": signalID, "signal_name": name, "signal_origin": origin, "reason": reason,
	}); err != nil {
		return Incident{}, err
	}
	if err := recomputeIncident(ctx, tx, incidentID, now); err != nil {
		return Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Incident{}, fmt.Errorf("commit signal invalidation: %w", err)
	}
	return store.Get(ctx, incidentID)
}

func (store *PostgresStore) AcknowledgeLocal(ctx context.Context, incidentID, actorID, actorName string) (AcknowledgementPlan, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AcknowledgementPlan{}, fmt.Errorf("begin incident acknowledgement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, syncStatus string
	var acknowledgedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT status, acknowledged_at, acknowledgement_sync_status
		FROM cairnops_incidents
		WHERE id = $1::uuid
		FOR UPDATE
	`, incidentID).Scan(&status, &acknowledgedAt, &syncStatus); errors.Is(err, pgx.ErrNoRows) {
		return AcknowledgementPlan{}, ErrNotFound
	} else if err != nil {
		return AcknowledgementPlan{}, fmt.Errorf("lock incident for acknowledgement: %w", err)
	}
	if status != "active" {
		return AcknowledgementPlan{}, fmt.Errorf("%w: a resolved incident cannot be acknowledged", ErrConflict)
	}

	var externalCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM cairnops_incident_signals
		WHERE incident_id = $1::uuid AND active AND origin = 'zabbix'
	`, incidentID).Scan(&externalCount); err != nil {
		return AcknowledgementPlan{}, fmt.Errorf("count acknowledgement targets: %w", err)
	}
	if acknowledgedAt == nil {
		nextSyncStatus := "not_applicable"
		if externalCount > 0 {
			nextSyncStatus = "pending"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incidents
			SET acknowledged_at = now(), acknowledged_by = $2::uuid,
			    acknowledgement_origin = 'user', acknowledgement_sync_status = $3,
			    acknowledgement_sync_error = '', updated_at = now()
			WHERE id = $1::uuid
		`, incidentID, actorID, nextSyncStatus); err != nil {
			return AcknowledgementPlan{}, fmt.Errorf("acknowledge incident: %w", err)
		}
		message := "Incident acquitté"
		if strings.TrimSpace(actorName) != "" {
			message += " par " + strings.TrimSpace(actorName)
		}
		if err := insertActivity(ctx, tx, incidentID, "acknowledged", "user", actorID, message, nil); err != nil {
			return AcknowledgementPlan{}, err
		}
		syncStatus = nextSyncStatus
	}

	targets := make([]AcknowledgementTarget, 0)
	if externalCount > 0 && syncStatus != "synchronized" {
		rows, err := tx.Query(ctx, `
			SELECT origin, connector_id::text, external_event_id
			FROM cairnops_incident_signals
			WHERE incident_id = $1::uuid AND active AND origin = 'zabbix'
			ORDER BY connector_id, external_event_id
		`, incidentID)
		if err != nil {
			return AcknowledgementPlan{}, fmt.Errorf("load acknowledgement targets: %w", err)
		}
		for rows.Next() {
			var target AcknowledgementTarget
			if err := rows.Scan(&target.Origin, &target.ConnectorID, &target.ExternalEventID); err != nil {
				rows.Close()
				return AcknowledgementPlan{}, fmt.Errorf("scan acknowledgement target: %w", err)
			}
			targets = append(targets, target)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return AcknowledgementPlan{}, fmt.Errorf("iterate acknowledgement targets: %w", err)
		}
		rows.Close()
	}
	if err := tx.Commit(ctx); err != nil {
		return AcknowledgementPlan{}, fmt.Errorf("commit incident acknowledgement: %w", err)
	}
	incident, err := store.Get(ctx, incidentID)
	if err != nil {
		return AcknowledgementPlan{}, err
	}
	return AcknowledgementPlan{Incident: incident, Targets: targets}, nil
}

func (store *PostgresStore) CompleteAcknowledgement(ctx context.Context, incidentID, status, syncError string) (Incident, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Incident{}, fmt.Errorf("begin acknowledgement completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents
		SET acknowledgement_sync_status = $2, acknowledgement_sync_error = $3, updated_at = now()
		WHERE id = $1::uuid AND acknowledged_at IS NOT NULL
	`, incidentID, status, syncError)
	if err != nil {
		return Incident{}, fmt.Errorf("complete acknowledgement: %w", err)
	}
	if result.RowsAffected() != 1 {
		return Incident{}, ErrNotFound
	}
	kind, message := "ack_sync_succeeded", "Acquittement synchronisé avec Zabbix"
	if status == "failed" {
		kind, message = "ack_sync_failed", "Acquittement conservé dans CairnOps ; synchronisation Zabbix en échec"
	}
	if err := insertActivity(ctx, tx, incidentID, kind, "cairnops", "", message, map[string]any{"error": syncError}); err != nil {
		return Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Incident{}, fmt.Errorf("commit acknowledgement completion: %w", err)
	}
	return store.Get(ctx, incidentID)
}

func (store *PostgresStore) ReconcileZabbix(ctx context.Context, input ReconcileZabbixInput) error {
	observedAt := input.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin Zabbix incident reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seen := make(map[string]struct{}, len(input.Signals))
	impacted := make(map[string]struct{})
	upstreamAcknowledgements := make(map[string]string)
	for _, signal := range input.Signals {
		key := signal.BindingID + "\x00" + signal.ExternalEventID
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}

		var existingSignalID string
		var invalidated bool
		err := tx.QueryRow(ctx, `
			SELECT id::text, invalidated_at IS NOT NULL
			FROM cairnops_incident_signals
			WHERE origin = 'zabbix' AND connector_id = $1::uuid
			  AND connector_binding_id = $2::uuid AND external_event_id = $3
		`, input.ConnectorID, signal.BindingID, signal.ExternalEventID).Scan(&existingSignalID, &invalidated)
		newSignal := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !newSignal {
			return fmt.Errorf("find Zabbix incident signal: %w", err)
		}
		if invalidated {
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_signals SET last_seen_at = $2, updated_at = now()
				WHERE id = $1::uuid
			`, existingSignalID, observedAt); err != nil {
				return fmt.Errorf("refresh invalidated Zabbix signal: %w", err)
			}
			continue
		}
		incidentID, created, err := ensureActiveIncident(
			ctx, tx, signal.TargetID, "zabbix:trigger:"+signal.ExternalObjectID,
			signal.Name, signal.Severity, signal.OpenedAt,
		)
		if errors.Is(err, ErrTargetArchived) {
			continue
		}
		if err != nil {
			return err
		}
		impacted[incidentID] = struct{}{}
		if created {
			if err := insertActivity(ctx, tx, incidentID, "opened", "zabbix", "", "Incident ouvert depuis un problème Zabbix", map[string]any{"event_id": signal.ExternalEventID}); err != nil {
				return err
			}
		}

		metadata, err := json.Marshal(map[string]any{"suppressed": signal.Suppressed})
		if err != nil {
			return fmt.Errorf("encode Zabbix signal metadata: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_signals (
				incident_id, target_id, origin, connector_id, connector_binding_id,
				external_event_id, external_object_id, name, active, severity,
				opened_at, upstream_acknowledged, last_seen_at, metadata
			) VALUES ($1::uuid, $2::uuid, 'zabbix', $3::uuid, $4::uuid,
			          $5, $6, $7, true, $8, $9, $10, $11, $12::jsonb)
			ON CONFLICT (origin, connector_id, connector_binding_id, external_event_id)
			WHERE connector_id IS NOT NULL
			DO UPDATE SET incident_id = EXCLUDED.incident_id, name = EXCLUDED.name,
				active = true, severity = EXCLUDED.severity, resolved_at = NULL,
				upstream_acknowledged = EXCLUDED.upstream_acknowledged,
				last_seen_at = EXCLUDED.last_seen_at, metadata = EXCLUDED.metadata,
				updated_at = now()
		`, incidentID, signal.TargetID, input.ConnectorID, signal.BindingID,
			signal.ExternalEventID, signal.ExternalObjectID, signal.Name, signal.Severity,
			signal.OpenedAt, signal.UpstreamAcknowledged, observedAt, metadata); err != nil {
			return fmt.Errorf("upsert Zabbix incident signal: %w", err)
		}
		if newSignal {
			if err := insertActivity(ctx, tx, incidentID, "signal_added", "zabbix", "", signal.Name, map[string]any{"event_id": signal.ExternalEventID}); err != nil {
				return err
			}
		}
		if signal.UpstreamAcknowledged {
			upstreamAcknowledgements[incidentID] = signal.ExternalEventID
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, incident_id::text, connector_binding_id::text, external_event_id, name
		FROM cairnops_incident_signals
		WHERE connector_id = $1::uuid AND origin = 'zabbix' AND active
		FOR UPDATE
	`, input.ConnectorID)
	if err != nil {
		return fmt.Errorf("list active Zabbix incident signals: %w", err)
	}
	type activeSignal struct{ id, incidentID, bindingID, eventID, name string }
	active := make([]activeSignal, 0)
	for rows.Next() {
		var signal activeSignal
		if err := rows.Scan(&signal.id, &signal.incidentID, &signal.bindingID, &signal.eventID, &signal.name); err != nil {
			rows.Close()
			return fmt.Errorf("scan active Zabbix incident signal: %w", err)
		}
		active = append(active, signal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate active Zabbix incident signals: %w", err)
	}
	rows.Close()
	for _, signal := range active {
		if _, stillActive := seen[signal.bindingID+"\x00"+signal.eventID]; stillActive {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET active = false, resolved_at = $2, last_seen_at = $2, updated_at = now()
			WHERE id = $1::uuid
		`, signal.id, observedAt); err != nil {
			return fmt.Errorf("resolve Zabbix incident signal: %w", err)
		}
		impacted[signal.incidentID] = struct{}{}
		if err := insertActivity(ctx, tx, signal.incidentID, "signal_resolved", "zabbix", "", signal.name, map[string]any{"event_id": signal.eventID}); err != nil {
			return err
		}
	}
	for incidentID, eventID := range upstreamAcknowledgements {
		if err := acknowledgeFromZabbix(ctx, tx, incidentID, eventID); err != nil {
			return err
		}
	}

	for incidentID := range impacted {
		if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Zabbix incident reconciliation: %w", err)
	}
	return nil
}

func (store *PostgresStore) ReconcileUptimeKuma(ctx context.Context, input ReconcileUptimeKumaInput) error {
	observedAt := input.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin Uptime Kuma incident reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seen := make(map[string]struct{}, len(input.Signals))
	impacted := make(map[string]struct{})
	for _, signal := range input.Signals {
		if _, duplicate := seen[signal.BindingID]; duplicate {
			continue
		}
		seen[signal.BindingID] = struct{}{}

		var signalID, incidentID string
		err := tx.QueryRow(ctx, `
			SELECT id::text, incident_id::text
			FROM cairnops_incident_signals
			WHERE origin = 'uptime_kuma' AND connector_id = $1::uuid
			  AND connector_binding_id = $2::uuid AND active
			FOR UPDATE
		`, input.ConnectorID, signal.BindingID).Scan(&signalID, &incidentID)
		if err == nil {
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_signals
				SET name = $2, severity = $3, last_seen_at = $4, updated_at = now()
				WHERE id = $1::uuid
			`, signalID, signal.Name, signal.Severity, observedAt); err != nil {
				return fmt.Errorf("refresh Uptime Kuma incident signal: %w", err)
			}
			impacted[incidentID] = struct{}{}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find active Uptime Kuma incident signal: %w", err)
		}
		var invalidatedSignalID string
		err = tx.QueryRow(ctx, `
			SELECT id::text
			FROM cairnops_incident_signals
			WHERE origin = 'uptime_kuma' AND connector_id = $1::uuid
			  AND connector_binding_id = $2::uuid
			  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
			ORDER BY invalidated_at DESC
			LIMIT 1
			FOR UPDATE
		`, input.ConnectorID, signal.BindingID).Scan(&invalidatedSignalID)
		if err == nil {
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_signals SET last_seen_at = $2, updated_at = now()
				WHERE id = $1::uuid
			`, invalidatedSignalID, observedAt); err != nil {
				return fmt.Errorf("refresh invalidated Uptime Kuma signal: %w", err)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find invalidated Uptime Kuma signal: %w", err)
		}

		natureKey := "uptime-kuma:monitor:" + signal.ExternalMonitor
		incidentID, created, err := ensureActiveIncident(
			ctx, tx, signal.TargetID, natureKey,
			"Moniteur Uptime Kuma indisponible", signal.Severity, observedAt,
		)
		if errors.Is(err, ErrTargetArchived) {
			continue
		}
		if err != nil {
			return err
		}
		impacted[incidentID] = struct{}{}
		if created {
			if err := insertActivity(ctx, tx, incidentID, "opened", "uptime_kuma", "", "Incident ouvert depuis Uptime Kuma", map[string]any{"monitor_id": signal.ExternalMonitor}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_signals (
				incident_id, target_id, origin, connector_id, connector_binding_id,
				external_event_id, external_object_id, name, active, severity,
				opened_at, upstream_acknowledged, last_seen_at
			) VALUES ($1::uuid, $2::uuid, 'uptime_kuma', $3::uuid, $4::uuid,
			          $5 || ':' || gen_random_uuid()::text, $5, $6, true, $7,
			          $8, false, $8)
		`, incidentID, signal.TargetID, input.ConnectorID, signal.BindingID,
			signal.ExternalMonitor, signal.Name, signal.Severity, observedAt); err != nil {
			return fmt.Errorf("insert Uptime Kuma incident signal: %w", err)
		}
		if err := insertActivity(ctx, tx, incidentID, "signal_added", "uptime_kuma", "", signal.Name, map[string]any{"monitor_id": signal.ExternalMonitor}); err != nil {
			return err
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, incident_id::text, connector_binding_id::text, name, external_object_id
		FROM cairnops_incident_signals
		WHERE connector_id = $1::uuid AND origin = 'uptime_kuma' AND active
		FOR UPDATE
	`, input.ConnectorID)
	if err != nil {
		return fmt.Errorf("list active Uptime Kuma incident signals: %w", err)
	}
	type activeKumaSignal struct{ id, incidentID, bindingID, name, monitorID string }
	active := make([]activeKumaSignal, 0)
	for rows.Next() {
		var signal activeKumaSignal
		if err := rows.Scan(&signal.id, &signal.incidentID, &signal.bindingID, &signal.name, &signal.monitorID); err != nil {
			rows.Close()
			return fmt.Errorf("scan active Uptime Kuma incident signal: %w", err)
		}
		active = append(active, signal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate active Uptime Kuma incident signals: %w", err)
	}
	rows.Close()
	for _, signal := range active {
		if _, stillDown := seen[signal.bindingID]; stillDown {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET active = false, resolved_at = $2, last_seen_at = $2, updated_at = now()
			WHERE id = $1::uuid
		`, signal.id, observedAt); err != nil {
			return fmt.Errorf("resolve Uptime Kuma incident signal: %w", err)
		}
		impacted[signal.incidentID] = struct{}{}
		if err := insertActivity(ctx, tx, signal.incidentID, "signal_resolved", "uptime_kuma", "", signal.name, map[string]any{"monitor_id": signal.monitorID}); err != nil {
			return err
		}
	}

	rearmRows, err := tx.Query(ctx, `
		SELECT id::text, connector_binding_id::text
		FROM cairnops_incident_signals
		WHERE connector_id = $1::uuid AND origin = 'uptime_kuma'
		  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
		FOR UPDATE
	`, input.ConnectorID)
	if err != nil {
		return fmt.Errorf("list invalidated Uptime Kuma signals: %w", err)
	}
	type invalidatedKumaSignal struct{ id, bindingID string }
	invalidatedSignals := make([]invalidatedKumaSignal, 0)
	for rearmRows.Next() {
		var signal invalidatedKumaSignal
		if err := rearmRows.Scan(&signal.id, &signal.bindingID); err != nil {
			rearmRows.Close()
			return fmt.Errorf("scan invalidated Uptime Kuma signal: %w", err)
		}
		invalidatedSignals = append(invalidatedSignals, signal)
	}
	if err := rearmRows.Err(); err != nil {
		rearmRows.Close()
		return fmt.Errorf("iterate invalidated Uptime Kuma signals: %w", err)
	}
	rearmRows.Close()
	for _, signal := range invalidatedSignals {
		if _, stillDown := seen[signal.bindingID]; stillDown {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET rearmed_at = greatest($2, invalidated_at), last_seen_at = $2, updated_at = now()
			WHERE id = $1::uuid AND rearmed_at IS NULL
		`, signal.id, observedAt); err != nil {
			return fmt.Errorf("rearm invalidated Uptime Kuma signal: %w", err)
		}
	}

	for incidentID := range impacted {
		if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Uptime Kuma incident reconciliation: %w", err)
	}
	return nil
}

func (store *PostgresStore) ReconcilePatchMon(ctx context.Context, input ReconcilePatchMonInput) error {
	observedAt := input.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin PatchMon incident reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seen := make(map[string]struct{}, len(input.Signals))
	observedBindings := make(map[string]struct{}, len(input.ObservedBindings))
	for _, bindingID := range input.ObservedBindings {
		observedBindings[bindingID] = struct{}{}
	}
	impacted := make(map[string]struct{})
	for _, signal := range input.Signals {
		key := signal.BindingID + "\x00" + signal.ConditionKey
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		details, err := json.Marshal(signal.Details)
		if err != nil {
			return fmt.Errorf("encode PatchMon signal details: %w", err)
		}

		var signalID, incidentID string
		err = tx.QueryRow(ctx, `
			SELECT id::text, incident_id::text
			FROM cairnops_incident_signals
			WHERE origin = 'patchmon' AND connector_id = $1::uuid
			  AND connector_binding_id = $2::uuid AND external_object_id = $3 AND active
			FOR UPDATE
		`, input.ConnectorID, signal.BindingID, signal.ConditionKey).Scan(&signalID, &incidentID)
		if err == nil {
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_signals
				SET name = $2, severity = $3, metadata = $4::jsonb,
				    last_seen_at = $5, updated_at = now()
				WHERE id = $1::uuid
			`, signalID, signal.Name, signal.Severity, details, observedAt); err != nil {
				return fmt.Errorf("refresh PatchMon incident signal: %w", err)
			}
			impacted[incidentID] = struct{}{}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find active PatchMon incident signal: %w", err)
		}

		var invalidatedSignalID string
		err = tx.QueryRow(ctx, `
			SELECT id::text
			FROM cairnops_incident_signals
			WHERE origin = 'patchmon' AND connector_id = $1::uuid
			  AND connector_binding_id = $2::uuid AND external_object_id = $3
			  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
			ORDER BY invalidated_at DESC
			LIMIT 1
			FOR UPDATE
		`, input.ConnectorID, signal.BindingID, signal.ConditionKey).Scan(&invalidatedSignalID)
		if err == nil {
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_signals
				SET name = $2, severity = $3, metadata = $4::jsonb,
				    last_seen_at = $5, updated_at = now()
				WHERE id = $1::uuid
			`, invalidatedSignalID, signal.Name, signal.Severity, details, observedAt); err != nil {
				return fmt.Errorf("refresh invalidated PatchMon signal: %w", err)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find invalidated PatchMon signal: %w", err)
		}

		incidentID, created, err := ensureActiveIncident(
			ctx, tx, signal.TargetID, signal.NatureKey, signal.NatureLabel, signal.Severity, observedAt,
		)
		if errors.Is(err, ErrTargetArchived) {
			continue
		}
		if err != nil {
			return err
		}
		impacted[incidentID] = struct{}{}
		if created {
			if err := insertActivity(ctx, tx, incidentID, "opened", "patchmon", "", "Incident ouvert depuis PatchMon", signal.Details); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_signals (
				incident_id, target_id, origin, connector_id, connector_binding_id,
				external_event_id, external_object_id, name, active, severity,
				opened_at, upstream_acknowledged, last_seen_at, metadata
			) VALUES ($1::uuid, $2::uuid, 'patchmon', $3::uuid, $4::uuid,
			          $5 || ':' || gen_random_uuid()::text, $5, $6, true, $7,
			          $8, false, $8, $9::jsonb)
		`, incidentID, signal.TargetID, input.ConnectorID, signal.BindingID,
			signal.ConditionKey, signal.Name, signal.Severity, observedAt, details); err != nil {
			return fmt.Errorf("insert PatchMon incident signal: %w", err)
		}
		if err := insertActivity(ctx, tx, incidentID, "signal_added", "patchmon", "", signal.Name, signal.Details); err != nil {
			return err
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, incident_id::text, connector_binding_id::text, name, external_object_id
		FROM cairnops_incident_signals
		WHERE connector_id = $1::uuid AND origin = 'patchmon' AND active
		FOR UPDATE
	`, input.ConnectorID)
	if err != nil {
		return fmt.Errorf("list active PatchMon incident signals: %w", err)
	}
	type activePatchMonSignal struct{ id, incidentID, bindingID, name, conditionKey string }
	active := make([]activePatchMonSignal, 0)
	for rows.Next() {
		var signal activePatchMonSignal
		if err := rows.Scan(&signal.id, &signal.incidentID, &signal.bindingID, &signal.name, &signal.conditionKey); err != nil {
			rows.Close()
			return fmt.Errorf("scan active PatchMon incident signal: %w", err)
		}
		active = append(active, signal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate active PatchMon incident signals: %w", err)
	}
	rows.Close()
	for _, signal := range active {
		if _, observed := observedBindings[signal.bindingID]; !observed {
			continue
		}
		if _, stillActive := seen[signal.bindingID+"\x00"+signal.conditionKey]; stillActive {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET active = false, resolved_at = $2, last_seen_at = $2, updated_at = now()
			WHERE id = $1::uuid
		`, signal.id, observedAt); err != nil {
			return fmt.Errorf("resolve PatchMon incident signal: %w", err)
		}
		impacted[signal.incidentID] = struct{}{}
		if err := insertActivity(ctx, tx, signal.incidentID, "signal_resolved", "patchmon", "", signal.name, map[string]any{"condition": signal.conditionKey}); err != nil {
			return err
		}
	}

	rearmRows, err := tx.Query(ctx, `
		SELECT id::text, connector_binding_id::text, external_object_id
		FROM cairnops_incident_signals
		WHERE connector_id = $1::uuid AND origin = 'patchmon'
		  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
		FOR UPDATE
	`, input.ConnectorID)
	if err != nil {
		return fmt.Errorf("list invalidated PatchMon signals: %w", err)
	}
	type invalidatedPatchMonSignal struct{ id, bindingID, conditionKey string }
	invalidated := make([]invalidatedPatchMonSignal, 0)
	for rearmRows.Next() {
		var signal invalidatedPatchMonSignal
		if err := rearmRows.Scan(&signal.id, &signal.bindingID, &signal.conditionKey); err != nil {
			rearmRows.Close()
			return fmt.Errorf("scan invalidated PatchMon signal: %w", err)
		}
		invalidated = append(invalidated, signal)
	}
	if err := rearmRows.Err(); err != nil {
		rearmRows.Close()
		return fmt.Errorf("iterate invalidated PatchMon signals: %w", err)
	}
	rearmRows.Close()
	for _, signal := range invalidated {
		if _, observed := observedBindings[signal.bindingID]; !observed {
			continue
		}
		if _, stillActive := seen[signal.bindingID+"\x00"+signal.conditionKey]; stillActive {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET rearmed_at = greatest($2, invalidated_at), last_seen_at = $2, updated_at = now()
			WHERE id = $1::uuid AND rearmed_at IS NULL
		`, signal.id, observedAt); err != nil {
			return fmt.Errorf("rearm invalidated PatchMon signal: %w", err)
		}
	}

	for incidentID := range impacted {
		if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PatchMon incident reconciliation: %w", err)
	}
	return nil
}

func (store *PostgresStore) ApplyWebhook(ctx context.Context, signal WebhookSignal) error {
	observedAt := signal.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	details := signal.Details
	if details == nil {
		details = map[string]any{}
	}
	metadata, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode webhook signal details: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin webhook incident update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var signalID, incidentID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, incident_id::text
		FROM cairnops_incident_signals
		WHERE origin = 'webhook' AND connector_id = $1::uuid
		  AND connector_binding_id = $2::uuid AND external_object_id = $3 AND active
		FOR UPDATE
	`, signal.ConnectorID, signal.BindingID, signal.ExternalEventKey).Scan(&signalID, &incidentID)
	activeSignalFound := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find active webhook signal: %w", err)
	}

	if signal.Status == "resolved" {
		if !activeSignalFound {
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_signals
				SET rearmed_at = greatest($4, invalidated_at), last_seen_at = $4, updated_at = now()
				WHERE origin = 'webhook' AND connector_id = $1::uuid
				  AND connector_binding_id = $2::uuid AND external_object_id = $3
				  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
			`, signal.ConnectorID, signal.BindingID, signal.ExternalEventKey, observedAt); err != nil {
				return fmt.Errorf("rearm invalidated webhook signal: %w", err)
			}
			return tx.Commit(ctx)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET active = false, resolved_at = $2, last_seen_at = $2,
			    name = $3, severity = $4, metadata = $5::jsonb, updated_at = now()
			WHERE id = $1::uuid
		`, signalID, observedAt, signal.Summary, signal.Severity, metadata); err != nil {
			return fmt.Errorf("resolve webhook incident signal: %w", err)
		}
		if err := insertActivity(ctx, tx, incidentID, "signal_resolved", "webhook", "", signal.Summary, map[string]any{"event_key": signal.ExternalEventKey}); err != nil {
			return err
		}
		if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit webhook resolution: %w", err)
		}
		return nil
	}

	if activeSignalFound {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET name = $2, severity = $3, last_seen_at = $4,
			    metadata = $5::jsonb, updated_at = now()
			WHERE id = $1::uuid
		`, signalID, signal.Summary, signal.Severity, observedAt, metadata); err != nil {
			return fmt.Errorf("refresh webhook incident signal: %w", err)
		}
		if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit webhook signal refresh: %w", err)
		}
		return nil
	}

	var invalidatedSignalID string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM cairnops_incident_signals
		WHERE origin = 'webhook' AND connector_id = $1::uuid
		  AND connector_binding_id = $2::uuid AND external_object_id = $3
		  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
		ORDER BY invalidated_at DESC
		LIMIT 1
		FOR UPDATE
	`, signal.ConnectorID, signal.BindingID, signal.ExternalEventKey).Scan(&invalidatedSignalID)
	if err == nil {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals SET last_seen_at = $2, updated_at = now()
			WHERE id = $1::uuid
		`, invalidatedSignalID, observedAt); err != nil {
			return fmt.Errorf("refresh invalidated webhook signal: %w", err)
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find invalidated webhook signal: %w", err)
	}

	incidentID, created, err := ensureActiveIncident(
		ctx, tx, signal.TargetID, signal.NatureKey, signal.NatureLabel, signal.Severity, observedAt,
	)
	if errors.Is(err, ErrTargetArchived) {
		return nil
	}
	if err != nil {
		return err
	}
	if created {
		if err := insertActivity(ctx, tx, incidentID, "opened", "webhook", "", "Incident ouvert depuis un webhook autorisé", map[string]any{"event_key": signal.ExternalEventKey}); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_signals (
			incident_id, target_id, origin, connector_id, connector_binding_id,
			external_event_id, external_object_id, name, active, severity,
			opened_at, upstream_acknowledged, last_seen_at, metadata
		) VALUES ($1::uuid, $2::uuid, 'webhook', $3::uuid, $4::uuid,
		          $5 || ':' || gen_random_uuid()::text, $5, $6, true, $7,
		          $8, false, $8, $9::jsonb)
	`, incidentID, signal.TargetID, signal.ConnectorID, signal.BindingID,
		signal.ExternalEventKey, signal.Summary, signal.Severity, observedAt, metadata); err != nil {
		return fmt.Errorf("insert webhook incident signal: %w", err)
	}
	if err := insertActivity(ctx, tx, incidentID, "signal_added", "webhook", "", signal.Summary, map[string]any{"event_key": signal.ExternalEventKey}); err != nil {
		return err
	}
	if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit webhook incident update: %w", err)
	}
	return nil
}

func ensureActiveIncident(ctx context.Context, tx pgx.Tx, targetID, natureKey, natureLabel string, severity Severity, openedAt time.Time) (string, bool, error) {
	var incidentID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM cairnops_incidents
		WHERE target_id = $1::uuid AND nature_key = $2 AND status = 'active'
		FOR UPDATE
	`, targetID, natureKey).Scan(&incidentID)
	if err == nil {
		return incidentID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("find active incident: %w", err)
	}
	// Une Cible archivée a quitté l'Espace opérationnel : aucun signal ne la
	// ressuscite, pas même une Intégration qui continue de publier son état.
	err = tx.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status,
			source_severity, effective_severity, opened_at
		)
		SELECT target.id, $2, $3, 'active', $4, $4, $5
		FROM cairnops_targets target
		WHERE target.id = $1::uuid AND target.archived_at IS NULL
		RETURNING id::text
	`, targetID, natureKey, natureLabel, severity, openedAt).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrTargetArchived
	}
	if err != nil {
		return "", false, fmt.Errorf("open incident from external signal: %w", err)
	}
	return incidentID, true, nil
}

func acknowledgeFromZabbix(ctx context.Context, tx pgx.Tx, incidentID, eventID string) error {
	result, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents
		SET acknowledged_at = coalesce(acknowledged_at, now()),
		    acknowledgement_origin = coalesce(acknowledgement_origin, 'connector'),
		    acknowledgement_sync_status = 'synchronized', acknowledgement_sync_error = '',
		    updated_at = now()
		WHERE id = $1::uuid
		  AND (acknowledged_at IS NULL OR acknowledgement_sync_status <> 'synchronized')
		  AND NOT EXISTS (
		      SELECT 1
		      FROM cairnops_incident_signals signal
		      WHERE signal.incident_id = cairnops_incidents.id
		        AND signal.active AND NOT signal.upstream_acknowledged
		  )
	`, incidentID)
	if err != nil {
		return fmt.Errorf("apply upstream Zabbix acknowledgement: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	return insertActivity(ctx, tx, incidentID, "upstream_acknowledged", "zabbix", "", "Acquittement confirmé par Zabbix", map[string]any{"event_id": eventID})
}

// ResolveForArchivedTarget clôt ce que l'archivage d'une Cible rend sans objet.
//
// Les Incidents actifs sont résolus et leurs preuves closes, mais rien n'est
// effacé : le Journal garde la raison, et l'histoire reste lisible après coup.
func ResolveForArchivedTarget(ctx context.Context, tx pgx.Tx, targetID string) error {
	rows, err := tx.Query(ctx, `
		UPDATE cairnops_incidents
		SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE target_id = $1::uuid AND status = 'active'
		RETURNING id::text
	`, targetID)
	if err != nil {
		return fmt.Errorf("resolve incidents of archived target: %w", err)
	}
	incidentIDs := make([]string, 0)
	for rows.Next() {
		var incidentID string
		if err := rows.Scan(&incidentID); err != nil {
			rows.Close()
			return fmt.Errorf("scan resolved incident: %w", err)
		}
		incidentIDs = append(incidentIDs, incidentID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate resolved incidents: %w", err)
	}

	for _, incidentID := range incidentIDs {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET active = false, resolved_at = coalesce(resolved_at, now()), updated_at = now()
			WHERE incident_id = $1::uuid AND active
		`, incidentID); err != nil {
			return fmt.Errorf("close signals of archived target: %w", err)
		}
		if err := insertActivity(ctx, tx, incidentID, "resolved", "cairnops", "",
			"Incident résolu : la Cible a été archivée", nil); err != nil {
			return err
		}
	}
	return nil
}

func recomputeIncident(ctx context.Context, tx pgx.Tx, incidentID string, observedAt time.Time) error {
	var count int
	var severity *string
	if err := tx.QueryRow(ctx, `
		SELECT count(*)::integer,
		       CASE max(CASE severity WHEN 'information' THEN 1 WHEN 'warning' THEN 2 WHEN 'major' THEN 3 WHEN 'critical' THEN 4 END)
		           WHEN 1 THEN 'information' WHEN 2 THEN 'warning' WHEN 3 THEN 'major' WHEN 4 THEN 'critical' END
		FROM cairnops_incident_signals
		WHERE incident_id = $1::uuid AND active
	`, incidentID).Scan(&count, &severity); err != nil {
		return fmt.Errorf("summarize incident signals: %w", err)
	}
	if count == 0 {
		result, err := tx.Exec(ctx, `
			UPDATE cairnops_incidents
			SET status = 'resolved', resolved_at = $2, updated_at = now()
			WHERE id = $1::uuid AND status = 'active'
		`, incidentID, observedAt)
		if err != nil {
			return fmt.Errorf("resolve incident: %w", err)
		}
		if result.RowsAffected() == 1 {
			return insertActivity(ctx, tx, incidentID, "resolved", "cairnops", "", "Toutes les preuves actives sont rétablies", nil)
		}
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents
		SET source_severity = $2, effective_severity = $2, updated_at = now()
		WHERE id = $1::uuid AND status = 'active'
	`, incidentID, *severity)
	if err != nil {
		return fmt.Errorf("refresh incident severity: %w", err)
	}
	return nil
}

func insertActivity(ctx context.Context, tx pgx.Tx, incidentID, kind, origin, actorID, message string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode incident activity: %w", err)
	}
	var actor any
	if strings.TrimSpace(actorID) != "" {
		actor = actorID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_activity (incident_id, kind, origin, actor_id, message, data)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6::jsonb)
	`, incidentID, kind, origin, actor, message, encoded); err != nil {
		return fmt.Errorf("append incident activity: %w", err)
	}
	return nil
}

const incidentSelect = `
	SELECT incident.id::text, incident.target_id::text, target.name,
	       incident.nature_key, incident.nature_label, incident.status,
	       incident.source_severity, incident.effective_severity,
	       incident.opened_at, incident.resolved_at, incident.acknowledged_at,
	       coalesce(actor.display_name, ''), coalesce(incident.acknowledgement_origin, ''),
	       incident.acknowledgement_sync_status, incident.acknowledgement_sync_error,
	       EXISTS (
	           SELECT 1
	           FROM cairnops_maintenance_targets maintenance_target
	           JOIN cairnops_maintenances maintenance ON maintenance.id = maintenance_target.maintenance_id
	           WHERE maintenance_target.target_id = incident.target_id
	             AND maintenance.cancelled_at IS NULL
	             AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
	       ),
	       (
	           SELECT max(maintenance.ends_at)
	           FROM cairnops_maintenance_targets maintenance_target
	           JOIN cairnops_maintenances maintenance ON maintenance.id = maintenance_target.maintenance_id
	           WHERE maintenance_target.target_id = incident.target_id
	             AND maintenance.cancelled_at IS NULL
	             AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
	       ),
	       incident.created_at, incident.updated_at
	FROM cairnops_incidents incident
	JOIN cairnops_targets target ON target.id = incident.target_id
	LEFT JOIN cairnops_users actor ON actor.id = incident.acknowledged_by
`

type scanner interface {
	Scan(...any) error
}

func scanIncident(row scanner) (Incident, error) {
	var incident Incident
	if err := row.Scan(
		&incident.ID, &incident.TargetID, &incident.TargetName,
		&incident.NatureKey, &incident.NatureLabel, &incident.Status,
		&incident.SourceSeverity, &incident.EffectiveSeverity,
		&incident.OpenedAt, &incident.ResolvedAt, &incident.AcknowledgedAt,
		&incident.AcknowledgedBy, &incident.AcknowledgementOrigin,
		&incident.AcknowledgementSyncStatus, &incident.AcknowledgementSyncError,
		&incident.MaintenanceActive, &incident.MaintenanceEndsAt,
		&incident.CreatedAt, &incident.UpdatedAt,
	); err != nil {
		return Incident{}, fmt.Errorf("scan incident: %w", err)
	}
	return incident, nil
}
