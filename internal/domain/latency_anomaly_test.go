package domain

import (
	"testing"
	"time"
)

func TestDetectLatencyAnomaliesKeepsTrainingAndEvaluationSeparate(t *testing.T) {
	t.Parallel()
	trainingEnd := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	profiles := map[string]LatencyProfile{
		"native": {
			WindowEnd: trainingEnd,
			Buckets: []LatencyBucket{
				{Hour: 13, Samples: 40, Median: 100 * time.Millisecond, P99: 160 * time.Millisecond},
				{Hour: AllHours, Samples: 300, Median: 800 * time.Millisecond, P99: 1200 * time.Millisecond},
			},
		},
	}
	observations := []LatencyObservation{
		{ID: "training", SourceID: "native", ObservedAt: trainingEnd.Add(-time.Minute), Latency: 3 * time.Second},
		{ID: "expected", SourceID: "native", ObservedAt: trainingEnd.Add(time.Hour), Latency: 250 * time.Millisecond},
		{ID: "ordinary", SourceID: "native", ObservedAt: trainingEnd.Add(time.Hour), Latency: 180 * time.Millisecond},
		{ID: "other-source", SourceID: "other", ObservedAt: trainingEnd.Add(time.Hour), Latency: 3 * time.Second},
	}
	anomalies := DetectLatencyAnomalies(observations, profiles)
	if len(anomalies) != 1 || anomalies[0].Observation.ID != "expected" {
		t.Fatalf("expected only the held-out slow observation, got %#v", anomalies)
	}
	if anomalies[0].Threshold != 200*time.Millisecond || anomalies[0].Hour != 13 {
		t.Fatalf("expected the hour-specific learned baseline, got %#v", anomalies[0])
	}
}

func TestDetectLatencyAnomaliesDoesNotInventBaseline(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)
	profiles := map[string]LatencyProfile{
		"native": {
			WindowEnd: at.Add(-time.Hour),
			Buckets:   []LatencyBucket{{Hour: 13, Samples: 29, Median: 100 * time.Millisecond, P99: 150 * time.Millisecond}},
		},
	}
	observations := []LatencyObservation{{ID: "slow", SourceID: "native", ObservedAt: at, Latency: time.Second}}
	if anomalies := DetectLatencyAnomalies(observations, profiles); len(anomalies) != 0 {
		t.Fatalf("a thin baseline must not produce candidates, got %#v", anomalies)
	}
}
