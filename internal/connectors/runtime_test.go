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
	"github.com/M0okz/cairnops/internal/softwareupdates"
)

type runtimeStore struct {
	discoveries        []DiscoveryObject
	discoveredBindings []RuntimeBinding
	discoveryErr       error
	connectors         []RuntimeConnector
	completed          bool
	failed             string
	claimedKind        string
	observations       []IntegrationObservation
	argusBindings      []ArgusBindingSnapshot
}

func (store *runtimeStore) RefreshDiscovery(_ context.Context, c RuntimeConnector, _, _ string, objects []DiscoveryObject) ([]RuntimeBinding, error) {
	store.discoveries = objects
	if store.discoveryErr != nil {
		return nil, store.discoveryErr
	}
	return append(c.Bindings, store.discoveredBindings...), nil
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
	hosts      []zabbix.Host
	inspectErr error
	problems   []zabbix.Problem
	err        error
}

func (client problemClient) Inspect(context.Context, string, string) (zabbix.Inspection, error) {
	return zabbix.Inspection{Hosts: client.hosts}, client.inspectErr
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

type releaseSecurity map[string]softwareupdates.SecurityAssessment

func (assessments releaseSecurity) SecurityAssessments(context.Context, []string) (map[string]softwareupdates.SecurityAssessment, error) {
	return assessments, nil
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
	}}, releaseSecurity{"binding-api": {Installed: "1.2.2", Target: "1.2.3", Status: softwareupdates.SecurityFixes}}, box, "server-one", nil)
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
	if len(store.argusBindings) != 4 || store.argusBindings[0].ExternalName != "Public API" {
		t.Fatalf("Argus service labels and snapshots must stay mutable: %#v", store.argusBindings)
	}
	if store.argusBindings[3].Metadata["unknown"] != true {
		t.Fatal("removed Argus services must retain an explicitly unknown snapshot")
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
	if len(reconciler.argusInput.Signals) != 1 || reconciler.argusInput.Signals[0].LatestVersion != "1.2.3" || reconciler.argusInput.Signals[0].NatureKey != "software-security-update-available" || reconciler.argusInput.Signals[0].Severity != incidents.SeverityMajor {
		t.Fatalf("expected one active security update signal: %#v", reconciler.argusInput)
	}
	if len(reconciler.argusInput.ObservedBindings) != 2 {
		t.Fatalf("only valid update and skipped states may resolve incidents: %#v", reconciler.argusInput)
	}
}

// Cas relevés en production : une différence de chaînes n'établit pas une
// mise à jour. Ces services restent observés afin de résoudre une preuve
// ouverte par l'ancienne comparaison.
func TestArgusSynchronizerSignalsOnlyOrderedUpdates(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x3c}, 32))
	credential, _ := json.Marshal(argus.Credentials{})
	sealed, _ := box.Seal(credential, "connector:argus:https://argus.example.net")
	services := []argus.Service{
		{ID: "grafana", DeployedVersion: "12.4.9", LatestVersion: "13.2.2"},
		{ID: "mattermost", DeployedVersion: "11.6.0", LatestVersion: "12.0.0-rc1"},
		{ID: "cairnops", DeployedVersion: "0.1.147", LatestVersion: "0.1.146"},
		{ID: "it-tools", DeployedVersion: "2024.10.22", LatestVersion: "2024.10.22-7ca5933"},
		{ID: "custom", DeployedVersion: "stable", LatestVersion: "edge"},
	}
	bindings := make([]RuntimeBinding, 0, len(services))
	for index := range services {
		services[index].Name, services[index].Active, services[index].Importable = services[index].ID, true, true
		services[index].LatestQueryOK, services[index].DeployedQueryOK = true, true
		bindings = append(bindings, RuntimeBinding{ID: "binding-" + services[index].ID, TargetID: "target-" + services[index].ID, ExternalID: services[index].ID})
	}
	store := &runtimeStore{connectors: []RuntimeConnector{{ID: "connector-argus", Endpoint: "https://argus.example.net", CredentialSealed: sealed, Bindings: bindings}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewArgusSynchronizer(store, reconciler, argusInspectionClient{inspection: argus.Inspection{
		Endpoint: "https://argus.example.net", Version: "0.38.0", Compatibility: "supported", Services: services,
	}}, releaseSecurity{"binding-grafana": {Installed: "12.4.9", Target: "13.2.2", Status: softwareupdates.SecurityFixes}}, box, "server-one", nil)
	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reconciler.argusInput.Signals) != 1 || reconciler.argusInput.Signals[0].BindingID != "binding-grafana" {
		t.Fatalf("only the ordered stable update may open an incident: %#v", reconciler.argusInput.Signals)
	}
	if len(reconciler.argusInput.ObservedBindings) != len(services) {
		t.Fatalf("every readable service must be able to resolve an earlier signal: %#v", reconciler.argusInput.ObservedBindings)
	}
	if !store.completed || store.failed != "" {
		t.Fatalf("readable services do not degrade the connector: %#v", store)
	}
	situations := map[string]any{}
	for _, snapshot := range store.argusBindings {
		situations[snapshot.BindingID] = snapshot.Metadata["situation"]
	}
	for binding, want := range map[string]string{"binding-grafana": "update", "binding-mattermost": "prerelease", "binding-cairnops": "target_older", "binding-it-tools": "current", "binding-custom": "unordered"} {
		if situations[binding] != want {
			t.Errorf("%s situation = %v, want %s", binding, situations[binding], want)
		}
	}
}

