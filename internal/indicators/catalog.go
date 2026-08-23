package indicators

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
)

var bracketDimension = regexp.MustCompile(`\[([^,\]]+)`)

func zabbixCandidate(item zabbix.Item) (Candidate, bool) {
	key := strings.ToLower(strings.TrimSpace(item.Key))
	name := strings.ToLower(strings.TrimSpace(item.Name))
	candidate := Candidate{ExternalID: item.ID, Label: strings.TrimSpace(item.Name), Available: true, Metadata: map[string]any{"key": item.Key, "units": item.Units}}
	switch {
	case strings.Contains(key, "system.cpu.util") || strings.Contains(name, "cpu utilization") || strings.Contains(name, "utilisation cpu"):
		candidate.SemanticKey, candidate.Unit, candidate.Label = "cpu.utilization", "percent", "Utilisation CPU"
	case strings.Contains(key, "vm.memory.size") && (strings.Contains(key, "pused") || strings.Contains(name, "utilization")) || strings.Contains(name, "memory utilization"):
		candidate.SemanticKey, candidate.Unit, candidate.Label = "memory.utilization", "percent", "Utilisation mémoire"
	case strings.Contains(key, "vfs.fs.size") && (strings.Contains(key, "pused") || strings.Contains(name, "space utilization")):
		candidate.SemanticKey, candidate.Unit = "filesystem.utilization", "percent"
		candidate.Dimension = zabbixDimension(item.Key)
		candidate.Label = fmt.Sprintf("Volume %s", displayDimension(candidate.Dimension))
	case strings.Contains(key, "net.if.in"):
		candidate.SemanticKey, candidate.Unit = "network.in", "bytes_per_second"
		candidate.Dimension = zabbixDimension(item.Key)
		candidate.Label = fmt.Sprintf("Réseau entrant · %s", displayDimension(candidate.Dimension))
		candidate.Metadata["scale"] = zabbixNetworkScale(item.Units)
	case strings.Contains(key, "net.if.out"):
		candidate.SemanticKey, candidate.Unit = "network.out", "bytes_per_second"
		candidate.Dimension = zabbixDimension(item.Key)
		candidate.Label = fmt.Sprintf("Réseau sortant · %s", displayDimension(candidate.Dimension))
		candidate.Metadata["scale"] = zabbixNetworkScale(item.Units)
	default:
		return Candidate{}, false
	}
	candidate.Recommended = candidate.SemanticKey == "cpu.utilization" || candidate.SemanticKey == "memory.utilization"
	return candidate, true
}

// zabbixCandidates réduit les variantes techniques à un seul item exact par
// sémantique et dimension. Le catalogue reste ainsi court et la confirmation
// ne peut pas sélectionner deux CPU concurrents pour la même Cible.
func zabbixCandidates(items []zabbix.Item) []Candidate {
	type ranked struct {
		candidate Candidate
		priority  int
	}
	best := map[string]ranked{}
	for _, item := range items {
		candidate, ok := zabbixCandidate(item)
		if !ok {
			continue
		}
		key := candidate.SemanticKey + "\x00" + candidate.Dimension
		next := ranked{candidate: candidate, priority: zabbixCandidatePriority(item, candidate)}
		current, known := best[key]
		if !known || next.priority > current.priority || (next.priority == current.priority && next.candidate.ExternalID < current.candidate.ExternalID) {
			best[key] = next
		}
	}
	candidates := make([]Candidate, 0, len(best))
	for _, entry := range best {
		candidates = append(candidates, entry.candidate)
	}
	sortCandidates(candidates)
	return candidates
}

func zabbixCandidatePriority(item zabbix.Item, candidate Candidate) int {
	key := strings.ToLower(strings.TrimSpace(item.Key))
	name := strings.ToLower(strings.TrimSpace(item.Name))
	priority := 0
	if name == "cpu utilization" || name == "utilisation cpu" || name == "memory utilization" || name == "utilisation mémoire" {
		priority += 100
	}
	if key == "system.cpu.util" || strings.Contains(key, "pused") {
		priority += 50
	}
	if strings.Contains(key, "idle") || strings.Contains(key, "iowait") || strings.Contains(key, "interrupt") {
		priority -= 50
	}
	units := strings.ToLower(strings.TrimSpace(item.Units))
	if candidate.SemanticKey == "network.in" || candidate.SemanticKey == "network.out" {
		if units == "b/s" || units == "bps" {
			priority += 20
		}
	}
	return priority
}

func zabbixDimension(key string) string {
	match := bracketDimension.FindStringSubmatch(key)
	if len(match) < 2 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(match[1]), `"`)
}

func displayDimension(value string) string {
	if strings.TrimSpace(value) == "" {
		return "À vérifier"
	}
	return value
}

func zabbixNetworkScale(unit string) float64 {
	unit = strings.ToLower(strings.TrimSpace(unit))
	if strings.Contains(unit, "bps") && !strings.Contains(unit, "b/s") {
		return 0.125
	}
	return 1
}

func uptimeCandidates(monitor uptimekuma.Monitor) []Candidate {
	candidates := []Candidate{{
		SemanticKey: "response.time", Label: "Temps de réponse", ExternalID: "response:" + monitor.ID,
		Unit: "milliseconds", Recommended: true, Available: monitor.ResponseMilliseconds != nil,
		Reason: "Non publié par cet endpoint", Metadata: map[string]any{"monitor_id": monitor.ID},
	}}
	if monitor.ResponseMilliseconds != nil {
		candidates[0].Reason = ""
	}
	if monitor.CertificateDaysRemaining != nil || monitor.CertificateValid != nil {
		candidates = append(candidates,
			Candidate{SemanticKey: "certificate.days_remaining", Label: "Expiration du certificat", ExternalID: "certificate_days:" + monitor.ID, Unit: "days", Recommended: true, Available: monitor.CertificateDaysRemaining != nil, Metadata: map[string]any{"monitor_id": monitor.ID}},
			Candidate{SemanticKey: "certificate.valid", Label: "Validité du certificat", ExternalID: "certificate_valid:" + monitor.ID, Unit: "boolean", Recommended: true, Available: monitor.CertificateValid != nil, Metadata: map[string]any{"monitor_id": monitor.ID}},
		)
	}
	return candidates
}

func patchMonCandidates(host patchmon.Host) []Candidate {
	return []Candidate{
		{SemanticKey: "updates.count", Label: "Mises à jour disponibles", ExternalID: "updates:" + host.ID, Unit: "count", Recommended: true, Available: true, Metadata: map[string]any{"host_id": host.ID}},
		{SemanticKey: "security_updates.count", Label: "Correctifs de sécurité", ExternalID: "security_updates:" + host.ID, Unit: "count", Recommended: true, Available: true, Metadata: map[string]any{"host_id": host.ID}},
		{SemanticKey: "reboot.required", Label: "Redémarrage requis", ExternalID: "reboot:" + host.ID, Unit: "boolean", Recommended: true, Available: true, Metadata: map[string]any{"host_id": host.ID}},
		{SemanticKey: "reporting.age", Label: "Fraîcheur de remontée", ExternalID: "reporting_age:" + host.ID, Unit: "seconds", Recommended: true, Available: host.LastUpdate != nil, Reason: reportingReason(host), Metadata: map[string]any{"host_id": host.ID}},
	}
}

func reportingReason(host patchmon.Host) string {
	if host.LastUpdate == nil {
		return "Dernière remontée non publiée"
	}
	return ""
}
