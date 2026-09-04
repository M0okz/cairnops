package indicators

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecordKeepsTheSourceObservationTimestamp(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var targetID, connectorID, bindingID, indicatorID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Horodatage') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			compatibility, encrypted_transport
		) VALUES ('zabbix', 'Zabbix horodatage', 'https://zabbix-timestamp.example.test/api_jsonrpc.php',
		          repeat('x', 40), 'connected', 'supported', true)
		RETURNING id::text
	`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (
			connector_id, target_id, external_id, external_name, indicators_enabled
		) VALUES ($1::uuid, $2::uuid, 'host-timestamp', 'Horodatage', true)
		RETURNING id::text
	`, connectorID, targetID).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_context_indicators (
			connector_id, connector_binding_id, target_id, semantic_key,
			label, external_id, unit
		) VALUES ($1::uuid, $2::uuid, $3::uuid, 'cpu.utilization',
		          'Utilisation CPU', 'item-timestamp', 'percent')
		RETURNING id::text
	`, connectorID, bindingID, targetID).Scan(&indicatorID); err != nil {
		t.Fatal(err)
	}

	collectedAt := time.Date(2026, 8, 23, 10, 42, 30, 0, time.UTC)
	observedAt := collectedAt.Add(-17 * time.Minute).Add(12 * time.Second)
	if err := NewStore(pool).Record(ctx, connectorID, collectedAt, []Reading{{
		IndicatorID: indicatorID, Value: 37.5, ObservedAt: observedAt,
	}}, nil); err != nil {
		t.Fatal(err)
	}

	var sampleAt, lastObservedAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT sample.observed_at, indicator.last_observed_at
		FROM cairnops_indicator_samples sample
		JOIN cairnops_context_indicators indicator ON indicator.id = sample.indicator_id
		WHERE sample.indicator_id = $1::uuid
	`, indicatorID).Scan(&sampleAt, &lastObservedAt); err != nil {
		t.Fatal(err)
	}
	if !sampleAt.Equal(observedAt.Truncate(time.Minute)) || !lastObservedAt.Equal(observedAt) {
		t.Fatalf("source timestamp was rewritten: sample=%s last=%s want=%s", sampleAt, lastObservedAt, observedAt)
	}
}

func TestIncidentProjectionIncludesEveryAffectedTarget(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID, connectorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('incident-indicators', 'Incident Indicators', 'unused', 'operator')
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			compatibility, encrypted_transport
		) VALUES ('zabbix', 'Zabbix incident',
		          'https://zabbix-incident-indicators.example.test/api_jsonrpc.php',
		          repeat('x', 40), 'connected', 'supported', true)
		RETURNING id::text
	`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}

	targetIDs := make([]string, 2)
	for index, name := range []string{"API indicateurs", "Base indicateurs"} {
		var bindingID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text
		`, name).Scan(&targetIDs[index]); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO cairnops_connector_bindings (
				connector_id, target_id, external_id, external_name, indicators_enabled
			) VALUES ($1::uuid, $2::uuid, $3, $4, true)
			RETURNING id::text
		`, connectorID, targetIDs[index], []string{"host-api", "host-db"}[index], name).Scan(&bindingID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO cairnops_context_indicators (
				connector_id, connector_binding_id, target_id, semantic_key,
				label, external_id, unit, last_value, last_observed_at
			) VALUES ($1::uuid, $2::uuid, $3::uuid, 'cpu.utilization',
			          'CPU', $4, 'percent', $5, now())
		`, connectorID, bindingID, targetIDs[index], "cpu-"+name, 40+index); err != nil {
			t.Fatal(err)
		}
	}

	var incidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			nature_key, nature_label, nature_scope, nature_namespace,
			nature_fingerprint, propagation_eligible, status,
			propagation_status, severity, opened_at, last_impact_at,
			propagation_window_seconds, propagation_ends_at,
			active_impact_count, impact_count, affected_target_count,
			max_affected_targets
		) VALUES (
			'availability', 'Indisponibilité', 'canonical', 'cairnops',
			'availability', true, 'active', 'open', 'major', now(), now(),
			60, now() + interval '60 seconds', 2, 2, 2, 2
		) RETURNING id::text
	`).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	for _, targetID := range targetIDs {
		if _, err := pool.Exec(ctx, `
			INSERT INTO cairnops_incident_impacts (
				incident_id, target_id, status, source_severity,
				effective_severity, opened_at
			) VALUES ($1::uuid, $2::uuid, 'active', 'major', 'major', now())
		`, incidentID, targetID); err != nil {
			t.Fatal(err)
		}
	}

	projection, err := NewStore(pool).Incident(ctx, userID, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.TargetIDs) != 2 || len(projection.Indicators) != 2 || len(projection.Snapshots) != 2 {
		t.Fatalf("multi-target context was truncated: %#v", projection)
	}
	seenTargets := map[string]bool{}
	for _, snapshot := range projection.Snapshots {
		seenTargets[snapshot.TargetID] = snapshot.TargetName != "" && snapshot.ImpactID != ""
	}
	for _, targetID := range targetIDs {
		if !seenTargets[targetID] {
			t.Fatalf("missing contextual snapshot for target %s: %#v", targetID, projection.Snapshots)
		}
	}
}

func TestIndicatorScopeStaysIndependentFromOperationalConnectorScope(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID, targetID, connectorID, bindingID, sourceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('indicator-admin', 'Indicator Admin', 'unused', 'administrator')
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('API') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			compatibility, encrypted_transport, sync_interval_seconds
		) VALUES ('zabbix', 'Zabbix', 'https://zabbix.example.test/api_jsonrpc.php',
		          repeat('x', 32), 'connected', 'supported', true, 60)
		RETURNING id::text
	`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (
			connector_id, target_id, external_id, external_name
		) VALUES ($1::uuid, $2::uuid, 'host-1', 'API') RETURNING id::text
	`, connectorID, targetID).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, origin, connector_binding_id,
			interval_seconds, timeout_milliseconds, config
		) VALUES ($1::uuid, 'API', 'zabbix', 'integration', $2::uuid, 60, 1000, '{}')
		RETURNING id::text
	`, targetID, bindingID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	store := NewStore(pool)
	selection := Selection{SemanticKey: "cpu.utilization", Label: "Utilisation CPU", ExternalID: "item-42", Unit: "percent", Metadata: map[string]any{}}
	if err := store.Apply(ctx, userID, connectorID, ApplyInput{
		Bindings: []BindingInput{{ID: bindingID, TargetID: targetID, ExternalID: "host-1", ExternalName: "API", Enabled: true, Indicators: []Selection{selection}}},
		Summary:  "CPU activé",
	}); err != nil {
		t.Fatal(err)
	}
	assertScopes(t, pool, bindingID, sourceID, true, true, true)

	if err := store.Apply(ctx, userID, connectorID, ApplyInput{
		Bindings: []BindingInput{{ID: bindingID, TargetID: targetID, ExternalID: "host-1", ExternalName: "API", Enabled: false, Indicators: []Selection{selection}}},
		Summary:  "Contexte masqué",
	}); err != nil {
		t.Fatal(err)
	}
	assertScopes(t, pool, bindingID, sourceID, true, false, false)
}

