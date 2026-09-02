package incidents

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
	"time"
)

// activeIncidents rend les Incidents actifs. Le test est seul dans sa base :
// « tous les Incidents » ne peut donc désigner que les siens.
func activeIncidents(t *testing.T, ctx context.Context, store *PostgresStore) []Incident {
	t.Helper()
	incidents, err := store.List(ctx, "active", 200)
	if err != nil {
		t.Fatal(err)
	}
	return incidents
}

func TestOpenedByDayKeepsDaysContainingOnlyArchivedTargets(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	var today time.Time
	if err := pool.QueryRow(ctx, `
		SELECT date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
	`).Scan(&today); err != nil {
		t.Fatal(err)
	}

	var activeTargetID, archivedTargetID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name) VALUES ('Daily active target') RETURNING id::text
	`).Scan(&activeTargetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name, archived_at)
		VALUES ('Daily archived target', now()) RETURNING id::text
	`).Scan(&archivedTargetID); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status,
			source_severity, effective_severity, opened_at
		) VALUES
			($1::uuid, 'daily:active', 'Active target incident', 'active', 'warning', 'warning', $3),
			($2::uuid, 'daily:archived', 'Archived target incident', 'active', 'warning', 'warning', $3 - interval '1 day')
	`, activeTargetID, archivedTargetID, today.Add(12*time.Hour)); err != nil {
		t.Fatal(err)
	}

	series, err := NewPostgresStore(pool).OpenedByDay(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 3 {
		t.Fatalf("expected every calendar day, got %#v", series)
	}
	if series[1].Opened != 0 {
		t.Fatalf("an archived-only day must remain present with zero, got %#v", series[1])
	}
	if series[2].Opened != 1 {
		t.Fatalf("expected today's active target incident, got %#v", series[2])
	}
}

func TestResolvedIncidentListPrioritizesLatestResolution(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	var targetID, latestResolutionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name)
		VALUES ('Resolved incident ordering target')
		RETURNING id::text
	`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status,
			source_severity, effective_severity, opened_at, resolved_at
		) VALUES
			($1::uuid, 'older-opening', 'Older opening', 'resolved',
			 'warning', 'warning', '2026-08-01T10:00:00Z', '2026-08-31T10:00:00Z')
		RETURNING id::text
	`, targetID).Scan(&latestResolutionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status,
			source_severity, effective_severity, opened_at, resolved_at
		) VALUES
			($1::uuid, 'newer-opening', 'Newer opening', 'resolved',
			 'warning', 'warning', '2026-08-30T10:00:00Z', '2026-08-30T11:00:00Z')
	`, targetID); err != nil {
		t.Fatal(err)
	}

	listed, err := NewPostgresStore(pool).List(ctx, "resolved", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != latestResolutionID {
		t.Fatalf("latest resolution must remain visible at the list limit, got %#v", listed)
	}
}

