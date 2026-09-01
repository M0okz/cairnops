package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/bursts"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

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

func (store *PostgresStore) Schedule(ctx context.Context) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin notification scheduling: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UTC()
	if err := bursts.Project(ctx, tx, now); err != nil {
		return fmt.Errorf("project incident bursts: %w", err)
	}

	// Si l'ouverture du premier Incident est déjà partie, elle devient l'ancre
	// de la Rafale. Toute livraison encore en attente pour les autres membres est
	// annulée avant de pouvoir quitter cette transaction.
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET burst_id = burst.id, burst_revision = CASE
		        WHEN delivery.status = 'delivered' THEN delivery.burst_revision
		        ELSE burst.revision
		    END,
		    target_name = CASE WHEN burst.affected_target_count > 1
		        THEN burst.affected_target_count::text || ' Cibles affectées'
		        ELSE delivery.target_name END,
		    nature_label = burst.nature_label, severity = burst.severity,
		    incident_count = greatest(burst.incident_count, 1),
		    affected_target_count = burst.affected_target_count,
		    max_affected_targets = greatest(burst.max_affected_targets, 1),
		    burst_status = burst.status, burst_extended = burst.extended,
		    updated_at = now()
		FROM cairnops_incident_bursts burst
		WHERE delivery.incident_id = burst.anchor_incident_id
		  AND delivery.event_kind = 'firing' AND delivery.burst_id IS NULL
	`); err != nil {
		return fmt.Errorf("attach opening notifications to incident bursts: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET status = 'cancelled', last_error = 'Incident regroupé dans une Rafale',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		FROM cairnops_incident_burst_members member
		JOIN cairnops_incident_bursts burst ON burst.id = member.burst_id
		WHERE delivery.incident_id = member.incident_id
		  AND delivery.incident_id <> burst.anchor_incident_id
		  AND delivery.event_kind IN ('firing', 'resolved')
		  AND delivery.status IN ('pending', 'failed')
	`); err != nil {
		return fmt.Errorf("cancel redundant incident notifications: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET status = 'cancelled', last_error = 'incident acquitté, résolu ou placé en maintenance',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		FROM cairnops_incidents incident
		WHERE delivery.incident_id = incident.id
		  AND delivery.event_kind = 'firing'
		  AND delivery.burst_id IS NULL
		  AND delivery.status IN ('pending', 'failed')
		  AND (
		      incident.status <> 'active' OR incident.acknowledged_at IS NOT NULL
		      OR EXISTS (
		          SELECT 1
		          FROM cairnops_maintenance_targets maintenance_target
		          JOIN cairnops_maintenances maintenance ON maintenance.id = maintenance_target.maintenance_id
		          WHERE maintenance_target.target_id = incident.target_id
		            AND maintenance.cancelled_at IS NULL
		            AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
		      )
		  )
	`); err != nil {
		return fmt.Errorf("cancel obsolete firing notifications: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET status = 'cancelled', last_error = CASE
		        WHEN burst.acknowledged_at IS NOT NULL THEN 'Rafale acquittée'
		        ELSE 'Rafale résolue avant livraison'
		    END,
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		FROM cairnops_incident_bursts burst
		WHERE delivery.burst_id = burst.id AND delivery.event_kind = 'firing'
		  AND delivery.status IN ('pending', 'failed')
		  AND (burst.acknowledged_at IS NOT NULL OR burst.status = 'resolved')
	`); err != nil {
		return fmt.Errorf("cancel obsolete incident burst openings: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, channel_id, event_kind, event_key, target_name, nature_label,
			severity, opened_at
		)
		SELECT incident.id, channel.id, 'firing', 'firing', target.name, incident.nature_label,
		       incident.effective_severity, incident.opened_at
		FROM cairnops_incidents incident
		JOIN cairnops_targets target ON target.id = incident.target_id
		JOIN cairnops_notification_channels channel
		  ON channel.enabled AND channel.status <> 'disabled'
		 AND incident.effective_severity = ANY(channel.severities)
		WHERE incident.status = 'active' AND incident.acknowledged_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM cairnops_incident_burst_members member
		      WHERE member.incident_id = incident.id
		  )
		  AND NOT EXISTS (
		      SELECT 1
		      FROM cairnops_maintenance_targets maintenance_target
		      JOIN cairnops_maintenances maintenance ON maintenance.id = maintenance_target.maintenance_id
		      WHERE maintenance_target.target_id = incident.target_id
		        AND maintenance.cancelled_at IS NULL
		        AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
		  )
		ON CONFLICT DO NOTHING
	`); err != nil {
		return fmt.Errorf("schedule firing notifications: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, burst_id, burst_revision, channel_id, event_kind, event_key,
			target_name, nature_label, severity, opened_at,
			incident_count, affected_target_count, max_affected_targets,
			burst_status, burst_extended
		)
		SELECT burst.anchor_incident_id, burst.id, burst.revision, channel.id,
		       'firing', 'firing',
		       CASE WHEN burst.affected_target_count > 1
		           THEN burst.affected_target_count::text || ' Cibles affectées'
		           ELSE target.name END,
		       burst.nature_label, burst.severity, burst.opened_at,
		       greatest(burst.incident_count, 1), burst.affected_target_count,
		       greatest(burst.max_affected_targets, 1), burst.status, burst.extended
		FROM cairnops_incident_bursts burst
		JOIN cairnops_incidents anchor ON anchor.id = burst.anchor_incident_id
		JOIN cairnops_targets target ON target.id = anchor.target_id
		JOIN cairnops_notification_channels channel
		  ON channel.enabled AND channel.status <> 'disabled'
		 AND burst.severity = ANY(channel.severities)
		WHERE burst.status <> 'resolved' AND burst.acknowledged_at IS NULL
		  AND burst.active_incident_count > 0
		ON CONFLICT DO NOTHING
	`); err != nil {
		return fmt.Errorf("schedule incident burst openings: %w", err)
	}

	// Tant que l'ouverture intégrée n'est pas livrée, elle absorbe la dernière
	// révision. Après livraison, une révision dédiée met à jour la même entrée.
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox opening
		SET burst_revision = burst.revision,
		    target_name = CASE WHEN burst.affected_target_count > 1
		        THEN burst.affected_target_count::text || ' Cibles affectées'
		        ELSE opening.target_name END,
		    nature_label = burst.nature_label, severity = burst.severity,
		    incident_count = greatest(burst.incident_count, 1),
		    affected_target_count = burst.affected_target_count,
		    max_affected_targets = greatest(burst.max_affected_targets, 1),
		    burst_status = burst.status, burst_extended = burst.extended,
		    updated_at = now()
		FROM cairnops_incident_bursts burst
		WHERE opening.burst_id = burst.id AND opening.event_key = 'firing'
		  AND opening.status IN ('pending', 'failed')
	`); err != nil {
		return fmt.Errorf("refresh pending incident burst openings: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, burst_id, burst_revision, channel_id,
			event_kind, event_key, presentation,
			target_name, nature_label, severity, opened_at,
			incident_count, affected_target_count, max_affected_targets,
			burst_status, burst_extended
		)
		SELECT burst.anchor_incident_id, burst.id, burst.revision, opening.channel_id,
		       'burst_update', 'revision:' || burst.revision::text,
		       CASE WHEN burst.status = 'resolved'
		                  OR cairnops_severity_rank(burst.severity) > coalesce((
		                      SELECT max(cairnops_severity_rank(previous.severity))
		                      FROM cairnops_notification_outbox previous
		                      WHERE previous.burst_id = burst.id
		                        AND previous.channel_id = opening.channel_id
		                        AND previous.status = 'delivered'
		                  ), 0)
		                  OR (burst.extended AND NOT EXISTS (
		                      SELECT 1 FROM cairnops_notification_outbox previous
		                      WHERE previous.burst_id = burst.id
		                        AND previous.channel_id = opening.channel_id
		                        AND previous.status = 'delivered'
		                        AND previous.burst_extended
		                  ))
		            THEN 'alert' ELSE 'silent' END,
		       CASE WHEN burst.status = 'resolved'
		           THEN burst.max_affected_targets::text || ' Cibles affectées au maximum'
		           WHEN burst.affected_target_count > 1
		           THEN burst.affected_target_count::text || ' Cibles affectées'
		           ELSE opening.target_name END,
		       burst.nature_label, burst.severity, burst.opened_at,
		       greatest(burst.incident_count, 1), burst.affected_target_count,
		       greatest(burst.max_affected_targets, 1), burst.status, burst.extended
		FROM cairnops_incident_bursts burst
		JOIN cairnops_notification_outbox opening
		  ON opening.burst_id = burst.id AND opening.event_key = 'firing'
		 AND opening.status = 'delivered'
		JOIN cairnops_notification_channels channel
		  ON channel.id = opening.channel_id AND channel.kind = 'in_app'
		 AND channel.enabled AND channel.status <> 'disabled'
		WHERE burst.revision > coalesce((
		    SELECT max(previous.burst_revision)
		    FROM cairnops_notification_outbox previous
		    WHERE previous.burst_id = burst.id
		      AND previous.channel_id = opening.channel_id
		      AND previous.status IN ('pending', 'failed', 'delivered')
		), -1)
		ON CONFLICT DO NOTHING
	`); err != nil {
		return fmt.Errorf("schedule incident burst in-app updates: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_outbox delivery
		SET status = 'cancelled', last_error = 'révision de Rafale remplacée',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		FROM cairnops_incident_bursts burst
		WHERE delivery.burst_id = burst.id AND delivery.event_kind = 'burst_update'
		  AND delivery.status IN ('pending', 'failed')
		  AND delivery.burst_revision < burst.revision
		  AND EXISTS (
		      SELECT 1 FROM cairnops_notification_outbox current
		      WHERE current.burst_id = burst.id
		        AND current.channel_id = delivery.channel_id
		        AND current.burst_revision = burst.revision
		        AND current.status IN ('pending', 'failed', 'delivered')
		  )
	`); err != nil {
		return fmt.Errorf("replace stale incident burst updates: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, channel_id, event_kind, event_key, target_name, nature_label,
			severity, opened_at, resolved_at
		)
		SELECT incident.id, opening.channel_id, 'resolved', 'resolved', opening.target_name,
		       opening.nature_label, opening.severity, opening.opened_at, incident.resolved_at
		FROM cairnops_incidents incident
		JOIN cairnops_notification_outbox opening
		  ON opening.incident_id = incident.id
		 AND opening.event_kind = 'firing' AND opening.status = 'delivered'
		JOIN cairnops_notification_channels channel
		  ON channel.id = opening.channel_id AND channel.enabled AND channel.status <> 'disabled'
		WHERE incident.status = 'resolved' AND incident.resolved_at IS NOT NULL
		  AND opening.burst_id IS NULL
		ON CONFLICT DO NOTHING
	`); err != nil {
		return fmt.Errorf("schedule resolution notifications: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, burst_id, burst_revision, channel_id,
			event_kind, event_key, presentation,
			target_name, nature_label, severity, opened_at, resolved_at,
			incident_count, affected_target_count, max_affected_targets,
			burst_status, burst_extended
		)
		SELECT burst.anchor_incident_id, burst.id, burst.revision, opening.channel_id,
		       'resolved', 'resolved', 'alert',
		       burst.max_affected_targets::text || ' Cibles affectées au maximum',
		       burst.nature_label, burst.severity, burst.opened_at, burst.resolved_at,
		       greatest(burst.incident_count, 1), 0,
		       greatest(burst.max_affected_targets, 1), burst.status, burst.extended
		FROM cairnops_incident_bursts burst
		JOIN cairnops_notification_outbox opening
		  ON opening.burst_id = burst.id AND opening.event_key = 'firing'
		 AND opening.status = 'delivered'
		JOIN cairnops_notification_channels channel
		  ON channel.id = opening.channel_id AND channel.kind = 'mattermost'
		 AND channel.enabled AND channel.status <> 'disabled'
		WHERE burst.status = 'resolved' AND burst.resolved_at IS NOT NULL
		ON CONFLICT DO NOTHING
	`); err != nil {
		return fmt.Errorf("schedule incident burst Mattermost resolutions: %w", err)
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
		    LEFT JOIN cairnops_incident_bursts burst ON burst.id = delivery.burst_id
		    WHERE delivery.status IN ('pending', 'failed')
		      AND delivery.next_attempt_at <= now()
		      AND (delivery.lease_until IS NULL OR delivery.lease_until < now())
		      AND channel.enabled AND channel.status <> 'disabled'
		      AND (
		          delivery.event_kind IN ('resolved', 'burst_update')
		          OR (
		              delivery.event_kind = 'firing'
		              AND (
		                  (delivery.burst_id IS NULL
		                   AND incident.status = 'active' AND incident.acknowledged_at IS NULL)
		                  OR (delivery.burst_id IS NOT NULL
		                      AND burst.status <> 'resolved'
		                      AND burst.acknowledged_at IS NULL
		                      AND burst.active_incident_count > 0)
		              )
		              AND (
		                  delivery.burst_id IS NOT NULL
		                  OR NOT EXISTS (
		                      SELECT 1
		                      FROM cairnops_maintenance_targets maintenance_target
		                      JOIN cairnops_maintenances maintenance ON maintenance.id = maintenance_target.maintenance_id
		                      WHERE maintenance_target.target_id = incident.target_id
		                        AND maintenance.cancelled_at IS NULL
		                        AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at
		                  )
		              )
		          )
		      )
		    ORDER BY delivery.next_attempt_at, delivery.id
		    FOR UPDATE OF delivery SKIP LOCKED
		    LIMIT 1
		), claimed AS (
		    UPDATE cairnops_notification_outbox delivery
		    SET lease_owner = $1, lease_until = now() + interval '30 seconds', updated_at = now()
		    FROM candidate
		    WHERE delivery.id = candidate.id
		    RETURNING delivery.*
		)
		SELECT claimed.id, claimed.incident_id::text, coalesce(claimed.burst_id::text, ''),
		       claimed.burst_revision, claimed.channel_id::text,
		       channel.kind, claimed.event_kind, claimed.presentation, claimed.target_name,
		       claimed.nature_label, claimed.severity,
		       claimed.incident_count, claimed.affected_target_count,
		       claimed.max_affected_targets, coalesce(claimed.burst_status, ''),
		       claimed.burst_extended, claimed.opened_at,
		       claimed.resolved_at, channel.credential_sealed
		FROM claimed
		JOIN cairnops_notification_channels channel ON channel.id = claimed.channel_id
	`, strings.TrimSpace(workerID)).Scan(
		&delivery.ID, &delivery.IncidentID, &delivery.BurstID,
		&delivery.BurstRevision, &delivery.ChannelID,
		&delivery.ChannelKind, &delivery.EventKind, &delivery.Presentation,
		&delivery.TargetName, &delivery.NatureLabel, &delivery.Severity,
		&delivery.IncidentCount, &delivery.AffectedTargets,
		&delivery.MaxAffected, &delivery.BurstStatus, &delivery.BurstExtended,
		&delivery.OpenedAt,
		&delivery.ResolvedAt, &delivery.CredentialSealed,
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
		    WHERE id = $1 AND lease_owner = $2
		    RETURNING channel_id
		)
		UPDATE cairnops_notification_channels channel
		SET status = 'connected', last_checked_at = now(), last_error = '', updated_at = now()
		FROM delivered
		WHERE channel.id = delivered.channel_id
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
		    WHERE id = $1 AND lease_owner = $2
		    RETURNING channel_id
		)
		UPDATE cairnops_notification_channels channel
		SET status = 'degraded', last_checked_at = now(), last_error = $3, updated_at = now()
		FROM failed
		WHERE channel.id = failed.channel_id
	`, deliveryID, strings.TrimSpace(workerID), deliveryError)
	if err != nil {
		return fmt.Errorf("fail notification delivery: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("notification delivery lease lost")
	}
	return nil
}

// Deliver porte une livraison intégrée jusqu'aux personnes. Elle ne sort pas de
// l'instance : la livraison est l'écriture elle-même.
//
// L'ouverture s'adresse à tous les comptes actifs — la V1 n'a pas de Groupes de
// notification, et un Observateur a le droit de savoir même s'il ne décide de
// rien. La Résolution, elle, ne s'adresse qu'à ceux qui ont reçu l'ouverture :
// c'est ce que veut dire « aux mêmes destinataires ». Quelqu'un désactivé
// depuis n'en reçoit pas la fin, et quelqu'un arrivé depuis ne reçoit pas la
// fin d'une histoire dont il n'a pas eu le début.
func (store *PostgresStore) Deliver(ctx context.Context, delivery Delivery) (int, error) {
	if delivery.BurstID != "" {
		return store.deliverBurst(ctx, delivery)
	}
	occurredAt := delivery.OpenedAt
	if delivery.EventKind == "resolved" && delivery.ResolvedAt != nil {
		occurredAt = *delivery.ResolvedAt
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin in-app delivery: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	recipients := `
		SELECT id FROM cairnops_users
		WHERE deactivated_at IS NULL AND external_suspended_at IS NULL
	`
	if delivery.EventKind == "resolved" {
		recipients = `
			SELECT inbox.user_id
			FROM cairnops_notification_inbox inbox
			WHERE inbox.incident_id = $1::uuid AND inbox.event_kind = 'firing'
		`
	}

	result, err := tx.Exec(ctx, `
		INSERT INTO cairnops_notification_inbox (
			user_id, incident_id, target_id, event_kind, target_name,
			nature_label, severity, occurred_at
		)
		SELECT recipient.id, $1::uuid, incident.target_id, $2, $3, $4, $5, $6
		FROM (`+recipients+`) AS recipient(id)
		JOIN cairnops_incidents incident ON incident.id = $1::uuid
		ON CONFLICT (user_id, incident_id, event_kind) DO NOTHING
	`, delivery.IncidentID, delivery.EventKind, delivery.TargetName,
		delivery.NatureLabel, string(delivery.Severity), occurredAt)
	if err != nil {
		return 0, fmt.Errorf("deposit in-app notifications: %w", err)
	}

	// Un seul signalement pour toute la volée : les sessions ouvertes relisent
	// leur propre boîte, et une entrée par personne en émettrait autant.
	if result.RowsAffected() > 0 {
		// Le Push reprend exactement les destinataires de la boîte intégrée, mais
		// possède une reprise par appareil. Le relais ne recevra ensuite que son
		// identifiant opaque et une enveloppe chiffrée.
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_push_outbox (device_id, inbox_id, revision, presentation)
			SELECT device.id, inbox.id, inbox.revision, 'alert'
			FROM cairnops_notification_inbox inbox
			JOIN cairnops_devices device ON device.user_id = inbox.user_id
			JOIN cairnops_users users ON users.id = device.user_id
			WHERE inbox.incident_id = $1::uuid AND inbox.event_kind = $2
			  AND device.revoked_at IS NULL AND device.push_disabled_at IS NULL
			  AND device.push_recipient_sealed IS NOT NULL
			  AND users.deactivated_at IS NULL
			  AND users.external_suspended_at IS NULL
			ON CONFLICT (device_id, inbox_id, revision) DO NOTHING
		`, delivery.IncidentID, delivery.EventKind); err != nil {
			return 0, fmt.Errorf("schedule device push notifications: %w", err)
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
	return int(result.RowsAffected()), nil
}

