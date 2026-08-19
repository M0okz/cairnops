package checks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/M0okz/cairnops/internal/domain"
)

func TestDNSCheckResolvesLocalhost(t *testing.T) {
	t.Parallel()

	config, _ := json.Marshal(DNSConfig{Name: "localhost", Type: "A", Expected: []string{"127.0.0.1"}})
	result := (DNS{}).Check(context.Background(), testSource(domain.SourceDNS, config))
	if result.Outcome != domain.OutcomeHealthy {
		t.Fatalf("expected healthy observation, got %#v", result)
	}
}

func TestDNSCheckRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	config, _ := json.Marshal(DNSConfig{Name: "localhost", Type: "NAPTR"})
	result := (DNS{}).Check(context.Background(), testSource(domain.SourceDNS, config))
	if result.Outcome != domain.OutcomeUnknown || result.Reason != "invalid_config" {
		t.Fatalf("expected unsupported query observation, got %#v", result)
	}
}
