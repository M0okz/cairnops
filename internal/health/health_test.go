package health

import (
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/alerttext"
)

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time {
	instant := now.Add(-d)
	return &instant
}

func availabilityCheck(outcome string, seen time.Duration, cadence time.Duration) Check {
	return Check{
		LastObservedAt: ago(seen), LatestOutcome: outcome, Cadence: cadence,
		Enabled: true, MeasuresAvailability: true,
	}
}

func condition(kind alerttext.Kind, seen time.Duration) Condition {
	return Condition{LastSeenAt: ago(seen), Kind: kind}
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name  string
		facts Facts
		want  State
	}{{
		name:  "sans Contrôle ni Preuve, rien ne permet de conclure",
		facts: Facts{},
		want:  Unknown,
	}, {
		name:  "un Contrôle de disponibilité récent et favorable établit Disponible",
		facts: Facts{Checks: []Check{availabilityCheck("healthy", time.Minute, time.Minute)}},
		want:  Available,
	}, {
		name: "une preuve d'indisponibilité récente l'emporte sur un Contrôle favorable",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.Unavailable, time.Minute)},
		},
		want: Unavailable,
	}, {
		name: "une indisponibilité périmée ne maintient pas l'état Indisponible",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.Unavailable, 2*time.Hour)},
		},
		want: Available,
	}, {
		name: "une preuve invalidée est écartée des preuves actives",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{{LastSeenAt: ago(time.Minute), Kind: alerttext.Unavailable, Invalidated: true}},
		},
		want: Available,
	}, {
		name: "une latence élevée établie altère le fonctionnement",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.DiskLatency, time.Minute)},
		},
		want: Degraded,
	}, {
		name:  "une sauvegarde trop ancienne altère le fonctionnement d'une tâche",
		facts: Facts{Conditions: []Condition{condition(alerttext.BackupFreshness, time.Minute)}},
		want:  Degraded,
	}, {
		name: "une indisponibilité l'emporte sur une altération simultanée",
		facts: Facts{Conditions: []Condition{
			condition(alerttext.DiskLatency, time.Minute),
			condition(alerttext.Unavailable, time.Minute),
		}},
		want: Unavailable,
	}, {
		name: "l'ordre des preuves ne change pas la conclusion",
		facts: Facts{Conditions: []Condition{
			condition(alerttext.Unavailable, time.Minute),
			condition(alerttext.DiskLatency, time.Minute),
		}},
		want: Unavailable,
	}, {
		name: "une mise à jour disponible ne dégrade pas un service qui fonctionne",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.SoftwareUpdate, time.Minute)},
		},
		want: Available,
	}, {
		name: "une mise à jour de sécurité disponible ne dégrade pas davantage",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.SoftwareSecurityUpdate, time.Minute)},
		},
		want: Available,
	}, {
		name: "une expiration de certificat encore à venir n'altère pas le fonctionnement",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.CertificateExpiry, time.Minute)},
		},
		want: Available,
	}, {
		name: "un certificat déjà invalide, lui, altère le fonctionnement",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.CertificateInvalid, time.Minute)},
		},
		want: Degraded,
	}, {
		name: "un Problème signalé de Nature inconnue ne dégrade pas",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.Kind("connecteur.local.42"), time.Minute)},
		},
		want: Available,
	}, {
		name: "une altération périmée ne maintient pas la dégradation",
		facts: Facts{
			Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
			Conditions: []Condition{condition(alerttext.DiskLatency, 2*time.Hour)},
		},
		want: Available,
	}, {
		name:  "une Observation périmée ne peut pas établir un état disponible",
		facts: Facts{Checks: []Check{availabilityCheck("healthy", 3*time.Hour, time.Minute)}},
		want:  Unknown,
	}, {
		name: "un Contrôle suspendu ne peut pas établir un état disponible",
		facts: Facts{Checks: []Check{{
			LastObservedAt: ago(time.Minute), LatestOutcome: "healthy",
			Cadence: time.Minute, Enabled: false, MeasuresAvailability: true,
		}}},
		want: Unknown,
	}, {
		name: "un Contrôle qui ne conclut pas sur la disponibilité ne l'établit pas",
		facts: Facts{Checks: []Check{{
			LastObservedAt: ago(time.Minute), LatestOutcome: "healthy",
			Cadence: time.Minute, Enabled: true, MeasuresAvailability: false,
		}}},
		want: Unknown,
	}, {
		name:  "une Observation défavorable sans preuve d'Incident laisse l'état inconnu",
		facts: Facts{Checks: []Check{availabilityCheck("unhealthy", time.Minute, time.Minute)}},
		want:  Unknown,
	}, {
		name:  "une Observation Inconnue ne conclut rien",
		facts: Facts{Checks: []Check{availabilityCheck("unknown", time.Minute, time.Minute)}},
		want:  Unknown,
	}, {
		name:  "un Contrôle lent reste frais jusqu'à trois cadences",
		facts: Facts{Checks: []Check{availabilityCheck("healthy", 50*time.Minute, 20*time.Minute)}},
		want:  Available,
	}, {
		name:  "au-delà de trois cadences, il ne prouve plus rien",
		facts: Facts{Checks: []Check{availabilityCheck("healthy", 70*time.Minute, 20*time.Minute)}},
		want:  Unknown,
	}, {
		name:  "un Contrôle très rapide bénéficie du plancher de fraîcheur",
		facts: Facts{Checks: []Check{availabilityCheck("healthy", 10*time.Minute, 20*time.Second)}},
		want:  Available,
	}, {
		name: "un second Contrôle favorable suffit quand le premier est muet",
		facts: Facts{Checks: []Check{
			availabilityCheck("healthy", 5*time.Hour, time.Minute),
			availabilityCheck("healthy", time.Minute, time.Minute),
		}},
		want: Available,
	}}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Evaluate(testCase.facts, now); got != testCase.want {
				t.Errorf("Evaluate = %q, attendu %q", got, testCase.want)
			}
		})
	}
}

