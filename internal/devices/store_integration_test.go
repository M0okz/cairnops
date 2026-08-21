package devices

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/M0okz/cairnops/internal/testsupport"
	"golang.org/x/crypto/curve25519"
)

func TestPairingCreatesOneRevocableDeviceIdentity(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('mobile-admin', 'Mobile Admin', 'not-used', 'administrator')
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	secrets, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool, secrets, "https://cairnops.example.test")
	invitation, err := store.CreatePairing(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	var persistedDigest []byte
	if err := pool.QueryRow(ctx, `
		SELECT token_digest FROM cairnops_device_pairings WHERE id = $1::uuid
	`, invitation.Pairing.ID).Scan(&persistedDigest); err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256([]byte(invitation.Token))
	if !bytes.Equal(persistedDigest, wantDigest[:]) || bytes.Equal(persistedDigest, []byte(invitation.Token)) {
		t.Fatal("the pairing token was not persisted only as its SHA-256 digest")
	}

	privateKey := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		t.Fatal(err)
	}
	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	recipient := "opaque-relay-recipient-0123456789"
	if _, err := store.ClaimPairing(ctx, invitation.Token, ClaimInput{
		Name: "iPhone astreinte", Platform: "ios", AppVersion: "1.0.0",
		Locale: "fr", NotificationContent: "discreet",
		EncryptionPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
		PushRecipient:       recipient,
	}); err != nil {
		t.Fatal(err)
	}
	pairing, err := store.GetPairing(ctx, userID, invitation.Pairing.ID)
	if err != nil || pairing.Status != "awaiting_confirmation" {
		t.Fatalf("unexpected claimed pairing: %#v (%v)", pairing, err)
	}
	pairing, err = store.ConfirmPairing(ctx, userID, invitation.Pairing.ID)
	if err != nil || pairing.Status != "confirmed" || pairing.DeviceID == "" {
		t.Fatalf("unexpected confirmed pairing: %#v (%v)", pairing, err)
	}
	result, err := store.PairingResult(ctx, invitation.Token)
	if err != nil || result.DeviceToken == "" || result.DeviceID != pairing.DeviceID {
		t.Fatalf("credential was not returned once: %#v (%v)", result, err)
	}
	authenticated, err := store.Authenticate(ctx, result.DeviceToken)
	if err != nil || authenticated.DeviceID != pairing.DeviceID || authenticated.Principal.ID != userID {
		t.Fatalf("device identity does not authenticate its owner: %#v (%v)", authenticated, err)
	}
	if _, err := store.PairingResult(ctx, invitation.Token); !errors.Is(err, ErrCredentialConsumed) {
		t.Fatalf("credential was returned more than once: %v", err)
	}
	if err := store.Revoke(ctx, identitymodel.Principal{ID: userID, Role: "administrator"}, pairing.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, result.DeviceToken); !errors.Is(err, ErrInvalidDevice) {
		t.Fatalf("revoked device still authenticates: %v", err)
	}
	var sealedRecipient string
	if err := pool.QueryRow(ctx, `
		SELECT push_recipient_sealed FROM cairnops_devices WHERE id = $1::uuid
	`, pairing.DeviceID).Scan(&sealedRecipient); err != nil {
		t.Fatal(err)
	}
	if sealedRecipient == recipient {
		t.Fatal("the opaque relay recipient was persisted in plaintext")
	}
}
