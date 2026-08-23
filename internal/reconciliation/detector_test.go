package reconciliation

import (
	"testing"

	"github.com/M0okz/cairnops/internal/connectors"
)

func TestConservativeConfidenceKeepsNameOnlyAsWeakLead(t *testing.T) {
	match := connectors.TargetMatch{
		Confidence: "high",
		Score:      100,
		Evidence:   []connectors.MatchEvidence{{Kind: "same_name", Value: "authentik"}},
	}
	if got := conservativeConfidence(match); got != "low" {
		t.Fatalf("name-only match became actionable: got %q", got)
	}
}

func TestConservativeConfidencePromotesCorroboratedAddress(t *testing.T) {
	match := connectors.TargetMatch{Evidence: []connectors.MatchEvidence{
		{Kind: "same_name", Value: "authentik"},
		{Kind: "same_hostname", Value: "trust-auth-01.example.test"},
	}}
	if got := conservativeConfidence(match); got != "high" {
		t.Fatalf("corroborated address confidence = %q, want high", got)
	}
}

func TestConservativeConfidenceKeepsStableIdentityStrong(t *testing.T) {
	match := connectors.TargetMatch{Evidence: []connectors.MatchEvidence{
		{Kind: "same_machine_id", Value: "machine-42"},
	}}
	if got := conservativeConfidence(match); got != "high" {
		t.Fatalf("stable identity confidence = %q, want high", got)
	}
}

func TestConservativeConfidenceAbstainsOnExplicitContradiction(t *testing.T) {
	match := connectors.TargetMatch{
		Evidence: []connectors.MatchEvidence{
			{Kind: "same_name", Value: "authentik"},
			{Kind: "same_hostname", Value: "auth.example.test"},
		},
		Contradictions: []connectors.MatchEvidence{{Kind: "different_machine_id", Value: "a ≠ b"}},
	}
	if got := conservativeConfidence(match); got != "low" {
		t.Fatalf("explicit contradiction became actionable: %q", got)
	}
}
