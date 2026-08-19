package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSourceValidateIntervalBoundaries(t *testing.T) {
	t.Parallel()

	for _, interval := range []time.Duration{MinimumInterval, MaximumInterval} {
		source := Source{
			ID: "source", TargetID: "target", Kind: SourceHTTP,
			Interval: interval, Timeout: time.Second, Config: json.RawMessage(`{}`),
		}
		if err := source.Validate(); err != nil {
			t.Fatalf("expected interval %s to be valid: %v", interval, err)
		}
	}
}

func TestSourceValidateRejectsUnsupportedKind(t *testing.T) {
	t.Parallel()

	source := Source{
		ID: "source", TargetID: "target", Kind: "script",
		Interval: time.Minute, Timeout: time.Second, Config: json.RawMessage(`{}`),
	}
	if err := source.Validate(); err == nil {
		t.Fatal("expected unsupported kind to be rejected")
	}
}
