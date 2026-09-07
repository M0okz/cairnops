package synthesis

import "testing"

func TestMinimalFactualPresentation(t *testing.T) {
	for _, tt := range []struct {
		name, locale string
		situation    Situation
		want         Text
	}{
		{"group", "fr", Situation{NatureScope: "canonical", NatureKey: "storage.latency", NatureLabel: "VM 1: vda slow", AffectedTargets: 15}, Text{"Latence disque élevée", "15 Cibles concernées"}},
		{"english", "en", Situation{NatureScope: "canonical", NatureKey: "storage.latency", AffectedTargets: 15}, Text{"High disk latency", "15 affected targets"}},
		{"one source suffices", "fr", Situation{NatureScope: "canonical", NatureKey: "availability", TargetName: "API", AffectedTargets: 1}, Text{"Indisponibilité", "API"}},
		{"partial recovery", "fr", Situation{NatureScope: "canonical", NatureKey: "storage.latency", AffectedTargets: 3, MaxAffected: 15}, Text{"Latence disque élevée", "3 Cibles concernées"}},
		{"resolved", "fr", Situation{NatureScope: "canonical", NatureKey: "storage.latency", Resolved: true, MaxAffected: 15}, Text{"Résolu · Latence disque élevée", "Jusqu’à 15 Cibles concernées"}},
		{"resolved sequential targets", "fr", Situation{NatureScope: "canonical", NatureKey: "storage.latency", Resolved: true, MaxAffected: 1, TotalTargets: 2, TargetName: "First VM"}, Text{"Résolu · Latence disque élevée", "Jusqu’à 1 Cible concernée à la fois"}},
		{"resolved sequential targets english", "en", Situation{NatureScope: "canonical", NatureKey: "storage.latency", Resolved: true, MaxAffected: 1, TotalTargets: 2, TargetName: "First VM"}, Text{"Resolved · High disk latency", "Up to 1 affected target at a time"}},
		{"local nature", "fr", Situation{NatureScope: "canonical", NatureKey: "zabbix:local", NatureLabel: "Disk\nwarning", AffectedTargets: 2}, Text{"Signalement : Disk warning", "2 Cibles concernées"}},
		{"no invented recovery", "fr", Situation{NatureScope: "canonical", NatureKey: "availability"}, Text{"Indisponibilité", "Aucune Cible encore affectée · rétablissement en cours de confirmation"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Render(tt.situation, tt.locale); got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLocalKeyCannotClaimACanonicalMeaning(t *testing.T) {
	got := Render(Situation{NatureKey: "storage.latency", NatureScope: "connector", NatureLabel: "Provider-specific condition", AffectedTargets: 2}, "fr")
	if got.Title != "Signalement : Provider-specific condition" {
		t.Fatalf("local key was promoted to a canonical conclusion: %#v", got)
	}
}

func TestWorseningChangesTheVisibleMessage(t *testing.T) {
	s := Situation{NatureKey: "storage.latency", NatureScope: "canonical", AffectedTargets: 15, Severity: "warning"}
	before := Render(s, "fr")
	s.Severity = "critical"
	after := Render(s, "fr")
	if before == after || after.Body != "15 Cibles concernées · gravité critique" {
		t.Fatalf("worsening is not visible: %#v -> %#v", before, after)
	}
}
