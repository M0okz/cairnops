package synthesis

import (
	"regexp"
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

// Traductions abrégées des libellés signalés par Zabbix.
// Elles ne classent pas la Nature et ne modifient jamais son identité, ses Preuves
// ou ses seuils. L'ancrage complet préserve les conditions personnalisées.
// https://github.com/zabbix/zabbix/blob/release/7.0/templates/os/linux/template_os_linux.yaml
var reportedSystemLoad = regexp.MustCompile(`^Linux: Load average is too high(?: \(per CPU load over (?:[0-9]+(?:\.[0-9]+)?|\{\$LOAD_AVG_PER_CPU\.MAX\.WARN\}) for 5m\))?$`)

var reportedStorageSpace = regexp.MustCompile(`^Linux: FS \[[^\]]+\]: Space is (?:critically )?low \(used > [0-9]+(?:\.[0-9]+)?%, total [0-9]+(?:\.[0-9]+)?[KMGTPE]?B\)$`)

var reportedProxmoxFilesystem = regexp.MustCompile(`^Proxmox VE: Node \[[^\]]+\]: QEMU \[[^\]]+\]\[[^\]]+\]: Filesystem used high$`)

func notificationSourceTitle(s Situation, locale string) string {
	label := strings.Join(strings.Fields(s.NatureLabel), " ")
	if s.NatureScope == "connector" && strings.HasPrefix(s.NatureKey, "zabbix:") {
		var labels [2]string
		switch {
		case reportedSystemLoad.MatchString(label):
			labels = [2]string{"Charge système élevée", "High system load"}
		case reportedStorageSpace.MatchString(label):
			labels = [2]string{"Espace de stockage insuffisant", "Low storage space"}
		case reportedProxmoxFilesystem.MatchString(label):
			labels = [2]string{"Occupation du stockage élevée", "High filesystem usage"}
		case label == "Linux: Number of installed packages has been changed":
			labels = [2]string{"Paquets installés modifiés", "Installed packages changed"}
		}
		if labels[0] != "" {
			if locale == "en" {
				return labels[1]
			}
			return labels[0]
		}
	}
	if label != "" {
		return label
	}
	if locale == "en" {
		return "Monitoring signal"
	}
	return "Signal de supervision"
}