func TestPostgresZabbixIncidentLifecycleAndAcknowledgement(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("incident-test-%d", suffix)
	targetName := fmt.Sprintf("Database incident test %d", suffix)
	endpoint := fmt.Sprintf("https://incident-zabbix-%d.example/api_jsonrpc.php", suffix)
	var actorID, targetID, connectorID, bindingID, secondBindingID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Incident Test', 'not-used', 'operator')
		RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, targetName).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status, encrypted_transport
		) VALUES ('zabbix', 'Incident Zabbix', $1,
		          'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text
	`, endpoint).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, '10084', $3)
		RETURNING id::text
	`, connectorID, targetID, targetName).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, '10085', $3)
		RETURNING id::text
	`, connectorID, targetID, targetName).Scan(&secondBindingID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	openedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	if err := store.ReconcileZabbix(ctx, ReconcileZabbixInput{
		ConnectorID: connectorID, ObservedAt: openedAt.Add(time.Minute),
		Signals: []ZabbixSignal{{
			TargetID: targetID, BindingID: bindingID, ExternalEventID: "20427",
			ExternalObjectID: "15112", Name: "Database unavailable",
			Severity: SeverityCritical, OpenedAt: openedAt,
		}, {
			TargetID: targetID, BindingID: secondBindingID, ExternalEventID: "20428",
			ExternalObjectID: "15112", Name: "Database unavailable",
			Severity: SeverityCritical, OpenedAt: openedAt,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	active := activeIncidents(t, ctx, store)
	wantNatureKey := "zabbix:" + connectorID + ":15112"
	if len(active) != 1 || len(active[0].Signals) != 2 || active[0].NatureKey != wantNatureKey ||
		active[0].NatureScope != "connector" || active[0].NatureNamespace != connectorID ||
		active[0].NatureFingerprint != "15112" || !active[0].BurstEligible {
		t.Fatalf("unexpected active incident: %#v", active)
	}

	plan, err := store.AcknowledgeLocal(ctx, active[0].ID, actorID, "Incident Test")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Incident.AcknowledgementSyncStatus != "pending" || len(plan.Targets) != 2 {
		t.Fatalf("unexpected acknowledgement plan: %#v", plan)
	}
	acknowledged, err := store.CompleteAcknowledgement(ctx, active[0].ID, "failed", "temporary Zabbix outage")
	if err != nil {
		t.Fatal(err)
	}
	if acknowledged.AcknowledgedAt == nil || acknowledged.AcknowledgementSyncStatus != "failed" {
		t.Fatalf("unexpected acknowledged incident: %#v", acknowledged)
	}
	if err := store.ReconcileZabbix(ctx, ReconcileZabbixInput{
		ConnectorID: connectorID, ObservedAt: openedAt.Add(2 * time.Minute),
		Signals: []ZabbixSignal{{
			TargetID: targetID, BindingID: bindingID, ExternalEventID: "20427",
			ExternalObjectID: "15112", Name: "Database unavailable",
			Severity: SeverityCritical, OpenedAt: openedAt, UpstreamAcknowledged: true,
		}, {
			TargetID: targetID, BindingID: secondBindingID, ExternalEventID: "20428",
			ExternalObjectID: "15112", Name: "Database unavailable",
			Severity: SeverityCritical, OpenedAt: openedAt,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	acknowledged, err = store.Get(ctx, active[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if acknowledged.AcknowledgementSyncStatus != "failed" {
		t.Fatalf("partially acknowledged signals must not mark the incident synchronized: %#v", acknowledged)
	}
	if err := store.ReconcileZabbix(ctx, ReconcileZabbixInput{
		ConnectorID: connectorID, ObservedAt: openedAt.Add(3 * time.Minute),
		Signals: []ZabbixSignal{{
			TargetID: targetID, BindingID: bindingID, ExternalEventID: "20427",
			ExternalObjectID: "15112", Name: "Database unavailable",
			Severity: SeverityCritical, OpenedAt: openedAt, UpstreamAcknowledged: true,
		}, {
			TargetID: targetID, BindingID: secondBindingID, ExternalEventID: "20428",
			ExternalObjectID: "15112", Name: "Database unavailable",
			Severity: SeverityCritical, OpenedAt: openedAt, UpstreamAcknowledged: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	acknowledged, err = store.Get(ctx, active[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if acknowledged.AcknowledgementOrigin != "user" || acknowledged.AcknowledgementSyncStatus != "synchronized" || acknowledged.AcknowledgementSyncError != "" {
		t.Fatalf("unexpected upstream acknowledgement reconciliation: %#v", acknowledged)
	}

	resolvedAt := openedAt.Add(5 * time.Minute)
	if err := store.ReconcileZabbix(ctx, ReconcileZabbixInput{ConnectorID: connectorID, ObservedAt: resolvedAt}); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Get(ctx, active[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" || resolved.ResolvedAt == nil || len(resolved.Activity) < 5 {
		t.Fatalf("unexpected resolved incident: %#v", resolved)
	}
}

func TestPostgresZabbixReconciliationResolvesIncidentWhenSignalChangesNature(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	var targetID, connectorID, bindingID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name)
		VALUES ('Zabbix nature migration target')
		RETURNING id::text
	`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status, encrypted_transport
		) VALUES ('zabbix', 'Zabbix nature migration', 'https://zabbix-nature.example/api_jsonrpc.php',
		          'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text
	`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, '10084', 'Zabbix nature migration target')
		RETURNING id::text
	`, connectorID, targetID).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	openedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	input := ReconcileZabbixInput{
		ConnectorID: connectorID,
		ObservedAt:  openedAt,
		Signals: []ZabbixSignal{{
			TargetID: targetID, BindingID: bindingID, ExternalEventID: "20427",
			ExternalObjectID: "15112", NatureFingerprint: "legacy-trigger-15112",
			Name: "Filesystem space is low", Severity: SeverityWarning, OpenedAt: openedAt,
		}},
	}
	if err := store.ReconcileZabbix(ctx, input); err != nil {
		t.Fatal(err)
	}
	before := activeIncidents(t, ctx, store)
	if len(before) != 1 {
		t.Fatalf("expected the original active Incident, got %#v", before)
	}
	previousIncidentID := before[0].ID

	input.ObservedAt = openedAt.Add(time.Minute)
	input.Signals[0].NatureFingerprint = "template-root-uuid"
	if err := store.ReconcileZabbix(ctx, input); err != nil {
		t.Fatal(err)
	}

	after := activeIncidents(t, ctx, store)
	if len(after) != 1 || after[0].ID == previousIncidentID || len(after[0].Signals) != 1 {
		t.Fatalf("changing a signal Nature must leave exactly its new Incident active, got %#v", after)
	}
	previous, err := store.Get(ctx, previousIncidentID)
	if err != nil {
		t.Fatal(err)
	}
	if previous.Status != "resolved" || previous.ResolvedAt == nil || len(previous.Signals) != 0 {
		t.Fatalf("the Incident emptied by a Nature change must be resolved, got %#v", previous)
	}
}

