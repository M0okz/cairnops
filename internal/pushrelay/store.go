package pushrelay

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/secretbox"
)

const (
	relayTokenSize       = 32
	deviceTokenPurpose   = "push-relay-device-token-v1"
	registrationFileMode = 0o600
)

var (
	ErrRegistrationNotFound = errors.New("push registration not found")
	ErrInvalidManagement    = errors.New("invalid push registration management token")
)

type Registration struct {
	DeviceToken string
	Platform    string
	Environment string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type RegistrationCredentials struct {
	Recipient       string `json:"recipient"`
	ManagementToken string `json:"management_token"`
}

type registrationRecord struct {
	Platform          string    `json:"platform"`
	Environment       string    `json:"environment"`
	DeviceTokenSealed string    `json:"device_token_sealed"`
	ManagementDigest  string    `json:"management_digest"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type FileStore struct {
	directory string
	secrets   *secretbox.Box
	now       func() time.Time
	mu        sync.RWMutex
}

func NewFileStore(directory string, secrets *secretbox.Box) (*FileStore, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, fmt.Errorf("registration directory must not be empty")
	}
	if secrets == nil {
		return nil, fmt.Errorf("registration secret box must not be nil")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create registration directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("protect registration directory: %w", err)
	}
	if err := ensurePrivateDirectory(directory); err != nil {
		return nil, err
	}
	return &FileStore{directory: directory, secrets: secrets, now: time.Now}, nil
}

func (store *FileStore) Register(platform, environment, deviceToken string) (RegistrationCredentials, error) {
	platform, environment, deviceToken, err := normalizeRegistration(platform, environment, deviceToken)
	if err != nil {
		return RegistrationCredentials{}, err
	}
	recipient, err := newRelayToken()
	if err != nil {
		return RegistrationCredentials{}, err
	}
	managementToken, err := newRelayToken()
	if err != nil {
		return RegistrationCredentials{}, err
	}
	sealed, err := store.secrets.Seal([]byte(deviceToken), deviceTokenPurpose)
	if err != nil {
		return RegistrationCredentials{}, fmt.Errorf("seal APNs device token: %w", err)
	}
	now := store.now().UTC()
	record := registrationRecord{
		Platform: platform, Environment: environment, DeviceTokenSealed: sealed,
		ManagementDigest: digestToken(managementToken), CreatedAt: now, UpdatedAt: now,
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.writeRecord(recipient, record); err != nil {
		return RegistrationCredentials{}, err
	}
	return RegistrationCredentials{Recipient: recipient, ManagementToken: managementToken}, nil
}

func (store *FileStore) Rotate(recipient, managementToken, platform, environment, deviceToken string) error {
	platform, environment, deviceToken, err := normalizeRegistration(platform, environment, deviceToken)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.readRecord(recipient)
	if err != nil {
		return err
	}
	if !matchesToken(record.ManagementDigest, managementToken) {
		return ErrInvalidManagement
	}
	sealed, err := store.secrets.Seal([]byte(deviceToken), deviceTokenPurpose)
	if err != nil {
		return fmt.Errorf("seal rotated APNs device token: %w", err)
	}
	record.Platform = platform
	record.Environment = environment
	record.DeviceTokenSealed = sealed
	record.UpdatedAt = store.now().UTC()
	return store.writeRecord(recipient, record)
}

func (store *FileStore) Resolve(recipient string) (Registration, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	record, err := store.readRecord(recipient)
	if err != nil {
		return Registration{}, err
	}
	plaintext, err := store.secrets.Open(record.DeviceTokenSealed, deviceTokenPurpose)
	if err != nil {
		return Registration{}, fmt.Errorf("open APNs device token: %w", err)
	}
	return Registration{
		DeviceToken: string(plaintext), Platform: record.Platform, Environment: record.Environment,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func (store *FileStore) Delete(recipient, managementToken string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.readRecord(recipient)
	if err != nil {
		return err
	}
	if !matchesToken(record.ManagementDigest, managementToken) {
		return ErrInvalidManagement
	}
	return store.removeRecord(recipient)
}

func (store *FileStore) Expire(recipient string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.removeRecord(recipient)
}

func (store *FileStore) Ping() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	probe, err := os.CreateTemp(store.directory, ".health-*")
	if err != nil {
		return fmt.Errorf("create registration health probe: %w", err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close registration health probe: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove registration health probe: %w", err)
	}
	return nil
}

func normalizeRegistration(platform, environment, token string) (string, string, string, error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != "ios" {
		return "", "", "", fmt.Errorf("platform must be ios")
	}
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment != "sandbox" && environment != "production" {
		return "", "", "", fmt.Errorf("environment must be sandbox or production")
	}
	token = strings.ToLower(strings.TrimSpace(token))
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) == 0 || len(decoded) > 512 {
		return "", "", "", fmt.Errorf("device_token must be hexadecimal APNs token bytes")
	}
	return platform, environment, token, nil
}

func ensurePrivateDirectory(directory string) error {
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("inspect registration directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("registration path must be a directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("registration directory permissions must not allow group or other access")
	}
	return nil
}

func newRelayToken() (string, error) {
	raw := make([]byte, relayTokenSize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate relay capability: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func digestToken(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func matchesToken(expectedDigest, token string) bool {
	expected, expectedErr := base64.RawURLEncoding.Strict().DecodeString(expectedDigest)
	actualDigest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return expectedErr == nil && len(expected) == sha256.Size &&
		subtle.ConstantTimeCompare(expected, actualDigest[:]) == 1
}

func validRelayToken(token string) bool {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimSpace(token))
	return err == nil && len(raw) == relayTokenSize
}

func (store *FileStore) recordPath(recipient string) (string, error) {
	if !validRelayToken(recipient) {
		return "", ErrRegistrationNotFound
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(recipient)))
	return filepath.Join(store.directory, hex.EncodeToString(digest[:])+".json"), nil
}

func (store *FileStore) readRecord(recipient string) (registrationRecord, error) {
	path, err := store.recordPath(recipient)
	if err != nil {
		return registrationRecord{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registrationRecord{}, ErrRegistrationNotFound
	}
	if err != nil {
		return registrationRecord{}, fmt.Errorf("read push registration: %w", err)
	}
	var record registrationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return registrationRecord{}, fmt.Errorf("decode push registration: %w", err)
	}
	return record, nil
}

func (store *FileStore) writeRecord(recipient string, record registrationRecord) error {
	path, err := store.recordPath(recipient)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode push registration: %w", err)
	}
	temporary, err := os.CreateTemp(store.directory, ".registration-*")
	if err != nil {
		return fmt.Errorf("create push registration file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(registrationFileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect push registration file: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write push registration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync push registration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close push registration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace push registration: %w", err)
	}
	return nil
}

func (store *FileStore) removeRecord(recipient string) error {
	path, err := store.recordPath(recipient)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return ErrRegistrationNotFound
	} else if err != nil {
		return fmt.Errorf("remove push registration: %w", err)
	}
	return nil
}
