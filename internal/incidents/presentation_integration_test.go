package incidents

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestPresentationRequiresEveryContributingEvidence(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	at := time.Now().UTC()
	first := webhookEvidence("one", insertCycleTarget(t, ctx, pool, "First"), at)
	first.Nature = ConnectorNature("local", "custom", "A custom source label", "same-condition")
	first.Alert = alerttext.Fact{Kind: alerttext.DiskLatency, Resource: "sda"}
	second := webhookEvidence("two", insertCycleTarget(t, ctx, pool, "Second"), at)
	second.Nature = first.Nature
	// Details supplied by a webhook cannot inject a structured meaning.
	second.Metadata = map[string]any{"alert_facts": map[string]any{"kind": "disk.latency.high"}, "alert_kind": "disk.latency.high"}
	for _, kind := range []alerttext.Kind{"", alerttext.DiskSpace, alerttext.DiskLatency} {
		second.Alert = alerttext.Fact{Kind: kind, Resource: "nvme0n1"}
		if err := store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{Origin: "webhook", ObservedAt: at, Facts: []EvidenceFact{first, second}}); err != nil {
			t.Fatal(err)
		}
		items, err := store.List(ctx, "active", 10)
		if err != nil || len(items) != 1 {
			t.Fatalf("group changed: %+v %v", items, err)
		}
		item := items[0]
		if kind == alerttext.DiskLatency {
			if item.Presentation.FR.Title != "Latence disque élevée" || item.Presentation.FR.Description != "" {
				t.Fatalf("aggregate copied individual parameters: %+v", item.Presentation)
			}
		} else if item.AlertKind != "" || item.Presentation.FR.Title != "" {
			t.Fatalf("unrecognized/mixed evidence guessed: %+v", item)
		}
	}
}

func TestPatchMonPresentationUsesConditionData(t *testing.T) {
	for _, tc := range []struct {
		key                string
		details            map[string]any
		title, description string
	}{
		{"security_updates", map[string]any{"security_updates_count": 3}, "Correctifs de sécurité requis", "3 correctifs de sécurité disponibles"},
		{"security_updates", nil, "Correctifs de sécurité requis", ""},
		{"reboot_required", nil, "Redémarrage requis", ""},
		{"custom", map[string]any{"security_updates_count": 3}, "", ""},
	} {
		got := alerttext.Render(patchMonPresentation(PatchMonSignal{ConditionKey: tc.key, Details: tc.details}), "fr")
		if got.Title != tc.title || got.Description != tc.description {
			t.Fatalf("%s: %+v", tc.key, got)
		}
	}
}

func TestOtherConnectorPresentationsSurvivePersistence(t *testing.T) {
	for _, kind := range []string{"patchmon", "argus", "uptime_kuma", "generic_webhook"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			pool := testsupport.Pool(t)
			store := NewPostgresStore(pool)
			target := insertCycleTarget(t, ctx, pool, "Independent fixture target")
			var connector, binding string
			if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connectors
    (kind,name,endpoint,credential_sealed,status,encrypted_transport)
    VALUES ($1,$1,'https://source.example.test',repeat('x',32),'connected',true) RETURNING id::text`, kind).Scan(&connector); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings
    (connector_id,target_id,external_id,external_name)
    VALUES ($1::uuid,$2::uuid,'fixture','Independent fixture target') RETURNING id::text`, connector, target).Scan(&binding); err != nil {
				t.Fatal(err)
			}
			at := time.Now().UTC()
			source := "Operator's original source wording"
			var err error
			var expected alerttext.Kind
			var description string
			switch kind {
			case "patchmon":
				expected = alerttext.SecurityUpdates
				description = "3 security updates available"
				err = store.ReconcilePatchMon(ctx, ReconcilePatchMonInput{ConnectorID: connector, ObservedAt: at, ObservedBindings: []string{binding}, Signals: []PatchMonSignal{{
					TargetID: target, BindingID: binding, ConditionKey: "security_updates", NatureKey: "security-patches-required", NatureLabel: "Correctifs de sécurité requis", Name: source, Severity: SeverityMajor, Details: map[string]any{"security_updates_count": 3},
				}}})
			case "argus":
				expected = alerttext.SoftwareUpdate
				description = "Version 2.4.0 available · 2.3.1 deployed"
				err = store.ReconcileArgus(ctx, ReconcileArgusInput{ConnectorID: connector, ObservedAt: at, ObservedBindings: []string{binding}, Signals: []ArgusSignal{{
					TargetID: target, BindingID: binding, NatureKey: "software-update-available", NatureLabel: "Mise à jour disponible", Name: source, Severity: SeverityWarning, DeployedVersion: "2.3.1", LatestVersion: "2.4.0",
				}}})
			case "uptime_kuma":
				expected = alerttext.Unavailable
				err = store.ReconcileUptimeKuma(ctx, ReconcileUptimeKumaInput{ConnectorID: connector, ObservedAt: at, ObservedBindings: []string{binding}, Signals: []UptimeKumaSignal{{
					TargetID: target, BindingID: binding, ExternalMonitor: "7", Name: source, Severity: SeverityMajor,
				}}})
			case "generic_webhook":
				source = "Linux: High CPU utilization"
				err = store.ApplyWebhook(ctx, WebhookSignal{ConnectorID: connector, BindingID: binding, TargetID: target, ExternalEventKey: "custom-event", NatureKey: "custom-rule", NatureLabel: source, Summary: source, Status: "firing", Severity: SeverityWarning, ObservedAt: at,
					Details: map[string]any{"alert_kind": "cpu.usage.high", "alert_facts": map[string]any{"kind": "cpu.usage.high"}},
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			items, err := store.List(ctx, "active", 10)
			if err != nil || len(items) != 1 || len(items[0].Impacts) != 1 || len(items[0].Impacts[0].Evidence) != 1 {
				t.Fatalf("unexpected persisted incident: %+v %v", items, err)
			}
			item := items[0]
			evidence := item.Impacts[0].Evidence[0]
			title, _ := alerttext.Title(expected, "en")
			if item.AlertKind != expected || item.Presentation.EN.Title != title || evidence.Alert.Kind != expected || evidence.Presentation.EN.Title != title || evidence.Presentation.EN.Description != description || evidence.Name != source {
				t.Fatalf("connector meaning/parameters/source changed in persistence: incident=%+v evidence=%+v", item.Presentation, evidence)
			}
			if expected != "" && item.Summary.EN.Title != title {
				t.Fatalf("summary differs from shared catalog: %+v", item.Summary)
			}
		})
	}
}
