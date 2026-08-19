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
