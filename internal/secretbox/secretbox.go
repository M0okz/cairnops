package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const keySize = 32

type Box struct {
	aead cipher.AEAD
}

func New(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("master key must contain exactly %d bytes", keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize master cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize authenticated encryption: %w", err)
	}
	return &Box{aead: aead}, nil
}

func LoadOrCreate(path string) (*Box, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("master key path must not be empty")
	}
	key, err := readKey(path)
	if err == nil {
		return New(key)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create master key directory: %w", err)
	}
	key = make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key) + "\n"
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		key, err = readKey(path)
		if err != nil {
			return nil, err
		}
		return New(key)
	}
	if err != nil {
		return nil, fmt.Errorf("create master key: %w", err)
	}
	if _, err := file.WriteString(encoded); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write master key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close master key: %w", err)
	}
	return New(key)
}

func readKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("master key must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("master key permissions must not allow group or other access")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(key) != keySize {
		return nil, fmt.Errorf("master key must be an unpadded base64url value containing %d bytes", keySize)
	}
	return key, nil
}

func (box *Box) Seal(plaintext []byte, purpose string) (string, error) {
	if box == nil || box.aead == nil {
		return "", fmt.Errorf("secret box is not initialized")
	}
	nonce := make([]byte, box.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate secret nonce: %w", err)
	}
	sealed := box.aead.Seal(nonce, nonce, plaintext, []byte(purpose))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (box *Box) Open(encoded, purpose string) ([]byte, error) {
	if box == nil || box.aead == nil {
		return nil, fmt.Errorf("secret box is not initialized")
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(sealed) < box.aead.NonceSize()+box.aead.Overhead() {
		return nil, fmt.Errorf("invalid sealed secret")
	}
	nonce := sealed[:box.aead.NonceSize()]
	plaintext, err := box.aead.Open(nil, nonce, sealed[box.aead.NonceSize():], []byte(purpose))
	if err != nil {
		return nil, fmt.Errorf("open sealed secret: authentication failed")
	}
	return plaintext, nil
}
