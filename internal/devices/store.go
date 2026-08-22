package devices

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool      *pgxpool.Pool
	secrets   *secretbox.Box
	publicURL string
	now       func() time.Time
}

func NewStore(pool *pgxpool.Pool, secrets *secretbox.Box, publicURL string) *Store {
	return &Store{
		pool: pool, secrets: secrets, publicURL: strings.TrimSuffix(publicURL, "/"), now: time.Now,
	}
}

func (store *Store) CreatePairing(ctx context.Context, userID string) (Invitation, error) {
	token, digest, err := newToken()
	if err != nil {
		return Invitation{}, err
	}
	expiresAt := store.now().UTC().Add(pairingLifetime)
	pairing := Pairing{Status: "awaiting_scan", ExpiresAt: expiresAt}
	if err := store.pool.QueryRow(ctx, `
		INSERT INTO cairnops_device_pairings (user_id, token_digest, expires_at)
		SELECT id, $2, $3
		FROM cairnops_users
		WHERE id = $1::uuid AND deactivated_at IS NULL
		RETURNING id::text, created_at
	`, userID, digest[:], expiresAt).Scan(&pairing.ID, &pairing.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, ErrNotFound
		}
		return Invitation{}, fmt.Errorf("create device pairing: %w", err)
	}
	return Invitation{
		Pairing: pairing, Instance: store.publicURL, Token: token,
		QRPayload: pairingPayload(store.publicURL, token),
	}, nil
}

func (store *Store) GetPairing(ctx context.Context, userID, pairingID string) (Pairing, error) {
	pairing, cancelled, consumed, err := store.readPairing(ctx, userID, pairingID)
	if err != nil {
		return Pairing{}, err
	}
	pairing.Status = pairingStatus(store.now().UTC(), pairing, cancelled, consumed)
	return pairing, nil
}

func (store *Store) readPairing(ctx context.Context, userID, pairingID string) (Pairing, bool, bool, error) {
	var pairing Pairing
	var cancelledAt, consumedAt *time.Time
	err := store.pool.QueryRow(ctx, `
		SELECT id::text, expires_at, coalesce(claimed_name, ''),
		       coalesce(claimed_platform, ''), claimed_at, confirmed_at,
		       coalesce(device_id::text, ''), created_at, cancelled_at,
		       credential_consumed_at
		FROM cairnops_device_pairings
		WHERE id = $1::uuid AND user_id = $2::uuid
	`, pairingID, userID).Scan(
		&pairing.ID, &pairing.ExpiresAt, &pairing.ClaimedName,
		&pairing.ClaimedPlatform, &pairing.ClaimedAt, &pairing.ConfirmedAt,
		&pairing.DeviceID, &pairing.CreatedAt, &cancelledAt, &consumedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pairing{}, false, false, ErrNotFound
	}
	if err != nil {
		return Pairing{}, false, false, fmt.Errorf("read device pairing: %w", err)
	}
	return pairing, cancelledAt != nil, consumedAt != nil, nil
}

