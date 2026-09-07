package alerttext

import "testing"

func TestFactsRenderWithoutInventingMissingParameters(t *testing.T) {
	count := 1
	for _, tt := range []struct {
		fact   Fact
		locale string
		want   Text
	}{
		{Fact{Kind: DiskLatency, Resource: "nvme0n1"}, "fr", Text{"Latence disque élevée", "Ressource : nvme0n1"}},
		{Fact{Kind: CPUUsage}, "en", Text{"High CPU utilization", ""}},
		{Fact{Kind: SystemLoad}, "fr", Text{"Charge système moyenne élevée", ""}},
		{Fact{Kind: SecurityUpdates, Count: &count}, "fr", Text{"Correctifs de sécurité requis", "1 correctif de sécurité disponible"}},
		{Fact{Kind: SecurityUpdates}, "en", Text{"Security updates required", ""}},
		{Fact{Kind: SoftwareUpdate, CurrentVersion: "1.0", AvailableVersion: "2.0"}, "en", Text{"Software update available", "Version 2.0 available · 1.0 deployed"}},
		{Fact{Kind: SoftwareUpdate, AvailableVersion: "2.0"}, "fr", Text{"Mise à jour logicielle disponible", ""}},
		{Fact{Kind: "custom", Resource: "CPU usage high"}, "fr", Text{}},
	} {
		if got := Render(tt.fact, tt.locale); got != tt.want {
			t.Errorf("%+v: got %+v want %+v", tt.fact, got, tt.want)
		}
	}
}

func TestCommonMeaningRequiresEveryMemberAndNeverBorrowsParameters(t *testing.T) {
	for _, tt := range []struct {
		facts []Fact
		want  Kind
	}{
		{nil, ""},
		{[]Fact{{Kind: DiskLatency, Resource: "sda"}, {Kind: DiskLatency, Resource: "sdb"}}, DiskLatency},
		{[]Fact{{Kind: CPUUsage}, {Kind: SystemLoad}}, ""},
		{[]Fact{{Kind: CPUUsage}, {}}, ""},
		{[]Fact{{}, {Kind: CPUUsage}}, ""},
		{[]Fact{{Kind: "custom"}}, ""},
	} {
		got := Common(tt.facts)
		if got.Kind != tt.want || got.Resource != "" || got.Count != nil {
			t.Errorf("unexpected shared fact: %+v", got)
		}
	}
}

func TestEveryCatalogMeaningHasDistinctBilingualTitles(t *testing.T) {
	for kind := range titles {
		localized := Localize(Fact{Kind: kind})
		if localized.FR.Title == "" || localized.EN.Title == "" {
			t.Fatalf("missing locale for %s: %+v", kind, localized)
		}
	}
	for _, locale := range []string{"fr", "en"} {
		cpu, _ := Title(CPUUsage, locale)
		load, _ := Title(SystemLoad, locale)
		disk, _ := Title(DiskSpace, locale)
		inodes, _ := Title(DiskInodes, locale)
		if cpu == load || disk == inodes {
			t.Fatalf("different physical quantities share a title in %s", locale)
		}
	}
}
