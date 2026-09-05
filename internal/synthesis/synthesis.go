// Package synthesis rend une même situation factuelle sur tous les Canaux.
// Il ne décide ni du regroupement, ni de la livraison, ni d'une cause.
package synthesis

import (
	"fmt"
	"strings"
)

// NatureLabel est le catalogue fermé des conclusions indépendantes d'un
// produit. Un adapter ne choisit une clé que si ses données établissent ce sens.
// Une clé inconnue reste locale au Connecteur.
func NatureLabel(key, locale string) (string, bool) {
	labels, ok := map[string][2]string{
		"availability":     {"Indisponibilité", "Unavailability"},
		"storage.latency":  {"Latence de stockage élevée", "High storage latency"},
		"storage.capacity": {"Espace de stockage insuffisant", "Low storage space"},
		"backup.failure":   {"Échec de sauvegarde", "Backup failure"},
		"backup.freshness": {"Sauvegarde trop ancienne", "Outdated backup"},
		"tls.expiry":       {"Expiration de certificat proche", "Certificate nearing expiry"},
	}[key]
	if locale == "en" {
		return labels[1], ok
	}
	return labels[0], ok
}

// Situation contient exclusivement les faits établis par le cycle d'Incident.
// Les compteurs portent sur les Cibles distinctes, jamais sur les Preuves.
// Une seule Preuve suffit ; aucun quorum ni enrichissement n'est requis.
type Situation struct {
	NatureKey       string
	NatureScope     string
	NatureLabel     string
	Severity        string
	TargetName      string
	AffectedTargets int
	MaxAffected     int
	TotalTargets    int // Cibles distinctes sur tout le cycle, pas seulement au pic.
	Resolved        bool
}

type Text struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Localized struct {
	FR Text `json:"fr"`
	EN Text `json:"en"`
}

func Localize(s Situation) Localized {
	return Localized{FR: Render(s, "fr"), EN: Render(s, "en")}
}

func Render(s Situation, locale string) Text {
	english := locale == "en"
	title, known := NatureLabel(s.NatureKey, locale)
	known = known && s.NatureScope == "canonical"
	if !known {
		// Le libellé source est attribué comme signalement, sans devenir une
		// conclusion canonique ni une identité de regroupement.
		label := oneLine(s.NatureLabel, 150)
		if label == "" {
			title = "Signal de supervision"
			if english {
				title = "Monitoring signal"
			}
		} else {
			title = fmt.Sprintf("Signalement : %s", label)
			if english {
				title = fmt.Sprintf("Reported: %s", label)
			}
		}
	}
	count := s.AffectedTargets
	if s.Resolved {
		count = s.MaxAffected
	}
	body := "Aucune Cible encore affectée · rétablissement en cours de confirmation"
	if english {
		body = "No targets still affected · recovery awaiting confirmation"
	}
	if count == 1 {
		body = "1 Cible concernée"
		if english {
			body = "1 affected target"
		}
		if name := oneLine(s.TargetName, 120); name != "" && (!s.Resolved || s.TotalTargets <= 1) {
			body = name
		}
	} else if count > 1 {
		body = fmt.Sprintf("%d Cibles concernées", count)
		if english {
			body = fmt.Sprintf("%d affected targets", count)
		}
	}
	if s.Resolved {
		if english {
			title = fmt.Sprintf("Resolved · %s", title)
		} else {
			title = fmt.Sprintf("Résolu · %s", title)
		}
		if count == 1 && s.TotalTargets > 1 {
			body = "Jusqu’à 1 Cible concernée à la fois"
			if english {
				body = "Up to 1 affected target at a time"
			}
		} else if count > 1 {
			body = fmt.Sprintf("Jusqu’à %d Cibles concernées", count)
			if english {
				body = fmt.Sprintf("Up to %d affected targets", count)
			}
		} else if count == 0 {
			body = "Rétablissement confirmé"
			if english {
				body = "Recovery confirmed"
			}
		}
	} else if severity, ok := severityLabel(s.Severity, english); ok {
		body = fmt.Sprintf("%s · %s", body, severity)
	}
	return Text{Title: title, Body: body}
}

func severityLabel(severity string, english bool) (string, bool) {
	labels, ok := map[string][2]string{
		"information": {"information", "information"},
		"warning":     {"avertissement", "warning"},
		"major":       {"gravité majeure", "major severity"},
		"critical":    {"gravité critique", "critical severity"},
	}[severity]
	if english {
		return labels[1], ok
	}
	return labels[0], ok
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return value
}
