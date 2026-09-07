package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/M0okz/cairnops/internal/incidents"
)

func TestMattermostResolutionMessageKeepsOperationalContext(t *testing.T) {
	t.Parallel()
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewMattermostClient(server.Client())
	if err := client.Send(context.Background(), server.URL, Message{
		EventKind: "resolved", IncidentID: "incident-1", TargetName: "Nextcloud",
		NatureLabel: "Réponse HTTP invalide", Severity: incidents.SeverityCritical,
		MaxAffected: 1,
		PublicURL:   "https://cairnops.example.test",
	}); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload)
	for _, expected := range []string{"Résolu", "Nextcloud", "Réponse HTTP invalide", "https://cairnops.example.test/incidents?incident=incident-1"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("Mattermost payload does not contain %q: %s", expected, encoded)
		}
	}
}

func TestMattermostMultiTargetIncidentKeepsItsImpactSummary(t *testing.T) {
	t.Parallel()
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewMattermostClient(server.Client())
	if err := client.Send(context.Background(), server.URL, Message{
		EventKind: "resolved", IncidentID: "incident-1",
		TargetName: "7 Cibles affectées", NatureLabel: "Latence disque élevée",
		Severity: incidents.SeverityCritical, MaxAffected: 7,
		PublicURL: "https://cairnops.example.test",
	}); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload)
	for _, expected := range []string{"Résolu", "Jusqu’à 7 Cibles", "https://cairnops.example.test/incidents?incident=incident-1"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("Mattermost incident payload does not contain %q: %s", expected, encoded)
		}
	}
}

func TestMattermostUsesTheCompactNotificationTemplate(t *testing.T) {
	var payload struct {
		Attachments []struct{ Title, Text string } `json:"attachments"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := NewMattermostClient(server.Client()).Send(context.Background(), server.URL, Message{
		EventKind: "firing", IncidentID: "incident-load", TargetName: "VictoriaLogs",
		AlertKind: alerttext.SystemLoad, NatureKey: "zabbix:connector:load", NatureScope: "connector",
		NatureLabel: "Linux: Load average is too high (per CPU load over 1.5 for 5m)",
		Severity:    incidents.SeverityMajor, AffectedTargets: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if len(payload.Attachments) != 1 || payload.Attachments[0].Title != "⚠️ Charge système moyenne élevée" || payload.Attachments[0].Text != "VictoriaLogs · majeur" {
		t.Fatalf("Mattermost diverged from the shared template: %+v", payload)
	}
}
