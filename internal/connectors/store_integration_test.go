package connectors

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestPostgresZabbixImportIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("connector-test-%d", suffix)
	databaseName := fmt.Sprintf("Database %d", suffix)
	webName := fmt.Sprintf("Web %d", suffix)
	endpoint := fmt.Sprintf("https://zabbix-%d.example.net/api_jsonrpc.php", suffix)
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Connector Test', 'not-used', 'administrator')
		RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO cairnops_targets (name) VALUES ($1)", databaseName); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	input := PersistZabbixInput{
		ActorID: actorID, Name: "Production", Endpoint: endpoint,
		CredentialSealed: "sealed-credential-with-sufficient-length", Version: "7.4.2",
		Compatibility: "supported", EncryptedTransport: true,
		Hosts: []zabbix.Host{{ID: "10084", Name: databaseName}, {ID: "10085", Name: webName}},
	}
	first, err := store.ImportZabbix(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Connector.BindingCount != 2 || first.Targets[0].Disposition != "reused" || first.Targets[1].Disposition != "created" {
		t.Fatalf("unexpected first import: %#v", first)
	}
	second, err := store.ImportZabbix(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Connector.ID != first.Connector.ID || second.Connector.BindingCount != 2 {
		t.Fatalf("connector was not updated idempotently: first=%#v second=%#v", first.Connector, second.Connector)
	}
	for _, target := range second.Targets {
		if target.Disposition != "already_imported" {
			t.Fatalf("expected existing binding, got %#v", target)
		}
	}
	var targetCount, connectorEvents int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cairnops_targets WHERE name = ANY($1::text[])", []string{databaseName, webName}).Scan(&targetCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cairnops_events WHERE kind = 'connector.changed'").Scan(&connectorEvents); err != nil {
		t.Fatal(err)
	}
	if targetCount != 2 || connectorEvents == 0 {
		t.Fatalf("unexpected persisted state: targets=%d connector_events=%d", targetCount, connectorEvents)
	}
}

