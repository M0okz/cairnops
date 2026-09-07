// Package synthesis rend une même situation factuelle sur tous les Canaux.
// Il ne décide ni du regroupement, ni de la livraison, ni d'une cause.
package synthesis

import (
	"fmt"
	"strings"

	"github.com/M0okz/cairnops/internal/alerttext"
)

// NatureLabel est le catalogue fermé des conclusions indépendantes d'un
// produit. Un adapter ne choisit une clé que si ses données établissent ce sens.
// Une clé inconnue reste locale au Connecteur.
func NatureLabel(key, locale string) (string, bool) {
	kind, ok := map[string]alerttext.Kind{
		"availability":     alerttext.Unavailable,
		"storage.latency":  alerttext.DiskLatency,
		"storage.capacity": alerttext.DiskSpace,
		"backup.failure":   alerttext.BackupFailure,
		"backup.freshness": alerttext.BackupFreshness,
		"tls.expiry":       alerttext.CertificateExpiry,
	}[key]
	if !ok {
		return "", false
	}
	return alerttext.Title(kind, locale)
}

// Situation contient exclusivement les faits établis par le cycle d'Incident.
// Les compteurs portent sur les Cibles distinctes, jamais sur les Preuves.
// Une seule Preuve suffit ; aucun quorum ni enrichissement n'est requis.
type Situation struct {
	AlertKind       alerttext.Kind
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

// Presentation describes a verified meaning independently of the incident's
// lifecycle. Empty titles instruct clients to keep the original source label.
func Presentation(s Situation) alerttext.Localized {
	return alerttext.Localized{FR: alerttext.Text{Title: presentationTitle(s, "fr")}, EN: alerttext.Text{Title: presentationTitle(s, "en")}}
}

func presentationTitle(s Situation, locale string) string {
	if s.NatureScope == "canonical" {
		if title, ok := NatureLabel(s.NatureKey, locale); ok {
			return title
		}
	}
	title, _ := alerttext.Title(s.AlertKind, locale)
	return title
}

func Localize(s Situation) Localized {
	return Localized{FR: Render(s, "fr"), EN: Render(s, "en")}
}

func Render(s Situation, locale string) Text {
	return render(s, locale, false)
}

func render(s Situation, locale string, notification bool) Text {
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
	if notification && !known {
		title = notificationSourceTitle(s, locale)
	}
	if translated := presentationTitle(s, locale); translated != "" {
		title = translated
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
		if notification {
			switch s.Severity {
			case "major":
				severity = "majeur"
				if english {
					severity = "major"
				}
			case "critical":
				severity = "critique"
				if english {
					severity = "critical"
				}
			}
		}
		body = fmt.Sprintf("%s · %s", body, severity)
	}
	if notification {
		title = oneLine(title, 80)
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
