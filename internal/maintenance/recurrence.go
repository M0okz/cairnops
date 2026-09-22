package maintenance

import (
	"fmt"
	"time"
	_ "time/tzdata" // Keep IANA rules available in minimal deployment images.
)

// Recurrence fixes the weekday and local start time of the first window.
// Until is an inclusive local date. Every occurrence is materialized at creation.
type Recurrence struct {
	Frequency string `json:"frequency"`
	Timezone  string `json:"timezone"`
	Until     string `json:"until"`
}

type occurrence struct{ startsAt, endsAt time.Time }

func occurrences(input CreateInput) ([]occurrence, error) {
	items := []occurrence{{input.StartsAt, input.EndsAt}}
	if input.Recurrence == nil {
		return items, nil
	}
	rule := input.Recurrence
	if rule.Frequency != "weekly" {
		return nil, fmt.Errorf("%w: seule la récurrence hebdomadaire est disponible", ErrInvalidInput)
	}
	if rule.Timezone == "" || rule.Timezone == "Local" {
		return nil, fmt.Errorf("%w: fuseau IANA requis", ErrInvalidInput)
	}
	zone, err := time.LoadLocation(rule.Timezone)
	if err != nil {
		return nil, fmt.Errorf("%w: fuseau IANA invalide", ErrInvalidInput)
	}
	last, err := time.Parse("2006-01-02", rule.Until)
	if err != nil {
		return nil, fmt.Errorf("%w: date de fin de récurrence requise (AAAA-MM-JJ)", ErrInvalidInput)
	}
	local := input.StartsAt.In(zone)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if last.Before(date.AddDate(0, 0, 7)) || last.After(date.AddDate(1, 0, 0)) {
		return nil, fmt.Errorf("%w: la récurrence doit se terminer entre une semaine et un an après son début", ErrInvalidInput)
	}
	duration := input.EndsAt.Sub(input.StartsAt)
	if duration <= 0 || duration > 24*time.Hour {
		return nil, fmt.Errorf("%w: chaque fenêtre récurrente doit durer au maximum 24 heures", ErrInvalidInput)
	}
	for day := date.AddDate(0, 0, 7); !day.After(last); day = day.AddDate(0, 0, 7) {
		start, exists := localTime(day.Year(), day.Month(), day.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), zone)
		if !exists {
			continue
		} // Spring-forward gaps have no occurrence.
		items = append(items, occurrence{start.UTC(), start.Add(duration).UTC()})
	}
	if len(items) < 2 {
		return nil, fmt.Errorf("%w: la période doit contenir au moins deux fenêtres", ErrInvalidInput)
	}
	return items, nil
}

// Resolve a civil time explicitly: skip gaps and choose the first instant in a
// fold. Looking at adjacent days discovers both UTC offsets around a transition,
// including half-hour DST changes without assuming a one-hour shift.
func localTime(year int, month time.Month, day, hour, minute, second, nano int, zone *time.Location) (time.Time, bool) {
	civil := time.Date(year, month, day, hour, minute, second, nano, time.UTC)
	offsets := map[int]struct{}{}
	for _, delta := range []time.Duration{-48 * time.Hour, -24 * time.Hour, 0, 24 * time.Hour, 48 * time.Hour} {
		_, offset := civil.Add(delta).In(zone).Zone()
		offsets[offset] = struct{}{}
	}
	var first time.Time
	for offset := range offsets {
		candidate := civil.Add(-time.Duration(offset) * time.Second)
		projected := candidate.In(zone)
		if projected.Year() == year && projected.Month() == month && projected.Day() == day && projected.Hour() == hour && projected.Minute() == minute && projected.Second() == second {
			if first.IsZero() || candidate.Before(first) {
				first = candidate
			}
		}
	}
	return first, !first.IsZero()
}
