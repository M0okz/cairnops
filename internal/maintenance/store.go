package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (store *PostgresStore) List(ctx context.Context, limit int) ([]Maintenance, error) {
	rows, err := store.pool.Query(ctx, maintenanceSelect+`
		WHERE maintenance.ends_at >= now() - interval '24 hours'
		ORDER BY
		  CASE WHEN maintenance.cancelled_at IS NULL AND now() BETWEEN maintenance.starts_at AND maintenance.ends_at THEN 0
		       WHEN maintenance.cancelled_at IS NULL AND maintenance.starts_at > now() THEN 1 ELSE 2 END,
		  maintenance.starts_at, maintenance.id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list maintenances: %w", err)
	}
	defer rows.Close()
	items := make([]Maintenance, 0)
	for rows.Next() {
		item, err := scanMaintenance(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate maintenances: %w", err)
	}
	return items, nil
}

func (store *PostgresStore) Create(ctx context.Context, actorID string, input CreateInput) (Maintenance, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Maintenance{}, fmt.Errorf("begin maintenance creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	windows, err := occurrences(input)
	if err != nil {
		return Maintenance{}, err
	}
	var seriesID *string
	if input.Recurrence != nil {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO cairnops_maintenance_series (timezone, until_date) VALUES ($1, $2::date) RETURNING id::text`, input.Recurrence.Timezone, input.Recurrence.Until).Scan(&id); err != nil {
			return Maintenance{}, fmt.Errorf("create maintenance series: %w", err)
		}
		seriesID = &id
	}
	var maintenanceID string
	for _, window := range windows {
		var id string
		if err := tx.QueryRow(ctx, `
   INSERT INTO cairnops_maintenances (name, reason, starts_at, ends_at, created_by, series_id)
   VALUES ($1, $2, $3, $4, $5::uuid, $6::uuid) RETURNING id::text
  `, input.Name, input.Reason, window.startsAt, window.endsAt, actorID, seriesID).Scan(&id); err != nil {
			return Maintenance{}, fmt.Errorf("create maintenance: %w", err)
		}
		if maintenanceID == "" {
			maintenanceID = id
		}
		result, err := tx.Exec(ctx, `
   INSERT INTO cairnops_maintenance_targets (maintenance_id, target_id)
   SELECT $1::uuid, id FROM cairnops_targets WHERE id = ANY($2::uuid[])
  `, id, input.TargetIDs)
		if err != nil {
			return Maintenance{}, fmt.Errorf("attach maintenance targets: %w", err)
		}
		if result.RowsAffected() != int64(len(input.TargetIDs)) {
			return Maintenance{}, fmt.Errorf("%w: une ressource sélectionnée est introuvable", ErrInvalidInput)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Maintenance{}, fmt.Errorf("commit maintenance creation: %w", err)
	}
	return store.get(ctx, maintenanceID)
}

func (store *PostgresStore) Cancel(ctx context.Context, maintenanceID, actorID string) (Maintenance, error) {
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_maintenances
		SET cancelled_at = now(), cancelled_by = $2::uuid
		WHERE id = $1::uuid AND cancelled_at IS NULL AND ends_at > now()
	`, maintenanceID, actorID)
	if err != nil {
		return Maintenance{}, fmt.Errorf("cancel maintenance: %w", err)
	}
	if result.RowsAffected() != 1 {
		var exists bool
		if err := store.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cairnops_maintenances WHERE id = $1::uuid)`, maintenanceID).Scan(&exists); err != nil {
			return Maintenance{}, fmt.Errorf("find maintenance after cancellation: %w", err)
		}
		if !exists {
			return Maintenance{}, ErrNotFound
		}
		return Maintenance{}, fmt.Errorf("%w: cette maintenance est déjà terminée ou annulée", ErrConflict)
	}
	return store.get(ctx, maintenanceID)
}

