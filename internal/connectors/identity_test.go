package connectors

import (
	"testing"

	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
)

func TestZabbixIdentityMatchesKumaTargetByIPAddress(t *testing.T) {
	t.Parallel()
	target := TargetIdentity{
		TargetReference: TargetReference{ID: "11111111-1111-4111-8111-111111111111", Name: "Reverse proxy"},
		Names:           []string{"Reverse proxy", "nginx-prod"},
		Addresses:       []string{"https://192.0.2.42:443/health"},
	}
	host := zabbix.Host{
		ID: "42", Name: "gw-prod", Technical: "gw-prod.internal",
		Interfaces: []zabbix.Interface{{Address: "192.0.2.42", Port: "10050", Main: true}},
	}

	matches := matchTargets(identityForZabbix(host), []TargetIdentity{target})

	if len(matches) != 1 || matches[0].Target.ID != target.ID {
		t.Fatalf("expected the existing target, got %#v", matches)
	}
	if len(matches[0].Evidence) != 1 || matches[0].Evidence[0].Kind != "same_ip" || matches[0].Evidence[0].Value != "192.0.2.42" {
		t.Fatalf("expected explainable IP evidence, got %#v", matches[0].Evidence)
	}
}

func TestKumaIdentityMatchesZabbixTargetByCanonicalHostname(t *testing.T) {
	t.Parallel()
	target := TargetIdentity{
		TargetReference: TargetReference{ID: "22222222-2222-4222-8222-222222222222", Name: "Serveur applicatif"},
		Addresses:       []string{"APP-01.INTERNAL."},
	}
	monitor := uptimekuma.Monitor{ID: "8", Name: "API publique", URL: "https://app-01.internal:8443/ready"}

	matches := matchTargets(identityForUptimeKuma(monitor), []TargetIdentity{target})

	if len(matches) != 1 || matches[0].Evidence[0].Kind != "same_hostname" || matches[0].Evidence[0].Value != "app-01.internal" {
		t.Fatalf("expected canonical hostname evidence, got %#v", matches)
	}
}

func TestSuggestedTargetRefusesAmbiguousAddress(t *testing.T) {
	t.Parallel()
	identity := DiscoveredIdentity{Addresses: []string{"192.0.2.42"}}
	targets := []TargetIdentity{
		{TargetReference: TargetReference{ID: "11111111-1111-4111-8111-111111111111", Name: "API"}, Addresses: []string{"192.0.2.42"}},
		{TargetReference: TargetReference{ID: "22222222-2222-4222-8222-222222222222", Name: "Site"}, Addresses: []string{"192.0.2.42"}},
	}

	if suggestion := suggestedTarget(matchTargets(identity, targets)); suggestion != nil {
		t.Fatalf("an ambiguous IP must not be preselected: %#v", suggestion)
	}
}

func TestBindingMetadataFeedsCrossConnectorIdentity(t *testing.T) {
	t.Parallel()
	identity := TargetIdentity{TargetReference: TargetReference{ID: "target", Name: "API publique"}}
	metadata := []byte(`{"technical_name":"app-prod","interfaces":[{"address":"192.0.2.42"}]}`)
	if err := addBindingIdentity(&identity, "zabbix", metadata); err != nil {
		t.Fatal(err)
	}
	monitor := uptimekuma.Monitor{ID: "8", Name: "Healthcheck", URL: "https://192.0.2.42/ready"}
	matches := matchTargets(identityForUptimeKuma(monitor), []TargetIdentity{identity})
	if len(matches) != 1 || matches[0].Evidence[0].Kind != "same_ip" {
		t.Fatalf("stored connector metadata did not feed matching: %#v", matches)
	}
}

func TestInfrastructureNameSuggestsExistingTarget(t *testing.T) {
	t.Parallel()
	target := TargetIdentity{
		TargetReference: TargetReference{ID: "target-bitwarden", Name: "Bitwarden"},
		Names:           []string{"Bitwarden"},
	}
	discovered := DiscoveredIdentity{Names: []string{"dmz-bitwarden-01"}}

	matches := matchTargets(discovered, []TargetIdentity{target})

	if len(matches) != 1 || matches[0].Target.ID != target.ID {
		t.Fatalf("expected the infrastructure alias to match, got %#v", matches)
	}
	if len(matches[0].Evidence) != 1 || matches[0].Evidence[0].Kind != "similar_name" || matches[0].Evidence[0].Value != "bitwarden" {
		t.Fatalf("expected explainable infrastructure-name evidence, got %#v", matches[0].Evidence)
	}
	if matches[0].Confidence != "low" {
		t.Fatalf("an infrastructure alias must remain a low-confidence suggestion: %#v", matches[0])
	}
	if suggestion := suggestedTarget(matches); suggestion != nil {
		t.Fatalf("a name alias alone must require confirmation: %#v", suggestion)
	}
}

func TestInfrastructureNameNormalizesSeparators(t *testing.T) {
	t.Parallel()
	target := TargetIdentity{
		TargetReference: TargetReference{ID: "target-victoria", Name: "Victoria Metrics"},
		Names:           []string{"Victoria Metrics"},
	}
	discovered := DiscoveredIdentity{Names: []string{"trust-victoria-metrics-01"}}

	matches := matchTargets(discovered, []TargetIdentity{target})

	if len(matches) != 1 || matches[0].Evidence[0].Kind != "similar_name" {
		t.Fatalf("expected spacing and separators to share an alias, got %#v", matches)
	}
}

func TestInfrastructureNameKeepsConflictingInstancesApart(t *testing.T) {
	t.Parallel()
	discovered := DiscoveredIdentity{Names: []string{"dmz-api-01"}}
	targets := []TargetIdentity{{
		TargetReference: TargetReference{ID: "target-api-02", Name: "trust-api-02"},
		Names:           []string{"trust-api-02"},
	}}

	if matches := matchTargets(discovered, targets); len(matches) != 0 {
		t.Fatalf("different explicit instances must not match: %#v", matches)
	}
}

func TestSupportingNameEvidenceBreaksSharedIPTie(t *testing.T) {
	t.Parallel()
	discovered := DiscoveredIdentity{Names: []string{"dmz-mailcow-01"}, Addresses: []string{"192.0.2.42"}}
	targets := []TargetIdentity{
		{TargetReference: TargetReference{ID: "target-proxy", Name: "Reverse proxy"}, Names: []string{"Reverse proxy"}, Addresses: []string{"192.0.2.42"}},
		{TargetReference: TargetReference{ID: "target-mailcow", Name: "Mailcow"}, Names: []string{"Mailcow"}, Addresses: []string{"192.0.2.42"}},
	}

	matches := matchTargets(discovered, targets)
	suggestion := suggestedTarget(matches)

	if suggestion == nil || suggestion.ID != "target-mailcow" {
		t.Fatalf("the corroborated target should win a shared-IP tie: matches=%#v suggestion=%#v", matches, suggestion)
	}
}
