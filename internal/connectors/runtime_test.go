package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/argus"
	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type runtimeStore struct {
	connectors    []RuntimeConnector
	completed     bool
	failed        string
	claimedKind   string
	observations  []IntegrationObservation
	argusBindings []ArgusBindingSnapshot
}

func (store *runtimeStore) ClaimDueConnector(_ context.Context, kind, _ string, _ int, _ time.Duration) ([]RuntimeConnector, error) {
	store.claimedKind = kind
	claimed := store.connectors
	store.connectors = nil
	return claimed, nil
}
func (store *runtimeStore) CompleteConnectorSync(context.Context, string, string, time.Time) error {
	store.completed = true
	return nil
}
func (store *runtimeStore) FailConnectorSync(_ context.Context, _, _ string, _ time.Time, message string) error {
	store.failed = message
	return nil
}
func (store *runtimeStore) RecordIntegrationObservations(_ context.Context, _ time.Time, observations []IntegrationObservation) error {
	store.observations = observations
	return nil
}

func (store *runtimeStore) UpdateArgusBindings(_ context.Context, _ string, bindings []ArgusBindingSnapshot) error {
	store.argusBindings = bindings
	return nil
}

type problemClient struct {
	problems []zabbix.Problem
	err      error
}

func (client problemClient) Problems(context.Context, string, string, []string) ([]zabbix.Problem, error) {
	return client.problems, client.err
}

type incidentReconciler struct {
	input         incidents.ReconcileZabbixInput
	kumaInput     incidents.ReconcileUptimeKumaInput
	patchMonInput incidents.ReconcilePatchMonInput
	argusInput    incidents.ReconcileArgusInput
}

func (reconciler *incidentReconciler) ReconcileZabbix(_ context.Context, input incidents.ReconcileZabbixInput) error {
	reconciler.input = input
	return nil
}

func (reconciler *incidentReconciler) ReconcileUptimeKuma(_ context.Context, input incidents.ReconcileUptimeKumaInput) error {
	reconciler.kumaInput = input
	return nil
}

func (reconciler *incidentReconciler) ReconcilePatchMon(_ context.Context, input incidents.ReconcilePatchMonInput) error {
	reconciler.patchMonInput = input
	return nil
}

func (reconciler *incidentReconciler) ReconcileArgus(_ context.Context, input incidents.ReconcileArgusInput) error {
	reconciler.argusInput = input
	return nil
}

type patchMonHostClient struct {
	hosts []patchmon.Host
	err   error
}

func (client patchMonHostClient) Hosts(context.Context, string, patchmon.Credentials) ([]patchmon.Host, error) {
	return client.hosts, client.err
}

func TestPatchMonSynchronizerProjectsPostureWithoutAvailabilitySemantics(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x79}, 32))
	credential, _ := json.Marshal(patchmon.Credentials{Key: "patchmon_key", Secret: "secret"})
	sealed, _ := box.Seal(credential, "connector:patchmon:https://patchmon.example.net/api/v1/api/hosts")
	store := &runtimeStore{connectors: []RuntimeConnector{{
		ID: "connector-patchmon", Endpoint: "https://patchmon.example.net/api/v1/api/hosts", CredentialSealed: sealed,
		Bindings: []RuntimeBinding{{ID: "binding-host", TargetID: "target-host", ExternalID: "host-1"}},
	}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewPatchMonSynchronizer(store, reconciler, patchMonHostClient{hosts: []patchmon.Host{{
		ID: "host-1", FriendlyName: "Web", ReportingState: "reporting", UpdateState: "security_required",
		UpdatesCount: 8, SecurityUpdatesCount: 2, NeedsReboot: true,
	}}}, box, "server-one", nil)
	synchronizer.now = func() time.Time { return time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC) }

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.claimedKind != "patchmon" || !store.completed || store.failed != "" {
		t.Fatalf("unexpected PatchMon synchronization: %#v", store)
	}
	if len(reconciler.patchMonInput.Signals) != 2 || len(reconciler.patchMonInput.ObservedBindings) != 1 {
		t.Fatalf("expected security and reboot posture signals: %#v", reconciler.patchMonInput)
	}
	if len(store.observations) != 1 || store.observations[0].Outcome != "unhealthy" || store.observations[0].LatencyMilliseconds != nil {
		t.Fatalf("unexpected posture observation: %#v", store.observations)
	}
}

type argusInspectionClient struct {
	inspection argus.Inspection
	err        error
}

func (client argusInspectionClient) Inspect(context.Context, string, argus.Credentials) (argus.Inspection, error) {
	return client.inspection, client.err
}

