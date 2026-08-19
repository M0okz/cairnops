package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	MinimumInterval = 20 * time.Second
	MaximumInterval = 24 * time.Hour
)

type SourceKind string

const (
	SourceHTTP      SourceKind = "http"
	SourceTCP       SourceKind = "tcp"
	SourceDNS       SourceKind = "dns"
	SourceICMP      SourceKind = "icmp"
	SourceHeartbeat SourceKind = "heartbeat"
)

func (kind SourceKind) Valid() bool {
	switch kind {
	case SourceHTTP, SourceTCP, SourceDNS, SourceICMP, SourceHeartbeat:
		return true
	default:
		return false
	}
}

type Source struct {
	ID                string
	TargetID          string
	Name              string
	Kind              SourceKind
	Interval          time.Duration
	Timeout           time.Duration
	Config            json.RawMessage
	LastSignalAt      *time.Time
	LastSignalOutcome *Outcome
	LastObservedAt    *time.Time
}

func (source Source) Validate() error {
	if source.ID == "" {
		return fmt.Errorf("source ID is required")
	}
	if source.TargetID == "" {
		return fmt.Errorf("target ID is required")
	}
	if !source.Kind.Valid() {
		return fmt.Errorf("unsupported source kind %q", source.Kind)
	}
	if source.Interval < MinimumInterval || source.Interval > MaximumInterval {
		return fmt.Errorf("interval must be between %s and %s", MinimumInterval, MaximumInterval)
	}
	if source.Timeout <= 0 || source.Timeout > source.Interval {
		return fmt.Errorf("timeout must be positive and no longer than interval")
	}
	if !json.Valid(source.Config) {
		return fmt.Errorf("config must be valid JSON")
	}
	return nil
}

type Outcome string

const (
	OutcomeHealthy   Outcome = "healthy"
	OutcomeUnhealthy Outcome = "unhealthy"
	OutcomeUnknown   Outcome = "unknown"
)

type Observation struct {
	SourceID   string
	TargetID   string
	ObservedAt time.Time
	Outcome    Outcome
	Latency    time.Duration
	Reason     string
	Message    string
	Details    map[string]any
}

func NewObservation(source Source, outcome Outcome, startedAt time.Time) Observation {
	return Observation{
		SourceID:   source.ID,
		TargetID:   source.TargetID,
		ObservedAt: time.Now().UTC(),
		Outcome:    outcome,
		Latency:    time.Since(startedAt),
		Details:    make(map[string]any),
	}
}
