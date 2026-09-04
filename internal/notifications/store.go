package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultStabilityDelay = 2 * time.Minute

type PostgresStore struct {
	pool           *pgxpool.Pool
	stabilityDelay time.Duration
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return NewPostgresStoreWithStabilityDelay(pool, defaultStabilityDelay)
}

func NewPostgresStoreWithStabilityDelay(pool *pgxpool.Pool, delay time.Duration) *PostgresStore {
	if delay < 0 {
		delay = 0
	}
	return &PostgresStore{pool: pool, stabilityDelay: delay}
}

func (store *PostgresStore) stabilityDelaySeconds() int64 {
	return int64(store.stabilityDelay / time.Second)
}

func (store *PostgresStore) List(ctx context.Context) ([]Channel, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id::text, kind, name, endpoint, severities, enabled, status,
		       encrypted_transport, last_checked_at, last_error, created_at, updated_at
		FROM cairnops_notification_channels
		ORDER BY enabled DESC, lower(name), id
	`)
	if err != nil {
		return nil, fmt.Errorf("list notification channels: %w", err)
	}
	defer rows.Close()
	items := make([]Channel, 0)
	for rows.Next() {
		item, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification channels: %w", err)
	}
	return items, nil
}

func (store *PostgresStore) CreateMattermost(ctx context.Context, input PersistMattermostInput) (Channel, error) {
	severityValues := make([]string, len(input.Severities))
	for index, severity := range input.Severities {
		severityValues[index] = string(severity)
	}
	item, err := scanChannel(store.pool.QueryRow(ctx, `
		INSERT INTO cairnops_notification_channels (
			kind, name, endpoint, credential_sealed, severities, enabled, status,
			encrypted_transport, last_checked_at, last_error, created_by
		) VALUES ('mattermost', $1, $2, $3, $4, true, 'connected', $5, now(), '', $6::uuid)
		RETURNING id::text, kind, name, endpoint, severities, enabled, status,
		          encrypted_transport, last_checked_at, last_error, created_at, updated_at
	`, input.Name, input.Endpoint, input.CredentialSealed, severityValues,
		input.EncryptedTransport, input.ActorID))
	if err != nil {
		return Channel{}, fmt.Errorf("create Mattermost notification channel: %w", err)
	}
	return item, nil
}

type channelScanner interface{ Scan(...any) error }

func scanChannel(row channelScanner) (Channel, error) {
	var item Channel
	var severities []string
	if err := row.Scan(
		&item.ID, &item.Kind, &item.Name, &item.Endpoint, &severities,
		&item.Enabled, &item.Status, &item.EncryptedTransport,
		&item.LastCheckedAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Channel{}, fmt.Errorf("scan notification channel: %w", err)
	}
	item.Severities = make([]incidents.Severity, len(severities))
	for index, severity := range severities {
		item.Severities[index] = incidents.Severity(severity)
	}
	return item, nil
}

// Schedule traduit les révisions d'Incident en Faits opérationnels livrables.
// Le cycle Incident–Preuve est déjà établi en amont : ce module ne connaît ni
// les adapters produit ni les règles de Propagation.
func (store *PostgresStore) Schedule(ctx context.Context) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin notification scheduling: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET status = 'cancelled', last_error = 'Incident acquitté, résolu ou entièrement Sous maintenance',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		FROM cairnops_incidents incident
		WHERE delivery.incident_id = incident.id
		  AND delivery.event_kind = 'firing'
		  AND delivery.status IN ('pending', 'failed')
		  AND (
		      incident.status = 'resolved' OR incident.acknowledged_at IS NOT NULL
		      OR NOT EXISTS (
		          SELECT 1 FROM cairnops_incident_impacts impact
		          WHERE impact.incident_id = incident.id AND impact.status = 'active'
		            AND NOT EXISTS (
		                SELECT 1 FROM cairnops_maintenance_targets membership
		                JOIN cairnops_maintenances maintenance ON maintenance.id = membership.maintenance_id
		                WHERE membership.target_id = impact.target_id
		                  AND maintenance.cancelled_at IS NULL
		                  AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
		            )
		      )
		  )
	`); err != nil {
		return fmt.Errorf("cancel obsolete incident openings: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, incident_revision, channel_id, event_kind, event_key,
			presentation, target_name, nature_label, severity, opened_at,
			impact_count, affected_target_count, max_affected_targets,
			propagation_status, extended, next_attempt_at
		)
		SELECT incident.id, incident.revision, channel.id, 'firing', 'firing', 'alert',
		       CASE WHEN incident.affected_target_count > 1
		            THEN incident.affected_target_count::text || ' Cibles affectées'
		            ELSE coalesce((
		                SELECT target.name FROM cairnops_incident_impacts impact
		                JOIN cairnops_targets target ON target.id = impact.target_id
		                WHERE impact.incident_id = incident.id AND impact.status = 'active'
		                ORDER BY impact.opened_at, impact.id LIMIT 1
		            ), 'Cible affectée') END,
		       incident.nature_label, incident.severity, incident.opened_at,
		       incident.impact_count, incident.affected_target_count,
		       greatest(incident.max_affected_targets, 1),
		       incident.propagation_status, incident.extended,
		       CASE WHEN incident.severity = 'critical' THEN now()
		            ELSE incident.created_at + $1 * interval '1 second' END
		FROM cairnops_incidents incident
		JOIN cairnops_notification_channels channel
		  ON channel.enabled AND channel.status <> 'disabled'
		 AND incident.severity = ANY(channel.severities)
		WHERE incident.status = 'active' AND incident.acknowledged_at IS NULL
		  AND incident.active_impact_count > 0
		  AND EXISTS (
		      SELECT 1 FROM cairnops_incident_impacts impact
		      WHERE impact.incident_id = incident.id AND impact.status = 'active'
		        AND NOT EXISTS (
		            SELECT 1 FROM cairnops_maintenance_targets membership
		            JOIN cairnops_maintenances maintenance ON maintenance.id = membership.maintenance_id
		            WHERE membership.target_id = impact.target_id
		              AND maintenance.cancelled_at IS NULL
		              AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
		        )
		  )
		ON CONFLICT DO NOTHING
	`, store.stabilityDelaySeconds()); err != nil {
		return fmt.Errorf("schedule incident openings: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET incident_revision = incident.revision,
		    next_attempt_at = CASE WHEN incident.severity = 'critical' THEN now()
		        ELSE incident.created_at + $1 * interval '1 second' END,
		    target_name = CASE WHEN incident.affected_target_count > 1
		        THEN incident.affected_target_count::text || ' Cibles affectées'
		        ELSE coalesce((
		            SELECT target.name FROM cairnops_incident_impacts impact
		            JOIN cairnops_targets target ON target.id = impact.target_id
		            WHERE impact.incident_id = incident.id AND impact.status = 'active'
		            ORDER BY impact.opened_at, impact.id LIMIT 1
		        ), delivery.target_name) END,
		    nature_label = incident.nature_label, severity = incident.severity,
		    impact_count = incident.impact_count,
		    affected_target_count = incident.affected_target_count,
		    max_affected_targets = greatest(incident.max_affected_targets, 1),
		    propagation_status = incident.propagation_status,
		    extended = incident.extended, updated_at = now()
		FROM cairnops_incidents incident
		WHERE delivery.incident_id = incident.id AND delivery.event_key = 'firing'
		  AND delivery.status IN ('pending', 'failed')
	`, store.stabilityDelaySeconds()); err != nil {
		return fmt.Errorf("refresh pending incident openings: %w", err)
	}

	// Une entrée intégrée est une projection révisable. Seuls une Gravité
	// encore jamais signalée et le premier passage en Propagation étendue sont
	// des alertes ; les autres révisions restent silencieuses.
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, incident_revision, channel_id, event_kind, event_key,
			presentation, target_name, nature_label, severity, opened_at, resolved_at,
			impact_count, affected_target_count, max_affected_targets,
			propagation_status, extended
		)
		SELECT incident.id, incident.revision, opening.channel_id,
		       'incident_update', 'revision:' || incident.revision::text,
		       CASE WHEN incident.status = 'resolved' THEN 'silent'
		            WHEN cairnops_severity_rank(incident.severity) > coalesce((
		                SELECT max(cairnops_severity_rank(previous.severity))
		                FROM cairnops_notification_outbox previous
		                WHERE previous.incident_id = incident.id
		                  AND previous.channel_id = opening.channel_id
		                  AND previous.status = 'delivered'
		            ), 0) OR (incident.extended AND NOT EXISTS (
		                SELECT 1 FROM cairnops_notification_outbox previous
		                WHERE previous.incident_id = incident.id
		                  AND previous.channel_id = opening.channel_id
		                  AND previous.status = 'delivered' AND previous.extended
		            )) THEN 'alert' ELSE 'silent' END,
		       CASE WHEN incident.status = 'resolved'
		            THEN incident.max_affected_targets::text || ' Cibles affectées au maximum'
		            WHEN incident.affected_target_count > 1
		            THEN incident.affected_target_count::text || ' Cibles affectées'
		            ELSE coalesce((
		                SELECT target.name FROM cairnops_incident_impacts impact
		                JOIN cairnops_targets target ON target.id = impact.target_id
		                WHERE impact.incident_id = incident.id AND impact.status = 'active'
		                ORDER BY impact.opened_at, impact.id LIMIT 1
		            ), opening.target_name) END,
		       incident.nature_label, incident.severity, incident.opened_at,
		       incident.resolved_at, incident.impact_count,
		       incident.affected_target_count, greatest(incident.max_affected_targets, 1),
		       incident.propagation_status, incident.extended
		FROM cairnops_incidents incident
		JOIN cairnops_notification_outbox opening
		  ON opening.incident_id = incident.id AND opening.event_key = 'firing'
		 AND opening.status = 'delivered'
		JOIN cairnops_notification_channels channel
		  ON channel.id = opening.channel_id AND channel.kind = 'in_app'
		 AND channel.enabled AND channel.status <> 'disabled'
		WHERE incident.revision > coalesce((
		    SELECT max(previous.incident_revision)
		    FROM cairnops_notification_outbox previous
		    WHERE previous.incident_id = incident.id
		      AND previous.channel_id = opening.channel_id
		      AND previous.status IN ('pending', 'failed', 'delivered')
		), -1)
		ON CONFLICT DO NOTHING
	`); err != nil {
		return fmt.Errorf("schedule incident in-app updates: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET status = 'cancelled', last_error = 'révision d’Incident remplacée',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		FROM cairnops_incidents incident
		WHERE delivery.incident_id = incident.id
		  AND delivery.event_kind = 'incident_update'
		  AND delivery.status IN ('pending', 'failed')
		  AND delivery.incident_revision < incident.revision
	`); err != nil {
		return fmt.Errorf("replace stale incident updates: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, incident_revision, channel_id, event_kind, event_key,
			presentation, target_name, nature_label, severity, opened_at, resolved_at,
			impact_count, affected_target_count, max_affected_targets,
			propagation_status, extended
		)
		SELECT incident.id, incident.revision, opening.channel_id,
		       'resolved', 'resolved', 'alert',
		       incident.max_affected_targets::text || ' Cibles affectées au maximum',
		       incident.nature_label, incident.severity, incident.opened_at,
		       incident.resolved_at, incident.impact_count, 0,
		       greatest(incident.max_affected_targets, 1),
		       incident.propagation_status, incident.extended
		FROM cairnops_incidents incident
		JOIN cairnops_notification_outbox opening
		  ON opening.incident_id = incident.id AND opening.event_key = 'firing'
		 AND opening.status = 'delivered'
		JOIN cairnops_notification_channels channel
		  ON channel.id = opening.channel_id AND channel.kind = 'mattermost'
		 AND channel.enabled AND channel.status <> 'disabled'
		WHERE incident.status = 'resolved' AND incident.resolved_at IS NOT NULL
		ON CONFLICT DO NOTHING
	`); err != nil {
		return fmt.Errorf("schedule incident resolutions: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET status = 'cancelled', last_error = 'canal désactivé',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		FROM cairnops_notification_channels channel
		WHERE delivery.channel_id = channel.id
		  AND delivery.status IN ('pending', 'failed')
		  AND (NOT channel.enabled OR channel.status = 'disabled')
	`); err != nil {
		return fmt.Errorf("cancel disabled channel notifications: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notification scheduling: %w", err)
	}
	return nil
}

