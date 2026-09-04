package incidents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
)

type recordingAcknowledger struct {
	eventIDs  []string
	failingID string
}

func (acknowledger *recordingAcknowledger) Acknowledge(_ context.Context, target AcknowledgementTarget, _ string) error {
	acknowledger.eventIDs = append(acknowledger.eventIDs, target.ExternalEventID)
	if target.ExternalEventID == acknowledger.failingID {
		return errors.New("temporary upstream failure")
	}
	return nil
}

func TestRuntimeSynchronizesAcknowledgementForEvidenceJoiningLater(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)

	var actorID, connectorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('incident-runtime', 'Incident Runtime', 'not-used', 'operator')
		RETURNING id::text
	`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			compatibility, encrypted_transport, created_by
		) VALUES (
			'zabbix', 'Zabbix runtime', 'https://zabbix-runtime.example.net/api_jsonrpc.php',
			repeat('x', 40), 'connected', 'supported', true, $1::uuid
		) RETURNING id::text
	`, actorID).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}

	firstTarget := insertCycleTarget(t, ctx, pool, "API runtime")
	secondTarget := insertCycleTarget(t, ctx, pool, "Worker runtime")
	bindings := make([]string, 2)
	for index, targetID := range []string{firstTarget, secondTarget} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO cairnops_connector_bindings (
				connector_id, target_id, external_id, external_name
			) VALUES ($1::uuid, $2::uuid, $3, $4)
			RETURNING id::text
		`, connectorID, targetID, []string{"host-api", "host-worker"}[index],
			[]string{"API runtime", "Worker runtime"}[index]).Scan(&bindings[index]); err != nil {
			t.Fatal(err)
		}
	}

	startedAt := time.Now().UTC().Add(-time.Second)
	first := ZabbixSignal{
		TargetID: firstTarget, BindingID: bindings[0], ExternalEventID: "event-api",
		ExternalObjectID: "trigger-availability", CanonicalNature: NatureAvailability,
		Name: "Indisponibilité", Severity: SeverityMajor, OpenedAt: startedAt,
	}
	if err := store.ReconcileZabbix(ctx, ReconcileZabbixInput{
		ConnectorID: connectorID, ObservedAt: startedAt, Signals: []ZabbixSignal{first},
	}); err != nil {
		t.Fatal(err)
	}
	active, err := store.List(ctx, "active", 10)
	if err != nil || len(active) != 1 {
		t.Fatalf("expected one active Incident, got incidents=%#v err=%v", active, err)
	}

	acknowledger := &recordingAcknowledger{}
	service := NewService(store, acknowledger)
	if _, err := service.Acknowledge(ctx, active[0].ID, actorID, "Incident Runtime"); err != nil {
		t.Fatal(err)
	}
	acknowledger.eventIDs = nil

	second := ZabbixSignal{
		TargetID: secondTarget, BindingID: bindings[1], ExternalEventID: "event-worker",
		ExternalObjectID: "trigger-availability", CanonicalNature: NatureAvailability,
		Name: "Indisponibilité", Severity: SeverityCritical, OpenedAt: startedAt.Add(time.Second),
	}
	if err := store.ReconcileZabbix(ctx, ReconcileZabbixInput{
		ConnectorID: connectorID, ObservedAt: startedAt.Add(time.Second),
		Signals: []ZabbixSignal{first, second},
	}); err != nil {
		t.Fatal(err)
	}
	joined, err := store.Get(ctx, active[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if joined.AcknowledgementSyncStatus != "pending" || len(joined.Impacts) != 2 {
		t.Fatalf("later Evidence must inherit the acknowledgement, got %#v", joined)
	}

	runtime := NewRuntime(store, service, nil)
	acknowledger.failingID = "event-worker"
	if err := runtime.Process(ctx); err != nil {
		t.Fatal(err)
	}
	joined, err = store.Get(ctx, active[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if joined.AcknowledgementSyncStatus != "failed" {
		t.Fatalf("worker did not retain the failed inherited acknowledgement: %#v", joined)
	}
	for _, impact := range joined.Impacts {
		for _, evidence := range impact.Evidence {
			if evidence.ExternalEventID == "event-api" &&
				(evidence.AcknowledgementSyncStatus != "synchronized" || evidence.AcknowledgementSyncedAt == nil) {
				t.Fatalf("successful Evidence did not keep its synchronization result: %#v", evidence)
			}
			if evidence.ExternalEventID == "event-worker" &&
				(evidence.AcknowledgementSyncStatus != "failed" || evidence.AcknowledgementSyncError == "") {
				t.Fatalf("failed Evidence did not keep its synchronization result: %#v", evidence)
			}
		}
	}
	if !containsString(acknowledger.eventIDs, "event-worker") {
		t.Fatalf("new Evidence was not acknowledged upstream: %#v", acknowledger.eventIDs)
	}
	acknowledger.eventIDs = nil
	if err := runtime.Process(ctx); err != nil {
		t.Fatal(err)
	}
	if len(acknowledger.eventIDs) != 0 {
		t.Fatalf("failed Evidence retried before its backoff elapsed: %#v", acknowledger.eventIDs)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incident_evidence
		SET updated_at = now() - interval '31 seconds'
		WHERE incident_id = $1::uuid AND acknowledgement_sync_status = 'failed'
	`, active[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET acknowledgement_sync_status = 'failed',
		    acknowledgement_sync_error = 'temporary upstream failure',
		    updated_at = now() - interval '31 seconds'
		WHERE id = $1::uuid
	`, active[0].ID); err != nil {
		t.Fatal(err)
	}
	acknowledger.failingID = ""
	acknowledger.eventIDs = nil
	if err := runtime.Process(ctx); err != nil {
		t.Fatal(err)
	}
	joined, err = store.Get(ctx, active[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if joined.AcknowledgementSyncStatus != "synchronized" || len(acknowledger.eventIDs) == 0 {
		t.Fatalf("worker did not retry failed synchronization: incident=%#v events=%#v", joined, acknowledger.eventIDs)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
