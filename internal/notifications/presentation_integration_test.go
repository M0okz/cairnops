package notifications_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestStructuredPresentationDoesNotChangeIncidentOrDeliveryDecisions(t *testing.T) {
	for _, initiallyKnown := range []bool{false, true} {
		name := "enrichment"
		if initiallyKnown {
			name = "loss-of-recognition"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			pool := testsupport.Pool(t)
			cycle := incidents.NewPostgresStore(pool)
			deliveryStore := immediateNotificationStore(pool)
			account := seedAccount(t, pool, "operator")
			target, _ := seedTarget(t, pool)
			var connector, binding string
			if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connectors
    (kind,name,endpoint,credential_sealed,status,encrypted_transport)
    VALUES ('zabbix','Standard fixture','https://example.test',repeat('x',32),'connected',true)
    RETURNING id::text`).Scan(&connector); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings
    (connector_id,target_id,external_id,external_name) VALUES ($1::uuid,$2::uuid,'host-42','Unrelated installation')
    RETURNING id::text`, connector, target).Scan(&binding); err != nil {
				t.Fatal(err)
			}
			at := time.Now().UTC().Add(-10 * time.Minute)
			signal := incidents.ZabbixSignal{TargetID: target, BindingID: binding, ExternalEventID: "event-7", ExternalObjectID: "trigger-11",
				NatureFingerprint: "official-load-template", Name: "Source wording kept verbatim", Severity: incidents.SeverityWarning, OpenedAt: at,
				EvaluationWindow: 5 * time.Minute}
			if initiallyKnown {
				signal.Alert = alerttext.Fact{Kind: alerttext.SystemLoad}
			}
			apply := func(observed time.Time) {
				t.Helper()
				if err := cycle.ReconcileZabbix(ctx, incidents.ReconcileZabbixInput{ConnectorID: connector, ObservedAt: observed, Signals: []incidents.ZabbixSignal{signal}}); err != nil {
					t.Fatal(err)
				}
			}
			apply(at)
			items, err := cycle.List(ctx, "active", 10)
			if err != nil || len(items) != 1 {
				t.Fatalf("incident list: %+v %v", items, err)
			}
			before := items[0]
			if err := deliveryStore.Schedule(ctx); err != nil {
				t.Fatal(err)
			}
			delivery, err := deliveryStore.Claim(ctx, "presentation-worker")
			if err != nil {
				t.Fatal(err)
			}
			initialKind := signal.Alert.Kind
			if delivery.AlertKind != initialKind {
				t.Fatalf("outbox lost presentation snapshot: %+v", delivery)
			}
			if initiallyKnown {
				signal.Alert = alerttext.Fact{}
			} else {
				signal.Alert = alerttext.Fact{Kind: alerttext.SystemLoad}
			}
			apply(at.Add(10 * time.Second))
			after, err := cycle.Get(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if before.NatureKey != after.NatureKey || before.NatureFingerprint != after.NatureFingerprint || before.NatureScope != after.NatureScope ||
				before.Revision != after.Revision || before.Severity != after.Severity || before.AffectedTargetCount != after.AffectedTargetCount ||
				before.PropagationStatus != after.PropagationStatus || !before.PropagationEndsAt.Equal(after.PropagationEndsAt) || len(before.Activity) != len(after.Activity) ||
				before.Impacts[0].ID != after.Impacts[0].ID || before.Impacts[0].Evidence[0].ID != after.Impacts[0].Evidence[0].ID {
				t.Fatalf("presentation changed operational state: before=%+v after=%+v", before, after)
			}
			evidence := after.Impacts[0].Evidence[0]
			if evidence.Name != signal.Name || evidence.Alert.Kind != signal.Alert.Kind {
				t.Fatalf("source or facts lost: %+v", evidence)
			}
			if !initiallyKnown && (after.Summary.FR.Title != "Charge système moyenne élevée" || after.Presentation.EN.Title != "High average system load" || evidence.Presentation.EN.Title != "High average system load") {
				t.Fatalf("shared web/mobile presentation missing: %+v %+v", after.Presentation, evidence.Presentation)
			}
			if initiallyKnown && (after.Presentation.FR.Title != "" || evidence.Presentation.FR.Title != "") {
				t.Fatal("unknown alert retained a guessed translation")
			}
			// The claimed snapshot must not be reinterpreted using today's incident.
			if _, err := deliveryStore.Deliver(ctx, delivery); err != nil {
				t.Fatal(err)
			}
			if err := deliveryStore.Complete(ctx, delivery.ID, "presentation-worker"); err != nil {
				t.Fatal(err)
			}
			if err := deliveryStore.Schedule(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := deliveryStore.Claim(ctx, "presentation-worker"); !errors.Is(err, notifications.ErrNoDelivery) {
				t.Fatalf("presentation generated another delivery: %v", err)
			}
			inbox, err := deliveryStore.Inbox(ctx, account, 0)
			if err != nil || len(inbox.Entries) != 1 {
				t.Fatalf("inbox: %+v %v", inbox, err)
			}
			entry := inbox.Entries[0]
			if entry.AlertKind != initialKind {
				t.Fatalf("inbox did not snapshot kind: %+v", entry)
			}
			if initiallyKnown && entry.Summary.FR.Title != "Charge système moyenne élevée" {
				t.Fatalf("snapshot lost translation: %+v", entry.Summary)
			}
			if !initiallyKnown && entry.Summary.FR.Title != "Source wording kept verbatim" {
				t.Fatalf("old delivery retroactively translated: %+v", entry.Summary)
			}
			// Recovery keeps the evidence's meaning; it doesn't classify the recovery text.
			if err := cycle.ReconcileZabbix(ctx, incidents.ReconcileZabbixInput{ConnectorID: connector, ObservedAt: at.Add(time.Minute)}); err != nil {
				t.Fatal(err)
			}
			if err := cycle.Advance(ctx, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			resolved, err := cycle.Get(ctx, before.ID)
			if err != nil || resolved.Status != "resolved" || resolved.AlertKind != signal.Alert.Kind {
				t.Fatalf("recovery presentation: %+v %v", resolved, err)
			}
		})
	}
}
