package synthesis

import (
	"strings"
)

// RenderNotification adapte les mêmes faits au gabarit compact partagé par
// le Push, la boîte intégrée et Mattermost : problème, puis Cible et Gravité.
// Les compteurs et la Résolution suivent exactement les règles de Render.
func RenderNotification(s Situation, locale string) Text {
	return render(s, locale, true)
}

func LocalizeNotification(s Situation) Localized {
	return Localized{FR: RenderNotification(s, "fr"), EN: RenderNotification(s, "en")}
}

// Unknown/custom conditions retain their source wording. Recognition belongs
// to the connector adapter and is carried separately in Situation.AlertKind.
func notificationSourceTitle(s Situation, locale string) string {
	label := strings.Join(strings.Fields(s.NatureLabel), " ")
	if label != "" {
		return label
	}
	if locale == "en" {
		return "Monitoring signal"
	}
	return "Signal de supervision"
}
