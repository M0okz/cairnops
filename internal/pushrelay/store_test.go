package pushrelay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/secretbox"
)

func TestFileStoreSeparatesRecipientAndManagementCapabilities(t *testing.T) {
	directory := t.TempDir()
	store := testFileStore(t, directory)
	credentials, err := store.Register("ios", "production", "001122aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	if !validRelayToken(credentials.Recipient) || !validRelayToken(credentials.ManagementToken) {
		t.Fatalf("unexpected relay credentials: %#v", credentials)
	}
	if credentials.Recipient == credentials.ManagementToken {
		t.Fatal("recipient and management capabilities must be distinct")
	}
	registration, err := store.Resolve(credentials.Recipient)
	if err != nil {
		t.Fatal(err)
	}
	if registration.DeviceToken != "001122aabbccddeeff" || registration.Platform != "ios" ||
		registration.Environment != "production" {
		t.Fatalf("unexpected registration: %#v", registration)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), registration.DeviceToken) ||
			strings.Contains(string(contents), credentials.Recipient) ||
			strings.Contains(string(contents), credentials.ManagementToken) {
			t.Fatalf("registration file exposed a raw capability: %s", contents)
		}
		var record registrationRecord
		if err := json.Unmarshal(contents, &record); err != nil {
			t.Fatal(err)
		}
		if record.DeviceTokenSealed == "" || record.ManagementDigest == "" {
			t.Fatalf("registration secrets were not protected: %#v", record)
		}
	}
}

func TestFileStoreRequiresManagementCapabilityForRotationAndDeletion(t *testing.T) {
	store := testFileStore(t, t.TempDir())
	credentials, err := store.Register("ios", "sandbox", "0011")
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := newRelayToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Rotate(credentials.Recipient, wrong, "ios", "production", "aabb"); err != ErrInvalidManagement {
		t.Fatalf("unexpected rotation error: %v", err)
	}
	if err := store.Rotate(credentials.Recipient, credentials.ManagementToken, "ios", "production", "aabb"); err != nil {
		t.Fatal(err)
	}
	registration, err := store.Resolve(credentials.Recipient)
	if err != nil {
		t.Fatal(err)
	}
	if registration.DeviceToken != "aabb" || registration.Environment != "production" {
		t.Fatalf("rotation did not update token: %#v", registration)
	}
	if err := store.Delete(credentials.Recipient, wrong); err != ErrInvalidManagement {
		t.Fatalf("unexpected deletion error: %v", err)
	}
	if err := store.Delete(credentials.Recipient, credentials.ManagementToken); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(credentials.Recipient); err != ErrRegistrationNotFound {
		t.Fatalf("deleted registration remains readable: %v", err)
	}
}

func TestRateLimiterResetsAfterItsWindow(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(2, time.Minute)
	limiter.now = func() time.Time { return now }
	if !limiter.Allow("recipient") || !limiter.Allow("recipient") || limiter.Allow("recipient") {
		t.Fatal("rate limiter did not enforce its window")
	}
	now = now.Add(time.Minute)
	if !limiter.Allow("recipient") {
		t.Fatal("rate limiter did not reset after its window")
	}
}

func testFileStore(t *testing.T, directory string) *FileStore {
	t.Helper()
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(directory, box)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