func TestArgusSynchronizerKeepsValidServicesAndDegradesPartialFailures(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x7a}, 32))
	credential, _ := json.Marshal(argus.Credentials{Username: "reader", Password: "secret"})
	sealed, _ := box.Seal(credential, "connector:argus:https://argus.example.net")
	store := &runtimeStore{connectors: []RuntimeConnector{{
		ID: "connector-argus", Endpoint: "https://argus.example.net", CredentialSealed: sealed,
		Bindings: []RuntimeBinding{
			{ID: "binding-api", TargetID: "target-api", ExternalID: "api"},
			{ID: "binding-skip", TargetID: "target-skip", ExternalID: "skipped"},
			{ID: "binding-broken", TargetID: "target-broken", ExternalID: "broken"},
			{ID: "binding-removed", TargetID: "target-removed", ExternalID: "removed", ExternalName: "Removed", Metadata: map[string]any{
				"deployed_version": "4.0.0", "latest_version": "4.1.0", "approved": false,
				"skipped": false, "last_checked": "2026-08-28T08:00:00Z",
			}},
		},
	}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewArgusSynchronizer(store, reconciler, argusInspectionClient{inspection: argus.Inspection{
		Endpoint: "https://argus.example.net", Version: "0.35.0", Compatibility: "supported",
		Services: []argus.Service{
			{ID: "api", Name: "Public API", Active: true, Importable: true, DeployedVersion: "1.2.2", LatestVersion: "1.2.3", LastChecked: "2026-08-29T08:00:00Z", Approved: true, DeploymentState: argus.DeploymentStateApproved, LatestQueryOK: true, DeployedQueryOK: true, VersionURL: "https://releases.example/1.2.3"},
			{ID: "skipped", Name: "Skipped", Active: true, Importable: true, DeployedVersion: "2.0.0", LatestVersion: "2.1.0", Skipped: true, DeploymentState: argus.DeploymentStateSkipped, LatestQueryOK: true, DeployedQueryOK: true},
			{ID: "broken", Name: "Broken", Active: true, Importable: true, DeployedVersion: "3.0.0", LatestVersion: "3.1.0", Unknown: true, UnknownReason: "latest_version_query_failed", DeploymentState: argus.DeploymentStateUnactioned, DeployedQueryOK: true},
		},
	}}, box, "server-one", nil)
	synchronizer.now = func() time.Time { return time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC) }

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.claimedKind != "argus" || store.completed || store.failed != "2 Sources Argus inconnues" {
		t.Fatalf("partial Argus failure should degrade after processing valid services: %#v", store)
	}
	if len(store.observations) != 4 {
		t.Fatalf("expected an observation for every imported Argus service: %#v", store.observations)
	}
	if len(store.argusBindings) != 3 || store.argusBindings[0].ExternalName != "Public API" {
		t.Fatalf("Argus service labels and snapshots must stay mutable: %#v", store.argusBindings)
	}
	outcomes := map[string]string{}
	for _, observation := range store.observations {
		outcomes[observation.BindingID] = observation.Outcome
	}
	if outcomes["binding-api"] != "unhealthy" || outcomes["binding-skip"] != "healthy" || outcomes["binding-broken"] != "unknown" || outcomes["binding-removed"] != "unknown" {
		t.Fatalf("unexpected Argus observation outcomes: %#v", outcomes)
	}
	for _, observation := range store.observations {
		if observation.BindingID == "binding-removed" && (observation.Details["deployed_version"] != "4.0.0" || observation.Details["last_checked"] != "2026-08-28T08:00:00Z") {
			t.Fatalf("a missing Argus service must keep its last posture details: %#v", observation)
		}
	}
	if len(reconciler.argusInput.Signals) != 1 || reconciler.argusInput.Signals[0].LatestVersion != "1.2.3" || reconciler.argusInput.Signals[0].NatureKey != "software-update-available" {
		t.Fatalf("expected one active software update signal: %#v", reconciler.argusInput)
	}
	if len(reconciler.argusInput.ObservedBindings) != 2 {
		t.Fatalf("only valid update and skipped states may resolve incidents: %#v", reconciler.argusInput)
	}
}

