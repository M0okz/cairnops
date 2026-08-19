package secretbox

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSealBindsCiphertextToPurpose(t *testing.T) {
	t.Parallel()
	box, err := New(bytes.Repeat([]byte{0x42}, keySize))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal([]byte("zabbix-token"), "connector:one")
	if err != nil {
		t.Fatal(err)
	}
	opened, err := box.Open(sealed, "connector:one")
	if err != nil {
		t.Fatal(err)
	}
	if string(opened) != "zabbix-token" {
		t.Fatalf("unexpected plaintext %q", opened)
	}
	if _, err := box.Open(sealed, "connector:two"); err == nil {
		t.Fatal("expected a different purpose to reject the ciphertext")
	}
}

func TestLoadOrCreatePersistsPrivateKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "secrets", "master.key")
	first, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := first.Seal([]byte("credential"), "test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := second.Open(sealed, "test")
	if err != nil || string(opened) != "credential" {
		t.Fatalf("persisted key did not reopen the secret: %q, %v", opened, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}
