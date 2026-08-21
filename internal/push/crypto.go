package push

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const envelopeVersion = 1

var envelopeContext = []byte("cairnops-push-envelope-v1")

type Envelope struct {
	Version            int    `json:"version"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
}

func Encrypt(publicKey []byte, payload any) (Envelope, error) {
	if len(publicKey) != curve25519.PointSize {
		return Envelope{}, fmt.Errorf("invalid device encryption public key")
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("encode push payload: %w", err)
	}
	ephemeralPrivate := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(ephemeralPrivate); err != nil {
		return Envelope{}, fmt.Errorf("generate ephemeral push key: %w", err)
	}
	ephemeralPublic, err := curve25519.X25519(ephemeralPrivate, curve25519.Basepoint)
	if err != nil {
		return Envelope{}, fmt.Errorf("derive ephemeral push public key: %w", err)
	}
	sharedSecret, err := curve25519.X25519(ephemeralPrivate, publicKey)
	if err != nil {
		return Envelope{}, fmt.Errorf("derive push shared secret: %w", err)
	}
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, sharedSecret, nil, envelopeContext), key); err != nil {
		return Envelope{}, fmt.Errorf("derive push encryption key: %w", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Envelope{}, fmt.Errorf("initialize push encryption: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Envelope{}, fmt.Errorf("generate push nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, envelopeContext)
	return Envelope{
		Version:            envelopeVersion,
		EphemeralPublicKey: base64.RawURLEncoding.EncodeToString(ephemeralPublic),
		Nonce:              base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext:         base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}