// Une mise à jour disponible n'est pas un problème : seule une faille corrigée,
// établie par l'analyse des notes de la comparaison observée, ouvre une preuve.
func TestArgusSynchronizerOpensIncidentsOnlyForSecurityFixes(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x4d}, 32))
	credential, _ := json.Marshal(argus.Credentials{})
	sealed, _ := box.Seal(credential, "connector:argus:https://argus.example.net")
	services := []argus.Service{
		{ID: "plain", DeployedVersion: "1.0.0", LatestVersion: "1.1.0"},
		{ID: "secure", DeployedVersion: "2.0.0", LatestVersion: "2.0.1"},
		{ID: "analysing", DeployedVersion: "3.0.0", LatestVersion: "3.1.0"},
		{ID: "stale", DeployedVersion: "4.0.0", LatestVersion: "4.2.0"},
		{ID: "untracked", DeployedVersion: "5.0.0", LatestVersion: "5.1.0"},
	}
	bindings := make([]RuntimeBinding, 0, len(services))
	for index := range services {
		services[index].Name, services[index].Active, services[index].Importable = services[index].ID, true, true
		services[index].LatestQueryOK, services[index].DeployedQueryOK = true, true
		bindings = append(bindings, RuntimeBinding{ID: "binding-" + services[index].ID, TargetID: "target-" + services[index].ID, ExternalID: services[index].ID})
	}
	store := &runtimeStore{connectors: []RuntimeConnector{{ID: "connector-argus", Endpoint: "https://argus.example.net", CredentialSealed: sealed, Bindings: bindings}}}
	reconciler := &incidentReconciler{}
	synchronizer := NewArgusSynchronizer(store, reconciler, argusInspectionClient{inspection: argus.Inspection{
		Endpoint: "https://argus.example.net", Version: "0.38.0", Compatibility: "supported", Services: services,
	}}, releaseSecurity{
		"binding-plain":     {Installed: "1.0.0", Target: "1.1.0", Status: softwareupdates.SecurityNotEstablished},
		"binding-secure":    {Installed: "2.0.0", Target: "2.0.1", Status: softwareupdates.SecurityFixes},
		"binding-analysing": {Installed: "3.0.0", Target: "3.1.0", Status: softwareupdates.SecurityPending},
		// Verdict d'une comparaison précédente : il ne vaut pas pour 4.2.0.
		"binding-stale": {Installed: "4.0.0", Target: "4.1.0", Status: softwareupdates.SecurityFixes},
	}, box, "server-one", nil)
	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reconciler.argusInput.Signals) != 1 {
		t.Fatalf("only the security fix may open an incident: %#v", reconciler.argusInput.Signals)
	}
	signal := reconciler.argusInput.Signals[0]
	if signal.BindingID != "binding-secure" || signal.NatureKey != "software-security-update-available" || signal.NatureLabel != "Mise à jour de sécurité disponible" || signal.Severity != incidents.SeverityMajor {
		t.Fatalf("unexpected security update signal: %#v", signal)
	}
	observed := map[string]bool{}
	for _, binding := range reconciler.argusInput.ObservedBindings {
		observed[binding] = true
	}
	for binding, want := range map[string]bool{"binding-plain": true, "binding-secure": true, "binding-analysing": false, "binding-stale": false, "binding-untracked": true} {
		if observed[binding] != want {
			t.Errorf("%s observed = %v, want %v: an analysis in progress must neither open nor resolve", binding, observed[binding], want)
		}
	}
	outcomes := map[string]IntegrationObservation{}
	for _, observation := range store.observations {
		outcomes[observation.BindingID] = observation
	}
	if got := outcomes["binding-plain"]; got.Outcome != "healthy" || got.Reason != "argus_update_available" {
		t.Errorf("an ordinary update is information, not a failure: %#v", got)
	}
	if got := outcomes["binding-secure"]; got.Outcome != "unhealthy" || got.Reason != "argus_security_update_available" {
		t.Errorf("a security fix must be reported as needing attention: %#v", got)
	}
	if !store.completed || store.failed != "" {
		t.Fatalf("security qualification does not degrade the connector: %#v", store)
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
		CanonicalNature: incidents.NatureAvailability, EvaluationWindow: 15 * time.Minute,
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
	if projected.EvaluationWindow != 15*time.Minute {
		t.Fatalf("synchronizer lost the condition period: %s", projected.EvaluationWindow)
	}

	// Un hôte importé qui porte une indisponibilité active conclut une
	// Observation en défaut : c'est elle qui donne à la Cible sa Disponibilité
	// et sa Couverture, comme un monitor Uptime Kuma DOWN.
	if len(store.observations) != 1 {
		t.Fatalf("expected an observation per imported host, got %#v", store.observations)
	}
	observation := store.observations[0]
	if observation.BindingID != "binding-one" || observation.Outcome != "unhealthy" || observation.LatencyMilliseconds != nil {
		t.Fatalf("an active Zabbix unavailability must conclude unhealthy without latency: %#v", observation)
	}
}