func TestSynchronizerProjectsProblemsThroughImportedBindings(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x52}, 32))
	sealed, _ := box.Seal([]byte("runtime-token"), "connector:zabbix:https://zabbix.example.net/api_jsonrpc.php")
	store := &runtimeStore{connectors: []RuntimeConnector{{
		ID: "connector-one", Endpoint: "https://zabbix.example.net/api_jsonrpc.php", CredentialSealed: sealed,
		Bindings: []RuntimeBinding{{ID: "binding-one", TargetID: "target-one", ExternalID: "10084"}},
	}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewSynchronizer(store, reconciler, problemClient{problems: []zabbix.Problem{{
		EventID: "20427", TriggerID: "15112", Name: "Database unavailable", Severity: 4,
		StartedAt: time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC), HostIDs: []string{"10084"},
	}}}, box, "server-one", nil)
	synchronizer.now = func() time.Time { return time.Date(2026, 8, 14, 12, 1, 0, 0, time.UTC) }

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.completed || store.failed != "" || len(reconciler.input.Signals) != 1 {
		t.Fatalf("unexpected synchronization: completed=%v failed=%q input=%#v", store.completed, store.failed, reconciler.input)
	}
	projected := reconciler.input.Signals[0]
	if projected.TargetID != "target-one" || projected.BindingID != "binding-one" || projected.Severity != incidents.SeverityCritical {
		t.Fatalf("unexpected projected signal: %#v", projected)
	}

	// Un hôte importé qui porte un problème actif conclut une Observation en
	// défaut : c'est elle qui donne à la Cible sa Disponibilité et sa
	// Couverture, comme un monitor Uptime Kuma DOWN.
	if len(store.observations) != 1 {
		t.Fatalf("expected an observation per imported host, got %#v", store.observations)
	}
	observation := store.observations[0]
	if observation.BindingID != "binding-one" || observation.Outcome != "unhealthy" || observation.LatencyMilliseconds != nil {
		t.Fatalf("an active Zabbix problem must conclude unhealthy without latency: %#v", observation)
	}
}

type kumaMonitorClient struct {
	monitors []uptimekuma.Monitor
	err      error
}

// Sans problème actif, un hôte importé conclut au bon fonctionnement : c'est
// cette Observation « healthy » qui remplit la Couverture d'une Cible Zabbix
// et met à jour sa fraîcheur, là où un Incident n'ouvrirait rien.
func TestSynchronizerMeasuresHealthyHostsWithoutProblems(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x52}, 32))
	sealed, _ := box.Seal([]byte("runtime-token"), "connector:zabbix:https://zabbix.example.net/api_jsonrpc.php")
	store := &runtimeStore{connectors: []RuntimeConnector{{
		ID: "connector-one", Endpoint: "https://zabbix.example.net/api_jsonrpc.php", CredentialSealed: sealed,
		Bindings: []RuntimeBinding{
			{ID: "binding-ok", TargetID: "target-ok", ExternalID: "10084"},
			{ID: "binding-down", TargetID: "target-down", ExternalID: "10099"},
		},
	}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewSynchronizer(store, reconciler, problemClient{problems: []zabbix.Problem{{
		EventID: "20427", TriggerID: "15112", Name: "Database unavailable", Severity: 4,
		StartedAt: time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC), HostIDs: []string{"10099"},
	}}}, box, "server-one", nil)
	synchronizer.now = func() time.Time { return time.Date(2026, 8, 14, 12, 1, 0, 0, time.UTC) }

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.completed || store.failed != "" {
		t.Fatalf("unexpected synchronization: completed=%v failed=%q", store.completed, store.failed)
	}
	if len(store.observations) != 2 {
		t.Fatalf("expected an observation per imported host, got %#v", store.observations)
	}
	byBinding := make(map[string]IntegrationObservation, len(store.observations))
	for _, observation := range store.observations {
		byBinding[observation.BindingID] = observation
	}
	if ok := byBinding["binding-ok"]; ok.Outcome != "healthy" {
		t.Fatalf("a host without any problem must conclude healthy: %#v", ok)
	}
	if down := byBinding["binding-down"]; down.Outcome != "unhealthy" {
		t.Fatalf("a host carrying a problem must conclude unhealthy: %#v", down)
	}
}

func (client kumaMonitorClient) Monitors(context.Context, string, string) ([]uptimekuma.Monitor, error) {
	return client.monitors, client.err
}

