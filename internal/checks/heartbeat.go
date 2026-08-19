package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

type Heartbeat struct{}

type HeartbeatConfig struct {
	ExpectedEverySeconds int  `json:"expected_every_seconds"`
	GraceSeconds         int  `json:"grace_seconds,omitempty"`
	Activated            bool `json:"activated"`
}

func (Heartbeat) Check(_ context.Context, source domain.Source) domain.Observation {
	startedAt := time.Now()
	var config HeartbeatConfig
	if err := decodeConfig(source.Config, &config); err != nil {
		return unknown(source, startedAt, "invalid_config", err)
	}
	if config.ExpectedEverySeconds < int(domain.MinimumInterval.Seconds()) || config.ExpectedEverySeconds > int(domain.MaximumInterval.Seconds()) {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("expected interval must be between 20 and 86400 seconds"))
	}
	if config.GraceSeconds < 0 || config.GraceSeconds > int(domain.MaximumInterval.Seconds()) {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("grace must be between 0 and 86400 seconds"))
	}
	if !config.Activated || source.LastSignalAt == nil {
		return unknown(source, startedAt, "heartbeat_waiting", fmt.Errorf("heartbeat is waiting for its first signal"))
	}

	deadline := source.LastSignalAt.Add(time.Duration(config.ExpectedEverySeconds+config.GraceSeconds) * time.Second)
	observation := healthy(source, startedAt)
	observation.Details["last_signal_at"] = source.LastSignalAt.UTC()
	observation.Details["deadline"] = deadline.UTC()
	if time.Now().After(deadline) {
		observation.Outcome = domain.OutcomeUnhealthy
		observation.Reason = "heartbeat_overdue"
		observation.Message = fmt.Sprintf("heartbeat was due at %s", deadline.UTC().Format(time.RFC3339))
	} else if source.LastSignalOutcome != nil && *source.LastSignalOutcome == domain.OutcomeUnhealthy {
		observation.Outcome = domain.OutcomeUnhealthy
		observation.Reason = "heartbeat_reported_failure"
		observation.Message = "the latest heartbeat reported a failure"
	}
	return observation
}
