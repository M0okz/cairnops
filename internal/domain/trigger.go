package domain

import "fmt"

const (
	MinimumThreshold = 1
	MaximumThreshold = 10

	DefaultFailureThreshold  = 3
	DefaultRecoveryThreshold = 2
)

// TriggerPolicy décide quand une suite d'Observations devient assez certaine
// pour signaler une dégradation ou un rétablissement d'une Source de signal.
type TriggerPolicy struct {
	FailureThreshold  int
	RecoveryThreshold int
}

func (policy TriggerPolicy) Validate() error {
	if policy.FailureThreshold < MinimumThreshold || policy.FailureThreshold > MaximumThreshold {
		return fmt.Errorf("failure threshold must be between %d and %d", MinimumThreshold, MaximumThreshold)
	}
	if policy.RecoveryThreshold < MinimumThreshold || policy.RecoveryThreshold > MaximumThreshold {
		return fmt.Errorf("recovery threshold must be between %d and %d", MinimumThreshold, MaximumThreshold)
	}
	return nil
}

// TriggerStreaks compte les Observations consécutives d'une même conclusion.
type TriggerStreaks struct {
	Unhealthy int
	Healthy   int
}

// TriggerDecision est le résultat d'une Observation confrontée à la Politique
// de déclenchement : les compteurs à persister et, le cas échéant, le passage
// en dégradation ou le rétablissement confirmé.
type TriggerDecision struct {
	Streaks   TriggerStreaks
	Triggered bool
	Recovered bool
}

// Evaluate confronte une Observation aux Observations qui la précèdent.
//
// Une Observation Inconnue ne conclut rien : elle ne rapproche ni d'une
// dégradation ni d'un rétablissement et laisse les compteurs intacts, car
// l'absence de preuve ne constitue jamais un rétablissement.
func (policy TriggerPolicy) Evaluate(streaks TriggerStreaks, outcome Outcome) TriggerDecision {
	switch outcome {
	case OutcomeUnhealthy:
		streaks = TriggerStreaks{Unhealthy: min(streaks.Unhealthy+1, policy.FailureThreshold)}
		return TriggerDecision{Streaks: streaks, Triggered: streaks.Unhealthy >= policy.FailureThreshold}
	case OutcomeHealthy:
		streaks = TriggerStreaks{Healthy: min(streaks.Healthy+1, policy.RecoveryThreshold)}
		return TriggerDecision{Streaks: streaks, Recovered: streaks.Healthy >= policy.RecoveryThreshold}
	default:
		return TriggerDecision{Streaks: streaks}
	}
}
