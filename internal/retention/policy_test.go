package retention

import (
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/incidents"
)

func TestDefaultPolicyIsValid(t *testing.T) {
	if err := DefaultPolicy().Validate(); err != nil {
		t.Fatal(err)
	}
}

// Les minimums suivent ce que CairnOps relit : s'ils passaient sous ces
// fenêtres, la purge effacerait ce qu'un autre composant attend encore.
func TestMinimumsCoverWhatCairnOpsStillReads(t *testing.T) {
	if MinimumObservations < domain.LatencyProfileWindow+domain.LatencyEvaluationWindow {
		t.Fatalf("observation retention %s is shorter than the latency profile window", MinimumObservations)
	}
	if MinimumDeliveries <= incidents.ResumptionWindow {
		t.Fatalf("delivery retention %s does not outlast the resumption window %s", MinimumDeliveries, incidents.ResumptionWindow)
	}
	// Le mois le plus court ne doit pas laisser la fenêtre de 30 jours sans agrégat.
	if MinimumObservationHourMonths*28*day < 30*day+time.Hour {
		t.Fatalf("observation hour retention of %d months does not cover the 30 days window", MinimumObservationHourMonths)
	}
}