func TestPostgresArgusIncidentTracksVersionChangesAndSkippedRearms(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	var targetID, connectorID, bindingID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Argus API') RETURNING id::text`).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status, encrypted_transport
		) VALUES ('argus', 'Argus incident test', $1,
		          'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text
		`, "https://argus.example").Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, 'api', 'Public API')
		RETURNING id::text
	`, connectorID, targetID).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	observedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	signal := ArgusSignal{
		TargetID: targetID, BindingID: bindingID, ExternalService: "api",
		NatureKey: "software-update-available", NatureLabel: "Mise à jour logicielle disponible",
		Name: "Public API · Version 1.1.0 disponible, 1.0.0 déployée", Severity: SeverityWarning,
		DeployedVersion: "1.0.0", LatestVersion: "1.1.0",
		Details: map[string]any{"deployed_version": "1.0.0", "latest_version": "1.1.0", "approved": false},
	}
	if err := store.ReconcileArgus(ctx, ReconcileArgusInput{
		ConnectorID: connectorID, ObservedAt: observedAt,
		ObservedBindings: []string{bindingID}, Signals: []ArgusSignal{signal},
	}); err != nil {
		t.Fatal(err)
	}
	active := activeIncidents(t, ctx, store)
	if len(active) != 1 || active[0].NatureKey != "software-update-available" || active[0].SourceSeverity != SeverityWarning {
		t.Fatalf("unexpected Argus incident: %#v", active)
	}
	firstIncidentID := active[0].ID

	signal.LatestVersion = "1.2.0"
	signal.Name = "Public API · Version 1.2.0 disponible, 1.0.0 déployée"
	signal.Details["latest_version"] = "1.2.0"
	if err := store.ReconcileArgus(ctx, ReconcileArgusInput{
		ConnectorID: connectorID, ObservedAt: observedAt.Add(time.Minute),
		ObservedBindings: []string{bindingID}, Signals: []ArgusSignal{signal},
	}); err != nil {
		t.Fatal(err)
	}
	active = activeIncidents(t, ctx, store)
	if len(active) != 1 || active[0].ID != firstIncidentID || active[0].Signals[0].Name != signal.Name {
		t.Fatalf("a newer version must update the same incident: %#v", active)
	}
	var changedActivities int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_incident_activity
		WHERE incident_id = $1::uuid AND kind = 'signal_updated'
	`, firstIncidentID).Scan(&changedActivities); err != nil {
		t.Fatal(err)
	}
	if changedActivities != 1 {
		t.Fatalf("expected exactly one version-change activity, got %d", changedActivities)
	}
	if err := store.ReconcileArgus(ctx, ReconcileArgusInput{
		ConnectorID: connectorID, ObservedAt: observedAt.Add(2 * time.Minute),
		ObservedBindings: []string{bindingID}, Signals: []ArgusSignal{signal},
	}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_incident_activity
		WHERE incident_id = $1::uuid AND kind = 'signal_updated'
	`, firstIncidentID).Scan(&changedActivities); err != nil {
		t.Fatal(err)
	}
	if changedActivities != 1 {
		t.Fatalf("an unchanged poll must not append activity, got %d", changedActivities)
	}

	// skipped est une décision Argus concluante : elle résout. Si cette même
	// version redevient ensuite disponible, elle constitue un nouvel Incident.
	if err := store.ReconcileArgus(ctx, ReconcileArgusInput{
		ConnectorID: connectorID, ObservedAt: observedAt.Add(3 * time.Minute),
		ObservedBindings: []string{bindingID}, Signals: nil,
	}); err != nil {
		t.Fatal(err)
	}
	if active = activeIncidents(t, ctx, store); len(active) != 0 {
		t.Fatalf("skipped state should resolve the incident: %#v", active)
	}
	if err := store.ReconcileArgus(ctx, ReconcileArgusInput{
		ConnectorID: connectorID, ObservedAt: observedAt.Add(4 * time.Minute),
		ObservedBindings: []string{bindingID}, Signals: []ArgusSignal{signal},
	}); err != nil {
		t.Fatal(err)
	}
	active = activeIncidents(t, ctx, store)
	if len(active) != 1 || active[0].ID == firstIncidentID {
		t.Fatalf("an unskipped version must open a new incident: %#v", active)
	}

	// Une Source Inconnue ne conclut rien et ne peut donc pas rétablir.
	if err := store.ReconcileArgus(ctx, ReconcileArgusInput{
		ConnectorID: connectorID, ObservedAt: observedAt.Add(5 * time.Minute),
		ObservedBindings: nil, Signals: nil,
	}); err != nil {
		t.Fatal(err)
	}
	if active = activeIncidents(t, ctx, store); len(active) != 1 {
		t.Fatalf("an unknown Argus state must preserve the active incident: %#v", active)
	}
}

