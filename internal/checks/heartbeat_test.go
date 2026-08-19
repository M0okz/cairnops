package checks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

func TestHeartbeatWaitsForActivation(t *testing.T) {
	t.Parallel()

	config, _ := json.Marshal(HeartbeatConfig{ExpectedEverySeconds: 60})
	result := (Heartbeat{}).Check(context.Background(), testSource(domain.SourceHeartbeat, config))
	if result.Outcome != domain.OutcomeUnknown || result.Reason != "heartbeat_waiting" {
		t.Fatalf("expected waiting observation, got %#v", result)
	}
}

func TestHeartbeatBecomesUnhealthyAfterGrace(t *testing.T) {
	t.Parallel()

	lastSignal := time.Now().Add(-2 * time.Minute)
	config, _ := json.Marshal(HeartbeatConfig{ExpectedEverySeconds: 60, GraceSeconds: 10, Activated: true})
	source := testSource(domain.SourceHeartbeat, config)
	source.LastSignalAt = &lastSignal
	result := (Heartbeat{}).Check(context.Background(), source)
	if result.Outcome != domain.OutcomeUnhealthy || result.Reason != "heartbeat_overdue" {
		t.Fatalf("expected overdue observation, got %#v", result)
	}
}

func TestHeartbeatKeepsLatestReportedFailure(t *testing.T) {
	t.Parallel()

	lastSignal := time.Now()
	failure := domain.OutcomeUnhealthy
	config, _ := json.Marshal(HeartbeatConfig{ExpectedEverySeconds: 60, GraceSeconds: 10, Activated: true})
	source := testSource(domain.SourceHeartbeat, config)
	source.LastSignalAt = &lastSignal
	source.LastSignalOutcome = &failure
	result := (Heartbeat{}).Check(context.Background(), source)
	if result.Outcome != domain.OutcomeUnhealthy || result.Reason != "heartbeat_reported_failure" {
		t.Fatalf("expected reported failure to persist, got %#v", result)
	}
}
