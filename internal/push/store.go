package push

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoDelivery = errors.New("no push delivery due")

type Delivery struct {
	ID                  int64
	DeviceID            string
	RecipientSealed     string
	EncryptionPublicKey []byte
	Locale              string
	NotificationContent string
	EventKind           string
	IncidentID          string
	BurstID             string
	Revision            int
	PresentationMode    string
	TargetName          string
	NatureLabel         string
	Severity            string
	IncidentCount       int
	AffectedTargets     int
	MaxAffected         int
	BurstStatus         string
	BurstExtended       bool
	OccurredAt          time.Time
}

type DeliveryStore interface {
	Claim(context.Context, string) (Delivery, error)
	Complete(context.Context, int64, string) error
	Fail(context.Context, int64, string, string) error
	DisableDevice(context.Context, int64, string, string) error
	SetRelayStatus(context.Context, bool, error) error
}

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (store *PostgresStore) Claim(ctx context.Context, workerID string) (Delivery, error) {
	var delivery Delivery
	err := store.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT outgoing.id
			FROM cairnops_push_outbox outgoing
			JOIN cairnops_devices device ON device.id = outgoing.device_id
			JOIN cairnops_users users ON users.id = device.user_id
			WHERE outgoing.status IN ('pending', 'failed')
			  AND outgoing.next_attempt_at <= now()
			  AND (outgoing.lease_until IS NULL OR outgoing.lease_until < now())
			  AND device.revoked_at IS NULL AND device.push_disabled_at IS NULL
			  AND device.push_recipient_sealed IS NOT NULL
			  AND users.deactivated_at IS NULL
			  AND users.external_suspended_at IS NULL
			ORDER BY outgoing.next_attempt_at, outgoing.id
			FOR UPDATE OF outgoing SKIP LOCKED
			LIMIT 1
		), claimed AS (
			UPDATE cairnops_push_outbox outgoing
			SET lease_owner = $1, lease_until = now() + interval '30 seconds', updated_at = now()
			FROM candidate
			WHERE outgoing.id = candidate.id
			RETURNING outgoing.*
		)
		SELECT claimed.id, device.id::text, device.push_recipient_sealed,
		       device.encryption_public_key, device.locale,
		       device.notification_content, inbox.event_kind,
		       inbox.incident_id::text, coalesce(inbox.burst_id::text, ''),
		       claimed.revision, claimed.presentation,
		       inbox.target_name, inbox.nature_label, inbox.severity,
		       inbox.incident_count, inbox.affected_target_count,
		       inbox.max_affected_targets, coalesce(inbox.burst_status, ''),
		       inbox.burst_extended, inbox.occurred_at
		FROM claimed
		JOIN cairnops_devices device ON device.id = claimed.device_id
		JOIN cairnops_notification_inbox inbox ON inbox.id = claimed.inbox_id
	`, strings.TrimSpace(workerID)).Scan(
		&delivery.ID, &delivery.DeviceID, &delivery.RecipientSealed,
		&delivery.EncryptionPublicKey, &delivery.Locale,
		&delivery.NotificationContent, &delivery.EventKind,
		&delivery.IncidentID, &delivery.BurstID, &delivery.Revision,
		&delivery.PresentationMode, &delivery.TargetName, &delivery.NatureLabel,
		&delivery.Severity, &delivery.IncidentCount, &delivery.AffectedTargets,
		&delivery.MaxAffected, &delivery.BurstStatus, &delivery.BurstExtended,
		&delivery.OccurredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNoDelivery
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("claim push delivery: %w", err)
	}
	return delivery, nil
}

func (store *PostgresStore) Complete(ctx context.Context, deliveryID int64, workerID string) error {
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_push_outbox
		SET status = 'delivered', delivered_at = now(), last_error = '',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE id = $1 AND lease_owner = $2
	`, deliveryID, strings.TrimSpace(workerID))
	if err != nil {
		return fmt.Errorf("complete push delivery: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("push delivery lease lost")
	}
	return nil
}

func (store *PostgresStore) Fail(ctx context.Context, deliveryID int64, workerID, deliveryError string) error {
	deliveryError = strings.TrimSpace(deliveryError)
	if len(deliveryError) > 500 {
		deliveryError = deliveryError[:500]
	}
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_push_outbox
		SET status = 'failed', attempts = attempts + 1,
		    next_attempt_at = now() + least(interval '10 minutes', interval '5 seconds' * power(2, least(attempts, 7))),
		    last_error = $3, lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE id = $1 AND lease_owner = $2
	`, deliveryID, strings.TrimSpace(workerID), deliveryError)
	if err != nil {
		return fmt.Errorf("fail push delivery: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("push delivery lease lost")
	}
	return nil
}

func (store *PostgresStore) DisableDevice(ctx context.Context, deliveryID int64, workerID, deliveryError string) error {
	deliveryError = strings.TrimSpace(deliveryError)
	if len(deliveryError) > 500 {
		deliveryError = deliveryError[:500]
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin expired push recipient handling: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var deviceID string
	if err := tx.QueryRow(ctx, `
		UPDATE cairnops_push_outbox
		SET status = 'cancelled', last_error = $3,
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE id = $1 AND lease_owner = $2
		RETURNING device_id::text
	`, deliveryID, strings.TrimSpace(workerID), deliveryError).Scan(&deviceID); err != nil {
		return fmt.Errorf("cancel expired push delivery: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_devices
		SET push_disabled_at = now(), updated_at = now()
		WHERE id = $1::uuid AND revoked_at IS NULL
	`, deviceID); err != nil {
		return fmt.Errorf("disable expired push recipient: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_push_outbox
		SET status = 'cancelled', last_error = $2,
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE device_id = $1::uuid AND status IN ('pending', 'failed')
	`, deviceID, deliveryError); err != nil {
		return fmt.Errorf("cancel expired device deliveries: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit expired push recipient handling: %w", err)
	}
	return nil
}

func (store *PostgresStore) SetRelayStatus(ctx context.Context, configured bool, relayError error) error {
	status := "operational"
	lastError := ""
	if !configured || relayError != nil {
		status = "unavailable"
	}
	if relayError != nil {
		lastError = relayError.Error()
		if len(lastError) > 500 {
			lastError = lastError[:500]
		}
	}
	_, err := store.pool.Exec(ctx, `
		UPDATE cairnops_push_relay_status
		SET configured = $1, status = $2, last_checked_at = now(),
		    last_success_at = CASE WHEN $2 = 'operational' THEN now() ELSE last_success_at END,
		    last_error = $3, updated_at = now()
		WHERE singleton = true
	`, configured, status, lastError)
	if err != nil {
		return fmt.Errorf("record push relay status: %w", err)
	}
	return nil
}