func (store *Store) ClaimPairing(ctx context.Context, token string, input ClaimInput) (PairingResult, error) {
	digest, err := tokenDigest(token)
	if err != nil {
		return PairingResult{}, ErrNotFound
	}
	claim, err := normalizeClaim(input)
	if err != nil {
		return PairingResult{}, err
	}
	var sealedRecipient any
	if claim.PushRecipient != "" {
		sealed, err := store.secrets.Seal([]byte(claim.PushRecipient), PushRecipientPurpose)
		if err != nil {
			return PairingResult{}, fmt.Errorf("seal push recipient: %w", err)
		}
		sealedRecipient = sealed
	}
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_device_pairings
		SET claimed_name = $2, claimed_platform = $3, claimed_app_version = $4,
		    claimed_locale = $5, claimed_notification_content = $6,
		    claimed_encryption_public_key = $7,
		    claimed_push_recipient_sealed = $8, claimed_at = now()
		WHERE token_digest = $1 AND claimed_at IS NULL AND confirmed_at IS NULL
		  AND cancelled_at IS NULL AND expires_at > now()
	`, digest[:], claim.Name, claim.Platform, claim.AppVersion, claim.Locale,
		claim.NotificationContent, claim.EncryptionPublicKey, sealedRecipient)
	if err != nil {
		return PairingResult{}, fmt.Errorf("claim device pairing: %w", err)
	}
	if result.RowsAffected() != 1 {
		return PairingResult{}, store.pairingTokenError(ctx, digest[:])
	}
	return PairingResult{Status: "awaiting_confirmation"}, nil
}

func (store *Store) ConfirmPairing(ctx context.Context, userID, pairingID string) (Pairing, error) {
	token, digest, err := newToken()
	if err != nil {
		return Pairing{}, err
	}
	sealedToken, err := store.secrets.Seal([]byte(token), pairingSecretPurpose)
	if err != nil {
		return Pairing{}, fmt.Errorf("seal device credential: %w", err)
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Pairing{}, fmt.Errorf("begin device pairing confirmation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var pairing Pairing
	var appVersion, locale, content, recipientSealed string
	var publicKey []byte
	var cancelledAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id::text, expires_at, coalesce(claimed_name, ''),
		       coalesce(claimed_platform, ''), coalesce(claimed_app_version, ''),
		       coalesce(claimed_locale, ''), coalesce(claimed_notification_content, ''),
		       claimed_encryption_public_key, coalesce(claimed_push_recipient_sealed, ''),
		       claimed_at, confirmed_at, cancelled_at, coalesce(device_id::text, ''), created_at
		FROM cairnops_device_pairings
		WHERE id = $1::uuid AND user_id = $2::uuid
		FOR UPDATE
	`, pairingID, userID).Scan(
		&pairing.ID, &pairing.ExpiresAt, &pairing.ClaimedName,
		&pairing.ClaimedPlatform, &appVersion, &locale, &content,
		&publicKey, &recipientSealed, &pairing.ClaimedAt, &pairing.ConfirmedAt,
		&cancelledAt, &pairing.DeviceID, &pairing.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pairing{}, ErrNotFound
	}
	if err != nil {
		return Pairing{}, fmt.Errorf("lock device pairing: %w", err)
	}
	if cancelledAt != nil || pairing.ConfirmedAt != nil {
		return Pairing{}, fmt.Errorf("%w: pairing is no longer confirmable", ErrConflict)
	}
	if !store.now().UTC().Before(pairing.ExpiresAt) {
		return Pairing{}, ErrPairingExpired
	}
	if pairing.ClaimedAt == nil {
		return Pairing{}, fmt.Errorf("%w: pairing has not been claimed", ErrConflict)
	}

	var deviceCreatedAt, deviceUpdatedAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO cairnops_devices (
			user_id, name, platform, app_version, locale, notification_content,
			encryption_public_key, push_recipient_sealed, token_digest
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id::text, created_at, updated_at
	`, userID, pairing.ClaimedName, pairing.ClaimedPlatform, appVersion, locale,
		content, publicKey, nullableText(recipientSealed), digest[:]).Scan(
		&pairing.DeviceID, &deviceCreatedAt, &deviceUpdatedAt,
	); err != nil {
		return Pairing{}, fmt.Errorf("create paired device: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		UPDATE cairnops_device_pairings
		SET confirmed_at = now(), device_id = $2::uuid, device_token_sealed = $3
		WHERE id = $1::uuid
		RETURNING confirmed_at
	`, pairing.ID, pairing.DeviceID, sealedToken).Scan(&pairing.ConfirmedAt); err != nil {
		return Pairing{}, fmt.Errorf("confirm device pairing: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Pairing{}, fmt.Errorf("commit device pairing: %w", err)
	}
	pairing.Status = "confirmed"
	return pairing, nil
}