func (store *PostgresStore) Claim(ctx context.Context, workerID string) (Delivery, error) {
	var delivery Delivery
	err := store.pool.QueryRow(ctx, `
		WITH candidate AS (
		    SELECT delivery.id
		    FROM cairnops_notification_outbox delivery
		    JOIN cairnops_notification_channels channel ON channel.id = delivery.channel_id
		    JOIN cairnops_incidents incident ON incident.id = delivery.incident_id
		    WHERE delivery.status IN ('pending', 'failed')
		      AND delivery.next_attempt_at <= now()
		      AND (delivery.lease_until IS NULL OR delivery.lease_until < now())
		      AND channel.enabled AND channel.status <> 'disabled'
		      AND (delivery.event_kind <> 'firing'
		           OR (incident.status = 'active' AND incident.acknowledged_at IS NULL))
		    ORDER BY delivery.next_attempt_at, delivery.id
		    FOR UPDATE OF delivery SKIP LOCKED LIMIT 1
		), claimed AS (
		    UPDATE cairnops_notification_outbox delivery
		    SET lease_owner = $1, lease_until = now() + interval '30 seconds', updated_at = now()
		    FROM candidate WHERE delivery.id = candidate.id RETURNING delivery.*
		)
		SELECT claimed.id, claimed.incident_id::text, claimed.incident_revision,
		       claimed.channel_id::text, channel.kind, claimed.event_kind,
		       claimed.presentation, claimed.target_name, claimed.nature_label,
		       claimed.severity, claimed.impact_count,
		       claimed.affected_target_count, claimed.max_affected_targets,
		       claimed.propagation_status, claimed.extended, claimed.opened_at,
		       claimed.resolved_at, channel.credential_sealed
		FROM claimed JOIN cairnops_notification_channels channel ON channel.id = claimed.channel_id
	`, strings.TrimSpace(workerID)).Scan(
		&delivery.ID, &delivery.IncidentID, &delivery.IncidentRevision,
		&delivery.ChannelID, &delivery.ChannelKind, &delivery.EventKind,
		&delivery.Presentation, &delivery.TargetName, &delivery.NatureLabel,
		&delivery.Severity, &delivery.ImpactCount, &delivery.AffectedTargets,
		&delivery.MaxAffected, &delivery.PropagationStatus, &delivery.Extended,
		&delivery.OpenedAt, &delivery.ResolvedAt, &delivery.CredentialSealed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNoDelivery
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("claim notification delivery: %w", err)
	}
	return delivery, nil
}