func TestUptimeKumaSynchronizerProjectsOnlyDownImportedMonitors(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x72}, 32))
	sealed, _ := box.Seal([]byte("uk2-runtime"), "connector:uptime_kuma:https://kuma.example.net/metrics")
	store := &runtimeStore{connectors: []RuntimeConnector{{
		ID: "connector-kuma", Endpoint: "https://kuma.example.net/metrics", CredentialSealed: sealed,
		Bindings: []RuntimeBinding{
			{ID: "binding-down", TargetID: "target-down", ExternalID: "12"},
			{ID: "binding-up", TargetID: "target-up", ExternalID: "13"},
		},
	}}}
	reconciler := &incidentReconciler{}
	responseTime := 148
	synchronizer := NewUptimeKumaSynchronizer(store, reconciler, kumaMonitorClient{monitors: []uptimekuma.Monitor{
		{ID: "12", Name: "Database", Status: 0},
		{ID: "13", Name: "API", Status: 1, ResponseMilliseconds: &responseTime},
		{ID: "99", Name: "Not imported", Status: 0},
	}}, box, "server-one", nil)
	synchronizer.now = func() time.Time { return time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC) }

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.claimedKind != "uptime_kuma" || !store.completed || store.failed != "" || len(reconciler.kumaInput.Signals) != 1 {
		t.Fatalf("unexpected Kuma synchronization: store=%#v input=%#v", store, reconciler.kumaInput)
	}
	if len(reconciler.kumaInput.ObservedBindings) != 2 {
		t.Fatalf("only conclusive imported monitors may resolve incidents: %#v", reconciler.kumaInput)
	}
	projected := reconciler.kumaInput.Signals[0]
	if projected.TargetID != "target-down" || projected.ExternalMonitor != "12" || projected.Severity != incidents.SeverityMajor {
		t.Fatalf("unexpected Kuma signal: %#v", projected)
	}

	// Seul l'état DOWN ouvre un Incident, mais chaque monitor importé produit
	// une Observation : c'est elle qui donne à une Cible importée sa
	// Disponibilité, sa Couverture et sa latence.
	if len(store.observations) != 2 {
		t.Fatalf("expected an observation per imported monitor, got %#v", store.observations)
	}
	byBinding := make(map[string]IntegrationObservation, len(store.observations))
	for _, observation := range store.observations {
		byBinding[observation.BindingID] = observation
	}
	if down := byBinding["binding-down"]; down.Outcome != "unhealthy" || down.LatencyMilliseconds != nil {
		t.Fatalf("a DOWN monitor must conclude unhealthy without latency: %#v", down)
	}
	up := byBinding["binding-up"]
	if up.Outcome != "healthy" || up.LatencyMilliseconds == nil || *up.LatencyMilliseconds != 148 {
		t.Fatalf("an UP monitor must carry the measured response time: %#v", up)
	}
}

// PENDING et MAINTENANCE ne concluent rien : ils n'ouvrent pas d'Incident et ne
// prononcent aucun rétablissement, ils font seulement baisser la Couverture.
func TestUptimeKumaSynchronizerKeepsPendingAndMaintenanceNeutral(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x73}, 32))
	sealed, _ := box.Seal([]byte("uk2-runtime"), "connector:uptime_kuma:https://kuma.example.net/metrics")
	store := &runtimeStore{connectors: []RuntimeConnector{{
		ID: "connector-kuma", Endpoint: "https://kuma.example.net/metrics", CredentialSealed: sealed,
		Bindings: []RuntimeBinding{
			{ID: "binding-pending", TargetID: "target-pending", ExternalID: "20"},
			{ID: "binding-maintenance", TargetID: "target-maintenance", ExternalID: "21"},
		},
	}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewUptimeKumaSynchronizer(store, reconciler, kumaMonitorClient{monitors: []uptimekuma.Monitor{
		{ID: "20", Name: "Pending", Status: 2},
		{ID: "21", Name: "Maintenance", Status: 3},
	}}, box, "server-one", nil)

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reconciler.kumaInput.Signals) != 0 {
		t.Fatalf("neutral states must not open an incident: %#v", reconciler.kumaInput.Signals)
	}
	if len(reconciler.kumaInput.ObservedBindings) != 0 {
		t.Fatalf("neutral states must not resolve an incident: %#v", reconciler.kumaInput)
	}
	for _, observation := range store.observations {
		if observation.Outcome != "unknown" {
			t.Fatalf("a neutral state must conclude nothing: %#v", observation)
		}
	}
	if len(store.observations) != 2 {
		t.Fatalf("expected an observation per imported monitor, got %#v", store.observations)
	}
}

func TestSynchronizerMarksConnectorDegradedWithoutResolvingIncidentsOnRemoteFailure(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x35}, 32))
	sealed, _ := box.Seal([]byte("runtime-token"), "connector:zabbix:https://zabbix.example.net/api_jsonrpc.php")
	store := &runtimeStore{connectors: []RuntimeConnector{{ID: "connector-one", Endpoint: "https://zabbix.example.net/api_jsonrpc.php", CredentialSealed: sealed}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewSynchronizer(store, reconciler, problemClient{err: errors.New("timeout")}, box, "server-one", nil)

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.failed != "timeout" || store.completed || reconciler.input.ConnectorID != "" {
		t.Fatalf("remote failure incorrectly reconciled state: completed=%v failed=%q input=%#v", store.completed, store.failed, reconciler.input)
	}
}
