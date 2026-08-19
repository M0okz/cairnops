package realtime

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	Version    int64     `json:"version"`
	Kind       string    `json:"kind"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (store *Store) LatestVersion(ctx context.Context) (int64, error) {
	var version int64
	if err := store.pool.QueryRow(ctx, "SELECT COALESCE(max(version), 0) FROM cairnops_events").Scan(&version); err != nil {
		return 0, fmt.Errorf("read latest event version: %w", err)
	}
	return version, nil
}

func (store *Store) ListAfter(ctx context.Context, version int64, limit int) ([]Event, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT version, kind, entity_type, entity_id, occurred_at
		FROM cairnops_events
		WHERE version > $1
		ORDER BY version
		LIMIT $2
	`, version, limit)
	if err != nil {
		return nil, fmt.Errorf("list events after version %d: %w", version, err)
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.Version, &event.Kind, &event.EntityType, &event.EntityID, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}
