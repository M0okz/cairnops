package indicators

import "sort"

type overviewPreference struct {
	semanticKey string
	ascending   bool
}

var overviewPreferences = []overviewPreference{
	{semanticKey: "filesystem.utilization"},
	{semanticKey: "memory.utilization"},
	{semanticKey: "response.time"},
	{semanticKey: "certificate.days_remaining", ascending: true},
	{semanticKey: "security_updates.count"},
	{semanticKey: "updates.count"},
	{semanticKey: "reboot.required"},
	{semanticKey: "reporting.age"},
	{semanticKey: "cpu.utilization"},
	{semanticKey: "network.in"},
	{semanticKey: "network.out"},
	{semanticKey: "certificate.valid", ascending: true},
}

// selectOverviewIndicators places explicit personal pins first, then fills the
// remaining slots with distinct, currently observed kinds of useful context.
// The suggestions never become pins and therefore never change user state.
func selectOverviewIndicators(indicators []Indicator, limit int) []Indicator {
	if limit <= 0 {
		return []Indicator{}
	}

	pinned := make([]Indicator, 0, limit)
	for _, indicator := range indicators {
		if indicator.Pinned {
			pinned = append(pinned, indicator)
		}
	}
	sort.SliceStable(pinned, func(left, right int) bool {
		leftPosition, rightPosition := limit, limit
		if pinned[left].PinPosition != nil {
			leftPosition = *pinned[left].PinPosition
		}
		if pinned[right].PinPosition != nil {
			rightPosition = *pinned[right].PinPosition
		}
		if leftPosition != rightPosition {
			return leftPosition < rightPosition
		}
		return pinned[left].ID < pinned[right].ID
	})
	if len(pinned) > limit {
		pinned = pinned[:limit]
	}

	selected := append([]Indicator{}, pinned...)
	selectedIDs := make(map[string]struct{}, len(selected))
	selectedSemantics := make(map[string]struct{}, len(selected))
	for _, indicator := range selected {
		selectedIDs[indicator.ID] = struct{}{}
		selectedSemantics[indicator.SemanticKey] = struct{}{}
	}

	eligible := make([]Indicator, 0, len(indicators))
	for _, indicator := range indicators {
		if _, exists := selectedIDs[indicator.ID]; exists {
			continue
		}
		if indicator.LastValue == nil || indicator.LastObservedAt == nil || indicator.LastError != "" {
			continue
		}
		eligible = append(eligible, indicator)
	}

	for _, preference := range overviewPreferences {
		if len(selected) >= limit {
			break
		}
		if _, exists := selectedSemantics[preference.semanticKey]; exists {
			continue
		}
		best := bestOverviewIndicator(eligible, preference)
		if best == nil {
			continue
		}
		selected = append(selected, *best)
		selectedIDs[best.ID] = struct{}{}
		selectedSemantics[best.SemanticKey] = struct{}{}
	}

	if len(selected) >= limit {
		return selected
	}

	// Les sémantiques inconnues restent affichables sans rendre la sélection
	// instable : les relevés les plus récents gagnent, puis leur identifiant.
	sort.SliceStable(eligible, func(left, right int) bool {
		if !eligible[left].LastObservedAt.Equal(*eligible[right].LastObservedAt) {
			return eligible[left].LastObservedAt.After(*eligible[right].LastObservedAt)
		}
		return eligible[left].ID < eligible[right].ID
	})
	for _, indicator := range eligible {
		if len(selected) >= limit {
			break
		}
		if _, exists := selectedIDs[indicator.ID]; exists {
			continue
		}
		if _, exists := selectedSemantics[indicator.SemanticKey]; exists {
			continue
		}
		selected = append(selected, indicator)
		selectedIDs[indicator.ID] = struct{}{}
		selectedSemantics[indicator.SemanticKey] = struct{}{}
	}
	return selected
}

func bestOverviewIndicator(indicators []Indicator, preference overviewPreference) *Indicator {
	var best *Indicator
	for index := range indicators {
		candidate := &indicators[index]
		if candidate.SemanticKey != preference.semanticKey || candidate.LastValue == nil {
			continue
		}
		if best == nil || overviewValueComesFirst(candidate, best, preference.ascending) {
			best = candidate
		}
	}
	return best
}

func overviewValueComesFirst(candidate, current *Indicator, ascending bool) bool {
	if *candidate.LastValue != *current.LastValue {
		if ascending {
			return *candidate.LastValue < *current.LastValue
		}
		return *candidate.LastValue > *current.LastValue
	}
	if !candidate.LastObservedAt.Equal(*current.LastObservedAt) {
		return candidate.LastObservedAt.After(*current.LastObservedAt)
	}
	return candidate.ID < current.ID
}
