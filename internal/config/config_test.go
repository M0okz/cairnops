package config

import (
	"os"
	"testing"
)

func TestLoadServerRejectsEmptyAddress(t *testing.T) {
	t.Setenv("CAIRNOPS_HTTP_ADDRESS", "")
	t.Setenv("CAIRNOPS_DATABASE_URL", "")
	t.Setenv("CAIRNOPS_WEB_DIR", "")
	t.Setenv("CAIRNOPS_BOOTSTRAP_TOKEN", "a-valid-bootstrap-token-with-32-characters")

	cfg, err := LoadServer()
	if err == nil {
		t.Fatal("expected an empty explicit address to be rejected")
	}
	if cfg != (Server{}) {
		t.Fatalf("expected zero configuration on error, got %#v", cfg)
	}
}

func TestLoadServerRejectsShortBootstrapToken(t *testing.T) {
	t.Setenv("CAIRNOPS_HTTP_ADDRESS", ":8080")
	t.Setenv("CAIRNOPS_BOOTSTRAP_TOKEN", "too-short")

	if _, err := LoadServer(); err == nil {
		t.Fatal("expected short bootstrap token to be rejected")
	}
}

func TestLoadServerRejectsEmptyMasterKeyFile(t *testing.T) {
	t.Setenv("CAIRNOPS_BOOTSTRAP_TOKEN", "a-valid-bootstrap-token-with-32-characters")
	t.Setenv("CAIRNOPS_MASTER_KEY_FILE", "")

	if _, err := LoadServer(); err == nil {
		t.Fatal("expected an empty master key file to be rejected")
	}
}

func TestLoadServerRequiresHTTPSOutsideLocalhost(t *testing.T) {
	t.Setenv("CAIRNOPS_BOOTSTRAP_TOKEN", "a-valid-bootstrap-token-with-32-characters")
	t.Setenv("CAIRNOPS_PUBLIC_URL", "http://cairnops.example.com")

	if _, err := LoadServer(); err == nil {
		t.Fatal("expected plain HTTP remote URL to be rejected")
	}
}

func TestLoadServerAcceptsHTTPSPublicURL(t *testing.T) {
	t.Setenv("CAIRNOPS_BOOTSTRAP_TOKEN", "a-valid-bootstrap-token-with-32-characters")
	t.Setenv("CAIRNOPS_PUBLIC_URL", "https://cairnops.example.com")

	cfg, err := LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://cairnops.example.com" {
		t.Fatalf("unexpected public URL: %s", cfg.PublicURL)
	}
}

func TestLoadWorkerFromEnvironment(t *testing.T) {
	t.Setenv("CAIRNOPS_WORKER_HEALTH_ADDRESS", "127.0.0.1:9091")
	t.Setenv("CAIRNOPS_DATABASE_URL", "postgres://example")
	t.Setenv("CAIRNOPS_INSTANCE_ID", "worker-a")

	cfg, err := LoadWorker()
	if err != nil {
		t.Fatalf("LoadWorker returned an error: %v", err)
	}
	if cfg.HealthAddress != "127.0.0.1:9091" || cfg.DatabaseURL != "postgres://example" || cfg.InstanceID != "worker-a" {
		t.Fatalf("unexpected configuration: %#v", cfg)
	}
}

func TestLoadWorkerAcceptsHTTPSPushRelay(t *testing.T) {
	t.Setenv("CAIRNOPS_INSTANCE_ID", "worker-push")
	t.Setenv("CAIRNOPS_PUSH_RELAY_URL", "https://push.cairnops.example")
	cfg, err := LoadWorker()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PushRelayURL != "https://push.cairnops.example" {
		t.Fatalf("unexpected push relay URL: %s", cfg.PushRelayURL)
	}
}

func TestLoadWorkerRejectsUnencryptedPushRelay(t *testing.T) {
	t.Setenv("CAIRNOPS_INSTANCE_ID", "worker-push")
	t.Setenv("CAIRNOPS_PUSH_RELAY_URL", "http://push.cairnops.example")
	if _, err := LoadWorker(); err == nil {
		t.Fatal("an unencrypted Push Relay URL was accepted")
	}
}

func TestLoadPushRelayRequiresAPNSIdentity(t *testing.T) {
	t.Setenv("CAIRNOPS_RELAY_APNS_KEY_ID", "")
	t.Setenv("CAIRNOPS_RELAY_APNS_TEAM_ID", "TEAMID1234")
	if _, err := LoadPushRelay(); err == nil {
		t.Fatal("relay accepted a missing APNs key ID")
	}
}

func TestLoadPushRelayAcceptsAPNSIdentity(t *testing.T) {
	t.Setenv("CAIRNOPS_RELAY_APNS_KEY_ID", "KEYID12345")
	t.Setenv("CAIRNOPS_RELAY_APNS_TEAM_ID", "TEAMID1234")
	cfg, err := LoadPushRelay()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APNSTopic != "fr.cairnops.ios" || cfg.HTTPAddress != ":8082" {
		t.Fatalf("unexpected relay configuration: %#v", cfg)
	}
}

func TestLoadWorkerRejectsExplicitEmptyInstanceID(t *testing.T) {
	t.Setenv("CAIRNOPS_INSTANCE_ID", "")

	if _, err := LoadWorker(); err == nil {
		t.Fatal("expected explicit empty instance ID to be rejected")
	}
}

func TestWorkerInstanceIDFallsBackToHostname(t *testing.T) {
	original, existed := os.LookupEnv("CAIRNOPS_INSTANCE_ID")
	if err := os.Unsetenv("CAIRNOPS_INSTANCE_ID"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("CAIRNOPS_INSTANCE_ID", original)
		} else {
			_ = os.Unsetenv("CAIRNOPS_INSTANCE_ID")
		}
	})

	instanceID, err := workerInstanceID()
	if err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	if instanceID != hostname {
		t.Fatalf("expected hostname %q, got %q", hostname, instanceID)
	}
}