// La Gravité n'établit pas un État de santé : CONTEXT.md sépare les deux, et
// « la gravité seule ne rend plus une ressource indisponible ».
func TestSeverityIsAbsentFromTheConclusion(t *testing.T) {
	// Facts ne porte aucune gravité : si un champ apparaissait, ce test
	// cesserait de compiler et la séparation serait à réexaminer.
	facts := Facts{
		Checks:     []Check{availabilityCheck("healthy", time.Minute, time.Minute)},
		Conditions: []Condition{condition(alerttext.SoftwareSecurityUpdate, time.Minute)},
	}
	if got := Evaluate(facts, now); got != Available {
		t.Errorf("une condition critique mais non altérante doit laisser Disponible, obtenu %q", got)
	}
}

// Une horloge externe légèrement en avance reste crue ; loin devant, non.
func TestFreshnessToleratesASmallClockSkewOnly(t *testing.T) {
	slightlyAhead := now.Add(30 * time.Second)
	if !Fresh(&slightlyAhead, now, time.Minute) {
		t.Error("30 secondes d'avance doivent rester acceptées")
	}
	farAhead := now.Add(10 * time.Minute)
	if Fresh(&farAhead, now, time.Minute) {
		t.Error("10 minutes d'avance ne doivent pas être crues")
	}
	if Fresh(nil, now, time.Minute) {
		t.Error("un instant absent n'est jamais frais")
	}
	var zero time.Time
	if Fresh(&zero, now, time.Minute) {
		t.Error("un instant nul n'est jamais frais")
	}
}

func TestOverall(t *testing.T) {
	cases := []struct {
		name   string
		states []State
		want   State
	}{
		{"sans Ressource active, aucune conclusion", nil, ""},
		{"toutes Disponibles donne Tout est opérationnel", []State{Available, Available}, Available},
		{"une Indisponible donne Incident en cours", []State{Available, Unknown, Unavailable}, Unavailable},
		{"une dégradée sans indisponibilité donne Services dégradés", []State{Available, Degraded, Unknown}, Degraded},
		{"une sans état connu donne Supervision incomplète", []State{Available, Unknown}, Unknown},
		{"l'indisponibilité prime sur la dégradation", []State{Degraded, Unavailable}, Unavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Overall(testCase.states); got != testCase.want {
				t.Errorf("Overall = %q, attendu %q", got, testCase.want)
			}
		})
	}
}

func TestStateValidityAndRank(t *testing.T) {
	for _, state := range States {
		if !state.Valid() {
			t.Errorf("%q devrait être valide", state)
		}
		if state.Rank() == 0 {
			t.Errorf("%q devrait porter un rang", state)
		}
	}
	if State("dégradée").Valid() {
		t.Error("un État inventé ne doit pas être valide")
	}
	if State("").Rank() != 0 {
		t.Error("un État vide ne porte pas de rang")
	}
	if Unavailable.Rank() <= Degraded.Rank() || Degraded.Rank() <= Unknown.Rank() ||
		Unknown.Rank() <= Available.Rank() {
		t.Error("l'ordre des rangs doit suivre celui de l'État global")
	}
	// « Sous maintenance » n'est pas une conclusion sur les preuves : le client
	// la compose, le serveur ne la prononce jamais.
	if State("maintenance").Valid() {
		t.Error("« maintenance » ne doit pas être un État de santé du serveur")
	}
}
