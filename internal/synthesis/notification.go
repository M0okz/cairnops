package synthesis

import (
	"fmt"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/alerttext"
)

// Context rassemble les faits complémentaires d'une notification. Ils sont
// figés au moment de la livraison, pour que la boîte intégrée relise ce que
// l'appareil a reçu. Un champ absent reste absent : rien n'est déduit.
type Context struct {
	// Fact n'est présent que si toutes les Preuves retenues portent le même.
	Fact *alerttext.Fact `json:"fact,omitempty"`
	// TargetNames liste les premières Ressources concernées, dans leur ordre
	// d'arrivée dans l'Incident.
	TargetNames []string `json:"target_names,omitempty"`
	// PreviousSeverity n'est renseignée que pour une hausse de Gravité notifiée.
	PreviousSeverity string `json:"previous_severity,omitempty"`
	// DurationSeconds mesure l'Incident résolu, de son ouverture à sa Résolution.
	DurationSeconds int64 `json:"duration_seconds,omitempty"`
}

// RenderNotification compose le gabarit compact partagé par le Push, la boîte
// intégrée et Mattermost, sur trois lignes :
//
//	Ressource · gravité
//	problème (jusqu'à deux lignes lorsqu'il est long)
//	contexte : hausse, durée, Ressources et détail
//
// Le titre reste court pour ne jamais tronquer le problème. Les compteurs et
// la Résolution suivent les mêmes faits que Render.
func RenderNotification(s Situation, locale string) Text {
	english := locale == "en"
	count := s.AffectedTargets
	if s.Resolved {
		count = max(s.MaxAffected, s.TotalTargets)
	}
	pending := !s.Resolved && count == 0

	var title string
	switch {
	case pending:
		title = pick(english, "Rétablissement à confirmer", "Recovery awaiting confirmation")
	case s.Resolved && count == 0:
		title = pick(english, "Résolu", "Resolved")
	case s.Resolved:
		title = notificationSubject(s, count, english) + " · " + pick(english, "résolu", "resolved")
	default:
		title = notificationSubject(s, count, english)
		if severity, ok := shortSeverity(s.Severity, english); ok {
			title += " · " + severity
		}
	}

	var details []string
	if pending {
		details = append(details, pick(english, "Aucune Ressource encore affectée", "No resources still affected"))
	}
	if !s.Resolved && s.Extended {
		details = append(details, pick(english, "Propagation étendue", "Extended propagation"))
	}
	if previous, ok := shortSeverity(s.Context.PreviousSeverity, english); ok && !s.Resolved &&
		severityRank(s.Context.PreviousSeverity) < severityRank(s.Severity) {
		details = append(details, pick(english, "Auparavant : ", "Previously: ")+previous)
	}
	if s.Resolved && s.Context.DurationSeconds > 0 {
		details = append(details, pick(english, "Rétabli après ", "Recovered after ")+
			formatDuration(time.Duration(s.Context.DurationSeconds)*time.Second, english))
	}
	if count > 1 {
		if names := targetNames(s.Context.TargetNames, count); names != "" {
			details = append(details, names)
		}
	}
	if detail := factDetail(s.Context.Fact, locale); detail != "" {
		details = append(details, detail)
	}

	body := notificationProblem(s, locale)
	if len(details) > 0 {
		body += "\n" + strings.Join(details, " · ")
	}
	return Text{Title: oneLine(title, 80), Body: body}
}

func LocalizeNotification(s Situation) Localized {
	return Localized{FR: RenderNotification(s, "fr"), EN: RenderNotification(s, "en")}
}

// NotificationMarker donne la pastille de Gravité placée en tête des titres
// Push et Mattermost. La couleur double toujours un libellé écrit.
func NotificationMarker(severity string, resolved bool) string {
	if resolved {
		return "🟢"
	}
	return map[string]string{
		"information": "🔵",
		"warning":     "🟡",
		"major":       "🟠",
		"critical":    "🔴",
	}[severity]
}

// Unknown/custom conditions retain their source wording. Recognition belongs
// to the connector adapter and is carried separately in Situation.AlertKind.
func notificationProblem(s Situation, locale string) string {
	if title := presentationTitle(s, locale); title != "" {
		return title
	}
	if s.NatureScope == "canonical" {
		if title, ok := NatureLabel(s.NatureKey, locale); ok {
			return title
		}
	}
	if label := oneLine(s.NatureLabel, 80); label != "" {
		return label
	}
	return pick(locale == "en", "Signal de supervision", "Monitoring signal")
}

func notificationSubject(s Situation, count int, english bool) string {
	if count == 1 {
		if name := oneLine(s.TargetName, 60); name != "" && (!s.Resolved || s.TotalTargets <= 1) {
			return name
		}
		return pick(english, "1 Ressource", "1 resource")
	}
	return fmt.Sprintf(pick(english, "%d Ressources", "%d resources"), count)
}

func shortSeverity(severity string, english bool) (string, bool) {
	labels, ok := map[string][2]string{
		"information": {"information", "information"},
		"warning":     {"avertissement", "warning"},
		"major":       {"majeur", "major"},
		"critical":    {"critique", "critical"},
	}[severity]
	if english {
		return labels[1], ok
	}
	return labels[0], ok
}

func severityRank(severity string) int {
	return map[string]int{"information": 1, "warning": 2, "major": 3, "critical": 4}[severity]
}

func targetNames(names []string, count int) string {
	shown := make([]string, 0, 3)
	for _, name := range names {
		if name = oneLine(name, 40); name != "" && len(shown) < 3 {
			shown = append(shown, name)
		}
	}
	if len(shown) == 0 {
		return ""
	}
	list := strings.Join(shown, ", ")
	if count > len(shown) {
		list += fmt.Sprintf(" +%d", count-len(shown))
	}
	return list
}

// factDetail garde la valeur utile d'un fait reconnu. La ressource technique
// (disque, système de fichiers, certificat) est affichée seule : le mot
// « Ressource » désigne déjà l'objet suivi placé en titre.
func factDetail(fact *alerttext.Fact, locale string) string {
	if fact == nil {
		return ""
	}
	normalized := fact.Normalize()
	if normalized.Kind == "" {
		return ""
	}
	description := alerttext.Render(normalized, locale).Description
	if normalized.Resource != "" && (description == "" ||
		strings.HasPrefix(description, "Ressource") || strings.HasPrefix(description, "Resource")) {
		return normalized.Resource
	}
	return description
}

func formatDuration(duration time.Duration, english bool) string {
	minutes := int(duration / time.Minute)
	switch {
	case minutes < 1:
		return pick(english, "moins d’1 min", "under 1 min")
	case minutes < 60:
		return fmt.Sprintf("%d min", minutes)
	case minutes < 24*60:
		if minutes%60 == 0 {
			return fmt.Sprintf("%d h", minutes/60)
		}
		return fmt.Sprintf("%d h %02d", minutes/60, minutes%60)
	default:
		days, hours := minutes/(24*60), minutes%(24*60)/60
		if hours == 0 {
			return fmt.Sprintf(pick(english, "%d j", "%d d"), days)
		}
		return fmt.Sprintf(pick(english, "%d j %d h", "%d d %d h"), days, hours)
	}
}

func pick(english bool, french, englishText string) string {
	if english {
		return englishText
	}
	return french
}
