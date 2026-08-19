package controlplane

import (
	"errors"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/incidents"
)

func TestValidSeverity(t *testing.T) {
	t.Parallel()

	for _, severity := range []incidents.Severity{
		incidents.SeverityInformation, incidents.SeverityWarning,
		incidents.SeverityMajor, incidents.SeverityCritical,
	} {
		if !validSeverity(severity) {
			t.Fatalf("expected %q to be accepted", severity)
		}
	}
	for _, severity := range []incidents.Severity{"", "down", "MAJOR", "urgent"} {
		if validSeverity(severity) {
			t.Fatalf("expected %q to be rejected", severity)
		}
	}
}

func TestValidateHeartbeatPayload(t *testing.T) {
	t.Parallel()

	duration := 1200
	if err := validateHeartbeatPayload(HeartbeatPayload{Status: "ok", DurationMilliseconds: &duration, Message: "backup complete"}); err != nil {
		t.Fatalf("expected payload to be valid: %v", err)
	}
	if err := validateHeartbeatPayload(HeartbeatPayload{Status: "unknown"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid status error, got %v", err)
	}
	if err := validateHeartbeatPayload(HeartbeatPayload{Message: strings.Repeat("x", 501)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected long message error, got %v", err)
	}
}
