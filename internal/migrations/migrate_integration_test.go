package migrations_test

import (
	"context"
	"testing"

	"github.com/M0okz/cairnops/internal/migrations"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestLatestOrphanedIncidentRepairMigrationResolvesExistingActiveIncident(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	var incidentID string
	if err := pool.QueryRow(ctx, `
		WITH target AS (
			INSERT INTO cairnops_targets (name)
			VALUES ('Incident orphelin existant')
			RETURNING id
		)
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status,
			source_severity, effective_severity, opened_at
		)
		SELECT id, 'availability', 'Indisponibilité', 'active',
		       'major', 'major', now()
		FROM target
		RETURNING id::text
	`).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM cairnops_schema_migrations
		WHERE version = 'sql/029_repair_rekeyed_zabbix_incidents.sql'
	`); err != nil {
		t.Fatal(err)
	}

	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	var status string
	var explained int
	if err := pool.QueryRow(ctx, `
		SELECT status FROM cairnops_incidents WHERE id = $1::uuid
	`, incidentID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer
		FROM cairnops_incident_activity
		WHERE incident_id = $1::uuid AND kind = 'resolved'
		  AND data->>'reason' = 'orphaned_incident_repair'
	`, incidentID).Scan(&explained); err != nil {
		t.Fatal(err)
	}
	if status != "resolved" || explained != 1 {
		t.Fatalf("migration must resolve and explain the orphaned Incident, status=%s activity=%d", status, explained)
	}
}
