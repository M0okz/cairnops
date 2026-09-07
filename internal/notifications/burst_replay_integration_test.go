package notifications_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/testsupport"
)

// Chronologie de la rafale du 6 septembre 2026, anonymisée : mêmes secondes
// d'ouverture/rétablissement, même prototype, collecte toutes les 30 secondes.
// Les premiers signaux se rétablissent avant l'arrivée de la dernière vague.
func TestRecordedStaggeredDiskBurstHasOneOpening(t *testing.T) {
	ctx := context.Background()
	api := testsupport.DiskLatencyAPI(t)
	problems, err := zabbix.NewClient().Problems(ctx, api.URL, "test-token", []string{"10"})
	if err != nil || len(problems) != 1 {
		t.Fatalf("load recorded disk rule: %v (%d problems)", err, len(problems))
	}
	rule := problems[0]
	pool := testsupport.Pool(t)
	cycle := incidents.NewPostgresStore(pool)
	store := immediateNotificationStore(pool)
	user := seedAccount(t, pool, "operator")
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	var connectorID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connectors
		(kind,name,endpoint,credential_sealed,sync_interval_seconds,status,encrypted_transport)
		VALUES ('zabbix','Replay','https://example.test',repeat('x',32),30,'connected',true)
		RETURNING id::text`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	timings := [][2]int{{0, 605}, {198, 573}, {241, 605}, {396, 605}, {464, 605}, {477, 605}, {712, 1405}, {721, 797}, {728, 925}, {728, 989}}
	signals := make([]incidents.ZabbixSignal, len(timings))
	for n := range 45 {
		target, _ := seedTarget(t, pool)
		if n >= len(timings) {
			continue
		}
		var binding string
		if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings
			(connector_id,target_id,external_id,external_name)
			VALUES ($1::uuid,$2::uuid,$3,$3) RETURNING id::text`, connectorID, target, fmt.Sprint(n)).Scan(&binding); err != nil {
			t.Fatal(err)
		}
		signals[n] = incidents.ZabbixSignal{
			TargetID: target, BindingID: binding, ExternalEventID: fmt.Sprint(n + 1), ExternalObjectID: fmt.Sprint(n + 1),
			NatureFingerprint: rule.NatureFingerprint, CanonicalNature: rule.CanonicalNature,
			EvaluationWindow: rule.EvaluationWindow, Name: rule.Name,
			Severity: incidents.SeverityWarning, OpenedAt: at.Add(time.Duration(timings[n][0]) * time.Second),
		}
	}
	for elapsed := 0; elapsed <= 1800; elapsed += 30 {
		active := []incidents.ZabbixSignal{}
		for n, timing := range timings {
			if elapsed >= timing[0] && elapsed < timing[1] {
				active = append(active, signals[n])
			}
		}
		if err := cycle.ReconcileZabbix(ctx, incidents.ReconcileZabbixInput{ConnectorID: connectorID, ObservedAt: at.Add(time.Duration(elapsed) * time.Second), Signals: active}); err != nil {
			t.Fatal(err)
		}
		if elapsed == 630 {
			items, err := cycle.List(ctx, "active", 50)
			if err != nil || len(items) != 1 || items[0].AffectedTargetCount != 0 || items[0].PropagationStatus != "open" {
				t.Fatalf("temporary recovery prematurely closed the group: %#v (%v)", items, err)
			}
		}
		if err := store.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
		for {
			delivery, err := store.Claim(ctx, "burst-replay")
			if err == notifications.ErrNoDelivery {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Deliver(ctx, delivery); err != nil {
				t.Fatal(err)
			}
			if err := store.Complete(ctx, delivery.ID, "burst-replay"); err != nil {
				t.Fatal(err)
			}
		}
	}
	var openings, alerts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE event_kind='firing'), count(*) FILTER (WHERE presentation='alert')
		FROM cairnops_notification_outbox WHERE status='delivered'`).Scan(&openings, &alerts); err != nil {
		t.Fatal(err)
	}
	inbox, err := store.Inbox(ctx, user, 50)
	if err != nil {
		t.Fatal(err)
	}
	if openings != 1 || alerts != 1 || len(inbox.Entries) != 1 {
		t.Fatalf("recorded disk burst produced %d openings, %d alerts and %d inbox entries; want 1 of each", openings, alerts, len(inbox.Entries))
	}
	if inbox.Entries[0].Summary.FR.Title != "Résolu · Latence disque élevée" {
		t.Fatalf("inbox lost the normalized disk condition: %#v", inbox.Entries[0].Summary)
	}
	items, err := cycle.List(ctx, "resolved", 50)
	if err != nil || len(items) != 1 || items[0].ImpactCount != 10 || items[0].MaxAffectedTargets != 6 {
		t.Fatalf("group lost targets or counted sequential impacts as simultaneous: %#v (%v)", items, err)
	}
}
