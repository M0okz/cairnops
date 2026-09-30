package httpapi

import (
	"context"
	"sort"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

// LatencyProfiles rend les Profils de latence des Sources d'une Cible. Une
// Source sans Profil calculé n'y figure pas : rien n'est inventé en attendant
// la première passe du worker.
type LatencyProfiles interface {
	Profiles(context.Context, string) (map[string]domain.LatencyProfile, error)
}

type LatencyObservations interface {
	RecentHealthyObservations(context.Context, string, time.Time, int) ([]domain.LatencyObservation, bool, error)
}

type latencyAnomalyView struct {
	ObservationID         string    `json:"observation_id"`
	SourceID              string    `json:"source_id"`
	ObservedAt            time.Time `json:"observed_at"`
	LatencyMilliseconds   int64     `json:"latency_milliseconds"`
	MedianMilliseconds    int64     `json:"median_milliseconds"`
	ThresholdMilliseconds int64     `json:"threshold_milliseconds"`
	ProfileHour           int       `json:"profile_hour"`
	ProfileSamples        int       `json:"profile_samples"`
}

type latencyEvaluationView struct {
	WindowStart         time.Time            `json:"window_start"`
	WindowEnd           time.Time            `json:"window_end"`
	TrainedSources      int                  `json:"trained_sources"`
	ScannedObservations int                  `json:"scanned_observations"`
	Truncated           bool                 `json:"truncated"`
	Available           bool                 `json:"available"`
	Candidates          []latencyAnomalyView `json:"candidates"`
}

func newLatencyAnomalyView(anomaly domain.LatencyAnomaly) latencyAnomalyView {
	return latencyAnomalyView{
		ObservationID:         anomaly.Observation.ID,
		SourceID:              anomaly.Observation.SourceID,
		ObservedAt:            anomaly.Observation.ObservedAt,
		LatencyMilliseconds:   anomaly.Observation.Latency.Milliseconds(),
		MedianMilliseconds:    anomaly.Median.Milliseconds(),
		ThresholdMilliseconds: anomaly.Threshold.Milliseconds(),
		ProfileHour:           anomaly.Hour,
		ProfileSamples:        anomaly.Samples,
	}
}

// latencyProfileView présente un Profil de latence. Les latences s'y lisent en
// millisecondes, comme partout ailleurs dans l'API.
type latencyProfileView struct {
	WindowStart time.Time           `json:"window_start"`
	WindowEnd   time.Time           `json:"window_end"`
	Samples     int                 `json:"samples"`
	ComputedAt  time.Time           `json:"computed_at"`
	Hours       []latencyBucketView `json:"hours"`
}

// latencyBucketView présente une heure du Profil.
//
// ThresholdMilliseconds reste absent lorsque le seau ne réunit pas assez
// d'Observations : c'est ainsi qu'une heure sur laquelle le Profil ne conclut
// rien se distingue d'une heure tolérante. Aucun champ n'exprime un score.
type latencyBucketView struct {
	Hour                  int    `json:"hour"`
	Samples               int    `json:"samples"`
	MedianMilliseconds    int64  `json:"median_milliseconds"`
	P95Milliseconds       int64  `json:"p95_milliseconds"`
	P99Milliseconds       int64  `json:"p99_milliseconds"`
	ThresholdMilliseconds *int64 `json:"threshold_milliseconds"`
}

func newLatencyProfileView(profile domain.LatencyProfile) latencyProfileView {
	view := latencyProfileView{
		WindowStart: profile.WindowStart,
		WindowEnd:   profile.WindowEnd,
		Samples:     profile.Samples,
		ComputedAt:  profile.ComputedAt,
		Hours:       make([]latencyBucketView, 0, len(profile.Buckets)),
	}
	for _, bucket := range profile.Buckets {
		hour := latencyBucketView{
			Hour:               bucket.Hour,
			Samples:            bucket.Samples,
			MedianMilliseconds: bucket.Median.Milliseconds(),
			P95Milliseconds:    bucket.P95.Milliseconds(),
			P99Milliseconds:    bucket.P99.Milliseconds(),
		}
		if threshold, established := bucket.Threshold(); established {
			milliseconds := threshold.Milliseconds()
			hour.ThresholdMilliseconds = &milliseconds
		}
		view.Hours = append(view.Hours, hour)
	}
	sort.Slice(view.Hours, func(left, right int) bool {
		return view.Hours[left].Hour < view.Hours[right].Hour
	})
	return view
}
