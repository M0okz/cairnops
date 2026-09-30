// Package health conclut sur le fonctionnement d'une Ressource.
//
// Cette conclusion appartient au serveur. CONTEXT.md le dit de l'État partagé —
// « État de référence maintenu par le serveur et projeté vers les interfaces
// Web, iOS et Android ; une copie locale ne constitue pas l'état de
// référence » — et l'ADR 0002 en fait la source de vérité. Trois clients qui
// réimplémentent la même arithmétique de fraîcheur divergent tôt ou tard ; un
// seul endroit décide, les autres affichent.
//
// Le paquet ne lit ni base ni réseau : il conclut à partir de faits qu'on lui
// remet. C'est ce qui rend chaque règle de CONTEXT.md testable isolément.
package health

import (
	"time"

	"github.com/M0okz/cairnops/internal/alerttext"
)

// State est l'État de santé d'une Ressource, dans le vocabulaire de CONTEXT.md.
type State string

const (
	// Unavailable — « une preuve valide et récente établit une indisponibilité
	// active, indépendamment de la gravité attribuée au problème ».
	Unavailable State = "unavailable"
	// Degraded — « altération du fonctionnement établie par des observations
	// pertinentes, dont la description précise permet de comprendre le
	// problème ». Une gravité ou une mise à jour disponible n'y suffisent pas.
	Degraded State = "degraded"
	// Available — « un Contrôle de disponibilité actif apporte un résultat
	// récent favorable, sans preuve active récente d'indisponibilité ». Des
	// problèmes portant sur d'autres aspects peuvent coexister.
	Available State = "available"
	// Unknown — « aucune preuve valide n'est assez récente pour conclure ».
	Unknown State = "unknown"
)

// States énumère les États de santé dans l'ordre où l'État global les
// considère : une seule Ressource Indisponible suffit à un Incident en cours.
//
// « Sous maintenance » n'en fait volontairement pas partie. Une Fenêtre de
// maintenance neutralise l'impact opérationnel sans altérer les preuves, et
// CONTEXT.md exige qu'un Incident encore actif à son terme « redevienne
// immédiatement visible ». Un serveur qui écraserait la conclusion sous
// « maintenance » priverait le client de quoi l'afficher avant son prochain
// sondage. Le serveur conclut donc sur les preuves ; le client compose la
// maintenance à partir des fenêtres qu'il détient, dont il connaît les dates.
var States = []State{Unavailable, Degraded, Unknown, Available}

func (state State) Valid() bool {
	for _, candidate := range States {
		if state == candidate {
			return true
		}
	}
	return false
}

// Rank classe les États du plus préoccupant au moins préoccupant. Il sert à
// l'État global ; il ne qualifie ni une Gravité ni une priorité d'action.
func (state State) Rank() int {
	for index, candidate := range States {
		if state == candidate {
			return len(States) - index
		}
	}
	return 0
}

const (
	// defaultCadence borne la fraîcheur d'une Preuve, qui ne déclare pas de
	// cadence : un Contrôle externe silencieux depuis un quart d'heure ne
	// prouve plus rien de l'instant présent.
	defaultCadence = 5 * time.Minute
	// minimumFreshness plancher la fenêtre de fraîcheur, pour qu'un Contrôle
	// très rapide ne devienne pas périmé au moindre retard du worker.
	minimumFreshness = 15 * time.Minute
	// clockSkew tolère l'horloge d'un système externe légèrement en avance.
	// Au-delà, l'horodatage n'est pas cru plutôt qu'extrapolé.
	clockSkew = time.Minute
)

// Fresh dit si un instant est assez récent pour soutenir une conclusion, pour
// une cadence attendue donnée. Une cadence nulle ou négative retombe sur la
// cadence par défaut.
//
// C'est la seule définition de la fraîcheur du produit. Elle vivait auparavant
// dans le navigateur, ce qui obligeait chaque client à la recopier exactement.
func Fresh(instant *time.Time, now time.Time, cadence time.Duration) bool {
	if instant == nil || instant.IsZero() {
		return false
	}
	if cadence <= 0 {
		cadence = defaultCadence
	}
	window := 3 * cadence
	if window < minimumFreshness {
		window = minimumFreshness
	}
	age := now.Sub(*instant)
	return age >= -clockSkew && age <= window
}

// Check est un Contrôle rattaché à la Ressource, quelle que soit son origine.
type Check struct {
	// LastObservedAt est l'instant de sa dernière Observation.
	LastObservedAt *time.Time
	// LatestOutcome est la conclusion de cette Observation.
	LatestOutcome string
	// Cadence est l'intervalle attendu entre deux Observations.
	Cadence time.Duration
	// Enabled distingue un Contrôle actif d'un Contrôle suspendu. Une
	// Suspension retire le Contrôle de la supervision : il ne peut donc plus
	// établir un état disponible.
	Enabled bool
	// MeasuresAvailability distingue les Contrôles qui concluent sur la
	// disponibilité de ceux qui observent un autre aspect. Seuls les premiers
	// peuvent établir qu'une Ressource est Disponible.
	MeasuresAvailability bool
}

// Condition est une Preuve d'Incident active rattachée à la Ressource.
type Condition struct {
	// LastSeenAt est le dernier instant où le Contrôle a confirmé la condition.
	LastSeenAt *time.Time
	// Kind est la Nature reconnue, vide pour une Nature locale de Connecteur.
	Kind alerttext.Kind
	// Invalidated marque une Preuve écartée par décision motivée.
	Invalidated bool
}

// Facts réunit tout ce qui permet de conclure sur une Ressource.
type Facts struct {
	TargetID string
	Checks   []Check
	// Conditions ne contient que les Preuves actives des Atteintes actives.
	Conditions []Condition
}

// Evaluate conclut sur une Ressource à partir de ses seules preuves. L'ordre
// des questions est celui de CONTEXT.md : une indisponibilité récente l'emporte
// sur tout le reste, une altération établie vient ensuite, et « Tout est
// opérationnel » n'est affirmé qu'avec une preuve récente et favorable.
//
// Une Fenêtre de maintenance n'entre pas dans cette conclusion : elle
// neutralise l'impact, elle ne change pas ce que les preuves établissent.
//
// Une absence d'Observation ne conclut jamais à un rétablissement : faute de
// preuve fraîche, l'état reste inconnu.
func Evaluate(facts Facts, now time.Time) State {
	impaired := false
	for _, condition := range facts.Conditions {
		if condition.Invalidated || !Fresh(condition.LastSeenAt, now, defaultCadence) {
			continue
		}
		if condition.Kind == alerttext.Unavailable {
			return Unavailable
		}
		if condition.Kind.ImpairsFunctioning() {
			impaired = true
		}
	}
	if impaired {
		return Degraded
	}

	for _, check := range facts.Checks {
		if !check.Enabled || !check.MeasuresAvailability {
			continue
		}
		if check.LatestOutcome == "healthy" && Fresh(check.LastObservedAt, now, check.Cadence) {
			return Available
		}
	}
	return Unknown
}

// Overall synthétise l'État global de l'Espace opérationnel. Sans Ressource
// active, aucune conclusion n'est possible : la Supervision n'est pas
// configurée.
func Overall(states []State) State {
	if len(states) == 0 {
		return ""
	}
	worst := Available
	for _, state := range states {
		if state.Rank() > worst.Rank() {
			worst = state
		}
	}
	return worst
}