func (store *PostgresStore) get(ctx context.Context, maintenanceID string) (Maintenance, error) {
	item, err := scanMaintenance(store.pool.QueryRow(ctx, maintenanceSelect+` WHERE maintenance.id = $1::uuid`, maintenanceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Maintenance{}, ErrNotFound
	}
	return item, err
}

const maintenanceSelect = `
	SELECT maintenance.id::text, maintenance.name, maintenance.reason,
	       CASE WHEN maintenance.cancelled_at IS NOT NULL THEN 'cancelled'
	            WHEN now() < maintenance.starts_at THEN 'upcoming'
	            WHEN now() <= maintenance.ends_at THEN 'active'
	            ELSE 'ended' END,
	       maintenance.starts_at, maintenance.ends_at, maintenance.cancelled_at,
	       coalesce(actor.display_name, ''), maintenance.created_at,
        coalesce(maintenance.series_id::text, ''), maintenance.extension_count,
        CASE WHEN series.id IS NOT NULL THEN jsonb_build_object('frequency', 'weekly', 'timezone', series.timezone, 'until', to_char(series.until_date, 'YYYY-MM-DD')) ELSE 'null'::jsonb END,
	       coalesce((
	           SELECT jsonb_agg(jsonb_build_object('id', target.id::text, 'name', target.name)
	                            ORDER BY lower(target.name), target.id)
	           FROM cairnops_maintenance_targets link
	           JOIN cairnops_targets target ON target.id = link.target_id
	           WHERE link.maintenance_id = maintenance.id
	       ), '[]'::jsonb)
	FROM cairnops_maintenances maintenance
	LEFT JOIN cairnops_users actor ON actor.id = maintenance.created_by
 LEFT JOIN cairnops_maintenance_series series ON series.id = maintenance.series_id
`

type scanner interface{ Scan(...any) error }

func scanMaintenance(row scanner) (Maintenance, error) {
	var item Maintenance
	var targets, recurrence []byte
	if err := row.Scan(&item.ID, &item.Name, &item.Reason, &item.State, &item.StartsAt, &item.EndsAt,
		&item.CancelledAt, &item.CreatedBy, &item.CreatedAt, &item.SeriesID, &item.ExtensionCount, &recurrence, &targets); err != nil {
		return Maintenance{}, err
	}
	if err := json.Unmarshal(recurrence, &item.Recurrence); err != nil {
		return Maintenance{}, fmt.Errorf("decode maintenance recurrence: %w", err)
	}
	if err := json.Unmarshal(targets, &item.Targets); err != nil {
		return Maintenance{}, fmt.Errorf("decode maintenance targets: %w", err)
	}
	return item, nil
}

func (store *PostgresStore) CancelSeries(ctx context.Context, maintenanceID, actorID string) (Maintenance, error) {
	item, err := store.get(ctx, maintenanceID)
	if err != nil {
		return Maintenance{}, err
	}
	if item.SeriesID == "" {
		return Maintenance{}, fmt.Errorf("%w: cette fenêtre ne fait pas partie d’une série", ErrConflict)
	}
	result, err := store.pool.Exec(ctx, `
  UPDATE cairnops_maintenances SET cancelled_at = now(), cancelled_by = $2::uuid
  WHERE series_id = $1::uuid AND cancelled_at IS NULL AND ends_at > now()
 `, item.SeriesID, actorID)
	if err != nil {
		return Maintenance{}, fmt.Errorf("cancel maintenance series: %w", err)
	}
	if result.RowsAffected() == 0 {
		return Maintenance{}, fmt.Errorf("%w: cette série est déjà terminée ou annulée", ErrConflict)
	}
	return store.get(ctx, maintenanceID)
}

func (store *PostgresStore) Extend(ctx context.Context, maintenanceID, actorID string, expectedEndsAt time.Time) (Maintenance, error) {
	// UPDATE compares and locks the occurrence. The audit insertion shares the
	// same statement, so failed or competing requests leave neither partial state
	// nor duplicate audit records.
	result, err := store.pool.Exec(ctx, `
  WITH extended AS (
   UPDATE cairnops_maintenances m
   SET ends_at = m.ends_at + interval '30 minutes', extension_count = m.extension_count + 1
   WHERE m.id = $1::uuid AND m.cancelled_at IS NULL
    AND m.starts_at <= now() AND m.ends_at > now() AND m.ends_at = $3
    AND m.ends_at + interval '30 minutes' <= m.starts_at + interval '31 days'
    AND NOT EXISTS (
     SELECT 1 FROM cairnops_maintenances following
     WHERE following.series_id = m.series_id AND following.cancelled_at IS NULL
      AND following.starts_at > m.starts_at AND following.starts_at < m.ends_at + interval '30 minutes'
    )
   RETURNING m.id, m.ends_at
  )
  INSERT INTO cairnops_maintenance_extensions (maintenance_id, actor_id, previous_ends_at, ends_at)
  SELECT id, $2::uuid, ends_at - interval '30 minutes', ends_at FROM extended
 `, maintenanceID, actorID, expectedEndsAt)
	if err != nil {
		return Maintenance{}, fmt.Errorf("extend maintenance: %w", err)
	}
	if result.RowsAffected() == 0 {
		if _, err := store.get(ctx, maintenanceID); err != nil {
			return Maintenance{}, err
		}
		return Maintenance{}, fmt.Errorf("%w: la fenêtre a changé, n’est plus active ou sa durée maximale est atteinte ; actualisez la liste", ErrConflict)
	}
	return store.get(ctx, maintenanceID)
}
