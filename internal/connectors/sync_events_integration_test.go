package connectors

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Une synchronisation réussie remet le statut « connected » et vide last_error
// à chaque cycle. Tant que ces valeurs ne changent pas, aucun client n'a à
// recharger la liste des Connecteurs.
func TestUnchangedConnectorSyncAppendsNoConnectorEvent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	connectorID := insertSyncConnector(t, ctx, pool)
	startedAt := time.Date(2026, time.October, 15, 10, 0, 0, 0, time.UTC)
	const owner = "worker-test"

	lease := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			UPDATE cairnops_connectors SET lease_owner = $2, lease_until = now() + interval '1 minute'
			WHERE id = $1::uuid
		`, connectorID, owner); err != nil {
			t.Fatal(err)
		}
	}
	before := countConnectorEvents(t, ctx, pool)
	for cycle := 1; cycle <= 10; cycle++ {
		lease()
		if err := store.CompleteConnectorSync(ctx, connectorID, owner, startedAt.Add(time.Duration(cycle)*30*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if got := countConnectorEvents(t, ctx, pool) - before; got != 0 {
		t.Fatalf("10 unchanged syncs appended %d connector.changed, want 0", got)
	}

	lease()
	if err := store.FailConnectorSync(ctx, connectorID, owner, startedAt.Add(6*time.Minute), "connexion refusée"); err != nil {
		t.Fatal(err)
	}
	if got := countConnectorEvents(t, ctx, pool) - before; got != 1 {
		t.Fatalf("a failed sync appended %d connector.changed, want 1", got)
	}
}

// Argus réécrit le nom et les métadonnées de chaque liaison à chaque cycle.
func TestUnchangedArgusBindingsAppendNoConnectorEvent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	connectorID := insertSyncConnector(t, ctx, pool)
	var targetID, bindingID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Nextcloud') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name, metadata)
		VALUES ($1::uuid, $2::uuid, 'nextcloud', 'Nextcloud', '{"installed_version":"30.0.1"}'::jsonb)
		RETURNING id::text
	`, connectorID, targetID).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	snapshot := ArgusBindingSnapshot{
		BindingID: bindingID, ExternalName: "Nextcloud",
		Metadata: map[string]any{"installed_version": "30.0.1"},
	}

	before := countConnectorEvents(t, ctx, pool)
	for cycle := 1; cycle <= 10; cycle++ {
		if err := store.UpdateArgusBindings(ctx, connectorID, []ArgusBindingSnapshot{snapshot}); err != nil {
			t.Fatal(err)
		}
	}
	if got := countConnectorEvents(t, ctx, pool) - before; got != 0 {
		t.Fatalf("10 unchanged Argus snapshots appended %d connector.changed, want 0", got)
	}

	snapshot.Metadata = map[string]any{"installed_version": "30.0.2"}
	if err := store.UpdateArgusBindings(ctx, connectorID, []ArgusBindingSnapshot{snapshot}); err != nil {
		t.Fatal(err)
	}
	if got := countConnectorEvents(t, ctx, pool) - before; got != 1 {
		t.Fatalf("a new installed version appended %d connector.changed, want 1", got)
	}
}

func insertSyncConnector(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, status, encrypted_transport)
		VALUES ('zabbix', 'Production', 'https://zabbix.example.test/api_jsonrpc.php',
		        'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text
	`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func countConnectorEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_events WHERE kind = 'connector.changed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
