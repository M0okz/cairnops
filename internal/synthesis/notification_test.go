package synthesis

import (
	"strings"
	"testing"
	"unicode/utf8"
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

func TestNotificationAbbreviationPreservesLocalMeaningAndDetailedEvidence(t *testing.T) {
	label := "Linux: Load average is too high (per CPU load over 1.5 for 5m)"
	s := Situation{NatureKey: "zabbix:connector:load", NatureScope: "connector", NatureLabel: label, TargetName: "VictoriaLogs", AffectedTargets: 1, Severity: "major"}
	if got := LocalizeNotification(s); got.FR != (Text{"Charge système élevée", "VictoriaLogs · majeur"}) || got.EN != (Text{"High system load", "VictoriaLogs · major"}) {
		t.Fatalf("wrong load presentation: %#v", got)
	}
	if got := Render(s, "fr"); got.Title != "Signalement : "+label || s.NatureLabel != label || s.NatureScope != "connector" {
		t.Fatalf("abbreviation changed the detailed facts: %#v, %#v", s, got)
	}
	// A description that merely resembles the known wording must remain literal.
	for _, label := range []string{
		"Linux: Load average is too high - false positive",
		"Linux: Load average is too high (but memory is healthy)",
		"Linux: Load average is not too high",
		"Local storage condition",
	} {
		s.NatureLabel = label
		if got := RenderNotification(s, "fr"); got.Title != label {
			t.Fatalf("invented a meaning for %q: %#v", label, got)
		}
	}
	s.NatureKey, s.NatureLabel = "storage.latency", "Provider-specific condition"
	if got := RenderNotification(s, "en"); got.Title != s.NatureLabel {
		t.Fatalf("a local key was promoted to a canonical conclusion: %#v", got)
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

func TestReportedNotificationLabelsStayFactual(t *testing.T) {
	for _, tt := range []struct{ label, fr, en string }{
		{"Linux: FS [/srv/nextcloud-data]: Space is critically low (used > 90%, total 97.9GB)", "Espace de stockage insuffisant", "Low storage space"},
		{"Linux: FS [/volume5]: Space is low (used > 80%, total 3554.9GB)", "Espace de stockage insuffisant", "Low storage space"},
		{"Proxmox VE: Node [pve-forum]: QEMU [dmz-nextcloud-01][/srv/nextcloud-data]: Filesystem used high", "Occupation du stockage élevée", "High filesystem usage"},
		{"Linux: Number of installed packages has been changed", "Paquets installés modifiés", "Installed packages changed"},
	} {
		s := Situation{NatureScope: "connector", NatureKey: "zabbix:connector:local", NatureLabel: tt.label, TargetName: "Host", AffectedTargets: 1, Severity: "major"}
		got := LocalizeNotification(s)
		if got.FR.Title != tt.fr || got.EN.Title != tt.en || got.FR.Body != "Host · majeur" || got.EN.Body != "Host · major" || s.NatureLabel != tt.label {
			t.Fatalf("incorrect abbreviation of %q: %#v", tt.label, got)
		}
		// Do not apply a source-specific translation to another integration.
		s.NatureKey = "webhook:custom"
		if got := RenderNotification(s, "fr"); got.Title != oneLine(tt.label, 80) {
			t.Fatalf("translated another integration's label: %#v", got)
		}
	}
}
