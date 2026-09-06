package notifications_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestSchedulingPreservesFailedDeliveryBackoff(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	seedActiveIncident(t, pool, "critical")
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery, err := store.Claim(ctx, "retry-worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Fail(ctx, delivery.ID, "retry-worker", "temporary outage"); err != nil {
		t.Fatal(err)
	}
	var before, after time.Time
	if err := pool.QueryRow(ctx, `SELECT next_attempt_at FROM cairnops_notification_outbox WHERE id = $1`, delivery.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT next_attempt_at FROM cairnops_notification_outbox WHERE id = $1`, delivery.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) {
		t.Fatalf("scheduler reset retry backoff: %s -> %s", before, after)
	}
	if _, err := store.Claim(ctx, "retry-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("premature retry: %v", err)
	}
}

func TestDiskBurstProducesOneOpeningForFifteenTargets(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)
	cycle := incidents.NewPostgresStore(pool)
	user := seedAccount(t, pool, "operator")
	at := time.Now().UTC()
	var connectorID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connectors
		(kind, name, endpoint, credential_sealed, status, encrypted_transport)
		VALUES ('zabbix', 'Zabbix', 'https://example.test', repeat('x', 32), 'connected', true)
		RETURNING id::text`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	signals := make([]incidents.ZabbixSignal, 0, 225)
	for targetIndex := range 15 {
		target, _ := seedTarget(t, pool)
		var bindingID string
		if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings
			(connector_id, target_id, external_id, external_name)
			VALUES ($1::uuid, $2::uuid, $3, $3) RETURNING id::text`, connectorID, target, fmt.Sprint(targetIndex)).Scan(&bindingID); err != nil {
			t.Fatal(err)
		}
		for diskIndex := range 15 {
			signals = append(signals, incidents.ZabbixSignal{
				BindingID: bindingID, ExternalEventID: fmt.Sprintf("%d-%d", targetIndex, diskIndex), ExternalObjectID: fmt.Sprint(diskIndex), TargetID: target,
				Name:            fmt.Sprintf("VM %d disk %d: slow", targetIndex, diskIndex),
				CanonicalNature: "storage.latency",
				Severity:        incidents.SeverityMajor, OpenedAt: at,
			})
		}
	}
	snapshot := incidents.ReconcileZabbixInput{ConnectorID: connectorID, ObservedAt: at, Signals: signals}
	if err := cycle.ReconcileZabbix(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, "incident-cycle-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("opening bypassed stability: %v", err)
	}
	items, err := cycle.List(ctx, "active", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AffectedTargetCount != 15 {
		t.Fatalf("burst produced %d incidents", len(items))
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET created_at=now()-interval '3 minutes' WHERE id=$1::uuid`, items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery := claimAndDeliver(t, ctx, store, "firing", "alert")
	if delivery.AffectedTargets != 15 {
		t.Fatalf("counted proofs instead of targets: %d", delivery.AffectedTargets)
	}
	if err := cycle.ReconcileZabbix(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, "incident-cycle-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("replay generated another notification: %v", err)
	}
	inbox, err := store.Inbox(ctx, user, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Entries) != 1 || inbox.Entries[0].Summary.FR.Body != "15 Cibles concernées · majeur" {
		t.Fatalf("unexpected inbox: %#v", inbox)
	}
}

func TestIncidentUpdateRetainsBackoffAcrossRevisions(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	_, id := seedActiveIncident(t, pool, "major")
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "firing", "alert")
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET severity='critical', revision=revision+1 WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	delivery, err := store.Claim(ctx, "retry-worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Fail(ctx, delivery.ID, "retry-worker", "temporary outage"); err != nil {
		t.Fatal(err)
	}
	var before, after time.Time
	if err := pool.QueryRow(ctx, `SELECT next_attempt_at FROM cairnops_notification_outbox WHERE id=$1`, delivery.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET revision=revision+1 WHERE id=$1::uuid`, id); err != nil {
			t.Fatal(err)
		}
		if err := store.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
		var attempts int
		if err := pool.QueryRow(ctx, `SELECT next_attempt_at, attempts FROM cairnops_notification_outbox WHERE incident_id=$1::uuid AND status IN ('pending','failed')`, id).Scan(&after, &attempts); err != nil {
			t.Fatal(err)
		}
		if !before.Equal(after) || attempts != 1 {
			t.Fatalf("revision reset retry: %s -> %s, attempts %d", before, after, attempts)
		}
	}
}

func TestOpeningCanResumeAfterATransientEmptyPropagation(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	_, id := seedActiveIncident(t, pool, "major")
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incident_impacts SET status='resolved', resolved_at=now() WHERE incident_id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET active_impact_count=0, affected_target_count=0, revision=revision+1 WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incident_impacts SET status='active', resolved_at=NULL WHERE incident_id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET active_impact_count=1, affected_target_count=1, revision=revision+1 WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "firing", "alert")
}

func TestMattermostOnlyInterruptsForNewOperationalFacts(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	actor := seedAccount(t, pool, "administrator")
	if _, err := store.CreateMattermost(ctx, notifications.PersistMattermostInput{
		ActorID: actor, Name: "Operations", Endpoint: "https://example.test", CredentialSealed: strings.Repeat("x", 32),
		Severities: []incidents.Severity{incidents.SeverityMajor, incidents.SeverityCritical}, EncryptedTransport: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_notification_channels SET enabled = false WHERE kind = 'in_app'`); err != nil {
		t.Fatal(err)
	}
	_, incidentID := seedActiveIncident(t, pool, "major")
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "firing", "alert")
	for _, step := range []struct {
		severity        string
		extended, alert bool
	}{
		{"major", false, false},    // ordinary enrichment
		{"critical", false, true},  // first worsening
		{"major", false, false},    // improvement
		{"critical", false, false}, // already notified severity
		{"critical", true, true},   // first extended propagation
		{"critical", true, false},  // no reminder
	} {
		if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET severity = $2, extended = $3, revision = revision + 1 WHERE id = $1::uuid`, incidentID, step.severity, step.extended); err != nil {
			t.Fatal(err)
		}
		if err := store.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
		if step.alert {
			claimAndDeliver(t, ctx, store, "incident_update", "alert")
		} else if delivery, err := store.Claim(ctx, "incident-cycle-worker"); err != notifications.ErrNoDelivery {
			t.Fatalf("ordinary revision interrupted Mattermost: %#v, %v", delivery, err)
		}
	}
}

func TestWorseningDuringMaintenanceIsNotConsumedBeforeExpiry(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := immediateNotificationStore(pool)
	target, id := seedActiveIncident(t, pool, "major")
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "firing", "alert")
	var maintenance string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_maintenances (name,reason,starts_at,ends_at)
		VALUES ('Test maintenance','Maintenance test fixture',now()-interval '1 minute',now()+interval '1 hour') RETURNING id::text`).Scan(&maintenance); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cairnops_maintenance_targets (maintenance_id,target_id) VALUES ($1::uuid,$2::uuid)`, maintenance, target); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET severity='critical',revision=revision+1 WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, "incident-cycle-worker"); err != notifications.ErrNoDelivery {
		t.Fatalf("maintenance consumed the worsening: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_maintenances SET ends_at=now()-interval '1 second' WHERE id=$1::uuid`, maintenance); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndDeliver(t, ctx, store, "incident_update", "alert")
}
