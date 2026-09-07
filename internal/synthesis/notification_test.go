package synthesis

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/alerttext"
)

func TestNotificationKeepsTheIncidentFactsInBothLanguages(t *testing.T) {
	for _, tt := range []struct {
		name   string
		s      Situation
		fr, en Text
	}{
		{"information", Situation{AffectedTargets: 1, TargetName: "API", Severity: "information"}, Text{"Indisponibilité", "API · information"}, Text{"Unavailability", "API · information"}},
		{"warning", Situation{AffectedTargets: 1, TargetName: "API", Severity: "warning"}, Text{"Indisponibilité", "API · avertissement"}, Text{"Unavailability", "API · warning"}},
		{"major group", Situation{AffectedTargets: 3, TargetName: "First VM", Severity: "major"}, Text{"Indisponibilité", "3 Cibles concernées · majeur"}, Text{"Unavailability", "3 affected targets · major"}},
		{"critical", Situation{AffectedTargets: 1, TargetName: "API", Severity: "critical"}, Text{"Indisponibilité", "API · critique"}, Text{"Unavailability", "API · critical"}},
		{"unnamed target", Situation{AffectedTargets: 1, Severity: "warning"}, Text{"Indisponibilité", "1 Cible concernée · avertissement"}, Text{"Unavailability", "1 affected target · warning"}},
		{"pending recovery", Situation{AffectedTargets: 0}, Text{"Indisponibilité", "Aucune Cible encore affectée · rétablissement en cours de confirmation"}, Text{"Unavailability", "No targets still affected · recovery awaiting confirmation"}},
		{"resolved target", Situation{Resolved: true, MaxAffected: 1, TotalTargets: 1, TargetName: "API", Severity: "critical"}, Text{"Résolu · Indisponibilité", "API"}, Text{"Resolved · Unavailability", "API"}},
		{"resolved group", Situation{Resolved: true, MaxAffected: 3, TotalTargets: 5}, Text{"Résolu · Indisponibilité", "Jusqu’à 3 Cibles concernées"}, Text{"Resolved · Unavailability", "Up to 3 affected targets"}},
		{"successive targets", Situation{Resolved: true, MaxAffected: 1, TotalTargets: 2, TargetName: "First VM"}, Text{"Résolu · Indisponibilité", "Jusqu’à 1 Cible concernée à la fois"}, Text{"Resolved · Unavailability", "Up to 1 affected target at a time"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.s
			s.NatureScope, s.NatureKey = "canonical", "availability"
			if got := LocalizeNotification(s); got.FR != tt.fr || got.EN != tt.en {
				t.Fatalf("wrong localized notification: %#v", got)
			}
		})
	}
}

func TestUnknownNotificationTitlesAreReadableAndBounded(t *testing.T) {
	s := Situation{NatureScope: "connector", NatureLabel: "  Préfixe\n" + strings.Repeat("é", 100), TargetName: "\t API\n publique  ", AffectedTargets: 1, Severity: "future"}
	got := RenderNotification(s, "fr")
	if !utf8.ValidString(got.Title) || utf8.RuneCountInString(got.Title) != 80 || !strings.HasSuffix(got.Title, "…") || strings.Contains(got.Title, "\n") || got.Body != "API publique" {
		t.Fatalf("unreadable fallback: %#v", got)
	}
	s.NatureLabel = "\n\t"
	if got := LocalizeNotification(s); got.FR.Title != "Signal de supervision" || got.EN.Title != "Monitoring signal" {
		t.Fatalf("missing fallback title: %#v", got)
	}
}

func TestNotificationTranslationsRequireStructuredFacts(t *testing.T) {
	for _, tt := range []struct {
		kind          alerttext.Kind
		label, fr, en string
	}{
		{alerttext.SystemLoad, "Linux: Load average is too high (per CPU load over 1.5 for 5m)", "Charge système moyenne élevée", "High average system load"},
		{alerttext.CPUUsage, "A user-renamed standard CPU condition", "Utilisation CPU élevée", "High CPU utilization"},
		{alerttext.DiskLatency, "Linux: sda: Disk read/write request responses are too high", "Latence disque élevée", "High disk latency"},
		{alerttext.DiskSpace, "Custom wording for a recognized filesystem condition", "Espace disque insuffisant", "Low disk space"},
	} {
		s := Situation{AlertKind: tt.kind, NatureScope: "connector", NatureKey: "provider:local", NatureLabel: tt.label, TargetName: "Host", AffectedTargets: 1, Severity: "major"}
		got := LocalizeNotification(s)
		if got.FR.Title != tt.fr || got.EN.Title != tt.en || got.FR.Body != "Host · majeur" || got.EN.Body != "Host · major" {
			t.Fatalf("wrong translation: %+v", got)
		}
		if Render(s, "fr").Title != tt.fr || s.NatureLabel != tt.label {
			t.Fatal("detail diverged or source text was overwritten")
		}
		// The exact same source text cannot establish a meaning without enrichment.
		s.AlertKind = ""
		for _, scope := range []string{"zabbix:connector:local", "webhook:custom", "storage.latency"} {
			s.NatureKey = scope
			if got := RenderNotification(s, "fr"); got.Title != oneLine(tt.label, 80) {
				t.Fatalf("source text guessed a meaning: %+v", got)
			}
		}
	}
}

func TestPresentationCatalogCannotExpandCanonicalNatures(t *testing.T) {
	for _, key := range []string{"cpu.usage.high", "system.load.high", "disk.latency.high", "software.security_updates", "certificate.invalid"} {
		if _, known := NatureLabel(key, "fr"); known {
			t.Fatalf("presentation key became a canonical Nature: %s", key)
		}
	}
}