func (store *Store) PairingResult(ctx context.Context, token string) (PairingResult, error) {
	digest, err := tokenDigest(token)
	if err != nil {
		return PairingResult{}, ErrNotFound
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PairingResult{}, fmt.Errorf("begin device credential retrieval: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var expiresAt time.Time
	var claimedAt, confirmedAt, cancelledAt, consumedAt *time.Time
	var deviceID, sealedToken string
	err = tx.QueryRow(ctx, `
		SELECT expires_at, claimed_at, confirmed_at, cancelled_at,
		       credential_consumed_at, coalesce(device_id::text, ''), device_token_sealed
		FROM cairnops_device_pairings
		WHERE token_digest = $1
		FOR UPDATE
	`, digest[:]).Scan(
		&expiresAt, &claimedAt, &confirmedAt, &cancelledAt, &consumedAt,
		&deviceID, &sealedToken,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PairingResult{}, ErrNotFound
	}
	if err != nil {
		return PairingResult{}, fmt.Errorf("read device pairing result: %w", err)
	}
	if cancelledAt != nil {
		return PairingResult{}, fmt.Errorf("%w: pairing was cancelled", ErrConflict)
	}
	if confirmedAt == nil && !store.now().UTC().Before(expiresAt) {
		return PairingResult{}, ErrPairingExpired
	}
	if consumedAt != nil || (confirmedAt != nil && sealedToken == "") {
		return PairingResult{}, ErrCredentialConsumed
	}
	if confirmedAt == nil {
		status := "awaiting_scan"
		if claimedAt != nil {
			status = "awaiting_confirmation"
		}
		return PairingResult{Status: status}, tx.Commit(ctx)
	}

	plaintext, err := store.secrets.Open(sealedToken, pairingSecretPurpose)
	if err != nil {
		return PairingResult{}, fmt.Errorf("open device credential: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_device_pairings
		SET credential_consumed_at = now(), device_token_sealed = ''
		WHERE token_digest = $1
	`, digest[:]); err != nil {
		return PairingResult{}, fmt.Errorf("consume device credential: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PairingResult{}, fmt.Errorf("commit device credential retrieval: %w", err)
	}
	return PairingResult{Status: "confirmed", DeviceID: deviceID, DeviceToken: string(plaintext)}, nil
}

func (store *Store) CancelPairing(ctx context.Context, userID, pairingID string) error {
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_device_pairings
		SET cancelled_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND confirmed_at IS NULL
		  AND cancelled_at IS NULL
	`, pairingID, userID)
	if err != nil {
		return fmt.Errorf("cancel device pairing: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (store *Store) pairingTokenError(ctx context.Context, digest []byte) error {
	var expiresAt time.Time
	var claimedAt, confirmedAt, cancelledAt *time.Time
	err := store.pool.QueryRow(ctx, `
		SELECT expires_at, claimed_at, confirmed_at, cancelled_at
		FROM cairnops_device_pairings WHERE token_digest = $1
	`, digest).Scan(&expiresAt, &claimedAt, &confirmedAt, &cancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read device pairing state: %w", err)
	}
	if cancelledAt != nil || claimedAt != nil || confirmedAt != nil {
		return fmt.Errorf("%w: pairing is no longer claimable", ErrConflict)
	}
	if !store.now().UTC().Before(expiresAt) {
		return ErrPairingExpired
	}
	return ErrConflict
}

func (store *Store) Authenticate(ctx context.Context, token string) (AuthenticatedDevice, error) {
	digest, err := tokenDigest(token)
	if err != nil {
		return AuthenticatedDevice{}, ErrInvalidDevice
	}
	var authenticated AuthenticatedDevice
	err = store.pool.QueryRow(ctx, `
		SELECT device.id::text, users.id::text, users.username,
		       users.display_name, users.role
		FROM cairnops_devices device
		JOIN cairnops_users users ON users.id = device.user_id
		WHERE device.token_digest = $1 AND device.revoked_at IS NULL
		  AND users.deactivated_at IS NULL
	`, digest[:]).Scan(
		&authenticated.DeviceID, &authenticated.Principal.ID,
		&authenticated.Principal.Username, &authenticated.Principal.DisplayName,
		&authenticated.Principal.Role,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthenticatedDevice{}, ErrInvalidDevice
	}
	if err != nil {
		return AuthenticatedDevice{}, fmt.Errorf("authenticate device: %w", err)
	}
	_, _ = store.pool.Exec(ctx, `
		UPDATE cairnops_devices SET last_seen_at = now(), updated_at = now()
		WHERE token_digest = $1
		  AND (last_seen_at IS NULL OR last_seen_at < now() - interval '5 minutes')
	`, digest[:])
	return authenticated, nil
}

func (store *Store) List(ctx context.Context, actor identitymodel.Principal) ([]Device, error) {
	filter := "WHERE device.user_id = $1::uuid"
	if actor.Role == "administrator" {
		filter = "WHERE ($1::uuid IS NOT NULL)"
	}
	rows, err := store.pool.Query(ctx, `
		SELECT device.id::text, device.user_id::text, users.display_name,
		       device.name, device.platform, device.app_version, device.locale,
		       device.notification_content,
		       device.push_recipient_sealed IS NOT NULL AND device.push_disabled_at IS NULL,
		       device.last_seen_at, device.revoked_at, device.created_at, device.updated_at
		FROM cairnops_devices device
		JOIN cairnops_users users ON users.id = device.user_id
		`+filter+`
		ORDER BY device.revoked_at NULLS FIRST, device.created_at DESC, device.id
	`, actor.ID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	items := make([]Device, 0)
	for rows.Next() {
		var device Device
		if err := rows.Scan(
			&device.ID, &device.UserID, &device.UserDisplayName, &device.Name,
			&device.Platform, &device.AppVersion, &device.Locale,
			&device.NotificationContent, &device.PushEnabled, &device.LastSeenAt,
			&device.RevokedAt, &device.CreatedAt, &device.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		items = append(items, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return items, nil
}

func (store *Store) Update(ctx context.Context, actor identitymodel.Principal, deviceID string, input UpdateInput) (Device, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Device{}, fmt.Errorf("begin device update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var device Device
	var publicKey []byte
	var recipientSealed string
	err = tx.QueryRow(ctx, `
		SELECT device.id::text, device.user_id::text, users.display_name,
		       device.name, device.platform, device.app_version, device.locale,
		       device.notification_content,
		       device.push_recipient_sealed IS NOT NULL AND device.push_disabled_at IS NULL,
		       device.encryption_public_key,
		       coalesce(device.push_recipient_sealed, ''), device.last_seen_at, device.revoked_at,
		       device.created_at, device.updated_at
		FROM cairnops_devices device
		JOIN cairnops_users users ON users.id = device.user_id
		WHERE device.id = $1::uuid
		  AND (device.user_id = $2::uuid OR $3 = 'administrator')
		FOR UPDATE OF device
	`, deviceID, actor.ID, actor.Role).Scan(
		&device.ID, &device.UserID, &device.UserDisplayName, &device.Name,
		&device.Platform, &device.AppVersion, &device.Locale,
		&device.NotificationContent, &device.PushEnabled, &publicKey, &recipientSealed,
		&device.LastSeenAt, &device.RevokedAt, &device.CreatedAt, &device.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("lock device: %w", err)
	}
	if device.RevokedAt != nil {
		return Device{}, fmt.Errorf("%w: revoked device cannot be changed", ErrConflict)
	}

	claim := ClaimInput{
		Name: device.Name, Platform: device.Platform, AppVersion: device.AppVersion,
		Locale: device.Locale, NotificationContent: device.NotificationContent,
		EncryptionPublicKey: encodePublicKey(publicKey), PushRecipient: "",
	}
	if input.Name != nil {
		claim.Name = *input.Name
	}
	if input.AppVersion != nil {
		claim.AppVersion = *input.AppVersion
	}
	if input.Locale != nil {
		claim.Locale = *input.Locale
	}
	if input.NotificationContent != nil {
		claim.NotificationContent = *input.NotificationContent
	}
	if input.EncryptionPublicKey != nil {
		claim.EncryptionPublicKey = *input.EncryptionPublicKey
	}
	if input.PushRecipient != nil {
		claim.PushRecipient = *input.PushRecipient
	}
	normalized, err := normalizeClaim(claim)
	if err != nil {
		return Device{}, err
	}
	if input.PushRecipient != nil {
		if normalized.PushRecipient == "" {
			recipientSealed = ""
		} else {
			recipientSealed, err = store.secrets.Seal([]byte(normalized.PushRecipient), PushRecipientPurpose)
			if err != nil {
				return Device{}, fmt.Errorf("seal push recipient: %w", err)
			}
		}
	}
	device.Name, device.AppVersion, device.Locale = normalized.Name, normalized.AppVersion, normalized.Locale
	device.NotificationContent, publicKey = normalized.NotificationContent, normalized.EncryptionPublicKey
	if err := tx.QueryRow(ctx, `
		UPDATE cairnops_devices
		SET name = $2, app_version = $3, locale = $4, notification_content = $5,
		    encryption_public_key = $6, push_recipient_sealed = $7,
		    push_disabled_at = CASE WHEN $8 THEN NULL ELSE push_disabled_at END,
		    updated_at = now()
		WHERE id = $1::uuid
		RETURNING updated_at
	`, device.ID, device.Name, device.AppVersion, device.Locale,
		device.NotificationContent, publicKey, nullableText(recipientSealed), input.PushRecipient != nil,
	).Scan(&device.UpdatedAt); err != nil {
		return Device{}, fmt.Errorf("update device: %w", err)
	}
	if input.PushRecipient != nil && normalized.PushRecipient == "" {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_push_outbox
			SET status = 'cancelled', last_error = 'Push désactivé',
			    lease_owner = NULL, lease_until = NULL, updated_at = now()
			WHERE device_id = $1::uuid AND status IN ('pending', 'failed')
		`, device.ID); err != nil {
			return Device{}, fmt.Errorf("cancel disabled device notifications: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Device{}, fmt.Errorf("commit device update: %w", err)
	}
	if input.PushRecipient != nil {
		device.PushEnabled = normalized.PushRecipient != ""
	}
	return device, nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func encodePublicKey(publicKey []byte) string {
	return base64.RawURLEncoding.EncodeToString(publicKey)
}

func (store *Store) Revoke(ctx context.Context, actor identitymodel.Principal, deviceID string) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin device revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `
		UPDATE cairnops_devices
		SET revoked_at = now(), updated_at = now()
		WHERE id = $1::uuid AND revoked_at IS NULL
		  AND (user_id = $2::uuid OR $3 = 'administrator')
	`, deviceID, actor.ID, actor.Role)
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_push_outbox
		SET status = 'cancelled', last_error = 'appareil révoqué',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE device_id = $1::uuid AND status IN ('pending', 'failed')
	`, deviceID); err != nil {
		return fmt.Errorf("cancel revoked device notifications: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit device revocation: %w", err)
	}
	return nil
}

func (store *Store) RevokeSelf(ctx context.Context, deviceID string) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin current device revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `
		UPDATE cairnops_devices SET revoked_at = now(), updated_at = now()
		WHERE id = $1::uuid AND revoked_at IS NULL
	`, deviceID)
	if err != nil {
		return fmt.Errorf("revoke current device: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_push_outbox
		SET status = 'cancelled', last_error = 'appareil révoqué',
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE device_id = $1::uuid AND status IN ('pending', 'failed')
	`, deviceID); err != nil {
		return fmt.Errorf("cancel current device notifications: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit current device revocation: %w", err)
	}
	return nil
}