func TestPostgresUptimeKumaDownRecoveryAndNewFailure(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("kuma-incident-%d", suffix)
	targetName := fmt.Sprintf("Kuma database %d", suffix)
	endpoint := fmt.Sprintf("https://kuma-incident-%d.example/metrics", suffix)
	var actorID, targetID, connectorID, bindingID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Kuma Operator', 'not-used', 'operator') RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, targetName).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, status, encrypted_transport)
		VALUES ('uptime_kuma', 'Incident Kuma', $1, 'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text
	`, endpoint).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, '12', $3) RETURNING id::text
	`, connectorID, targetID, targetName).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	down := UptimeKumaSignal{
		TargetID: targetID, BindingID: bindingID, ExternalMonitor: "12",
		Name: targetName, Severity: SeverityMajor,
	}
	firstObservedAt := time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)
	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: connectorID, ObservedAt: firstObservedAt, Signals: []UptimeKumaSignal{down},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: connectorID, ObservedAt: firstObservedAt.Add(time.Minute), Signals: []UptimeKumaSignal{down},
	}); err != nil {
		t.Fatal(err)
	}
	active := activeIncidents(t, ctx, store)
	if len(active) != 1 || len(active[0].Signals) != 1 || active[0].NatureKey != NatureAvailability ||
		active[0].NatureScope != "canonical" || active[0].NatureFingerprint != NatureAvailability ||
		!active[0].BurstEligible {
		t.Fatalf("unexpected Kuma incident: %#v", active)
	}
	firstIncidentID := active[0].ID
	plan, err := store.AcknowledgeLocal(ctx, firstIncidentID, actorID, "Kuma Operator")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 0 || plan.Incident.AcknowledgementSyncStatus != "not_applicable" {
		t.Fatalf("Kuma acknowledgement must remain local: %#v", plan)
	}
	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: connectorID, ObservedAt: firstObservedAt.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Get(ctx, firstIncidentID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" || len(resolved.Signals) != 1 || resolved.Signals[0].Active {
		t.Fatalf("unexpected Kuma recovery: %#v", resolved)
	}

	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: connectorID, ObservedAt: firstObservedAt.Add(3 * time.Minute), Signals: []UptimeKumaSignal{down},
	}); err != nil {
		t.Fatal(err)
	}
	active = activeIncidents(t, ctx, store)
	if len(active) != 1 || active[0].ID == firstIncidentID {
		t.Fatalf("a later Kuma failure must open a new incident: %#v", active)
	}
	secondIncidentID, secondSignalID := active[0].ID, active[0].Signals[0].ID
	invalidated, err := store.InvalidateSignal(
		ctx, secondIncidentID, secondSignalID, actorID, "Kuma Operator", "Le moniteur distant publie un état obsolète",
	)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Status != "resolved" || invalidated.Signals[0].InvalidatedAt == nil || invalidated.Signals[0].InvalidationReason == "" {
		t.Fatalf("motivated invalidation was not preserved: %#v", invalidated)
	}
	history, err := store.ListForTarget(ctx, "all", targetID, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].ID != secondIncidentID {
		t.Fatalf("target history lost an incident resolved by invalidation: %#v", history)
	}
	if history[0].Signals[0].InvalidatedAt == nil || history[0].Activity[0].Kind != "resolved" || history[0].Activity[1].Kind != "invalidated" {
		t.Fatalf("target history lost the invalidation journal: %#v", history[0])
	}
	if _, err := store.InvalidateSignal(ctx, secondIncidentID, secondSignalID, actorID, "Kuma Operator", "Une deuxième tentative doit échouer"); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict for an inactive signal, got %v", err)
	}

	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: connectorID, ObservedAt: firstObservedAt.Add(4 * time.Minute), Signals: []UptimeKumaSignal{down},
	}); err != nil {
		t.Fatal(err)
	}
	if active = activeIncidents(t, ctx, store); len(active) != 0 {
		t.Fatalf("persistent polling must not undo an invalidation: %#v", active)
	}
	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: connectorID, ObservedAt: firstObservedAt.Add(5 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{
		ConnectorID: connectorID, ObservedAt: firstObservedAt.Add(6 * time.Minute), Signals: []UptimeKumaSignal{down},
	}); err != nil {
		t.Fatal(err)
	}
	if active = activeIncidents(t, ctx, store); len(active) != 1 || active[0].ID == secondIncidentID {
		t.Fatalf("a healthy observation must rearm the next failure cycle: %#v", active)
	}
}

