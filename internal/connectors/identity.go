package connectors

import (
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
)

const (
	matchNameScore     = 100
	matchHostnameScore = 80
	matchIPScore       = 70
)

// TargetIdentity est l'identité observable d'une Cible. Elle agrège les noms
// et adresses déjà apportés par ses Sources sans en faire des attributs
// autoritaires : ils servent à proposer un rapprochement, jamais à fusionner
// silencieusement deux Cibles.
type TargetIdentity struct {
	TargetReference
	Names     []string
	Addresses []string
}

type DiscoveredIdentity struct {
	Names     []string
	Addresses []string
}

type MatchEvidence struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type TargetMatch struct {
	Target     TargetReference `json:"target"`
	Confidence string          `json:"confidence"`
	Evidence   []MatchEvidence `json:"evidence"`
	score      int
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

func matchTargets(discovered DiscoveredIdentity, targets []TargetIdentity) []TargetMatch {
	discoveredNames := normalizedSet(discovered.Names, normalizeName)
	discoveredAddresses := normalizedSet(discovered.Addresses, normalizeAddress)
	matches := make([]TargetMatch, 0)
	for _, candidate := range targets {
		evidence := make([]MatchEvidence, 0, 3)
		score := 0
		for name := range intersection(discoveredNames, normalizedSet(candidate.Names, normalizeName)) {
			evidence = append(evidence, MatchEvidence{Kind: "same_name", Value: name})
			if score < matchNameScore {
				score = matchNameScore
			}
		}
		for address := range intersection(discoveredAddresses, normalizedSet(candidate.Addresses, normalizeAddress)) {
			kind, addressScore := "same_hostname", matchHostnameScore
			if net.ParseIP(address) != nil {
				kind, addressScore = "same_ip", matchIPScore
			}
			evidence = append(evidence, MatchEvidence{Kind: kind, Value: address})
			if score < addressScore {
				score = addressScore
			}
		}
		if score == 0 {
			continue
		}
		sort.Slice(evidence, func(i, j int) bool {
			if evidence[i].Kind == evidence[j].Kind {
				return evidence[i].Value < evidence[j].Value
			}
			return evidence[i].Kind < evidence[j].Kind
		})
		confidence := "medium"
		if score >= matchNameScore {
			confidence = "high"
		}
		matches = append(matches, TargetMatch{
			Target: candidate.TargetReference, Confidence: confidence,
			Evidence: evidence, score: score,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if normalizeName(matches[i].Target.Name) != normalizeName(matches[j].Target.Name) {
			return normalizeName(matches[i].Target.Name) < normalizeName(matches[j].Target.Name)
		}
		return matches[i].Target.ID < matches[j].Target.ID
	})
	return matches
}

// suggestedTarget ne choisit que si le meilleur score est unique. Une IP
// partagée par deux Cibles reste visible parmi les candidats mais ne devient
// jamais une décision implicite.
func suggestedTarget(matches []TargetMatch) *TargetReference {
	if len(matches) == 0 || (len(matches) > 1 && matches[0].score == matches[1].score) {
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
