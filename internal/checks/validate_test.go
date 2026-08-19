package checks

import (
	"encoding/json"
	"testing"

	"github.com/M0okz/cairnops/internal/domain"
)

func TestValidateConfigRejectsRelativeHTTPURL(t *testing.T) {
	t.Parallel()

	if err := ValidateConfig(domain.SourceHTTP, json.RawMessage(`{"url":"/health"}`)); err == nil {
		t.Fatal("expected relative HTTP URL to be rejected")
	}
}

func TestValidateConfigAcceptsHeartbeat(t *testing.T) {
	t.Parallel()

	if err := ValidateConfig(domain.SourceHeartbeat, json.RawMessage(`{"expected_every_seconds":60,"grace_seconds":10,"activated":false}`)); err != nil {
		t.Fatalf("expected heartbeat config to be valid: %v", err)
	}
}
