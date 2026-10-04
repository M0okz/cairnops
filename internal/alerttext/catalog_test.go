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
		{Fact{Kind: SoftwareSecurityUpdate, CurrentVersion: "1.0", AvailableVersion: "1.1"}, "fr", Text{"Mise à jour de sécurité disponible", "Version 1.1 disponible · 1.0 déployée"}},
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

// Le classement doit rester exhaustif : une Nature ajoutée au catalogue sans
// décision explicite ferait silencieusement basculer une Ressource du côté
// « Disponible », ou l'inverse. Ce test force la décision.
func TestEveryRecognizedKindDeclaresWhetherItImpairsFunctioning(t *testing.T) {
	expected := map[Kind]bool{
		BackupFailure:          true,
		BackupFreshness:        true,
		DiskLatency:            true,
		DiskSpace:              true,
		DiskInodes:             true,
		CPUUsage:               true,
		SystemLoad:             true,
		MemoryUsage:            true,
		MemoryAvailable:        true,
		SwapSpace:              true,
		CertificateInvalid:     true,
		Unavailable:            false,
		CertificateExpiry:      false,
		RebootRequired:         false,
		PackageCountChanged:    false,
		SecurityUpdates:        false,
		SoftwareUpdate:         false,
		SoftwareSecurityUpdate: false,
		SoftwareMajorUpdate:    false,
	}
	for kind := range titles {
		want, declared := expected[kind]
		if !declared {
			t.Errorf("la Nature %q est reconnue mais ce test ne dit pas si elle altère le fonctionnement", kind)
			continue
		}
		if got := kind.ImpairsFunctioning(); got != want {
			t.Errorf("%q : altère le fonctionnement = %v, attendu %v", kind, got, want)
		}
	}
	for kind := range expected {
		if _, ok := titles[kind]; !ok {
			t.Errorf("la Nature %q n'est plus au catalogue : retirer sa décision", kind)
		}
	}
}

// Une mise à jour disponible ne dégrade jamais un service qui fonctionne :
// CONTEXT.md le dit du service comme de la mise à jour de sécurité.
func TestAvailableUpdatesNeverImpairFunctioning(t *testing.T) {
	for _, kind := range []Kind{SoftwareUpdate, SoftwareSecurityUpdate, SecurityUpdates} {
		if kind.ImpairsFunctioning() {
			t.Errorf("%q ne doit pas altérer le fonctionnement", kind)
		}
	}
}

// Une Nature locale de Connecteur n'est pas reconnue : elle reste un « Problème
// signalé » et ne peut pas établir une dégradation.
func TestUnknownKindDoesNotImpairFunctioning(t *testing.T) {
	if Kind("un.truc.inconnu").ImpairsFunctioning() {
		t.Error("une Nature inconnue ne doit pas établir une altération")
	}
	if Kind("").ImpairsFunctioning() {
		t.Error("une Nature vide ne doit pas établir une altération")
	}
}