type kumaMonitorClient struct {
	monitors []uptimekuma.Monitor
	err      error
}

// Sans indisponibilité active, un hôte importé conclut au bon fonctionnement :
// c'est cette Observation « healthy » qui remplit la Couverture d'une Cible
// Zabbix et met à jour sa fraîcheur. Un problème d'une autre Nature reste un
// problème de la Ressource, mais ne mesure pas sa Disponibilité (ADR 0039).
func TestSynchronizerMeasuresHealthyHostsWithoutProblems(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x52}, 32))
	sealed, _ := box.Seal([]byte("runtime-token"), "connector:zabbix:https://zabbix.example.net/api_jsonrpc.php")
	store := &runtimeStore{connectors: []RuntimeConnector{{
		ID: "connector-one", Endpoint: "https://zabbix.example.net/api_jsonrpc.php", CredentialSealed: sealed,
		Bindings: []RuntimeBinding{
			{ID: "binding-ok", TargetID: "target-ok", ExternalID: "10084"},
			{ID: "binding-down", TargetID: "target-down", ExternalID: "10099"},
			{ID: "binding-other", TargetID: "target-other", ExternalID: "10100"},
			{ID: "binding-maintenance", TargetID: "target-maintenance", ExternalID: "10101"},
		},
	}}}
	reconciler := &incidentReconciler{}
	startedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	synchronizer := NewSynchronizer(store, reconciler, problemClient{problems: []zabbix.Problem{{
		EventID: "20427", TriggerID: "15112", Name: "Database unavailable", Severity: 4,
		CanonicalNature: incidents.NatureAvailability, StartedAt: startedAt, HostIDs: []string{"10099"},
	}, {
		EventID: "20428", TriggerID: "15113", Name: "Traefik: Deployed version unavailable for 15 minutes", Severity: 4,
		StartedAt: startedAt, HostIDs: []string{"10100"},
	}, {
		EventID: "20429", TriggerID: "15112", Name: "Database unavailable", Severity: 4,
		CanonicalNature: incidents.NatureAvailability, Suppressed: true, StartedAt: startedAt, HostIDs: []string{"10101"},
	}}}, box, "server-one", nil)
	synchronizer.now = func() time.Time { return time.Date(2026, 8, 14, 12, 1, 0, 0, time.UTC) }

	if err := synchronizer.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.completed || store.failed != "" {
		t.Fatalf("unexpected synchronization: completed=%v failed=%q", store.completed, store.failed)
	}
	if len(store.observations) != 4 {
		t.Fatalf("expected an observation per imported host, got %#v", store.observations)
	}
	if len(reconciler.input.Signals) != 3 {
		t.Fatalf("every active problem must still reach the incident cycle: %#v", reconciler.input.Signals)
	}
	byBinding := make(map[string]IntegrationObservation, len(store.observations))
	for _, observation := range store.observations {
		byBinding[observation.BindingID] = observation
	}
	if ok := byBinding["binding-ok"]; ok.Outcome != "healthy" {
		t.Fatalf("a host without any problem must conclude healthy: %#v", ok)
	}
	if down := byBinding["binding-down"]; down.Outcome != "unhealthy" || down.Reason != "zabbix_unavailability_active" {
		t.Fatalf("a host carrying an unavailability must conclude unhealthy: %#v", down)
	}
	if other := byBinding["binding-other"]; other.Outcome != "healthy" {
		t.Fatalf("a problem of another nature must not measure unavailability: %#v", other)
	}
	if maintenance := byBinding["binding-maintenance"]; maintenance.Outcome != "unknown" || maintenance.Reason != "zabbix_problem_suppressed" {
		t.Fatalf("a suppressed unavailability must stay neutral: %#v", maintenance)
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

func TestZabbixDiscoveryObservesNewBindingDuringSameCycle(t *testing.T) {
	box, _ := secretbox.New(bytes.Repeat([]byte{0x71}, 32))
	endpoint := "https://zabbix.example.test/api_jsonrpc.php"
	credential, _ := box.Seal([]byte("token"), "connector:zabbix:"+endpoint)
	store := &runtimeStore{discoveredBindings: []RuntimeBinding{{ID: "new-binding", TargetID: "new-target", ExternalID: "100"}}}
	reconciler := &incidentReconciler{}
	client := problemClient{hosts: []zabbix.Host{{ID: "100", Name: "New host"}}, problems: []zabbix.Problem{{HostIDs: []string{"100"}, EventID: "event", Name: "Failure", Severity: 3}}}
	NewSynchronizer(store, reconciler, client, box, "worker", nil).syncOne(context.Background(), RuntimeConnector{ID: "connector", Endpoint: endpoint, CredentialSealed: credential})
	if !store.completed || len(store.discoveries) != 1 || len(reconciler.input.Signals) != 1 || reconciler.input.Signals[0].TargetID != "new-target" {
		t.Fatalf("new host was not supervised: store=%+v signals=%+v", store, reconciler.input.Signals)
	}
}

func TestFailedZabbixInventoryDoesNotDiscoverOrResolve(t *testing.T) {
	box, _ := secretbox.New(bytes.Repeat([]byte{0x72}, 32))
	endpoint := "https://zabbix.example.test/api_jsonrpc.php"
	credential, _ := box.Seal([]byte("token"), "connector:zabbix:"+endpoint)
	store := &runtimeStore{}
	reconciler := &incidentReconciler{}
	NewSynchronizer(store, reconciler, problemClient{inspectErr: errors.New("incomplete inventory")}, box, "worker", nil).syncOne(context.Background(), RuntimeConnector{ID: "connector", Endpoint: endpoint, CredentialSealed: credential})
	if store.failed == "" || store.completed || store.discoveries != nil || reconciler.input.ConnectorID != "" {
		t.Fatalf("failed inventory changed supervision: %+v", store)
	}
}

func TestDiscoveryFailureStopsEveryFamilyBeforeReconciliation(t *testing.T) {
	for _, kind := range []string{"zabbix", "uptime_kuma", "patchmon", "argus"} {
		t.Run(kind, func(t *testing.T) {
			box, _ := secretbox.New(bytes.Repeat([]byte{0x73}, 32))
			endpoint := "https://example.test"
			plain := []byte("token")
			if kind == "patchmon" {
				plain = []byte(`{"key":"key","secret":"secret"}`)
			}
			if kind == "argus" {
				plain = []byte(`{"username":"user","password":"secret"}`)
			}
			credential, _ := box.Seal(plain, "connector:"+kind+":"+endpoint)
			c := RuntimeConnector{ID: "connector", Endpoint: endpoint, CredentialSealed: credential}
			store := &runtimeStore{discoveryErr: errors.New("expired lease")}
			reconciler := &incidentReconciler{}
			switch kind {
			case "zabbix":
				NewSynchronizer(store, reconciler, problemClient{}, box, "worker", nil).syncOne(context.Background(), c)
			case "uptime_kuma":
				NewUptimeKumaSynchronizer(store, reconciler, kumaMonitorClient{}, box, "worker", nil).syncOne(context.Background(), c)
			case "patchmon":
				NewPatchMonSynchronizer(store, reconciler, patchMonHostClient{}, box, "worker", nil).syncOne(context.Background(), c)
			case "argus":
				NewArgusSynchronizer(store, reconciler, argusInspectionClient{}, releaseSecurity{}, box, "worker", nil).syncOne(context.Background(), c)
			}
			if store.failed == "" || store.completed || len(store.observations) != 0 || reconciler.input.ConnectorID != "" || reconciler.kumaInput.ConnectorID != "" || reconciler.patchMonInput.ConnectorID != "" || reconciler.argusInput.ConnectorID != "" {
				t.Fatalf("discovery failure changed supervision: %+v", store)
			}
		})
	}
}
