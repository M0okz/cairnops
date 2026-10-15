package retention

import (
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

const day = 24 * time.Hour

// Policy fixe ce que l'instance conserve. Les durées de l'ADR 0008 restent
// réglables par l'Administrateur de l'instance, sans descendre sous ce que
// CairnOps lit lui-même.
type Policy struct {
	// Observations est l'âge au-delà duquel une Observation brute consolidée
	// s'efface.
	Observations time.Duration
	// ObservationHourMonths est l'âge, en mois calendaires, au-delà duquel un
	// agrégat horaire s'efface.
	ObservationHourMonths int
	// Events est la fenêtre de reprise incrémentale du temps réel. Un client
	// absent plus longtemps recharge une projection complète (ADR 0022).
	Events time.Duration
	// Deliveries est l'âge au-delà duquel les livraisons closes d'un Incident
	// résolu s'effacent.
	Deliveries time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		Observations:          30 * day,
		ObservationHourMonths: 13,
		Events:                24 * time.Hour,
		Deliveries:            30 * day,
	}
}

const (
	// MinimumObservations couvre la fenêtre du Profil de latence, qui
	// s'apprend sur les Observations brutes.
	MinimumObservations = domain.LatencyProfileWindow + domain.LatencyEvaluationWindow
	// MinimumObservationHourMonths couvre la plus longue fenêtre de mesure,
	// 30 jours, quel que soit le mois.
	MinimumObservationHourMonths = 2
	// MinimumEvents laisse le temps d'une coupure réseau ordinaire.
	MinimumEvents = time.Hour
	// MinimumDeliveries reste au-delà de la fenêtre de Reprise d'un Incident
	// résolu (ADR 0054), pendant laquelle ses livraisons gouvernent encore les
	// notifications.
	MinimumDeliveries = day
)

func (policy Policy) Validate() error {
	if policy.Observations < MinimumObservations {
		return fmt.Errorf("observation retention must be at least %d days", int((MinimumObservations+day-1)/day))
	}
	if policy.ObservationHourMonths < MinimumObservationHourMonths {
		return fmt.Errorf("observation hour retention must be at least %d months", MinimumObservationHourMonths)
	}
	if policy.Events < MinimumEvents {
		return fmt.Errorf("event retention must be at least %d hour", int(MinimumEvents/time.Hour))
	}
	if policy.Deliveries < MinimumDeliveries {
		return fmt.Errorf("delivery retention must be at least %d day", int(MinimumDeliveries/day))
	}
	return nil
}