func (store *PostgresStore) deliverBurst(ctx context.Context, delivery Delivery) (int, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin incident burst in-app delivery: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result pgconn.CommandTag
	if delivery.EventKind == "firing" {
		result, err = tx.Exec(ctx, `
			INSERT INTO cairnops_notification_inbox (
				user_id, incident_id, target_id, burst_id, revision, event_kind,
				target_name, nature_label, severity, occurred_at,
				incident_count, affected_target_count, max_affected_targets,
				burst_status, burst_extended
			)
			SELECT users.id, $1::uuid, incident.target_id, $2::uuid, $3, 'firing',
			       $4, $5, $6, $7, $8, $9, $10, $11, $12
			FROM cairnops_users users
			JOIN cairnops_incidents incident ON incident.id = $1::uuid
			WHERE users.deactivated_at IS NULL AND users.external_suspended_at IS NULL
			ON CONFLICT (user_id, burst_id) WHERE burst_id IS NOT NULL
			DO UPDATE SET revision = EXCLUDED.revision,
			    target_name = EXCLUDED.target_name, nature_label = EXCLUDED.nature_label,
			    severity = EXCLUDED.severity, incident_count = EXCLUDED.incident_count,
			    affected_target_count = EXCLUDED.affected_target_count,
			    max_affected_targets = EXCLUDED.max_affected_targets,
			    burst_status = EXCLUDED.burst_status, burst_extended = EXCLUDED.burst_extended
		`, delivery.IncidentID, delivery.BurstID, delivery.BurstRevision,
			delivery.TargetName, delivery.NatureLabel, string(delivery.Severity),
			delivery.OpenedAt, delivery.IncidentCount, delivery.AffectedTargets,
			delivery.MaxAffected, delivery.BurstStatus, delivery.BurstExtended)
	} else {
		eventKind := "firing"
		occurredAt := time.Now().UTC()
		if delivery.BurstStatus == "resolved" {
			eventKind = "resolved"
			if delivery.ResolvedAt != nil {
				occurredAt = delivery.ResolvedAt.UTC()
			}
		}
		result, err = tx.Exec(ctx, `
			UPDATE cairnops_notification_inbox
			SET burst_id = $2::uuid, revision = $3, event_kind = $4, target_name = $5,
			    nature_label = $6, severity = $7, occurred_at = $8,
			    incident_count = $9, affected_target_count = $10,
			    max_affected_targets = $11, burst_status = $12,
			    burst_extended = $13,
			    read_at = CASE WHEN $14 = 'alert' THEN NULL ELSE read_at END,
			    dismissed_at = CASE WHEN $14 = 'alert' THEN NULL ELSE dismissed_at END
			WHERE incident_id = $1::uuid
			  AND (burst_id = $2::uuid OR burst_id IS NULL) AND revision < $3
		`, delivery.IncidentID, delivery.BurstID, delivery.BurstRevision,
			eventKind, delivery.TargetName, delivery.NatureLabel,
			string(delivery.Severity), occurredAt, delivery.IncidentCount,
			delivery.AffectedTargets, delivery.MaxAffected, delivery.BurstStatus,
			delivery.BurstExtended, delivery.Presentation)
	}
	if err != nil {
		return 0, fmt.Errorf("update incident burst in-app notification: %w", err)
	}

	if result.RowsAffected() > 0 {
		// Seule la révision la plus récente doit encore partir. Une alerte déjà
		// remise reste bien sûr dans l'historique ; seules les attentes obsolètes
		// sont remplacées.
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_push_outbox outgoing
			SET status = 'cancelled', last_error = 'révision de Rafale remplacée',
			    lease_owner = NULL, lease_until = NULL, updated_at = now()
			FROM cairnops_notification_inbox inbox
			WHERE outgoing.inbox_id = inbox.id AND inbox.burst_id = $1::uuid
			  AND outgoing.status IN ('pending', 'failed') AND outgoing.revision < $2
		`, delivery.BurstID, delivery.BurstRevision); err != nil {
			return 0, fmt.Errorf("replace stale incident burst pushes: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_push_outbox (
				device_id, inbox_id, revision, presentation
			)
			SELECT device.id, inbox.id, $2, $3
			FROM cairnops_notification_inbox inbox
			JOIN cairnops_devices device ON device.user_id = inbox.user_id
			JOIN cairnops_users users ON users.id = device.user_id
			WHERE inbox.burst_id = $1::uuid
			  AND device.revoked_at IS NULL AND device.push_disabled_at IS NULL
			  AND device.push_recipient_sealed IS NOT NULL
			  AND users.deactivated_at IS NULL AND users.external_suspended_at IS NULL
			ON CONFLICT (device_id, inbox_id, revision) DO NOTHING
		`, delivery.BurstID, delivery.BurstRevision, delivery.Presentation); err != nil {
			return 0, fmt.Errorf("schedule incident burst push update: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			SELECT cairnops_append_event('notification.changed', 'notification', $1)
		`, delivery.ChannelID); err != nil {
			return 0, fmt.Errorf("signal incident burst in-app notification: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit incident burst in-app delivery: %w", err)
	}
	return int(result.RowsAffected()), nil
}
