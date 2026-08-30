package indicators

import (
	"testing"
	"time"
)

func TestSelectOverviewIndicatorsSuggestsUsefulContextWithoutPins(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	value := func(number float64) *float64 { return &number }

	indicators := []Indicator{
		{ID: "disk-secondary", SemanticKey: "filesystem.utilization", LastValue: value(67), LastObservedAt: &observedAt},
		{ID: "disk-full", SemanticKey: "filesystem.utilization", LastValue: value(82.4), LastObservedAt: &observedAt},
		{ID: "memory-hot", SemanticKey: "memory.utilization", LastValue: value(91.2), LastObservedAt: &observedAt},
		{ID: "response-slow", SemanticKey: "response.time", LastValue: value(418), LastObservedAt: &observedAt},
		{ID: "certificate-soon", SemanticKey: "certificate.days_remaining", LastValue: value(12), LastObservedAt: &observedAt},
		{ID: "certificate-later", SemanticKey: "certificate.days_remaining", LastValue: value(35), LastObservedAt: &observedAt},
		{ID: "unobserved-cpu", SemanticKey: "cpu.utilization", LastValue: value(99)},
	}

	selected := selectOverviewIndicators(indicators, 4)
	if len(selected) != 4 {
		t.Fatalf("expected four suggested indicators, got %#v", selected)
	}
	want := []string{"disk-full", "memory-hot", "response-slow", "certificate-soon"}
	for index, indicator := range selected {
		if indicator.ID != want[index] {
			t.Fatalf("suggestion %d = %q, want %q", index, indicator.ID, want[index])
		}
	}
}

func TestSelectOverviewIndicatorsKeepsPersonalPinsFirst(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	value := func(number float64) *float64 { return &number }
	first, second := 0, 1

	indicators := []Indicator{
		{ID: "memory-pin", SemanticKey: "memory.utilization", LastValue: value(42), LastObservedAt: &observedAt, Pinned: true, PinPosition: &second},
		{ID: "disk-full", SemanticKey: "filesystem.utilization", LastValue: value(82), LastObservedAt: &observedAt},
		{ID: "response-pin", SemanticKey: "response.time", LastValue: value(120), LastObservedAt: &observedAt, Pinned: true, PinPosition: &first},
		{ID: "certificate-soon", SemanticKey: "certificate.days_remaining", LastValue: value(12), LastObservedAt: &observedAt},
	}

	selected := selectOverviewIndicators(indicators, 4)
	want := []string{"response-pin", "memory-pin", "disk-full", "certificate-soon"}
	for index, indicator := range selected {
		if indicator.ID != want[index] {
			t.Fatalf("selection %d = %q, want %q", index, indicator.ID, want[index])
		}
	}
}
