package monitoring

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

type Checker interface {
	Check(context.Context, domain.Source) domain.Observation
}

type Scheduler struct {
	store        Store
	checker      Checker
	owner        string
	logger       *slog.Logger
	pollInterval time.Duration
	lease        time.Duration
	batchSize    int
	parallelism  int
}

func NewScheduler(store Store, checker Checker, owner string, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		store: store, checker: checker, owner: owner, logger: logger,
		pollInterval: time.Second, lease: 2 * time.Minute, batchSize: 32, parallelism: 8,
	}
}

func (scheduler *Scheduler) Run(ctx context.Context) error {
	if err := scheduler.tick(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(scheduler.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := scheduler.tick(ctx); err != nil {
				return err
			}
		}
	}
}

func (scheduler *Scheduler) tick(ctx context.Context) error {
	sources, err := scheduler.store.ClaimDue(ctx, scheduler.owner, scheduler.batchSize, scheduler.lease)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}

	semaphore := make(chan struct{}, scheduler.parallelism)
	errors := make(chan error, len(sources))
	var waitGroup sync.WaitGroup
	for _, source := range sources {
		source := source
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			checkCtx, cancel := context.WithTimeout(ctx, source.Timeout)
			observation := scheduler.checker.Check(checkCtx, source)
			cancel()
			if err := scheduler.store.Complete(ctx, scheduler.owner, source, observation); err != nil {
				errors <- err
				return
			}
			scheduler.logger.Info("check completed",
				"source_id", source.ID,
				"kind", source.Kind,
				"outcome", observation.Outcome,
				"latency_ms", observation.Latency.Milliseconds(),
			)
		}()
	}
	waitGroup.Wait()
	close(errors)
	for completionError := range errors {
		return fmt.Errorf("persist observation: %w", completionError)
	}
	return nil
}
