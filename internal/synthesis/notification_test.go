package synthesis

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/alerttext"
)

func TestNotificationPutsTheResourceAndSeverityFirstInBothLanguages(t *testing.T) {
	for _, tt := range []struct {
		name   string
		s      Situation
		fr, en Text
	}{
		{"information", Situation{AffectedTargets: 1, TargetName: "API", Severity: "information"}, Text{"API · information", "Indisponibilité"}, Text{"API · information", "Unavailability"}},
		{"warning", Situation{AffectedTargets: 1, TargetName: "API", Severity: "warning"}, Text{"API · avertissement", "Indisponibilité"}, Text{"API · warning", "Unavailability"}},
		{"major group", Situation{AffectedTargets: 3, TargetName: "First VM", Severity: "major"}, Text{"3 Ressources · majeur", "Indisponibilité"}, Text{"3 resources · major", "Unavailability"}},
		{"critical", Situation{AffectedTargets: 1, TargetName: "API", Severity: "critical"}, Text{"API · critique", "Indisponibilité"}, Text{"API · critical", "Unavailability"}},
		{"unnamed target", Situation{AffectedTargets: 1, Severity: "warning"}, Text{"1 Ressource · avertissement", "Indisponibilité"}, Text{"1 resource · warning", "Unavailability"}},
		{"pending recovery", Situation{AffectedTargets: 0, Severity: "major"}, Text{"Rétablissement à confirmer", "Indisponibilité\nAucune Ressource encore affectée"}, Text{"Recovery awaiting confirmation", "Unavailability\nNo resources still affected"}},
		{"resolved target", Situation{Resolved: true, MaxAffected: 1, TotalTargets: 1, TargetName: "API", Severity: "critical"}, Text{"API · résolu", "Indisponibilité"}, Text{"API · resolved", "Unavailability"}},
		{"resolved group", Situation{Resolved: true, MaxAffected: 3, TotalTargets: 5}, Text{"5 Ressources · résolu", "Indisponibilité"}, Text{"5 resources · resolved", "Unavailability"}},
		{"successive targets", Situation{Resolved: true, MaxAffected: 1, TotalTargets: 2, TargetName: "First VM"}, Text{"2 Ressources · résolu", "Indisponibilité"}, Text{"2 resources · resolved", "Unavailability"}},
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

func TestNotificationContextLineStatesOnlyDeliveredFacts(t *testing.T) {
	count := 4
	for _, tt := range []struct {
		name   string
		s      Situation
		fr, en string
	}{
		{"no integration name", Situation{AffectedTargets: 1, TargetName: "Vikunja Todo", Severity: "critical", Context: Context{}},
			"Indisponibilité", "Unavailability"},
		{"technical resource", Situation{AlertKind: alerttext.DiskSpace, AffectedTargets: 1, TargetName: "trust-cairnops-01", Severity: "major", Context: Context{Fact: &alerttext.Fact{Kind: alerttext.DiskSpace, Resource: "/var"}}},
			"Espace disque insuffisant\n/var", "Low disk space\n/var"},
		{"security update count", Situation{AlertKind: alerttext.SecurityUpdates, AffectedTargets: 1, TargetName: "dmz-docker-01", Severity: "warning", Context: Context{Fact: &alerttext.Fact{Kind: alerttext.SecurityUpdates, Count: &count}}},
			"Correctifs de sécurité requis\n4 correctifs de sécurité disponibles", "Security updates required\n4 security updates available"},
		{"escalation", Situation{AffectedTargets: 1, TargetName: "trust-cairnops-01", Severity: "critical", Context: Context{PreviousSeverity: "major"}},
			"Indisponibilité\nAuparavant : majeur", "Unavailability\nPreviously: major"},
		{"lower previous alert is not an escalation", Situation{AffectedTargets: 1, TargetName: "API", Severity: "warning", Context: Context{PreviousSeverity: "major"}},
			"Indisponibilité", "Unavailability"},
		{"group names", Situation{AffectedTargets: 5, Severity: "critical", Extended: true, Context: Context{TargetNames: []string{"Vikunja Todo", "Outline", "Gitea"}}},
			"Indisponibilité\nPropagation étendue · Vikunja Todo, Outline, Gitea +2", "Unavailability\nExtended propagation · Vikunja Todo, Outline, Gitea +2"},
		{"resolution duration", Situation{Resolved: true, MaxAffected: 1, TotalTargets: 1, TargetName: "Vikunja Todo", Severity: "critical", Context: Context{DurationSeconds: 7500, PreviousSeverity: "warning"}},
			"Indisponibilité\nRétabli après 2 h 05", "Unavailability\nRecovered after 2 h 05"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.s
			s.NatureScope, s.NatureKey = "canonical", "availability"
			if s.AlertKind != "" {
				s.NatureScope, s.NatureKey, s.NatureLabel = "connector", "zabbix:connector:local", "Source text"
			}
			if got := LocalizeNotification(s); got.FR.Body != tt.fr || got.EN.Body != tt.en {
				t.Fatalf("wrong context line: %#v", got)
			}
		})
	}
}

func TestNotificationDurationsStayCompact(t *testing.T) {
	for seconds, want := range map[int64]string{30: "moins d’1 min", 180: "3 min", 3600: "1 h", 90000: "1 j 1 h", 172800: "2 j"} {
		s := Situation{Resolved: true, MaxAffected: 1, TotalTargets: 1, TargetName: "API", NatureScope: "canonical", NatureKey: "availability", Context: Context{DurationSeconds: seconds}}
		if got := RenderNotification(s, "fr").Body; got != "Indisponibilité\nRétabli après "+want {
			t.Fatalf("%d seconds rendered as %q", seconds, got)
		}
	}
}

func TestNotificationMarkersFollowTheSeverityRegister(t *testing.T) {
	for severity, want := range map[string]string{"information": "🔵", "warning": "🟡", "major": "🟠", "critical": "🔴", "future": ""} {
		if got := NotificationMarker(severity, false); got != want {
			t.Fatalf("%s marker = %q", severity, got)
		}
	}
	if NotificationMarker("critical", true) != "🟢" {
		t.Fatal("a resolution must use the recovery marker")
	}
}

func TestUnknownNotificationProblemsGetTwoLinesAndStayBounded(t *testing.T) {
	s := Situation{NatureScope: "connector", NatureLabel: "  Préfixe\n" + strings.Repeat("é", 100), TargetName: "\t API\n publique  ", AffectedTargets: 1, Severity: "future"}
	got := RenderNotification(s, "fr")
	if got.Title != "API publique" || !utf8.ValidString(got.Body) || utf8.RuneCountInString(got.Body) != 80 || !strings.HasSuffix(got.Body, "…") || strings.Contains(got.Body, "\n") {
		t.Fatalf("unreadable fallback: %#v", got)
	}
	s.NatureLabel = "\n\t"
	if got := LocalizeNotification(s); got.FR.Body != "Signal de supervision" || got.EN.Body != "Monitoring signal" {
		t.Fatalf("missing fallback problem: %#v", got)
	}
	s.TargetName = strings.Repeat("x", 120)
	if title := RenderNotification(s, "fr").Title; utf8.RuneCountInString(title) != 60 {
		t.Fatalf("resource title is not bounded: %q", title)
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
		if got.FR.Body != tt.fr || got.EN.Body != tt.en || got.FR.Title != "Host · majeur" || got.EN.Title != "Host · major" {
			t.Fatalf("wrong translation: %+v", got)
		}
		if Render(s, "fr").Title != tt.fr || s.NatureLabel != tt.label {
			t.Fatal("detail diverged or source text was overwritten")
		}
		// The exact same source text cannot establish a meaning without enrichment.
		s.AlertKind = ""
		for _, scope := range []string{"zabbix:connector:local", "webhook:custom", "storage.latency"} {
			s.NatureKey = scope
			if got := RenderNotification(s, "fr"); got.Body != oneLine(tt.label, 80) {
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
