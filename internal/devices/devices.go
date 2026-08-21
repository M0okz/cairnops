package devices

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"golang.org/x/crypto/curve25519"
)

const (
	pairingLifetime      = 10 * time.Minute
	pairingSecretPurpose = "device-pairing-token-v1"
	PushRecipientPurpose = "device-push-recipient-v1"
	NotificationComplete = "complete"
	NotificationDiscreet = "discreet"
	NotificationMasked   = "masked"
)

var (
	ErrInvalidInput       = errors.New("invalid device input")
	ErrNotFound           = errors.New("device resource not found")
	ErrConflict           = errors.New("device resource conflict")
	ErrPairingExpired     = errors.New("device pairing expired")
	ErrCredentialConsumed = errors.New("device credential already consumed")
	ErrInvalidDevice      = errors.New("invalid device identity")
)

type Device struct {
	ID                  string     `json:"id"`
	UserID              string     `json:"user_id"`
	UserDisplayName     string     `json:"user_display_name"`
	Name                string     `json:"name"`
	Platform            string     `json:"platform"`
	AppVersion          string     `json:"app_version"`
	Locale              string     `json:"locale"`
	NotificationContent string     `json:"notification_content"`
	PushEnabled         bool       `json:"push_enabled"`
	LastSeenAt          *time.Time `json:"last_seen_at"`
	RevokedAt           *time.Time `json:"revoked_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type Pairing struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	ExpiresAt       time.Time  `json:"expires_at"`
	ClaimedName     string     `json:"claimed_name,omitempty"`
	ClaimedPlatform string     `json:"claimed_platform,omitempty"`
	ClaimedAt       *time.Time `json:"claimed_at,omitempty"`
	ConfirmedAt     *time.Time `json:"confirmed_at,omitempty"`
	DeviceID        string     `json:"device_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Invitation struct {
	Pairing   Pairing `json:"pairing"`
	Instance  string  `json:"instance_url"`
	Token     string  `json:"token"`
	QRPayload string  `json:"qr_payload"`
}

type ClaimInput struct {
	Name                string `json:"name"`
	Platform            string `json:"platform"`
	AppVersion          string `json:"app_version"`
	Locale              string `json:"locale"`
	NotificationContent string `json:"notification_content"`
	EncryptionPublicKey string `json:"encryption_public_key"`
	PushRecipient       string `json:"push_recipient"`
}

type UpdateInput struct {
	Name                *string `json:"name"`
	AppVersion          *string `json:"app_version"`
	Locale              *string `json:"locale"`
	NotificationContent *string `json:"notification_content"`
	EncryptionPublicKey *string `json:"encryption_public_key"`
	PushRecipient       *string `json:"push_recipient"`
}

type PairingResult struct {
	Status      string `json:"status"`
	DeviceID    string `json:"device_id,omitempty"`
	DeviceToken string `json:"device_token,omitempty"`
}

type AuthenticatedDevice struct {
	Principal identitymodel.Principal
	DeviceID  string
}

type normalizedClaim struct {
	Name                string
	Platform            string
	AppVersion          string
	Locale              string
	NotificationContent string
	EncryptionPublicKey []byte
	PushRecipient       string
}

func normalizeClaim(input ClaimInput) (normalizedClaim, error) {
	name := strings.TrimSpace(input.Name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 {
		return normalizedClaim{}, fmt.Errorf("%w: device name must contain between 1 and 100 characters", ErrInvalidInput)
	}
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	if platform != "ios" && platform != "android" {
		return normalizedClaim{}, fmt.Errorf("%w: platform must be ios or android", ErrInvalidInput)
	}
	appVersion := strings.TrimSpace(input.AppVersion)
	if !utf8.ValidString(appVersion) || utf8.RuneCountInString(appVersion) > 64 {
		return normalizedClaim{}, fmt.Errorf("%w: app version must contain at most 64 characters", ErrInvalidInput)
	}
	locale := strings.ToLower(strings.TrimSpace(input.Locale))
	if locale == "" {
		locale = "fr"
	}
	if locale != "fr" && locale != "en" {
		return normalizedClaim{}, fmt.Errorf("%w: locale must be fr or en", ErrInvalidInput)
	}
	content := strings.ToLower(strings.TrimSpace(input.NotificationContent))
	if content == "" {
		content = NotificationComplete
	}
	if !validNotificationContent(content) {
		return normalizedClaim{}, fmt.Errorf("%w: notification content must be complete, discreet or masked", ErrInvalidInput)
	}
	publicKey, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimSpace(input.EncryptionPublicKey))
	if err != nil || len(publicKey) != 32 {
		return normalizedClaim{}, fmt.Errorf("%w: encryption public key must be an unpadded base64url value containing 32 bytes", ErrInvalidInput)
	}
	probeScalar := make([]byte, curve25519.ScalarSize)
	for index := range probeScalar {
		probeScalar[index] = byte(index + 1)
	}
	if _, err := curve25519.X25519(probeScalar, publicKey); err != nil {
		return normalizedClaim{}, fmt.Errorf("%w: encryption public key is not a valid X25519 point", ErrInvalidInput)
	}
	recipient := strings.TrimSpace(input.PushRecipient)
	if !validOpaqueRecipient(recipient) {
		return normalizedClaim{}, fmt.Errorf("%w: push recipient must be an opaque value containing between 16 and 1024 characters", ErrInvalidInput)
	}
	return normalizedClaim{
		Name: name, Platform: platform, AppVersion: appVersion, Locale: locale,
		NotificationContent: content, EncryptionPublicKey: publicKey, PushRecipient: recipient,
	}, nil
}

func validOpaqueRecipient(value string) bool {
	if !utf8.ValidString(value) || len(value) < 16 || len(value) > 1024 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validNotificationContent(value string) bool {
	return value == NotificationComplete || value == NotificationDiscreet || value == NotificationMasked
}

func newToken() (string, [sha256.Size]byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", [sha256.Size]byte{}, fmt.Errorf("generate device token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	return token, sha256.Sum256([]byte(token)), nil
}

func tokenDigest(token string) ([sha256.Size]byte, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimSpace(token))
	if err != nil || len(decoded) != 32 {
		return [sha256.Size]byte{}, ErrInvalidDevice
	}
	return sha256.Sum256([]byte(strings.TrimSpace(token))), nil
}

func pairingPayload(instanceURL, token string) string {
	query := url.Values{}
	query.Set("instance", strings.TrimSuffix(instanceURL, "/"))
	query.Set("token", token)
	return "cairnops://pair?" + query.Encode()
}

func pairingStatus(now time.Time, pairing Pairing, cancelled bool, consumed bool) string {
	switch {
	case cancelled:
		return "cancelled"
	case !now.Before(pairing.ExpiresAt) && pairing.ConfirmedAt == nil:
		return "expired"
	case consumed:
		return "credential_consumed"
	case pairing.ConfirmedAt != nil:
		return "confirmed"
	case pairing.ClaimedAt != nil:
		return "awaiting_confirmation"
	default:
		return "awaiting_scan"
	}
}