func TestNewIndicatorOnlyBindingDoesNotBecomeIncidentSource(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var userID, targetID, connectorID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users (username, display_name, password_hash, role) VALUES ('context-admin', 'Context Admin', 'unused', 'administrator') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Worker') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, status, compatibility, encrypted_transport) VALUES ('patchmon', 'PatchMon', 'https://patchmon.example.test/api/v1/api/hosts', repeat('x', 32), 'connected', 'supported', true) RETURNING id::text`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}

	store := NewStore(pool)
	input := ApplyInput{
		Bindings: []BindingInput{{
			TargetID: targetID, ExternalID: "host-new", ExternalName: "Worker", Enabled: true,
			Indicators: []Selection{{SemanticKey: "updates.count", Label: "Mises à jour disponibles", ExternalID: "updates:host-new", Unit: "count", Metadata: map[string]any{}}},
		}},
		Profiles: []ProfileInput{{Name: "Patch essentiel", Specification: []ProfileEntry{{SemanticKey: "updates.count", Enabled: true}}}},
		Summary:  "Contexte PatchMon",
	}
	if err := store.Apply(ctx, userID, connectorID, input); err != nil {
		t.Fatal(err)
	}

	var bindingID string
	var integrationEnabled, indicatorsEnabled bool
	if err := pool.QueryRow(ctx, `SELECT id::text, integration_enabled, indicators_enabled FROM cairnops_connector_bindings WHERE connector_id = $1::uuid AND external_id = 'host-new'`, connectorID).Scan(&bindingID, &integrationEnabled, &indicatorsEnabled); err != nil {
		t.Fatal(err)
	}
	if integrationEnabled || !indicatorsEnabled {
		t.Fatalf("unexpected scopes: integration=%t indicators=%t", integrationEnabled, indicatorsEnabled)
	}
	var sources int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_signal_sources WHERE connector_binding_id = $1::uuid`, bindingID).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if sources != 0 {
		t.Fatalf("indicator-only discovery created %d operational source(s)", sources)
	}

	configuration, err := store.Configuration(ctx, connectorID)
	if err != nil || len(configuration.Profiles) != 1 {
		t.Fatalf("read profile: configuration=%#v error=%v", configuration, err)
	}
	profileID := configuration.Profiles[0].ID
	input.Profiles[0].ID = profileID
	input.Profiles[0].Name = "Patch quotidien"
	if err := store.Apply(ctx, userID, connectorID, input); err != nil {
		t.Fatal(err)
	}
	configuration, err = store.Configuration(ctx, connectorID)
	if err != nil || len(configuration.Profiles) != 1 || configuration.Profiles[0].ID != profileID || configuration.Profiles[0].Name != "Patch quotidien" {
		t.Fatalf("profile identity was not preserved: configuration=%#v error=%v", configuration, err)
	}
}

func assertScopes(t *testing.T, pool *pgxpool.Pool, bindingID, sourceID string, wantIntegration, wantIndicators, wantIndicator bool) {
	t.Helper()
	var integrationEnabled, indicatorsEnabled, sourceEnabled, indicatorEnabled bool
	if err := pool.QueryRow(context.Background(), `
		SELECT binding.integration_enabled, binding.indicators_enabled,
		       source.enabled, indicator.enabled
		FROM cairnops_connector_bindings binding
		JOIN cairnops_signal_sources source ON source.connector_binding_id = binding.id
		JOIN cairnops_context_indicators indicator ON indicator.connector_binding_id = binding.id
		WHERE binding.id = $1::uuid AND source.id = $2::uuid
	`, bindingID, sourceID).Scan(&integrationEnabled, &indicatorsEnabled, &sourceEnabled, &indicatorEnabled); err != nil {
		t.Fatal(err)
	}
	if integrationEnabled != wantIntegration || indicatorsEnabled != wantIndicators || !sourceEnabled || indicatorEnabled != wantIndicator {
		t.Fatalf("unexpected scope state: integration=%t indicators=%t source=%t indicator=%t", integrationEnabled, indicatorsEnabled, sourceEnabled, indicatorEnabled)
	}
}
