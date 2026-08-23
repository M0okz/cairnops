package indicators

import (
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
)

func TestZabbixCatalogKeepsExactItemAndSemanticDimension(t *testing.T) {
	cases := []struct {
		item                      zabbix.Item
		semantic, unit, dimension string
		scale                     float64
	}{
		{zabbix.Item{ID: "101", Name: "CPU utilization", Key: "system.cpu.util[,system,avg1]", Units: "%"}, "cpu.utilization", "percent", "", 1},
		{zabbix.Item{ID: "102", Name: "Disk space utilization", Key: `vfs.fs.size[/srv,pused]`, Units: "%"}, "filesystem.utilization", "percent", "/srv", 1},
		{zabbix.Item{ID: "103", Name: "Interface eth0: Bits received", Key: `net.if.in[eth0]`, Units: "bps"}, "network.in", "bytes_per_second", "eth0", .125},
	}
	for _, test := range cases {
		candidate, ok := zabbixCandidate(test.item)
		if !ok {
			t.Fatalf("item %s was not classified", test.item.ID)
		}
		if candidate.ExternalID != test.item.ID || candidate.SemanticKey != test.semantic || candidate.Unit != test.unit || candidate.Dimension != test.dimension {
			t.Fatalf("unexpected candidate: %#v", candidate)
		}
		if selectionScale(candidate.Metadata) != test.scale {
			t.Fatalf("unexpected scale for %s: %#v", test.item.ID, candidate.Metadata)
		}
	}
	if _, ok := zabbixCandidate(zabbix.Item{ID: "999", Name: "Arbitrary custom metric", Key: "custom.value"}); ok {
		t.Fatal("arbitrary Zabbix item entered the short catalog")
	}
}

func TestZabbixCatalogKeepsOneCanonicalCandidatePerSemanticDimension(t *testing.T) {
	candidates := zabbixCandidates([]zabbix.Item{
		{ID: "102", Name: "CPU idle time", Key: "system.cpu.util[,idle]", Units: "%"},
		{ID: "101", Name: "CPU utilization", Key: "system.cpu.util", Units: "%"},
		{ID: "201", Name: "Interface eth0: Bits received", Key: "net.if.in[eth0]", Units: "bps"},
		{ID: "202", Name: "Interface eth0: Bytes received", Key: "net.if.in[eth0,bytes]", Units: "B/s"},
	})
	if len(candidates) != 2 {
		t.Fatalf("expected a short catalog, got %#v", candidates)
	}
	if candidates[0].SemanticKey != "cpu.utilization" || candidates[0].ExternalID != "101" {
		t.Fatalf("unexpected canonical CPU: %#v", candidates[0])
	}
}

func TestUptimeKumaCatalogOnlyOffersPublishedCapabilities(t *testing.T) {
	latency := 42
	days := 12.5
	valid := true
	candidates := uptimeCandidates(uptimekuma.Monitor{ID: "12", Name: "API", ResponseMilliseconds: &latency, CertificateDaysRemaining: &days, CertificateValid: &valid})
	if len(candidates) != 3 {
		t.Fatalf("expected response and two certificate candidates, got %#v", candidates)
	}
	for _, candidate := range candidates {
		if !candidate.Available || !candidate.Recommended {
			t.Fatalf("published candidate should be recommended: %#v", candidate)
		}
	}

	withoutTLS := uptimeCandidates(uptimekuma.Monitor{ID: "13", Name: "TCP"})
	if len(withoutTLS) != 1 || withoutTLS[0].Available {
		t.Fatalf("unexpected no-TLS catalog: %#v", withoutTLS)
	}
}

func TestPatchMonCatalogPreselectsFourMaskableIndicators(t *testing.T) {
	last := time.Now().UTC().Add(-time.Minute)
	candidates := patchMonCandidates(patchmon.Host{ID: "host-1", LastUpdate: &last})
	if len(candidates) != 4 {
		t.Fatalf("expected four candidates, got %#v", candidates)
	}
	for _, candidate := range candidates {
		if !candidate.Recommended || !candidate.Available {
			t.Fatalf("PatchMon candidate is not intelligently preselected: %#v", candidate)
		}
	}
}

func TestConfigurationRejectsMalformedNestedIdentities(t *testing.T) {
	validTarget := "11111111-1111-4111-8111-111111111111"
	base := ApplyInput{Bindings: []BindingInput{{
		TargetID: validTarget, ExternalID: "host-1", ExternalName: "API", Enabled: true,
		Indicators: []Selection{{SemanticKey: "cpu.utilization", Label: "Utilisation CPU", ExternalID: "item-1", Unit: "percent"}},
	}}}
	if err := validateApply(base); err != nil {
		t.Fatalf("valid configuration rejected: %v", err)
	}
	base.Bindings[0].TargetID = "not-a-uuid"
	if err := validateApply(base); err == nil {
		t.Fatal("malformed target identity was accepted")
	}
	base.Bindings[0].TargetID = validTarget
	base.Profiles = []ProfileInput{{ID: "not-a-uuid", Name: "Linux", Specification: []ProfileEntry{{SemanticKey: "cpu.utilization", Enabled: true}}}}
	if err := validateApply(base); err == nil {
		t.Fatal("malformed profile identity was accepted")
	}
}

func TestValidateApplyLimitsPinsAndRejectsThresholdLikeArbitraryMetrics(t *testing.T) {
	valid := ApplyInput{Bindings: []BindingInput{{ExternalID: "host", ExternalName: "Host", Enabled: true, Indicators: []Selection{{SemanticKey: "cpu.utilization", Label: "CPU", ExternalID: "42", Unit: "percent"}}}}}
	if err := validateApply(valid); err != nil {
		t.Fatalf("valid semantic selection rejected: %v", err)
	}
	valid.Bindings[0].Indicators[0].SemanticKey = "custom.threshold"
	if err := validateApply(valid); err == nil {
		t.Fatal("arbitrary metric should not enter the semantic catalog")
	}
}
