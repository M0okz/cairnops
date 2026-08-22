package devices

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/curve25519"
)

func TestNormalizeClaimAppliesMobileDefaults(t *testing.T) {
	publicKey := base64.RawURLEncoding.EncodeToString(curve25519.Basepoint)
	claim, err := normalizeClaim(ClaimInput{
		Name: "  Téléphone exploitation  ", Platform: "IOS",
		EncryptionPublicKey: publicKey, PushRecipient: strings.Repeat("r", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	if claim.Name != "Téléphone exploitation" || claim.Platform != "ios" || claim.Locale != "fr" || claim.NotificationContent != "complete" {
		t.Fatalf("unexpected normalized claim: %#v", claim)
	}
}

func TestNormalizeClaimRejectsInvalidCryptographicIdentity(t *testing.T) {
	_, err := normalizeClaim(ClaimInput{
		Name: "Phone", Platform: "ios", EncryptionPublicKey: "too-short",
		PushRecipient: strings.Repeat("r", 32),
	})
	if err == nil {
		t.Fatal("an invalid encryption key was accepted")
	}
}

func TestNormalizeClaimAllowsPushRegistrationAfterPairing(t *testing.T) {
	publicKey := base64.RawURLEncoding.EncodeToString(curve25519.Basepoint)
	claim, err := normalizeClaim(ClaimInput{
		Name: "iPhone", Platform: "ios", EncryptionPublicKey: publicKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if claim.PushRecipient != "" {
		t.Fatalf("unexpected provisional push recipient: %q", claim.PushRecipient)
	}
}

func TestPairingPayloadContainsInstanceAndSecret(t *testing.T) {
	payload := pairingPayload("https://cairnops.example.test", "secret-token")
	if !strings.HasPrefix(payload, "cairnops://pair?") || !strings.Contains(payload, "instance=https%3A%2F%2Fcairnops.example.test") || !strings.Contains(payload, "token=secret-token") {
		t.Fatalf("unexpected pairing payload: %s", payload)
	}
}
