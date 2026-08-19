package domain

import "testing"

func TestMeasureSeparatesAvailabilityFromCoverage(t *testing.T) {
	// Neuf Observations saines sur dix concluantes, mais vingt attendues :
	// la Cible paraît disponible et n'a été observée que la moitié du temps.
	counters := Counters{
		Healthy: 9, Unhealthy: 1, Unknown: 4, Expected: 20,
		LatencySumMilliseconds: 900, LatencyCount: 9, LatencyMaximum: 400,
	}
	measure := counters.Measure(WindowDay)

	if measure.Availability == nil || *measure.Availability != 0.9 {
		t.Fatalf("expected an availability of 0.9, got %v", measure.Availability)
	}
	if measure.Coverage == nil || *measure.Coverage != 0.5 {
		t.Fatalf("expected a coverage of 0.5, got %v", measure.Coverage)
	}
	if measure.AverageLatencyMilliseconds == nil || *measure.AverageLatencyMilliseconds != 100 {
		t.Fatalf("expected an average latency of 100 ms, got %v", measure.AverageLatencyMilliseconds)
	}
	if measure.MaximumLatencyMilliseconds == nil || *measure.MaximumLatencyMilliseconds != 400 {
		t.Fatalf("expected a maximum latency of 400 ms, got %v", measure.MaximumLatencyMilliseconds)
	}
	if measure.UnknownObservations != 4 || measure.ConclusiveObservations != 10 {
		t.Fatalf("unexpected observation counts: %#v", measure)
	}
}

// Une Observation Inconnue ne conclut rien : elle ne peut donc ni établir ni
// dégrader la Disponibilité, seulement la Couverture.
func TestMeasureIgnoresUnknownObservationsForAvailability(t *testing.T) {
	measure := Counters{Unknown: 12, Expected: 12}.Measure(WindowDay)
	if measure.Availability != nil {
		t.Fatalf("unknown observations must not establish an availability: %v", *measure.Availability)
	}
	if measure.Coverage == nil || *measure.Coverage != 0 {
		t.Fatalf("expected a coverage of 0, got %v", measure.Coverage)
	}
	if measure.AverageLatencyMilliseconds != nil {
		t.Fatal("no healthy observation can produce a latency")
	}
}

// Sans Observation attendue — Source créée à l'instant, ou suspendue sur toute
// la fenêtre — la Couverture reste absente plutôt que nulle.
func TestMeasureLeavesCoverageAbsentWithoutExpectation(t *testing.T) {
	measure := Counters{Healthy: 3, Expected: 0}.Measure(WindowWeek)
	if measure.Coverage != nil {
		t.Fatalf("expected an absent coverage, got %v", *measure.Coverage)
	}
	if measure.Availability == nil || *measure.Availability != 1 {
		t.Fatalf("expected a full availability, got %v", measure.Availability)
	}
	if measure.Window != WindowWeek {
		t.Fatalf("the measure must carry its window, got %q", measure.Window)
	}
}

// Une Source en avance sur sa cadence ne prouve pas plus que la totalité.
func TestMeasureCapsCoverage(t *testing.T) {
	measure := Counters{Healthy: 190, Expected: 180}.Measure(WindowDay)
	if measure.Coverage == nil || *measure.Coverage != 1 {
		t.Fatalf("expected a capped coverage of 1, got %v", measure.Coverage)
	}
}

// Additionner deux Sources somme les preuves et retient la pire latence.
func TestCountersAdd(t *testing.T) {
	total := Counters{Healthy: 2, Expected: 3, LatencySumMilliseconds: 40, LatencyCount: 2, LatencyMaximum: 30}.
		Add(Counters{Unhealthy: 1, Expected: 3, LatencySumMilliseconds: 0, LatencyMaximum: 12})

	if total.Healthy != 2 || total.Unhealthy != 1 || total.Expected != 6 {
		t.Fatalf("unexpected sum: %#v", total)
	}
	if total.LatencyMaximum != 30 {
		t.Fatalf("expected the worst latency to survive, got %d", total.LatencyMaximum)
	}
	if total.Conclusive() != 3 {
		t.Fatalf("expected three conclusive observations, got %d", total.Conclusive())
	}
}

func TestWindowHours(t *testing.T) {
	for window, hours := range map[Window]int{WindowDay: 24, WindowWeek: 168, WindowMonth: 720} {
		if window.Hours() != hours {
			t.Fatalf("window %q should span %d hours, got %d", window, hours, window.Hours())
		}
		if !window.Valid() {
			t.Fatalf("window %q should be valid", window)
		}
	}
	if Window("12h").Valid() {
		t.Fatal("an unsupported window must not validate")
	}
}
