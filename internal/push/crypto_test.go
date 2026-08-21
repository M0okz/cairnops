package push

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

func TestEncryptProducesEnvelopeDecryptableOnlyByDevice(t *testing.T) {
	privateKey := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		t.Fatal(err)
	}
	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := Encrypt(publicKey, map[string]string{"incident_id": "incident-1"})
	if err != nil {
		t.Fatal(err)
	}
	ephemeralPublic, _ := base64.RawURLEncoding.DecodeString(envelope.EphemeralPublicKey)
	nonce, _ := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	ciphertext, _ := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	sharedSecret, err := curve25519.X25519(privateKey, ephemeralPublic)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, sharedSecret, nil, envelopeContext), key); err != nil {
		t.Fatal(err)
	}
	aead, _ := chacha20poly1305.NewX(key)
	plaintext, err := aead.Open(nil, nonce, ciphertext, envelopeContext)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(plaintext, &payload); err != nil || payload["incident_id"] != "incident-1" {
		t.Fatalf("unexpected decrypted payload: %s (%v)", plaintext, err)
	}
}

func TestMaskedPresentationDoesNotExposeIncidentDetails(t *testing.T) {
	presentation := presentationFor(Delivery{
		EventKind: "firing", TargetName: "Secret database", NatureLabel: "Unavailable",
		NotificationContent: "masked", Locale: "en",
	})
	encoded, _ := json.Marshal(presentation)
	if string(encoded) == "" || containsAny(string(encoded), "Secret database", "Unavailable") {
		t.Fatalf("masked presentation leaked details: %s", encoded)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if len(candidate) > 0 && stringContains(value, candidate) {
			return true
		}
	}
	return false
}

func stringContains(value, candidate string) bool {
	for index := 0; index+len(candidate) <= len(value); index++ {
		if value[index:index+len(candidate)] == candidate {
			return true
		}
	}
	return false
}
