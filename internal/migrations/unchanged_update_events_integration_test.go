package migrations_test

import (
	"context"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les autres tables suivies par cairnops_events obéissent à la même règle que
// les Incidents, les Connecteurs et les Doublons possibles, vérifiés dans leurs
// paquets : réécrire une ligne à l'identique n'annonce rien, la changer
// annonce une fois.
func TestUnchangedUpdatesAppendNoRealtimeEvent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	ids := seedRealtimeRows(t, ctx, pool)

	for _, table := range []struct {
		name      string
		kind      string
		unchanged string
		changed   string
	}{
		{
			name: "Source de signal", kind: "source.changed",
			unchanged: `UPDATE cairnops_signal_sources SET name = name, config = config, updated_at = now() WHERE id = $1::uuid`,
			changed:   `UPDATE cairnops_signal_sources SET name = 'Nextcloud public' WHERE id = $1::uuid`,
		},
		{
			name: "liaison de Connecteur", kind: "connector.changed",
			unchanged: `UPDATE cairnops_connector_bindings SET external_name = external_name, metadata = metadata || '{}'::jsonb, updated_at = now() WHERE id = $1::uuid`,
			changed:   `UPDATE cairnops_connector_bindings SET metadata = metadata || '{"missing":true}'::jsonb WHERE id = $1::uuid`,
		},
		{
			name: "appareil", kind: "device.changed",
			unchanged: `UPDATE cairnops_devices SET name = name, locale = locale, revoked_at = revoked_at, updated_at = now() WHERE id = $1::uuid`,
			changed:   `UPDATE cairnops_devices SET name = 'iPhone de secours' WHERE id = $1::uuid`,
		},
		{
			name: "Indicateur", kind: "indicator.changed",
			unchanged: `UPDATE cairnops_context_indicators SET enabled = enabled, last_error = '', updated_at = now() WHERE id = $1::uuid`,
			changed:   `UPDATE cairnops_context_indicators SET last_value = 42, last_observed_at = now() WHERE id = $1::uuid`,
		},
	} {
		t.Run(table.name, func(t *testing.T) {
			id := ids[table.kind]
			before := countEvents(t, ctx, pool, table.kind)
			for range 10 {
				if _, err := pool.Exec(ctx, table.unchanged, id); err != nil {
					t.Fatal(err)
				}
			}
			if got := countEvents(t, ctx, pool, table.kind) - before; got != 0 {
				t.Fatalf("10 unchanged updates appended %d %s, want 0", got, table.kind)
			}
			if _, err := pool.Exec(ctx, table.changed, id); err != nil {
				t.Fatal(err)
			}
			if got := countEvents(t, ctx, pool, table.kind) - before; got != 1 {
				t.Fatalf("a real change appended %d %s, want 1", got, table.kind)
			}
		})
	}
}

// seedRealtimeRows renvoie, par genre d'événement, la ligne à réécrire.
func seedRealtimeRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
		return id
	}
	userID := insert(`INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('realtime', 'Temps réel', 'not-used', 'administrator') RETURNING id::text`)
	targetID := insert(`INSERT INTO cairnops_targets (name) VALUES ('Nextcloud') RETURNING id::text`)
	connectorID := insert(`INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, status, encrypted_transport)
		VALUES ('zabbix', 'Production', 'https://zabbix.example.test/api_jsonrpc.php',
		        'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text`)
	bindingID := insert(`INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, '10084', 'Nextcloud') RETURNING id::text`, connectorID, targetID)
	return map[string]string{
		"connector.changed": bindingID,
		"source.changed": insert(`INSERT INTO cairnops_signal_sources (target_id, name, kind, interval_seconds, timeout_milliseconds, config)
			VALUES ($1::uuid, 'Nextcloud', 'http', 60, 5000, '{"url":"https://nextcloud.example.test"}'::jsonb)
			RETURNING id::text`, targetID),
		"device.changed": insert(`INSERT INTO cairnops_devices (user_id, name, platform, encryption_public_key, token_digest)
			VALUES ($1::uuid, 'iPhone', 'ios', decode(repeat('01', 32), 'hex'), decode(repeat('02', 32), 'hex'))
			RETURNING id::text`, userID),
		"indicator.changed": insert(`INSERT INTO cairnops_context_indicators (connector_id, connector_binding_id, target_id, semantic_key, label, external_id, unit)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'cpu.utilization', 'Processeur', 'cpu', 'percent')
			RETURNING id::text`, connectorID, bindingID, targetID),
	}
}

func countEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, kind string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_events WHERE kind = $1`, kind).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