func TestPostgresWebhookIncidentRequiresExplicitResolutionAndCanReopen(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("webhook-incident-%d", suffix)
	targetName := fmt.Sprintf("Webhook service %d", suffix)
	endpoint := fmt.Sprintf("https://cairnops.example/webhooks/%d", suffix)
	var actorID, targetID, connectorID, bindingID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Webhook Operator', 'not-used', 'operator') RETURNING id::text
	`, username).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, targetName).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, status, encrypted_transport)
		VALUES ('generic_webhook', 'Incident Webhook', $1, 'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text
	`, endpoint).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, 'worker/api', $3) RETURNING id::text
	`, connectorID, targetID, targetName).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}

	store := NewPostgresStore(pool)
	observedAt := time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC)
	signal := WebhookSignal{
		ConnectorID: connectorID, BindingID: bindingID, TargetID: targetID,
		ExternalEventKey: "availability", NatureKey: "availability", NatureLabel: "Indisponibilité",
		Summary: "API unreachable", Status: "firing", Severity: SeverityCritical,
		ObservedAt: observedAt, Details: map[string]any{"region": "home"},
	}
	if err := store.ApplyWebhook(ctx, signal); err != nil {
		t.Fatal(err)
	}
	signal.ObservedAt = observedAt.Add(time.Minute)
	if err := store.ApplyWebhook(ctx, signal); err != nil {
		t.Fatal(err)
	}
	active := activeIncidents(t, ctx, store)
	if len(active) != 1 || len(active[0].Signals) != 1 || active[0].NatureKey != "availability" {
		t.Fatalf("webhook signal was not deduplicated: %#v", active)
	}
	firstIncidentID := active[0].ID
	plan, err := store.AcknowledgeLocal(ctx, firstIncidentID, actorID, "Webhook Operator")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Incident.AcknowledgementSyncStatus != "not_applicable" || len(plan.Targets) != 0 {
		t.Fatalf("generic webhook acknowledgement must remain local: %#v", plan)
	}

	signal.Status = "resolved"
	signal.Summary = "API reachable"
	signal.ObservedAt = observedAt.Add(2 * time.Minute)
	if err := store.ApplyWebhook(ctx, signal); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Get(ctx, firstIncidentID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" || resolved.Signals[0].Active {
		t.Fatalf("explicit webhook resolution was not applied: %#v", resolved)
	}

	signal.Status = "firing"
	signal.Summary = "API unreachable again"
	signal.ObservedAt = observedAt.Add(3 * time.Minute)
	if err := store.ApplyWebhook(ctx, signal); err != nil {
		t.Fatal(err)
	}
	if active = activeIncidents(t, ctx, store); len(active) != 1 || active[0].ID == firstIncidentID {
		t.Fatalf("a later webhook cycle must open a new incident: %#v", active)
	}
}
