package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	var maintenanceID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO cairnops_maintenances (name, reason, starts_at, ends_at, created_by)
		VALUES ($1, $2, $3, $4, $5::uuid)
		RETURNING id::text
	`, input.Name, input.Reason, input.StartsAt, input.EndsAt, actorID).Scan(&maintenanceID); err != nil {
		return Maintenance{}, fmt.Errorf("create maintenance: %w", err)
	}
	for _, targetID := range input.TargetIDs {
		result, err := tx.Exec(ctx, `
			INSERT INTO cairnops_maintenance_targets (maintenance_id, target_id)
			SELECT $1::uuid, id FROM cairnops_targets WHERE id = $2::uuid
		`, maintenanceID, targetID)
		if err != nil {
			return Maintenance{}, fmt.Errorf("attach maintenance target: %w", err)
		}
		if result.RowsAffected() != 1 {
			return Maintenance{}, fmt.Errorf("%w: cible %s introuvable", ErrInvalidInput, targetID)
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
	       coalesce((
	           SELECT jsonb_agg(jsonb_build_object('id', target.id::text, 'name', target.name)
	                            ORDER BY lower(target.name), target.id)
	           FROM cairnops_maintenance_targets link
	           JOIN cairnops_targets target ON target.id = link.target_id
	           WHERE link.maintenance_id = maintenance.id
	       ), '[]'::jsonb)
	FROM cairnops_maintenances maintenance
	LEFT JOIN cairnops_users actor ON actor.id = maintenance.created_by
`

type scanner interface{ Scan(...any) error }

func scanMaintenance(row scanner) (Maintenance, error) {
	var item Maintenance
	var targets []byte
	if err := row.Scan(&item.ID, &item.Name, &item.Reason, &item.State, &item.StartsAt, &item.EndsAt,
		&item.CancelledAt, &item.CreatedBy, &item.CreatedAt, &targets); err != nil {
		return Maintenance{}, err
	}
	if err := json.Unmarshal(targets, &item.Targets); err != nil {
		return Maintenance{}, fmt.Errorf("decode maintenance targets: %w", err)
	}
	return item, nil
}
