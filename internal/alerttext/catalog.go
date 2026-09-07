// Package alerttext presents recognized alert facts. Its vocabulary does not
// define incident identities, thresholds, severity or notification decisions.
package alerttext

import (
	"fmt"
	"strings"
)

type Kind string

const (
	BackupFailure       Kind = "backup.failure"
	BackupFreshness     Kind = "backup.freshness"
	Unavailable         Kind = "availability.unavailable"
	DiskLatency         Kind = "disk.latency.high"
	DiskSpace           Kind = "disk.space.low"
	DiskInodes          Kind = "disk.inodes.low"
	CPUUsage            Kind = "cpu.usage.high"
	SystemLoad          Kind = "system.load.high"
	MemoryUsage         Kind = "memory.usage.high"
	MemoryAvailable     Kind = "memory.available.low"
	SwapSpace           Kind = "swap.space.low"
	PackageCountChanged Kind = "packages.count.changed"
	CertificateExpiry   Kind = "certificate.expiring"
	CertificateInvalid  Kind = "certificate.invalid"
	SecurityUpdates     Kind = "software.security_updates"
	RebootRequired      Kind = "system.reboot_required"
	SoftwareUpdate      Kind = "software.update_available"
)

// Fact is presentation-only enrichment supplied by a trusted connector adapter.
// Empty fields mean unavailable. A zero count is never inferred from missing data.
// Source messages remain separately stored, verbatim, as evidence.
type Fact struct {
	Kind             Kind   `json:"kind,omitempty"`
	Resource         string `json:"resource,omitempty"`
	Count            *int   `json:"count,omitempty"`
	CurrentVersion   string `json:"current_version,omitempty"`
	AvailableVersion string `json:"available_version,omitempty"`
}

type Text struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type Localized struct {
	FR Text `json:"fr"`
	EN Text `json:"en"`
}

var titles = map[Kind][2]string{
	BackupFailure:       {"Échec de sauvegarde", "Backup failure"},
	BackupFreshness:     {"Sauvegarde trop ancienne", "Outdated backup"},
	Unavailable:         {"Indisponibilité", "Unavailability"},
	DiskLatency:         {"Latence disque élevée", "High disk latency"},
	DiskSpace:           {"Espace disque insuffisant", "Low disk space"},
	DiskInodes:          {"Peu d’inodes disponibles", "Low free inodes"},
	CPUUsage:            {"Utilisation CPU élevée", "High CPU utilization"},
	SystemLoad:          {"Charge système moyenne élevée", "High average system load"},
	MemoryUsage:         {"Utilisation mémoire élevée", "High memory utilization"},
	MemoryAvailable:     {"Mémoire disponible insuffisante", "Low available memory"},
	SwapSpace:           {"Espace swap insuffisant", "Low swap space"},
	PackageCountChanged: {"Nombre de paquets installés modifié", "Installed package count changed"},
	CertificateExpiry:   {"Expiration de certificat proche", "Certificate nearing expiry"},
	CertificateInvalid:  {"Certificat invalide", "Invalid certificate"},
	SecurityUpdates:     {"Correctifs de sécurité requis", "Security updates required"},
	RebootRequired:      {"Redémarrage requis", "Restart required"},
	SoftwareUpdate:      {"Mise à jour logicielle disponible", "Software update available"},
}

func Title(kind Kind, locale string) (string, bool) {
	labels, ok := titles[kind]
	if locale == "en" {
		return labels[1], ok
	}
	return labels[0], ok
}

// Normalize drops unknown kinds and parameters irrelevant to the recognized
// condition. It bounds source-controlled values without interpreting their text.
func (f Fact) Normalize() Fact {
	if _, ok := titles[f.Kind]; !ok {
		return Fact{}
	}
	f.Resource = bounded(f.Resource)
	if f.Kind != SecurityUpdates || f.Count == nil || *f.Count < 0 {
		f.Count = nil
	}
	if f.Kind != SoftwareUpdate {
		f.CurrentVersion, f.AvailableVersion = "", ""
	} else {
		f.CurrentVersion, f.AvailableVersion = bounded(f.CurrentVersion), bounded(f.AvailableVersion)
	}
	return f
}

func Render(f Fact, locale string) Text {
	f = f.Normalize()
	title, ok := Title(f.Kind, locale)
	if !ok {
		return Text{}
	}
	description := ""
	if f.Resource != "" {
		description = fmt.Sprintf("Ressource : %s", f.Resource)
		if locale == "en" {
			description = fmt.Sprintf("Resource: %s", f.Resource)
		}
	}
	if f.Kind == SecurityUpdates && f.Count != nil {
		description = fmt.Sprintf("%d correctifs de sécurité disponibles", *f.Count)
		if *f.Count == 1 {
			description = "1 correctif de sécurité disponible"
		}
		if locale == "en" {
			description = fmt.Sprintf("%d security updates available", *f.Count)
			if *f.Count == 1 {
				description = "1 security update available"
			}
		}
	}
	if f.Kind == SoftwareUpdate && f.CurrentVersion != "" && f.AvailableVersion != "" {
		description = fmt.Sprintf("Version %s disponible · %s déployée", f.AvailableVersion, f.CurrentVersion)
		if locale == "en" {
			description = fmt.Sprintf("Version %s available · %s deployed", f.AvailableVersion, f.CurrentVersion)
		}
	}
	return Text{Title: title, Description: description}
}

func Localize(f Fact) Localized {
	return Localized{FR: Render(f, "fr"), EN: Render(f, "en")}
}

// Common returns only a shared title meaning. Parameters belong to individual
// evidence and must never be copied from an arbitrary member of an incident.
// A missing or divergent member prevents a common presentation.
func Common(facts []Fact) Fact {
	if len(facts) == 0 {
		return Fact{}
	}
	kind := facts[0].Normalize().Kind
	if kind == "" {
		return Fact{}
	}
	for _, fact := range facts[1:] {
		if fact.Normalize().Kind != kind {
			return Fact{}
		}
	}
	return Fact{Kind: kind}
}

func bounded(value string) string {
	runes := []rune(strings.Join(strings.Fields(value), " "))
	if len(runes) > 160 {
		return string(runes[:159]) + "…"
	}
	return string(runes)
}