func TestPostgresImportUsesExplicitTargetAssignment(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	var actorID, targetID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('assignment-test', 'Assignment Test', 'not-used', 'administrator')
		RETURNING id::text
	`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Passerelle') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}

	result, err := NewPostgresStore(pool).ImportZabbix(ctx, PersistZabbixInput{
		ActorID: actorID, Name: "Production", Endpoint: "https://zabbix-assignment.example.net/api_jsonrpc.php",
		CredentialSealed: "sealed-credential-with-sufficient-length", Version: "7.4.2",
		Compatibility: "supported", EncryptedTransport: true,
		Hosts:             []zabbix.Host{{ID: "42", Name: "gw-prod", Interfaces: []zabbix.Interface{{Address: "192.0.2.42", Main: true}}}},
		TargetAssignments: map[string]string{"42": targetID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 || result.Targets[0].TargetID != targetID || result.Targets[0].Disposition != "reused" {
		t.Fatalf("explicit assignment was not used: %#v", result.Targets)
	}
}

func TestPostgresRemovalClosesIncidentsLeftWithoutEvidence(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("removal-test-%d", suffix)
	aloneName := fmt.Sprintf("Alone %d", suffix)
	sharedName := fmt.Sprintf("Shared %d", suffix)
	endpoint := fmt.Sprintf("https://zabbix-removal-%d.example.net/api_jsonrpc.php", suffix)
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Removal Test', 'not-used', 'administrator') RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	imported, err := store.ImportZabbix(ctx, PersistZabbixInput{
		ActorID: actorID, Name: "Production", Endpoint: endpoint,
		CredentialSealed: "sealed-credential-with-sufficient-length", Version: "7.4.2",
		Compatibility: "supported", EncryptedTransport: true,
		Hosts: []zabbix.Host{{ID: "20084", Name: aloneName}, {ID: "20085", Name: sharedName}},
	})
	if err != nil {
		t.Fatal(err)
	}
	connectorID := imported.Connector.ID

	// Deux Incidents actifs : le premier ne tient que par le Connecteur, le
	// second garde une preuve d'une autre origine et doit rester ouvert.
	openedAt := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	incidentIDs := make(map[string]string, 2)
	for _, imported := range imported.Targets {
		var incidentID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO cairnops_incidents (
				target_id, nature_key, nature_label, status,
				source_severity, effective_severity, opened_at
			) VALUES ($1::uuid, 'availability', 'Indisponibilité', 'active', 'major', 'major', $2)
			RETURNING id::text
		`, imported.TargetID, openedAt).Scan(&incidentID); err != nil {
			t.Fatal(err)
		}
		incidentIDs[imported.TargetName] = incidentID
		if _, err := pool.Exec(ctx, `
			INSERT INTO cairnops_incident_signals (
				incident_id, target_id, origin, connector_id, connector_binding_id,
				external_event_id, name, active, severity, opened_at
			)
			SELECT $1::uuid, $2::uuid, 'zabbix', $3::uuid, binding.id, $4, 'Trigger', true, 'major', $5
			FROM cairnops_connector_bindings binding
			WHERE binding.connector_id = $3::uuid AND binding.external_id = $6
		`, incidentID, imported.TargetID, connectorID, "event-"+imported.ExternalID, openedAt, imported.ExternalID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incident_signals (
			incident_id, target_id, origin, external_event_id, name, active, severity, opened_at
		)
		SELECT $1::uuid, target_id, 'webhook', 'own-evidence', 'Sonde maison', true, 'major', $2
		FROM cairnops_incidents WHERE id = $1::uuid
	`, incidentIDs[sharedName], openedAt); err != nil {
		t.Fatal(err)
	}

	activeSources := func() int {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)::integer
			FROM cairnops_signal_sources source
			JOIN cairnops_connector_bindings binding ON binding.id = source.connector_binding_id
			WHERE binding.connector_id = $1::uuid AND source.enabled
		`, connectorID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	suspended, err := store.SetStatus(ctx, connectorID, "disabled")
	if err != nil || suspended.Status != "disabled" || suspended.BindingCount != 2 {
		t.Fatalf("unexpected suspension: %#v err=%v", suspended, err)
	}
	if active := activeSources(); active != 0 {
		t.Fatalf("suspended connector left %d active sources", active)
	}
	resumed, err := store.SetStatus(ctx, connectorID, "connected")
	if err != nil || resumed.Status != "connected" {
		t.Fatalf("unexpected resumption: %#v err=%v", resumed, err)
	}
	if active := activeSources(); active != 2 {
		t.Fatalf("resumed connector left %d active sources", active)
	}

	removal, err := store.Delete(ctx, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	if removal.Bindings != 2 || removal.ResolvedIncidents != 1 || removal.Name != "Production" {
		t.Fatalf("unexpected removal report: %#v", removal)
	}
	if _, err := store.Delete(ctx, connectorID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected a missing connector on second removal, got %v", err)
	}

	var aloneStatus, sharedStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM cairnops_incidents WHERE id = $1::uuid`, incidentIDs[aloneName]).Scan(&aloneStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM cairnops_incidents WHERE id = $1::uuid`, incidentIDs[sharedName]).Scan(&sharedStatus); err != nil {
		t.Fatal(err)
	}
	if aloneStatus != "resolved" || sharedStatus != "active" {
		t.Fatalf("unexpected incident outcome: alone=%s shared=%s", aloneStatus, sharedStatus)
	}
	var explained int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_incident_activity
		WHERE incident_id = $1::uuid AND kind = 'resolved' AND data->>'connector' = 'Production'
	`, incidentIDs[aloneName]).Scan(&explained); err != nil {
		t.Fatal(err)
	}
	var survivingTargets, remainingSignals, remainingSources int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_targets WHERE name = ANY($1::text[])
	`, []string{aloneName, sharedName}).Scan(&survivingTargets); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_incident_signals WHERE incident_id = ANY($1::uuid[])
	`, []string{incidentIDs[aloneName], incidentIDs[sharedName]}).Scan(&remainingSignals); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_signal_sources
		WHERE origin = 'integration' AND target_id = ANY($1::uuid[])
	`, []string{imported.Targets[0].TargetID, imported.Targets[1].TargetID}).Scan(&remainingSources); err != nil {
		t.Fatal(err)
	}
	if explained != 1 || survivingTargets != 2 || remainingSignals != 1 || remainingSources != 0 {
		t.Fatalf("unexpected aftermath: explained=%d targets=%d signals=%d sources=%d",
			explained, survivingTargets, remainingSignals, remainingSources)
	}
}

func TestPostgresUptimeKumaImportIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("kuma-test-%d", suffix)
	targetName := fmt.Sprintf("Kuma API %d", suffix)
	endpoint := fmt.Sprintf("https://kuma-%d.example.net/metrics", suffix)
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Kuma Test', 'not-used', 'administrator')
		RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	input := PersistUptimeKumaInput{
		ActorID: actorID, Name: "Kuma production", Endpoint: endpoint,
		CredentialSealed: "sealed-credential-with-sufficient-length", EncryptedTransport: true,
		Monitors: []uptimekuma.Monitor{{ID: "12", Name: targetName, Type: "http", URL: "https://api.example.net", Status: 1}},
	}
	first, err := store.ImportUptimeKuma(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ImportUptimeKuma(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Connector.BindingCount != 1 || first.Targets[0].Disposition != "created" || second.Connector.ID != first.Connector.ID || second.Targets[0].Disposition != "already_imported" {
		t.Fatalf("unexpected idempotent Uptime Kuma import: first=%#v second=%#v", first, second)
	}

	// L'import fait exister une Source de signal pour la liaison : sans elle,
	// la Cible importée n'aurait jamais de Disponibilité ni de Couverture. Un
	// second import ne la duplique pas.
	var sources int
	var origin, kind string
	var interval int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer, min(source.origin), min(source.kind), min(source.interval_seconds)
		FROM cairnops_signal_sources source
		JOIN cairnops_connector_bindings binding ON binding.id = source.connector_binding_id
		WHERE binding.connector_id = $1::uuid
	`, first.Connector.ID).Scan(&sources, &origin, &kind, &interval); err != nil {
		t.Fatal(err)
	}
	if sources != 1 || origin != "integration" || kind != "uptime_kuma" {
		t.Fatalf("expected exactly one integration source, got %d %q %q", sources, origin, kind)
	}
	if interval < 20 {
		t.Fatalf("the source must follow the connector cadence, got %d seconds", interval)
	}

	// Le worker n'exécute que les Contrôles natifs : la Source d'une
	// Intégration ne doit jamais être réclamée par le scheduler.
	var claimable int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_signal_sources
		WHERE origin = 'native' AND connector_binding_id IS NOT NULL
	`).Scan(&claimable); err != nil {
		t.Fatal(err)
	}
	if claimable != 0 {
		t.Fatalf("an integration source must never be scheduled, found %d", claimable)
	}
}

