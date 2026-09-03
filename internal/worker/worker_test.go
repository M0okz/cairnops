package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type retryingRunner struct {
	calls atomic.Int32
}

func (runner *retryingRunner) Run(ctx context.Context) error {
	if runner.calls.Add(1) == 1 {
		return errors.New("temporary product failure")
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSuperviseRunnerRestartsAnIsolatedFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	runner := &retryingRunner{}
	done := make(chan struct{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		superviseRunner(ctx, logger, runner, time.Millisecond, 2*time.Millisecond)
		close(done)
	}()

	deadline := time.After(time.Second)
	for runner.calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("runner was not restarted after its isolated failure")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runner supervisor did not stop with its context")
	}
}