func (store *PostgresStore) Complete(ctx context.Context, deliveryID int64, workerID string) error {
	result, err := store.pool.Exec(ctx, `
		WITH delivered AS (
		    UPDATE cairnops_notification_outbox
		    SET status = 'delivered', delivered_at = now(), last_error = '',
		        lease_owner = NULL, lease_until = NULL, updated_at = now()
		    WHERE id = $1 AND lease_owner = $2 RETURNING channel_id
		)
		UPDATE cairnops_notification_channels channel
		SET status = 'connected', last_checked_at = now(), last_error = '', updated_at = now()
		FROM delivered WHERE channel.id = delivered.channel_id
	`, deliveryID, strings.TrimSpace(workerID))
	if err != nil {
		return fmt.Errorf("complete notification delivery: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("notification delivery lease lost")
	}
	return nil
}

func (store *PostgresStore) Fail(ctx context.Context, deliveryID int64, workerID, deliveryError string) error {
	deliveryError = strings.TrimSpace(deliveryError)
	if len(deliveryError) > 500 {
		deliveryError = deliveryError[:500]
	}
	result, err := store.pool.Exec(ctx, `
		WITH failed AS (
		    UPDATE cairnops_notification_outbox
		    SET status = 'failed', attempts = attempts + 1,
		        next_attempt_at = now() + least(interval '10 minutes', interval '5 seconds' * power(2, least(attempts, 7))),
		        last_error = $3, lease_owner = NULL, lease_until = NULL, updated_at = now()
		    WHERE id = $1 AND lease_owner = $2 RETURNING channel_id
		)
		UPDATE cairnops_notification_channels channel
		SET status = 'degraded', last_checked_at = now(), last_error = $3, updated_at = now()
		FROM failed WHERE channel.id = failed.channel_id
	`, deliveryID, strings.TrimSpace(workerID), deliveryError)
	if err != nil {
		return fmt.Errorf("fail notification delivery: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("notification delivery lease lost")
	}
	return nil
}

func (store *PostgresStore) Deliver(ctx context.Context, delivery Delivery) (int, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin in-app delivery: %w", err)
	}
	defer tx.Rollback(ctx)
	presentation := delivery.Presentation
	if presentation == "" {
		presentation = "alert"
	}
	var affected int64
	if delivery.EventKind == "firing" {
		command, err := tx.Exec(ctx, `
			INSERT INTO cairnops_notification_inbox (
				user_id, incident_id, target_id, revision, event_kind,
				target_name, nature_label, severity, occurred_at,
				impact_count, affected_target_count, max_affected_targets,
				propagation_status, extended
			)
			SELECT users.id, $1::uuid,
			       CASE WHEN $8 = 1 THEN (
			           SELECT target_id FROM cairnops_incident_impacts
			           WHERE incident_id = $1::uuid ORDER BY opened_at, id LIMIT 1
			       ) ELSE NULL END,
			       $2, 'firing', $3, $4, $5, $6, $7, $8, $9, $10, $11
			FROM cairnops_users users
			WHERE users.deactivated_at IS NULL AND users.external_suspended_at IS NULL
			ON CONFLICT (user_id, incident_id) DO UPDATE SET
				revision = EXCLUDED.revision, event_kind = EXCLUDED.event_kind,
				target_name = EXCLUDED.target_name, nature_label = EXCLUDED.nature_label,
				severity = EXCLUDED.severity, occurred_at = EXCLUDED.occurred_at,
				impact_count = EXCLUDED.impact_count,
				affected_target_count = EXCLUDED.affected_target_count,
				max_affected_targets = EXCLUDED.max_affected_targets,
				propagation_status = EXCLUDED.propagation_status,
				extended = EXCLUDED.extended
		`, delivery.IncidentID, delivery.IncidentRevision, delivery.TargetName,
			delivery.NatureLabel, string(delivery.Severity), delivery.OpenedAt,
			delivery.ImpactCount, delivery.AffectedTargets, delivery.MaxAffected,
			delivery.PropagationStatus, delivery.Extended)
		if err != nil {
			return 0, fmt.Errorf("deposit in-app incident opening: %w", err)
		}
		affected = command.RowsAffected()
	} else {
		occurredAt := time.Now().UTC()
		eventKind := "firing"
		if delivery.ResolvedAt != nil {
			occurredAt = delivery.ResolvedAt.UTC()
			eventKind = "resolved"
		}
		command, err := tx.Exec(ctx, `
			UPDATE cairnops_notification_inbox
			SET revision = $2, event_kind = $3,
			    target_id = CASE WHEN $9 = 1 THEN (
			        SELECT target_id FROM cairnops_incident_impacts
			        WHERE incident_id = $1::uuid AND status = 'active'
			        ORDER BY opened_at, id LIMIT 1
			    ) ELSE NULL END,
			    target_name = $4,
			    nature_label = $5, severity = $6, occurred_at = $7,
			    impact_count = $8, affected_target_count = $9,
			    max_affected_targets = $10, propagation_status = $11,
			    extended = $12,
			    read_at = CASE WHEN $13 = 'alert' THEN NULL ELSE read_at END,
			    dismissed_at = CASE WHEN $13 = 'alert' THEN NULL ELSE dismissed_at END
			WHERE incident_id = $1::uuid AND revision < $2
		`, delivery.IncidentID, delivery.IncidentRevision, eventKind,
			delivery.TargetName, delivery.NatureLabel, string(delivery.Severity),
			occurredAt, delivery.ImpactCount, delivery.AffectedTargets,
			delivery.MaxAffected, delivery.PropagationStatus, delivery.Extended,
			presentation)
		if err != nil {
			return 0, fmt.Errorf("revise in-app incident notification: %w", err)
		}
		affected = command.RowsAffected()
	}

	if affected > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_push_outbox outgoing
			SET status = 'cancelled', last_error = 'révision d’Incident remplacée',
			    lease_owner = NULL, lease_until = NULL, updated_at = now()
			FROM cairnops_notification_inbox inbox
			WHERE outgoing.inbox_id = inbox.id AND inbox.incident_id = $1::uuid
			  AND outgoing.status IN ('pending', 'failed') AND outgoing.revision < $2
		`, delivery.IncidentID, delivery.IncidentRevision); err != nil {
			return 0, fmt.Errorf("replace stale incident pushes: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_push_outbox (device_id, inbox_id, revision, presentation)
			SELECT device.id, inbox.id, $2, $3
			FROM cairnops_notification_inbox inbox
			JOIN cairnops_devices device ON device.user_id = inbox.user_id
			JOIN cairnops_users users ON users.id = device.user_id
			WHERE inbox.incident_id = $1::uuid
			  AND device.revoked_at IS NULL AND device.push_disabled_at IS NULL
			  AND device.push_recipient_sealed IS NOT NULL
			  AND users.deactivated_at IS NULL AND users.external_suspended_at IS NULL
			ON CONFLICT (device_id, inbox_id, revision) DO NOTHING
		`, delivery.IncidentID, delivery.IncidentRevision, presentation); err != nil {
			return 0, fmt.Errorf("schedule incident push: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			SELECT cairnops_append_event('notification.changed', 'notification', $1)
		`, delivery.ChannelID); err != nil {
			return 0, fmt.Errorf("signal in-app notifications: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit in-app delivery: %w", err)
	}
	return int(affected), nil
}
