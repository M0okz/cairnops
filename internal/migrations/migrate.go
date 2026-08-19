package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var files embed.FS

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := fs.Glob(files, "sql/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(entries)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('cairnops-schema-migrations'))"); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS cairnops_schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	for _, path := range entries {
		var applied bool
		if err := tx.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM cairnops_schema_migrations WHERE version = $1)", path,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", path, err)
		}
		if applied {
			continue
		}

		script, err := files.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}
		if _, err := tx.Exec(ctx, string(script)); err != nil {
			return fmt.Errorf("apply migration %s: %w", path, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO cairnops_schema_migrations (version) VALUES ($1)", path,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", path, err)
		}
	}

	return tx.Commit(ctx)
}
