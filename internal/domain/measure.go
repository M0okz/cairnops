package domain

// Window est une fenêtre de mesure. Les listes s'en tiennent à 24 heures ; le
// détail d'une Cible ouvre en plus 7 et 30 jours.
type Window string

const (
	WindowDay   Window = "24h"
	WindowWeek  Window = "7d"
	WindowMonth Window = "30d"
)

// MeasureWindows énumère les fenêtres dans l'ordre où le détail les présente.
var MeasureWindows = []Window{WindowDay, WindowWeek, WindowMonth}

func (window Window) Valid() bool {
	switch window {
	case WindowDay, WindowWeek, WindowMonth:
		return true
	default:
		return false
	}
}

// Hours donne la profondeur de la fenêtre en heures entamées.
func (window Window) Hours() int {
	switch window {
	case WindowWeek:
		return 7 * 24
	case WindowMonth:
		return 30 * 24
	default:
		return 24
	}
}

// Counters agrège les Observations d'une ou plusieurs Sources sur une période.
// Expected compte celles que la cadence des Sources laissait attendre.
type Counters struct {
	Healthy                int
	Unhealthy              int
	Unknown                int
	Expected               int
	LatencySumMilliseconds int64
	LatencyCount           int
	LatencyMaximum         int
}

// Add additionne deux périodes ou deux Sources. La somme des Sources d'une
// Cible pondère chacune par sa cadence réelle, ce qui est l'intention.
func (counters Counters) Add(other Counters) Counters {
	return Counters{
		Healthy:                counters.Healthy + other.Healthy,
		Unhealthy:              counters.Unhealthy + other.Unhealthy,
		Unknown:                counters.Unknown + other.Unknown,
		Expected:               counters.Expected + other.Expected,
		LatencySumMilliseconds: counters.LatencySumMilliseconds + other.LatencySumMilliseconds,
		LatencyCount:           counters.LatencyCount + other.LatencyCount,
		LatencyMaximum:         max(counters.LatencyMaximum, other.LatencyMaximum),
	}
}

// Conclusive compte les Observations qui ont conclu quelque chose. Une
// Observation Inconnue n'en fait jamais partie.
func (counters Counters) Conclusive() int {
	return counters.Healthy + counters.Unhealthy
}

// Measure est ce qu'une fenêtre permet d'affirmer. Une mesure absente vaut
// mieux qu'une mesure inventée : chaque ratio reste nul tant que rien ne
// l'établit.
type Measure struct {
	Window                     Window   `json:"window"`
	Availability               *float64 `json:"availability"`
	Coverage                   *float64 `json:"coverage"`
	AverageLatencyMilliseconds *int     `json:"average_latency_milliseconds"`
	MaximumLatencyMilliseconds *int     `json:"maximum_latency_milliseconds"`
	ConclusiveObservations     int      `json:"conclusive_observations"`
	UnknownObservations        int      `json:"unknown_observations"`
	ExpectedObservations       int      `json:"expected_observations"`
}

// Measure dérive les trois mesures de la fenêtre.
//
// La Disponibilité rapporte les Observations saines aux seules Observations
// concluantes. La Couverture rapporte les Observations concluantes à celles
// qui étaient attendues, et se plafonne à 1 : une Source légèrement en avance
// sur sa cadence ne prouve pas mieux que la totalité.
func (counters Counters) Measure(window Window) Measure {
	measure := Measure{
		Window:                 window,
		ConclusiveObservations: counters.Conclusive(),
		UnknownObservations:    counters.Unknown,
		ExpectedObservations:   counters.Expected,
	}
	if conclusive := counters.Conclusive(); conclusive > 0 {
		availability := float64(counters.Healthy) / float64(conclusive)
		measure.Availability = &availability
	}
	if counters.Expected > 0 {
		coverage := min(1, float64(counters.Conclusive())/float64(counters.Expected))
		measure.Coverage = &coverage
	}
	if counters.LatencyCount > 0 {
		average := int((counters.LatencySumMilliseconds + int64(counters.LatencyCount)/2) / int64(counters.LatencyCount))
		maximum := counters.LatencyMaximum
		measure.AverageLatencyMilliseconds = &average
		measure.MaximumLatencyMilliseconds = &maximum
	}
	return measure
}
