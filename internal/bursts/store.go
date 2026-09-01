package bursts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (store *PostgresStore) List(ctx context.Context, status string, limit int) ([]Burst, error) {
	rows, err := store.pool.Query(ctx, burstSelect+`
		WHERE ($1 = 'all' OR ($1 = 'active' AND burst.status <> 'resolved') OR burst.status = $1)
		ORDER BY CASE WHEN burst.status = 'resolved' THEN 1 ELSE 0 END,
		         CASE burst.severity WHEN 'critical' THEN 0 WHEN 'major' THEN 1
		              WHEN 'warning' THEN 2 ELSE 3 END,
		         burst.opened_at DESC, burst.id
		LIMIT $2
	`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list incident bursts: %w", err)
	}
	defer rows.Close()
	items := make([]Burst, 0)
	for rows.Next() {
		item, err := scanBurst(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident bursts: %w", err)
	}
	if err := store.loadChildren(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (store *PostgresStore) Get(ctx context.Context, burstID string) (Burst, error) {
	item, err := scanBurst(store.pool.QueryRow(ctx, burstSelect+` WHERE burst.id = $1::uuid`, burstID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Burst{}, ErrNotFound
	}
	if err != nil {
		return Burst{}, err
	}
	items := []Burst{item}
	if err := store.loadChildren(ctx, items); err != nil {
		return Burst{}, err
	}
	return items[0], nil
}

func (store *PostgresStore) Acknowledge(ctx context.Context, burstID, actorID, actorName string) ([]string, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin incident burst acknowledgement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	var alreadyAcknowledged bool
	if err := tx.QueryRow(ctx, `
		SELECT status, acknowledged_at IS NOT NULL
		FROM cairnops_incident_bursts WHERE id = $1::uuid FOR UPDATE
	`, burstID).Scan(&status, &alreadyAcknowledged); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock incident burst acknowledgement: %w", err)
	}
	if status == "resolved" {
		return nil, fmt.Errorf("%w: a resolved incident burst cannot be acknowledged", ErrConflict)
	}
	if !alreadyAcknowledged {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_bursts
			SET acknowledged_at = now(), acknowledged_by = $2::uuid,
			    revision = revision + 1, updated_at = now()
			WHERE id = $1::uuid
		`, burstID, actorID); err != nil {
			return nil, fmt.Errorf("acknowledge incident burst: %w", err)
		}
		message := "Rafale acquittée"
		if strings.TrimSpace(actorName) != "" {
			message += " par " + strings.TrimSpace(actorName)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_burst_activity (
				burst_id, kind, actor_id, message
			) VALUES ($1::uuid, 'acknowledged', $2::uuid, $3)
		`, burstID, actorID, message); err != nil {
			return nil, fmt.Errorf("record incident burst acknowledgement: %w", err)
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT incident_id::text FROM cairnops_incident_burst_members
		WHERE burst_id = $1::uuid ORDER BY joined_at, incident_id
	`, burstID)
	if err != nil {
		return nil, fmt.Errorf("list incident burst acknowledgement members: %w", err)
	}
	memberIDs := make([]string, 0)
	for rows.Next() {
		var incidentID string
		if err := rows.Scan(&incidentID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan incident burst acknowledgement member: %w", err)
		}
		memberIDs = append(memberIDs, incidentID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate incident burst acknowledgement members: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit incident burst acknowledgement: %w", err)
	}
	return memberIDs, nil
}

func (store *PostgresStore) loadChildren(ctx context.Context, bursts []Burst) error {
	if len(bursts) == 0 {
		return nil
	}
	ids := make([]string, len(bursts))
	indexByID := make(map[string]int, len(bursts))
	for index := range bursts {
		ids[index] = bursts[index].ID
		indexByID[bursts[index].ID] = index
		bursts[index].Members = make([]Member, 0)
		bursts[index].Activity = make([]Activity, 0)
	}
	rows, err := store.pool.Query(ctx, `
		SELECT member.burst_id::text, incident.id::text, incident.target_id::text, target.name,
		       incident.status, incident.effective_severity, incident.opened_at,
		       incident.resolved_at, incident.acknowledged_at,
		       EXISTS (
		           SELECT 1
		           FROM cairnops_maintenance_targets maintenance_target
		           JOIN cairnops_maintenances maintenance
		             ON maintenance.id = maintenance_target.maintenance_id
		           WHERE maintenance_target.target_id = incident.target_id
		             AND maintenance.cancelled_at IS NULL
		             AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
		       ), member.joined_at
		FROM cairnops_incident_burst_members member
		JOIN cairnops_incidents incident ON incident.id = member.incident_id
		JOIN cairnops_targets target ON target.id = incident.target_id
		WHERE member.burst_id = ANY($1::uuid[])
		ORDER BY member.burst_id, incident.status = 'active' DESC, member.joined_at, incident.id
	`, ids)
	if err != nil {
		return fmt.Errorf("list incident burst members: %w", err)
	}
	for rows.Next() {
		var burstID string
		var member Member
		if err := rows.Scan(
			&burstID, &member.IncidentID, &member.TargetID, &member.TargetName, &member.Status,
			&member.EffectiveSeverity, &member.OpenedAt, &member.ResolvedAt,
			&member.AcknowledgedAt, &member.MaintenanceActive, &member.JoinedAt,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan incident burst member: %w", err)
		}
		if index, exists := indexByID[burstID]; exists {
			bursts[index].Members = append(bursts[index].Members, member)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate incident burst members: %w", err)
	}
	rows.Close()

	activityRows, err := store.pool.Query(ctx, `
		SELECT ranked.burst_id::text, ranked.id, ranked.kind, ranked.actor_name,
		       ranked.message, ranked.data, ranked.occurred_at
		FROM (
			SELECT activity.burst_id, activity.id, activity.kind,
			       coalesce(actor.display_name, '') AS actor_name,
			       activity.message, activity.data, activity.occurred_at,
			       row_number() OVER (
			           PARTITION BY activity.burst_id
			           ORDER BY activity.occurred_at DESC, activity.id DESC
			       ) AS position
			FROM cairnops_incident_burst_activity activity
			LEFT JOIN cairnops_users actor ON actor.id = activity.actor_id
			WHERE activity.burst_id = ANY($1::uuid[])
		) ranked
		WHERE ranked.position <= 50
		ORDER BY ranked.burst_id, ranked.occurred_at DESC, ranked.id DESC
	`, ids)
	if err != nil {
		return fmt.Errorf("list incident burst activity: %w", err)
	}
	defer activityRows.Close()
	for activityRows.Next() {
		var burstID string
		var activity Activity
		var raw []byte
		if err := activityRows.Scan(
			&burstID, &activity.ID, &activity.Kind, &activity.ActorName,
			&activity.Message, &raw, &activity.OccurredAt,
		); err != nil {
			return fmt.Errorf("scan incident burst activity: %w", err)
		}
		if err := json.Unmarshal(raw, &activity.Data); err != nil {
			return fmt.Errorf("decode incident burst activity: %w", err)
		}
		if index, exists := indexByID[burstID]; exists {
			bursts[index].Activity = append(bursts[index].Activity, activity)
		}
	}
	if err := activityRows.Err(); err != nil {
		return fmt.Errorf("iterate incident burst activity: %w", err)
	}
	return nil
}

const burstSelect = `
	SELECT burst.id::text, burst.anchor_incident_id::text,
	       burst.nature_scope, burst.nature_namespace, burst.nature_fingerprint,
	       burst.nature_label, burst.status, burst.severity,
	       burst.opened_at, burst.last_joined_at, burst.propagation_window_seconds,
	       burst.propagation_ends_at, burst.sealed_at, burst.resolved_at,
	       burst.acknowledged_at, coalesce(actor.display_name, ''),
	       burst.extended, burst.active_incident_count, burst.incident_count,
	       burst.affected_target_count, burst.target_count,
	       burst.max_affected_targets, burst.revision,
	       CASE burst.nature_scope
	           WHEN 'canonical' THEN 'Même Nature canonique sur ' || burst.target_count::text
	               || ' Cibles en ' || greatest(0, extract(epoch FROM burst.last_joined_at - burst.opened_at)::integer)::text || ' s'
	           ELSE 'Même Nature issue du même Connecteur sur ' || burst.target_count::text
	               || ' Cibles en ' || greatest(0, extract(epoch FROM burst.last_joined_at - burst.opened_at)::integer)::text || ' s'
	       END,
	       burst.created_at, burst.updated_at
	FROM cairnops_incident_bursts burst
	LEFT JOIN cairnops_users actor ON actor.id = burst.acknowledged_by
`

type scanner interface{ Scan(...any) error }

func scanBurst(row scanner) (Burst, error) {
	var item Burst
	if err := row.Scan(
		&item.ID, &item.AnchorIncidentID, &item.NatureScope, &item.NatureNamespace,
		&item.NatureFingerprint, &item.NatureLabel, &item.Status, &item.Severity,
		&item.OpenedAt, &item.LastJoinedAt, &item.PropagationWindowSeconds,
		&item.PropagationEndsAt, &item.SealedAt, &item.ResolvedAt,
		&item.AcknowledgedAt, &item.AcknowledgedBy, &item.Extended,
		&item.ActiveIncidentCount, &item.IncidentCount, &item.AffectedTargetCount,
		&item.TargetCount, &item.MaxAffectedTargets, &item.Revision,
		&item.Explanation, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Burst{}, fmt.Errorf("scan incident burst: %w", err)
	}
	return item, nil
}
