package notifications

import (
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/M0okz/cairnops/internal/synthesis"
)

// Les entrées livrées avant le retrait des Intégrations gardent leur nom dans
// le contexte enregistré ; la notification ne doit plus l'afficher.
func TestStoredContextNoLongerNamesTheIntegration(t *testing.T) {
	stored := decodeContext([]byte(`{"sources":["Zabbix"],"fact":{"kind":"disk.space.low","resource":"/var"}}`))
	got := synthesis.RenderNotification(synthesis.Situation{
		AlertKind: alerttext.DiskSpace, NatureScope: "connector", NatureKey: "zabbix:connector:disk",
		TargetName: "trust-cairnops-01", AffectedTargets: 1, Severity: "major", Context: stored,
	}, "fr")
	if got.Body != "Espace disque insuffisant\n/var" || strings.Contains(got.Body, "Zabbix") {
		t.Fatalf("stored integration name is still rendered: %#v", got)
	}
}
