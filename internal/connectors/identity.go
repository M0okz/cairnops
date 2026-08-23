package connectors

import (
	"net"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
)

const (
	matchNameScore      = 100
	matchMachineIDScore = 120
	matchHostnameScore  = 80
	matchIPScore        = 70
	matchAliasScore     = 60
	matchSupportScore   = 5
)

var infrastructureZones = map[string]struct{}{
	"dmz": {}, "trust": {}, "untrust": {},
}

// TargetIdentity est l'identité observable d'une Cible. Elle agrège les noms
// et adresses déjà apportés par ses Sources sans en faire des attributs
// autoritaires : ils servent à proposer un rapprochement, jamais à fusionner
// silencieusement deux Cibles.
type TargetIdentity struct {
	TargetReference
	Names       []string
	Addresses   []string
	Identifiers []string
}

type DiscoveredIdentity struct {
	Names       []string
	Addresses   []string
	Identifiers []string
}

// IdentityCandidateKeys produit l'index grossier du moteur de rapprochement.
// Partager une clé autorise uniquement une comparaison détaillée ; MatchTargets
// reste l'autorité qui pondère les preuves, détecte les contradictions et peut
// s'abstenir. L'index évite ainsi un balayage quadratique des Cibles.
func IdentityCandidateKeys(names, addresses, identifiers []string) []string {
	keys := make(map[string]struct{})
	for value := range normalizedSet(names, normalizeName) {
		keys["name:"+value] = struct{}{}
	}
	for value := range normalizedSet(addresses, normalizeAddress) {
		keys["address:"+value] = struct{}{}
	}
	for value := range normalizedSet(identifiers, normalizeIdentifier) {
		keys["identifier:"+value] = struct{}{}
	}
	for _, fingerprint := range nameFingerprints(names) {
		keys["alias:"+fingerprint.Core] = struct{}{}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

type MatchEvidence struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type TargetMatch struct {
	Target         TargetReference `json:"target"`
	Confidence     string          `json:"confidence"`
	Evidence       []MatchEvidence `json:"evidence"`
	Contradictions []MatchEvidence `json:"contradictions,omitempty"`
	Score          int             `json:"score"`
}

func identityForZabbix(host zabbix.Host) DiscoveredIdentity {
	identity := DiscoveredIdentity{Names: []string{host.Name, host.Technical}}
	for _, item := range host.Interfaces {
		identity.Addresses = append(identity.Addresses, item.Address)
	}
	return identity
}

func identityForUptimeKuma(monitor uptimekuma.Monitor) DiscoveredIdentity {
	return DiscoveredIdentity{
		Names:     []string{monitor.Name},
		Addresses: []string{monitor.URL, monitor.Hostname, monitor.Address()},
	}
}

func identityForPatchMon(host patchmon.Host) DiscoveredIdentity {
	return DiscoveredIdentity{
		Names:       []string{host.Name(), host.FriendlyName, host.Hostname},
		Addresses:   []string{host.IP, host.Hostname},
		Identifiers: []string{host.MachineID},
	}
}

func matchTargets(discovered DiscoveredIdentity, targets []TargetIdentity) []TargetMatch {
	return MatchTargets(discovered, targets)
}

// MatchTargets expose le même moteur déterministe à la détection continue.
// Les preuves restent explicables et une contradiction d'identifiants stables
// force l'abstention : un rapprochement manuel reste toujours possible.
func MatchTargets(discovered DiscoveredIdentity, targets []TargetIdentity) []TargetMatch {
	discoveredNames := normalizedSet(discovered.Names, normalizeName)
	discoveredAddresses := normalizedSet(discovered.Addresses, normalizeAddress)
	discoveredAliases := nameFingerprints(discovered.Names)
	discoveredIdentifiers := normalizedSet(discovered.Identifiers, normalizeIdentifier)
	matches := make([]TargetMatch, 0)
	for _, candidate := range targets {
		candidateIdentifiers := normalizedSet(candidate.Identifiers, normalizeIdentifier)
		matchingIdentifiers := intersection(discoveredIdentifiers, candidateIdentifiers)
		stableIdentityConflict := len(discoveredIdentifiers) > 0 && len(candidateIdentifiers) > 0 && len(matchingIdentifiers) == 0
		evidenceByKey := make(map[string]MatchEvidence)
		evidenceKinds := make(map[string]struct{})
		score := 0
		for identifier := range matchingIdentifiers {
			addMatchEvidence(evidenceByKey, evidenceKinds, MatchEvidence{Kind: "same_machine_id", Value: identifier})
			if score < matchMachineIDScore {
				score = matchMachineIDScore
			}
		}
		for name := range intersection(discoveredNames, normalizedSet(candidate.Names, normalizeName)) {
			addMatchEvidence(evidenceByKey, evidenceKinds, MatchEvidence{Kind: "same_name", Value: name})
			if score < matchNameScore {
				score = matchNameScore
			}
		}
		for address := range intersection(discoveredAddresses, normalizedSet(candidate.Addresses, normalizeAddress)) {
			kind, addressScore := "same_hostname", matchHostnameScore
			if net.ParseIP(address) != nil {
				kind, addressScore = "same_ip", matchIPScore
			}
			addMatchEvidence(evidenceByKey, evidenceKinds, MatchEvidence{Kind: kind, Value: address})
			if score < addressScore {
				score = addressScore
			}
		}
		for alias := range matchingAliases(discoveredAliases, nameFingerprints(candidate.Names)) {
			addMatchEvidence(evidenceByKey, evidenceKinds, MatchEvidence{Kind: "similar_name", Value: alias})
			if score < matchAliasScore {
				score = matchAliasScore
			}
		}
		if score == 0 {
			continue
		}
		if len(evidenceKinds) > 1 {
			score += (len(evidenceKinds) - 1) * matchSupportScore
		}
		evidence := make([]MatchEvidence, 0, len(evidenceByKey))
		for _, item := range evidenceByKey {
			evidence = append(evidence, item)
		}
		sort.Slice(evidence, func(i, j int) bool {
			if evidencePriority(evidence[i].Kind) == evidencePriority(evidence[j].Kind) {
				return evidence[i].Value < evidence[j].Value
			}
			return evidencePriority(evidence[i].Kind) < evidencePriority(evidence[j].Kind)
		})
		confidence := "low"
		contradictions := make([]MatchEvidence, 0, 1)
		if stableIdentityConflict {
			contradictions = append(contradictions, MatchEvidence{
				Kind:  "different_machine_id",
				Value: strings.Join(sortedSetValues(discoveredIdentifiers), ", ") + " ≠ " + strings.Join(sortedSetValues(candidateIdentifiers), ", "),
			})
		} else if score >= matchNameScore {
			confidence = "high"
		} else if score >= matchIPScore {
			confidence = "medium"
		}
		matches = append(matches, TargetMatch{
			Target: candidate.TargetReference, Confidence: confidence,
			Evidence: evidence, Contradictions: contradictions, Score: score,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if normalizeName(matches[i].Target.Name) != normalizeName(matches[j].Target.Name) {
			return normalizeName(matches[i].Target.Name) < normalizeName(matches[j].Target.Name)
		}
		return matches[i].Target.ID < matches[j].Target.ID
	})
	return matches
}

func sortedSetValues(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func addMatchEvidence(byKey map[string]MatchEvidence, kinds map[string]struct{}, evidence MatchEvidence) {
	key := evidence.Kind + "\x00" + evidence.Value
	byKey[key] = evidence
	kinds[evidence.Kind] = struct{}{}
}

func evidencePriority(kind string) int {
	switch kind {
	case "same_machine_id":
		return 0
	case "same_name":
		return 1
	case "same_hostname":
		return 2
	case "same_ip":
		return 3
	case "similar_name":
		return 4
	default:
		return 5
	}
}

func normalizeIdentifier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// nameFingerprint retire uniquement les conventions d'infrastructure que
// CairnOps sait interpréter sans dictionnaire métier : une zone réseau en tête
// et un numéro d'instance en queue. Les zones et instances restent conservées
// pour empêcher qu'un alias rapproche deux objets explicitement différents.
type nameFingerprint struct {
	Core     string
	Zone     string
	Instance string
}

func nameFingerprints(values []string) []nameFingerprint {
	seen := make(map[nameFingerprint]struct{}, len(values))
	result := make([]nameFingerprint, 0, len(values))
	for _, value := range values {
		fingerprint := fingerprintName(value)
		if fingerprint.Core == "" {
			continue
		}
		if _, exists := seen[fingerprint]; exists {
			continue
		}
		seen[fingerprint] = struct{}{}
		result = append(result, fingerprint)
	}
	return result
}

func fingerprintName(value string) nameFingerprint {
	tokens := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char)
	})
	if len(tokens) == 0 {
		return nameFingerprint{}
	}
	fingerprint := nameFingerprint{}
	if len(tokens) > 1 {
		if _, known := infrastructureZones[tokens[0]]; known {
			fingerprint.Zone = tokens[0]
			tokens = tokens[1:]
		}
	}
	if len(tokens) > 1 && isInstanceToken(tokens[len(tokens)-1]) {
		fingerprint.Instance = strings.TrimLeft(tokens[len(tokens)-1], "0")
		if fingerprint.Instance == "" {
			fingerprint.Instance = "0"
		}
		tokens = tokens[:len(tokens)-1]
	}
	fingerprint.Core = strings.Join(tokens, "")
	return fingerprint
}

func isInstanceToken(value string) bool {
	if value == "" || len(value) > 3 {
		return false
	}
	for _, char := range value {
		if !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func matchingAliases(left, right []nameFingerprint) map[string]struct{} {
	result := make(map[string]struct{})
	for _, discovered := range left {
		for _, candidate := range right {
			if discovered.Core == "" || discovered.Core != candidate.Core {
				continue
			}
			if discovered.Zone != "" && candidate.Zone != "" && discovered.Zone != candidate.Zone {
				continue
			}
			if discovered.Instance != "" && candidate.Instance != "" && discovered.Instance != candidate.Instance {
				continue
			}
			// Un nom déjà strictement identique possède une preuve plus forte et
			// n'a pas besoin d'une seconde preuve d'alias identique.
			if discovered.Zone == candidate.Zone && discovered.Instance == candidate.Instance {
				continue
			}
			result[discovered.Core] = struct{}{}
		}
	}
	return result
}

// suggestedTarget ne choisit que si le meilleur score est unique, assez fort
// et sans contradiction. Une piste contradictoire reste visible mais ne
// devient jamais une décision implicite, même si son nom est identique.
func suggestedTarget(matches []TargetMatch) *TargetReference {
	if len(matches) == 0 || matches[0].Score < matchIPScore || len(matches[0].Contradictions) > 0 || (len(matches) > 1 && matches[0].Score == matches[1].Score) {
		return nil
	}
	target := matches[0].Target
	return &target
}

func normalizedSet(values []string, normalize func(string) string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if normalized := normalize(value); normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}

func intersection(left, right map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{})
	for value := range left {
		if _, ok := right[value]; ok {
			result[value] = struct{}{}
		}
	}
	return result
}

// normalizeAddress réduit URL, host:port, IPv4, IPv6 et FQDN à l'identité de
// leur hôte. Le port reste une caractéristique du service, pas une identité de
// machine ; il pourra participer plus tard au typage équipement/service.
func normalizeAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() && parsed.Hostname() != "" {
		value = parsed.Hostname()
	} else if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else {
		value = strings.Trim(value, "[]")
	}
	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return value
}