// Une Observation d'Intégration alimente la mesure sans passer par la Politique
// de déclenchement : l'Incident d'une Intégration reste décidé par le
// rapprochement de ses propres signaux.
func TestPostgresIntegrationObservationsFeedTheMeasureOnly(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("kuma-measure-%d", suffix)
	targetName := fmt.Sprintf("Kuma measured %d", suffix)
	endpoint := fmt.Sprintf("https://kuma-measure-%d.example.net/metrics", suffix)
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Kuma Measure', 'not-used', 'administrator')
		RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	imported, err := store.ImportUptimeKuma(ctx, PersistUptimeKumaInput{
		ActorID: actorID, Name: "Kuma measured", Endpoint: endpoint,
		CredentialSealed: "sealed-credential-with-sufficient-length", EncryptedTransport: true,
		Monitors: []uptimekuma.Monitor{{ID: "42", Name: targetName, Type: "http", Status: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var bindingID string
	if err := pool.QueryRow(ctx,
		`SELECT id::text FROM cairnops_connector_bindings WHERE connector_id = $1::uuid`, imported.Connector.ID,
	).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}

	latency := 148
	observedAt := time.Now().UTC()
	if err := store.RecordIntegrationObservations(ctx, observedAt, []IntegrationObservation{
		{BindingID: bindingID, Outcome: "healthy", LatencyMilliseconds: &latency},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordIntegrationObservations(ctx, observedAt.Add(time.Second), []IntegrationObservation{
		{BindingID: bindingID, Outcome: "unhealthy", Reason: "uptime_kuma_monitor_down"},
	}); err != nil {
		t.Fatal(err)
	}

	// Une Observation neutre — maintenance ou état en attente — observe sans
	// conclure : elle avance la fraîcheur mais ne remplace pas le dernier
	// verdict, que la contrainte réserve à healthy ou unhealthy.
	neutralAt := observedAt.Add(2 * time.Second)
	if err := store.RecordIntegrationObservations(ctx, neutralAt, []IntegrationObservation{
		{BindingID: bindingID, Outcome: "unknown", Reason: "uptime_kuma_monitor_maintenance"},
	}); err != nil {
		t.Fatalf("a neutral observation must record without violating the outcome constraint: %v", err)
	}
	var lastSignalOutcome *string
	var lastObservedAfterNeutral time.Time
	if err := pool.QueryRow(ctx, `
		SELECT last_signal_outcome, last_observed_at FROM cairnops_signal_sources
		WHERE connector_binding_id = $1::uuid
	`, bindingID).Scan(&lastSignalOutcome, &lastObservedAfterNeutral); err != nil {
		t.Fatal(err)
	}
	if lastSignalOutcome == nil || *lastSignalOutcome != "unhealthy" {
		t.Fatalf("a neutral observation must keep the last conclusive verdict, got %v", lastSignalOutcome)
	}
	if !lastObservedAfterNeutral.After(observedAt.Add(time.Second)) {
		t.Fatalf("a neutral observation must still advance freshness, got %s", lastObservedAfterNeutral)
	}

	var observed, unhealthy, latencySum int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer,
		       count(*) FILTER (WHERE outcome = 'unhealthy')::integer,
		       coalesce(sum(latency_milliseconds), 0)::integer
		FROM cairnops_observations observation
		JOIN cairnops_signal_sources source ON source.id = observation.source_id
		WHERE source.connector_binding_id = $1::uuid
	`, bindingID).Scan(&observed, &unhealthy, &latencySum); err != nil {
		t.Fatal(err)
	}
	if observed != 3 || unhealthy != 1 || latencySum != latency {
		t.Fatalf("unexpected integration observations: %d observed, %d unhealthy, %d ms", observed, unhealthy, latencySum)
	}

	// Les compteurs de la Politique de déclenchement restent intacts : rien
	// n'a été confronté à un seuil.
	var consecutiveUnhealthy int
	var lastObservedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT consecutive_unhealthy, last_observed_at FROM cairnops_signal_sources
		WHERE connector_binding_id = $1::uuid
	`, bindingID).Scan(&consecutiveUnhealthy, &lastObservedAt); err != nil {
		t.Fatal(err)
	}
	if consecutiveUnhealthy != 0 {
		t.Fatalf("an integration observation must not feed the trigger policy, got %d", consecutiveUnhealthy)
	}
	if lastObservedAt == nil {
		t.Fatal("the source must carry the freshness of its last observation")
	}

	// La Cible importée n'expose toujours aucun Contrôle natif : la Source
	// d'Intégration reste comptée comme externe.
	var nativeSources int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_signal_sources source
		JOIN cairnops_targets target ON target.id = source.target_id
		WHERE target.name = $1 AND source.origin = 'native'
	`, targetName).Scan(&nativeSources); err != nil {
		t.Fatal(err)
	}
	if nativeSources != 0 {
		t.Fatalf("an imported target must not gain a native control, got %d", nativeSources)
	}
}

func TestPostgresGenericWebhookQuarantinesThenBindsIdentity(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("webhook-test-%d", suffix)
	targetName := fmt.Sprintf("Webhook API %d", suffix)
	publicID := fmt.Sprintf("%032x", suffix)
	endpoint := "https://cairnops.example/api/v1/webhooks/" + publicID
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Webhook Test', 'not-used', 'administrator') RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	connector, err := store.CreateGenericWebhook(
		ctx, actorID, "Automations", endpoint, publicID,
		"sealed-webhook-credential-with-sufficient-length", true,
	)
	if err != nil {
		t.Fatal(err)
	}
	var createdTargetID string
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM cairnops_connectors WHERE id = $1::uuid`, connector.ID)
		if createdTargetID != "" {
			_, _ = pool.Exec(ctx, `DELETE FROM cairnops_targets WHERE id = $1::uuid`, createdTargetID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM cairnops_users WHERE id = $1::uuid`, actorID)
	}()

	credential, err := store.WebhookCredential(ctx, publicID)
	if err != nil || credential.ConnectorID != connector.ID || credential.CredentialSealed == "" {
		t.Fatalf("unexpected webhook credential: %#v err=%v", credential, err)
	}
	event := GenericWebhookEvent{
		Identity: "worker/api", TargetName: targetName, EventKey: "availability",
		NatureKey: "availability", Nature: "Indisponibilité", Status: "firing",
		Severity: "major", Summary: "API unreachable", Details: map[string]any{"region": "home"},
	}
	observedAt := time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC)
	first, err := store.RouteWebhook(ctx, connector.ID, event, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.RouteWebhook(ctx, connector.ID, event, observedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.QuarantineID == "" || second.QuarantineID != first.QuarantineID {
		t.Fatalf("quarantine was not deduplicated: first=%#v second=%#v", first, second)
	}
	pending, err := store.ListWebhookQuarantine(ctx, connector.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Occurrences != 2 || pending[0].Details["region"] != "home" {
		t.Fatalf("unexpected quarantined event: %#v", pending)
	}
	approval, err := store.ApproveWebhookIdentity(ctx, actorID, connector.ID, first.QuarantineID, "")
	if err != nil {
		t.Fatal(err)
	}
	createdTargetID = approval.TargetID
	if approval.Identity != "worker/api" || approval.BindingID == "" || len(approval.Events) != 1 {
		t.Fatalf("unexpected webhook approval: %#v", approval)
	}
	if err := store.CompleteWebhookApproval(ctx, connector.ID, approval.Identity, actorID); err != nil {
		t.Fatal(err)
	}
	pending, err = store.ListWebhookQuarantine(ctx, connector.ID)
	if err != nil || len(pending) != 0 {
		t.Fatalf("approved identity remained quarantined: %#v err=%v", pending, err)
	}
	route, err := store.RouteWebhook(ctx, connector.ID, event, observedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if route.BindingID != approval.BindingID || route.TargetID != approval.TargetID || route.QuarantineID != "" {
		t.Fatalf("approved identity was not routed: %#v", route)
	}
}
