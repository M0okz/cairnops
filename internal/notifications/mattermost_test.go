package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		PublicURL: "https://cairnops.example.test",
	}); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload)
	for _, expected := range []string{"RÉSOLU", "Nextcloud", "Réponse HTTP invalide", "même canal", "https://cairnops.example.test/#incidents"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("Mattermost payload does not contain %q: %s", expected, encoded)
		}
	}
}
