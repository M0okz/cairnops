package migrations

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIncidentEvidenceCutoverRemovesLegacyRealtimeEvents(t *testing.T) {
	databaseURL := os.Getenv("CAIRNOPS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CAIRNOPS_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	schema := fmt.Sprintf("cairnops_upgrade_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE")

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	entries, err := fs.Glob(files, "sql/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TABLE cairnops_schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		t.Fatal(err)
	}
	for _, path := range entries {
		if path == "sql/031_incident_evidence_cycle.sql" {
			break
		}
		script, err := files.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, string(script)); err != nil {
			t.Fatalf("apply prerequisite %s: %v", path, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO cairnops_schema_migrations (version) VALUES ($1)", path,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_events (kind, entity_type, entity_id)
		VALUES ('burst.changed', 'burst', gen_random_uuid()::text)
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("apply incident evidence cutover: %v", err)
	}
	var legacyEvents int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM cairnops_events
		WHERE kind = 'burst.changed' OR entity_type = 'burst'
	`).Scan(&legacyEvents); err != nil {
		t.Fatal(err)
	}
	if legacyEvents != 0 {
		t.Fatalf("legacy realtime events remain after cutover: %d", legacyEvents)
	}
}
