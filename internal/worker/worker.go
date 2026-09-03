package worker

import (
	"context"
	"log/slog"
	"reflect"
	"time"

	"github.com/M0okz/cairnops/internal/checks"
	"github.com/M0okz/cairnops/internal/metrics"
	"github.com/M0okz/cairnops/internal/monitoring"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Runner interface {
	Run(context.Context) error
}

type Worker struct {
	pool       *pgxpool.Pool
	instanceID string
	logger     *slog.Logger
	interval   time.Duration
	scheduler  *monitoring.Scheduler
	rollup     *metrics.Rollup
	critical   []Runner
	supervised []Runner
}

func New(pool *pgxpool.Pool, instanceID string, logger *slog.Logger, runners ...Runner) *Worker {
	store := monitoring.NewPostgresStore(pool)
	return &Worker{
		pool: pool, instanceID: instanceID, logger: logger, interval: 15 * time.Second,
		scheduler: monitoring.NewScheduler(store, checks.NewRegistry(), instanceID, logger),
		rollup:    metrics.NewRollup(pool, logger), critical: runners,
	}
}

// WithSupervisedRunners adds product-facing loops whose failure must stay
// isolated from the worker's native scheduler and delivery services.
func (w *Worker) WithSupervisedRunners(runners ...Runner) *Worker {
	w.supervised = append(w.supervised, runners...)
	return w
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.heartbeat(ctx); err != nil {
		return err
	}
	w.logger.Info("worker started", "instance_id", w.instanceID)

	errors := make(chan error, 3+len(w.critical))
	go func() { errors <- w.heartbeatLoop(ctx) }()
	go func() { errors <- w.scheduler.Run(ctx) }()
	go func() { errors <- w.rollup.Run(ctx) }()
	for _, runner := range w.critical {
		if runner != nil {
			go func(runner Runner) { errors <- runner.Run(ctx) }(runner)
		}
	}
	for _, runner := range w.supervised {
		if runner != nil {
			go superviseRunner(ctx, w.logger, runner, time.Second, time.Minute)
		}
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errors:
		return err
	}
}

// superviseRunner confines a continuous product capability to its own restart
// loop. A temporary failure in one connector adapter must not stop the native
// scheduler, notifications, or the other connector products.
func superviseRunner(ctx context.Context, logger *slog.Logger, runner Runner, initialDelay, maximumDelay time.Duration) {
	if logger == nil {
		logger = slog.Default()
	}
	if initialDelay <= 0 {
		initialDelay = time.Second
	}
	if maximumDelay < initialDelay {
		maximumDelay = initialDelay
	}
	runnerName := reflect.TypeOf(runner).String()
	delay := initialDelay
	for {
		err := runner.Run(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Error("background runner failed; retrying", "runner", runnerName, "error", err, "retry_in", delay)
		} else {
			logger.Warn("background runner stopped unexpectedly; retrying", "runner", runnerName, "retry_in", delay)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if delay < maximumDelay {
			delay *= 2
			if delay > maximumDelay {
				delay = maximumDelay
			}
		}
	}
}

func (w *Worker) heartbeatLoop(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.heartbeat(ctx); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) heartbeat(ctx context.Context) error {
	_, err := w.pool.Exec(ctx, `
		INSERT INTO cairnops_component_heartbeats (component, instance_id, last_seen_at)
		VALUES ('worker', $1, now())
		ON CONFLICT (component, instance_id)
		DO UPDATE SET last_seen_at = excluded.last_seen_at
	`, w.instanceID)
	return err
}
