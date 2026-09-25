package softwareupdates

import (
	"time"

	"github.com/M0okz/cairnops/internal/versions"
)

// Groupes de la vue des mises à jour. Un service appartient à un seul groupe,
// calculé ici afin que toutes les interfaces partagent le même classement.
const (
	GroupApply   = "apply"
	GroupReview  = "review"
	GroupCurrent = "current"
)

// Raisons pour lesquelles les versions affichées ne sont pas confirmées par
// la dernière lecture d'Argus. Les dernières valeurs valides restent visibles.
const (
	IssueArgusUnavailable    = "argus_unavailable"
	IssueInstalledUnreadable = "installed_unreadable"
	IssueTargetUnreadable    = "target_unreadable"
	IssueServiceMissing      = "service_missing"
	IssueServiceInactive     = "service_inactive"
	IssueUnconfirmed         = "unconfirmed"
)

type argusFacts struct {
	reachable     bool
	unknownReason string
	installedOK   string
	targetOK      string
}

func (s *Service) project(facts argusFacts) {
	assessment := versions.Assess(s.Installed, s.Target)
	s.Situation, s.Level = string(assessment.Situation), string(assessment.Level)
	if s.Installed == "" || s.Target == "" {
		s.Situation, s.Level = "unknown", ""
	}
	switch {
	case !s.Known || s.Situation == "unknown":
		s.Group = GroupReview
		s.VerificationIssue = verificationIssue(facts)
	case s.Skipped && assessment.Situation != versions.Current:
		s.Group = GroupCurrent
	case assessment.Situation == versions.Update:
		s.Group = GroupApply
	case assessment.Situation == versions.Current:
		s.Group = GroupCurrent
	default:
		s.Group = GroupReview
	}
}

func verificationIssue(facts argusFacts) string {
	if !facts.reachable {
		return IssueArgusUnavailable
	}
	switch facts.unknownReason {
	case "deployed_version_query_failed":
		return IssueInstalledUnreadable
	case "latest_version_query_failed":
		return IssueTargetUnreadable
	case "argus_service_missing":
		return IssueServiceMissing
	case "argus_service_inactive", "argus_service_deployed_version_not_configured":
		return IssueServiceInactive
	}
	switch {
	case facts.installedOK == "false":
		return IssueInstalledUnreadable
	case facts.targetOK == "false":
		return IssueTargetUnreadable
	}
	return IssueUnconfirmed
}

// Event est un fait lisible de l'historique : première observation, nouvelle
// version installée constatée ou nouvelle cible proposée par Argus.
type Event struct {
	Kind       string    `json:"kind"`
	Version    string    `json:"version"`
	Previous   string    `json:"previous,omitempty"`
	Target     string    `json:"target,omitempty"`
	Direction  string    `json:"direction,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

// events reconstruit les changements constatés depuis l'historique brut, qui
// mêle les changements d'installation et de cible. history est ordonné du plus
// récent au plus ancien ; les événements le sont aussi. Lorsque l'historique
// est tronqué, sa plus ancienne ligne sert seulement de référence.
func events(history []History, truncated bool) []Event {
	result := make([]Event, 0, len(history))
	for index := len(history) - 1; index >= 0; index-- {
		entry := history[index]
		if index == len(history)-1 {
			if !truncated {
				result = append(result, Event{Kind: "first", Version: entry.Installed, Target: entry.Target, ObservedAt: entry.ObservedAt})
			}
			continue
		}
		previous := history[index+1]
		if entry.Installed != previous.Installed {
			direction := "changed"
			if order, ok := versions.Compare(previous.Installed, entry.Installed); ok && order < 0 {
				direction = "upgrade"
			} else if ok && order > 0 {
				direction = "rollback"
			}
			result = append(result, Event{Kind: "installed", Version: entry.Installed, Previous: previous.Installed, Direction: direction, ObservedAt: entry.ObservedAt})
		}
		if entry.Target != previous.Target && entry.Target != entry.Installed {
			result = append(result, Event{Kind: "target", Version: entry.Target, Previous: previous.Target, ObservedAt: entry.ObservedAt})
		}
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}
