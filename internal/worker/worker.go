package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/M0okz/cairnops/internal/checks"
	"github.com/M0okz/cairnops/internal/metrics"
	"github.com/M0okz/cairnops/internal/monitoring"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	pool                   *pgxpool.Pool
	instanceID             string
	logger                 *slog.Logger
	interval               time.Duration
	scheduler              *monitoring.Scheduler
	rollup                 *metrics.Rollup
	notificationDispatcher interface{ Run(context.Context) error }
}

func New(pool *pgxpool.Pool, instanceID string, logger *slog.Logger, notificationDispatcher interface{ Run(context.Context) error }) *Worker {
	store := monitoring.NewPostgresStore(pool)
	return &Worker{
		pool: pool, instanceID: instanceID, logger: logger, interval: 15 * time.Second,
		scheduler: monitoring.NewScheduler(store, checks.NewRegistry(), instanceID, logger),
		rollup:    metrics.NewRollup(pool, logger), notificationDispatcher: notificationDispatcher,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.heartbeat(ctx); err != nil {
		return err
	}
	w.logger.Info("worker started", "instance_id", w.instanceID)

	errors := make(chan error, 4)
	go func() { errors <- w.heartbeatLoop(ctx) }()
	go func() { errors <- w.scheduler.Run(ctx) }()
	go func() { errors <- w.rollup.Run(ctx) }()
	if w.notificationDispatcher != nil {
		go func() { errors <- w.notificationDispatcher.Run(ctx) }()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errors:
		return err
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
