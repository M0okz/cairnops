package maintenance

import (
	"errors"
	"testing"
	"time"
)

func TestWeeklyOccurrencesKeepLocalTimeAcrossDST(t *testing.T) {
	cases := []struct {
		name, start, until, second string
		count                      int
	}{
		{"spring", "2026-03-22T03:00:00+01:00", "2026-04-05", "2026-03-29T01:00:00Z", 3},
		{"autumn", "2026-10-18T03:00:00+02:00", "2026-11-01", "2026-10-25T02:00:00Z", 3},
		{"gap skipped", "2026-03-22T02:30:00+01:00", "2026-04-05", "2026-04-05T00:30:00Z", 2},
		{"fold first instant", "2026-10-18T02:30:00+02:00", "2026-11-01", "2026-10-25T00:30:00Z", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, _ := time.Parse(time.RFC3339, tc.start)
			items, err := occurrences(CreateInput{StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: &Recurrence{Frequency: "weekly", Timezone: "Europe/Paris", Until: tc.until}})
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != tc.count || items[1].startsAt.Format(time.RFC3339) != tc.second {
				t.Fatalf("unexpected occurrences: %+v", items)
			}
			for _, item := range items {
				if item.endsAt.Sub(item.startsAt) != time.Hour {
					t.Fatal("duration changed across DST")
				}
			}
		})
	}
}

func TestRecurrenceBoundsAndTimezone(t *testing.T) {
	start := time.Date(2026, 1, 4, 3, 0, 0, 0, time.UTC)
	for _, rule := range []Recurrence{
		{Frequency: "daily", Timezone: "UTC", Until: "2026-02-01"},
		{Frequency: "weekly", Timezone: "invalid/zone", Until: "2026-02-01"},
		{Frequency: "weekly", Timezone: "Local", Until: "2026-02-01"},
		{Frequency: "weekly", Timezone: "UTC", Until: ""},
		{Frequency: "weekly", Timezone: "UTC", Until: "2026-01-04"},
		{Frequency: "weekly", Timezone: "UTC", Until: "2027-01-05"},
	} {
		_, err := occurrences(CreateInput{StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: &rule})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid recurrence %+v, got %v", rule, err)
		}
	}
	items, err := occurrences(CreateInput{StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: &Recurrence{Frequency: "weekly", Timezone: "UTC", Until: "2027-01-04"}})
	if err != nil || len(items) != 53 {
		t.Fatalf("expected 53 bounded windows, got %d, %v", len(items), err)
	}
}

func TestLocalTimeSupportsHalfHourDST(t *testing.T) {
	zone, err := time.LoadLocation("Australia/Lord_Howe")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := localTime(2026, time.October, 4, 2, 15, 0, 0, zone); ok {
		t.Fatal("expected missing half-hour")
	}
	got, ok := localTime(2026, time.April, 5, 1, 45, 0, 0, zone)
	if !ok || got.Format(time.RFC3339) != "2026-04-04T14:45:00Z" {
		t.Fatalf("unexpected fold %s", got)
	}
}
