package push

import (
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/M0okz/cairnops/internal/synthesis"
)

func TestCompleteNotificationsUseTheApprovedThreeLineTemplate(t *testing.T) {
	for _, tt := range []struct {
		name     string
		delivery Delivery
		want     Presentation
	}{
		{"approved storage notification", Delivery{NatureKey: "storage.latency", NatureScope: "canonical", NatureLabel: "Latence de stockage élevée", TargetName: "dmz-docker-01", Severity: "warning",
			Context: synthesis.Context{Fact: &alerttext.Fact{Kind: alerttext.DiskLatency, Resource: "sda"}}},
			Presentation{"🟡 dmz-docker-01 · avertissement", "Latence disque élevée\nsda"}},
		{"reported load notification", Delivery{AlertKind: alerttext.SystemLoad, NatureKey: "zabbix:connector:load", NatureScope: "connector", NatureLabel: "Linux: Load average is too high (per CPU load over 1.5 for 5m)", TargetName: "VictoriaLogs", Severity: "major"},
			Presentation{"🟠 VictoriaLogs · majeur", "Charge système moyenne élevée"}},
		{"unrecognized source wording", Delivery{NatureKey: "zabbix:connector:custom", NatureScope: "connector", NatureLabel: "Linux: Load average is too high (per CPU load over 1.5 for 5m)", TargetName: "VictoriaLogs", Severity: "major", Context: synthesis.Context{}},
			Presentation{"🟠 VictoriaLogs · majeur", "Linux: Load average is too high (per CPU load over 1.5 for 5m)"}},
		{"resolution", Delivery{EventKind: "resolved", NatureKey: "availability", NatureScope: "canonical", TargetName: "Vikunja Todo", Severity: "critical", Context: synthesis.Context{DurationSeconds: 180}},
			Presentation{"🟢 Vikunja Todo · résolu", "Indisponibilité\nRétabli après 3 min"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			delivery := tt.delivery
			delivery.Locale, delivery.NotificationContent = "fr", "complete"
			if delivery.EventKind == "" {
				delivery.EventKind = "firing"
			}
			delivery.AffectedTargets, delivery.ImpactCount, delivery.MaxAffected = 1, 1, 1
			if got := messageFor(delivery, "https://cairnops.example.test").Presentation; got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNotificationTemplatesRespectDevicePrivacy(t *testing.T) {
	for _, locale := range []string{"fr", "en"} {
		for _, mode := range []string{"discreet", "masked"} {
			for _, event := range []string{"firing", "incident_update", "resolved"} {
				delivery := Delivery{Locale: locale, NotificationContent: mode, EventKind: event,
					NatureScope: "canonical", NatureKey: "storage.latency", TargetName: "Secret database",
					NatureLabel: "Internal condition", Severity: "critical", AffectedTargets: 1,
					Context: synthesis.Context{TargetNames: []string{"Secret database"}}}
				got := messageFor(delivery, "https://cairnops.example.test").Presentation
				if got.Title != "CairnOps" || got.Body == "" || strings.Contains(got.Body, "Secret") || strings.Contains(got.Body, "Internal") || strings.Contains(got.Body, "storage") || strings.Contains(got.Body, "stockage") || strings.Contains(got.Body, "criti") || strings.Contains(got.Body, "🔴") {
					t.Fatalf("privacy mode %s/%s/%s leaked operational details: %#v", locale, mode, event, got)
				}
			}
		}
	}
}
