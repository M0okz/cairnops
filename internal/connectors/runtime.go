package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/argus"
	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type RuntimeBinding struct {
	ID           string
	TargetID     string
	ExternalID   string
	ExternalName string
	Metadata     map[string]any
}

type RuntimeConnector struct {
	ID               string
	Endpoint         string
	CredentialSealed string
	Bindings         []RuntimeBinding
}

type RuntimeCredential struct {
	Kind             string
	Endpoint         string
	CredentialSealed string
}

// IntegrationObservation est ce qu'un cycle de synchronisation a constaté sur
// une Source apportée par une Intégration. Elle alimente la mesure — jamais la
// Politique de déclenchement, puisque l'Incident vient déjà de l'Intégration.
type IntegrationObservation struct {
	BindingID           string
	Outcome             string
	LatencyMilliseconds *int
	Reason              string
	Message             string
	Details             map[string]any
}

type ArgusBindingSnapshot struct {
	BindingID    string
	ExternalName string
	Metadata     map[string]any
}

type RuntimeStore interface {
	ClaimDueConnector(context.Context, string, string, int, time.Duration) ([]RuntimeConnector, error)
	CompleteConnectorSync(context.Context, string, string, time.Time) error
	FailConnectorSync(context.Context, string, string, time.Time, string) error
	RecordIntegrationObservations(context.Context, time.Time, []IntegrationObservation) error
	UpdateArgusBindings(context.Context, string, []ArgusBindingSnapshot) error
}

type IncidentReconciler interface {
	ReconcileZabbix(context.Context, incidents.ReconcileZabbixInput) error
}

type ProblemClient interface {
	Problems(context.Context, string, string, []string) ([]zabbix.Problem, error)
}

type Synchronizer struct {
	store        RuntimeStore
	incidents    IncidentReconciler
	zabbix       ProblemClient
	secrets      *secretbox.Box
	owner        string
	logger       *slog.Logger
	pollInterval time.Duration
	lease        time.Duration
	batchSize    int
	parallelism  int
	now          func() time.Time
}

func NewSynchronizer(store RuntimeStore, incidentStore IncidentReconciler, client ProblemClient, secrets *secretbox.Box, owner string, logger *slog.Logger) *Synchronizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Synchronizer{
		store: store, incidents: incidentStore, zabbix: client, secrets: secrets,
		owner: owner, logger: logger, pollInterval: 2 * time.Second,
		lease: time.Minute, batchSize: 8, parallelism: 4, now: time.Now,
	}
}

func (synchronizer *Synchronizer) Run(ctx context.Context) error {
	if err := synchronizer.tick(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(synchronizer.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := synchronizer.tick(ctx); err != nil {
				return err
			}
		}
	}
}

func (synchronizer *Synchronizer) tick(ctx context.Context) error {
	connectors, err := synchronizer.store.ClaimDueConnector(ctx, "zabbix", synchronizer.owner, synchronizer.batchSize, synchronizer.lease)
	if err != nil {
		return fmt.Errorf("claim due Zabbix connectors: %w", err)
	}
	semaphore := make(chan struct{}, synchronizer.parallelism)
	var waitGroup sync.WaitGroup
	for _, connector := range connectors {
		connector := connector
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			synchronizer.syncOne(ctx, connector)
		}()
	}
	waitGroup.Wait()
	return nil
}

func (synchronizer *Synchronizer) syncOne(ctx context.Context, connector RuntimeConnector) {
	failed := func(cause error) {
		message := strings.TrimSpace(cause.Error())
		if len(message) > 500 {
			message = message[:500]
		}
		if err := synchronizer.store.FailConnectorSync(ctx, connector.ID, synchronizer.owner, synchronizer.now().UTC(), message); err != nil {
			synchronizer.logger.Error("record Zabbix synchronization failure", "connector_id", connector.ID, "error", err)
		}
		synchronizer.logger.Warn("Zabbix synchronization failed", "connector_id", connector.ID, "error", cause)
	}
	credential, err := synchronizer.secrets.Open(connector.CredentialSealed, "connector:zabbix:"+connector.Endpoint)
	if err != nil {
		failed(fmt.Errorf("open connector credential: %w", err))
		return
	}
	hostIDs := make([]string, 0, len(connector.Bindings))
	bindingByHost := make(map[string]RuntimeBinding, len(connector.Bindings))
	for _, binding := range connector.Bindings {
		hostIDs = append(hostIDs, binding.ExternalID)
		bindingByHost[binding.ExternalID] = binding
	}
	sort.Strings(hostIDs)
	problems, err := synchronizer.zabbix.Problems(ctx, connector.Endpoint, string(credential), hostIDs)
	if err != nil {
		failed(err)
		return
	}
	observedAt := synchronizer.now().UTC()
	signals := make([]incidents.ZabbixSignal, 0)
	// Chaque hôte importé conclut une Observation : c'est elle qui donne à une
	// Cible découverte par Zabbix sa Disponibilité, sa Couverture et sa
	// fraîcheur, exactement comme un monitor Uptime Kuma. Un problème actif
	// conclut à la défaillance ; un problème supprimé — une maintenance côté
	// Zabbix — reste neutre, comme la maintenance Uptime Kuma ; l'absence de
	// problème conclut au bon fonctionnement.
	outcomeByBinding := make(map[string]string, len(connector.Bindings))
	for _, binding := range connector.Bindings {
		outcomeByBinding[binding.ID] = "healthy"
	}
	for _, problem := range problems {
		for _, hostID := range problem.HostIDs {
			binding, ok := bindingByHost[hostID]
			if !ok {
				continue
			}
			if problem.Suppressed {
				if outcomeByBinding[binding.ID] == "healthy" {
					outcomeByBinding[binding.ID] = "unknown"
				}
			} else {
				outcomeByBinding[binding.ID] = "unhealthy"
			}
			signals = append(signals, incidents.ZabbixSignal{
				TargetID: binding.TargetID, BindingID: binding.ID,
				ExternalEventID: problem.EventID, ExternalObjectID: problem.TriggerID,
				Name: problem.Name, Severity: zabbixSeverity(problem.Severity),
				OpenedAt: problem.StartedAt, UpstreamAcknowledged: problem.Acknowledged,
				Suppressed: problem.Suppressed,
			})
		}
	}
	observations := make([]IntegrationObservation, 0, len(connector.Bindings))
	for _, binding := range connector.Bindings {
		observations = append(observations, zabbixObservation(binding.ID, outcomeByBinding[binding.ID]))
	}
	if err := synchronizer.incidents.ReconcileZabbix(ctx, incidents.ReconcileZabbixInput{
		ConnectorID: connector.ID, ObservedAt: observedAt, Signals: signals,
	}); err != nil {
		failed(err)
		return
	}
	// La mesure ne commande pas la synchronisation : une Observation qui ne
	// s'enregistre pas laisse un trou dans la Couverture, elle ne dégrade pas
	// le Connecteur ni ne réécrit l'Incident déjà rapproché.
	if err := synchronizer.store.RecordIntegrationObservations(ctx, observedAt, observations); err != nil {
		synchronizer.logger.Warn("record Zabbix observations", "connector_id", connector.ID, "error", err)
	}
	if err := synchronizer.store.CompleteConnectorSync(ctx, connector.ID, synchronizer.owner, observedAt); err != nil {
		synchronizer.logger.Error("complete Zabbix synchronization", "connector_id", connector.ID, "error", err)
		return
	}
	synchronizer.logger.Info("Zabbix synchronization completed", "connector_id", connector.ID, "problems", len(problems))
}

type UptimeKumaIncidentReconciler interface {
	ReconcileUptimeKuma(context.Context, incidents.ReconcileUptimeKumaInput) error
}

type UptimeKumaMonitorClient interface {
	Monitors(context.Context, string, string) ([]uptimekuma.Monitor, error)
}

type PatchMonIncidentReconciler interface {
	ReconcilePatchMon(context.Context, incidents.ReconcilePatchMonInput) error
}

type PatchMonHostClient interface {
	Hosts(context.Context, string, patchmon.Credentials) ([]patchmon.Host, error)
}

type PatchMonSynchronizer struct {
	store        RuntimeStore
	incidents    PatchMonIncidentReconciler
	client       PatchMonHostClient
	secrets      *secretbox.Box
	owner        string
	logger       *slog.Logger
	pollInterval time.Duration
	lease        time.Duration
	batchSize    int
	parallelism  int
	now          func() time.Time
}

func NewPatchMonSynchronizer(store RuntimeStore, incidentStore PatchMonIncidentReconciler, client PatchMonHostClient, secrets *secretbox.Box, owner string, logger *slog.Logger) *PatchMonSynchronizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &PatchMonSynchronizer{
		store: store, incidents: incidentStore, client: client, secrets: secrets,
		owner: owner, logger: logger, pollInterval: 2 * time.Second,
		lease: time.Minute, batchSize: 8, parallelism: 4, now: time.Now,
	}
}

func (synchronizer *PatchMonSynchronizer) Run(ctx context.Context) error {
	if err := synchronizer.tick(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(synchronizer.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := synchronizer.tick(ctx); err != nil {
				return err
			}
		}
	}
}

func (synchronizer *PatchMonSynchronizer) tick(ctx context.Context) error {
	claimed, err := synchronizer.store.ClaimDueConnector(ctx, "patchmon", synchronizer.owner, synchronizer.batchSize, synchronizer.lease)
	if err != nil {
		return fmt.Errorf("claim due PatchMon connectors: %w", err)
	}
	semaphore := make(chan struct{}, synchronizer.parallelism)
	var waitGroup sync.WaitGroup
	for _, connector := range claimed {
		connector := connector
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			synchronizer.syncOne(ctx, connector)
		}()
	}
	waitGroup.Wait()
	return nil
}

func (synchronizer *PatchMonSynchronizer) syncOne(ctx context.Context, connector RuntimeConnector) {
	failed := func(cause error) {
		message := strings.TrimSpace(cause.Error())
		if len(message) > 500 {
			message = message[:500]
		}
		if err := synchronizer.store.FailConnectorSync(ctx, connector.ID, synchronizer.owner, synchronizer.now().UTC(), message); err != nil {
			synchronizer.logger.Error("record PatchMon synchronization failure", "connector_id", connector.ID, "error", err)
		}
		synchronizer.logger.Warn("PatchMon synchronization failed", "connector_id", connector.ID, "error", cause)
	}
	credential, err := synchronizer.secrets.Open(connector.CredentialSealed, "connector:patchmon:"+connector.Endpoint)
	if err != nil {
		failed(fmt.Errorf("open connector credential: %w", err))
		return
	}
	var credentials patchmon.Credentials
	if err := json.Unmarshal(credential, &credentials); err != nil {
		failed(fmt.Errorf("decode connector credential: %w", err))
		return
	}
	hosts, err := synchronizer.client.Hosts(ctx, connector.Endpoint, credentials)
	if err != nil {
		failed(err)
		return
	}
	bindingByHost := make(map[string]RuntimeBinding, len(connector.Bindings))
	for _, binding := range connector.Bindings {
		bindingByHost[binding.ExternalID] = binding
	}
	observedAt := synchronizer.now().UTC()
	observedBindings := make([]string, 0, len(connector.Bindings))
	observations := make([]IntegrationObservation, 0, len(connector.Bindings))
	signals := make([]incidents.PatchMonSignal, 0, len(connector.Bindings)*2)
	for _, host := range hosts {
		binding, imported := bindingByHost[host.ID]
		if !imported {
			continue
		}
		observedBindings = append(observedBindings, binding.ID)
		details := patchMonDetails(host)
		observations = append(observations, patchMonObservation(binding.ID, host, details))
		if host.SecurityUpdatesCount > 0 || host.UpdateState == "security_required" {
			signals = append(signals, incidents.PatchMonSignal{
				TargetID: binding.TargetID, BindingID: binding.ID, ExternalHost: host.ID,
				ConditionKey: "security_updates", NatureKey: "security-patches-required",
				NatureLabel: "Correctifs de sécurité requis",
				Name:        fmt.Sprintf("%s · %d correctif(s) de sécurité", host.Name(), host.SecurityUpdatesCount),
				Severity:    incidents.SeverityMajor, Details: details,
			})
		}
		if host.NeedsReboot {
			signals = append(signals, incidents.PatchMonSignal{
				TargetID: binding.TargetID, BindingID: binding.ID, ExternalHost: host.ID,
				ConditionKey: "reboot_required", NatureKey: "reboot-required",
				NatureLabel: "Redémarrage requis",
				Name:        host.Name() + " · redémarrage requis après correctifs",
				Severity:    incidents.SeverityWarning, Details: details,
			})
		}
	}
	if err := synchronizer.incidents.ReconcilePatchMon(ctx, incidents.ReconcilePatchMonInput{
		ConnectorID: connector.ID, ObservedAt: observedAt,
		ObservedBindings: observedBindings, Signals: signals,
	}); err != nil {
		failed(err)
		return
	}
	if err := synchronizer.store.RecordIntegrationObservations(ctx, observedAt, observations); err != nil {
		synchronizer.logger.Warn("record PatchMon observations", "connector_id", connector.ID, "error", err)
	}
	if err := synchronizer.store.CompleteConnectorSync(ctx, connector.ID, synchronizer.owner, observedAt); err != nil {
		synchronizer.logger.Error("complete PatchMon synchronization", "connector_id", connector.ID, "error", err)
		return
	}
	synchronizer.logger.Info("PatchMon synchronization completed", "connector_id", connector.ID, "hosts", len(hosts))
}

func patchMonDetails(host patchmon.Host) map[string]any {
	return map[string]any{
		"host_id": host.ID, "hostname": host.Hostname, "ip": host.IP,
		"os_type": host.OSType, "os_version": host.OSVersion,
		"status":          host.Status,
		"reporting_state": host.ReportingState, "update_state": host.UpdateState,
		"updates_count": host.UpdatesCount, "security_updates_count": host.SecurityUpdatesCount,
		"needs_reboot": host.NeedsReboot,
	}
}

func patchMonObservation(bindingID string, host patchmon.Host, details map[string]any) IntegrationObservation {
	observation := IntegrationObservation{
		BindingID: bindingID, Outcome: "healthy", Details: details,
		Message: fmt.Sprintf("%d mise(s) à jour, dont %d de sécurité", host.UpdatesCount, host.SecurityUpdatesCount),
	}
	if host.NeedsReboot {
		observation.Message += " · redémarrage requis"
	}
	status := strings.ToLower(strings.TrimSpace(host.Status))
	if status != "" && status != "active" {
		observation.Outcome = "unknown"
		observation.Reason = "patchmon_host_" + status
	} else if host.SecurityUpdatesCount > 0 || host.NeedsReboot {
		observation.Outcome = "unhealthy"
		observation.Reason = "patchmon_attention_required"
	}
	return observation
}

type ArgusIncidentReconciler interface {
	ReconcileArgus(context.Context, incidents.ReconcileArgusInput) error
}

type ArgusInspectionClient interface {
	Inspect(context.Context, string, argus.Credentials) (argus.Inspection, error)
}

type ArgusSynchronizer struct {
	store        RuntimeStore
	incidents    ArgusIncidentReconciler
	client       ArgusInspectionClient
	secrets      *secretbox.Box
	owner        string
	logger       *slog.Logger
	pollInterval time.Duration
	lease        time.Duration
	batchSize    int
	parallelism  int
	now          func() time.Time
}

func NewArgusSynchronizer(store RuntimeStore, incidentStore ArgusIncidentReconciler, client ArgusInspectionClient, secrets *secretbox.Box, owner string, logger *slog.Logger) *ArgusSynchronizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &ArgusSynchronizer{
		store: store, incidents: incidentStore, client: client, secrets: secrets,
		owner: owner, logger: logger, pollInterval: 2 * time.Second,
		lease: time.Minute, batchSize: 8, parallelism: 4, now: time.Now,
	}
}

func (synchronizer *ArgusSynchronizer) Run(ctx context.Context) error {
	if err := synchronizer.tick(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(synchronizer.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := synchronizer.tick(ctx); err != nil {
				return err
			}
		}
	}
}

func (synchronizer *ArgusSynchronizer) tick(ctx context.Context) error {
	claimed, err := synchronizer.store.ClaimDueConnector(ctx, "argus", synchronizer.owner, synchronizer.batchSize, synchronizer.lease)
	if err != nil {
		return fmt.Errorf("claim due Argus connectors: %w", err)
	}
	semaphore := make(chan struct{}, synchronizer.parallelism)
	var waitGroup sync.WaitGroup
	for _, connector := range claimed {
		connector := connector
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			synchronizer.syncOne(ctx, connector)
		}()
	}
	waitGroup.Wait()
	return nil
}

func (synchronizer *ArgusSynchronizer) syncOne(ctx context.Context, connector RuntimeConnector) {
	fail := func(at time.Time, cause error) {
		message := strings.TrimSpace(cause.Error())
		if len(message) > 500 {
			message = message[:500]
		}
		if err := synchronizer.store.FailConnectorSync(ctx, connector.ID, synchronizer.owner, at, message); err != nil {
			synchronizer.logger.Error("record Argus synchronization failure", "connector_id", connector.ID, "error", err)
		}
		synchronizer.logger.Warn("Argus synchronization degraded", "connector_id", connector.ID, "error", cause)
	}
	credential, err := synchronizer.secrets.Open(connector.CredentialSealed, "connector:argus:"+connector.Endpoint)
	if err != nil {
		fail(synchronizer.now().UTC(), fmt.Errorf("open connector credential: %w", err))
		return
	}
	var credentials argus.Credentials
	if err := json.Unmarshal(credential, &credentials); err != nil {
		fail(synchronizer.now().UTC(), fmt.Errorf("decode connector credential: %w", err))
		return
	}
	inspection, err := synchronizer.client.Inspect(ctx, connector.Endpoint, credentials)
	if err != nil {
		fail(synchronizer.now().UTC(), err)
		return
	}
	serviceByID := make(map[string]argus.Service, len(inspection.Services))
	for _, discoveredService := range inspection.Services {
		serviceByID[discoveredService.ID] = discoveredService
	}
	observedAt := synchronizer.now().UTC()
	observations := make([]IntegrationObservation, 0, len(connector.Bindings))
	bindingSnapshots := make([]ArgusBindingSnapshot, 0, len(connector.Bindings))
	observedBindings := make([]string, 0, len(connector.Bindings))
	signals := make([]incidents.ArgusSignal, 0, len(connector.Bindings))
	unknownCount := 0
	for _, binding := range connector.Bindings {
		discoveredService, found := serviceByID[binding.ExternalID]
		if !found {
			unknownCount++
			details := argusMissingDetails(inspection.Endpoint, binding)
			observations = append(observations, IntegrationObservation{
				BindingID: binding.ID, Outcome: "unknown", Reason: "argus_service_missing",
				Message: "Service absent de la configuration Argus active",
				Details: details,
			})
			continue
		}
		details := argusDetails(inspection.Endpoint, discoveredService)
		bindingSnapshots = append(bindingSnapshots, ArgusBindingSnapshot{
			BindingID: binding.ID, ExternalName: discoveredService.Name, Metadata: details,
		})
		if !discoveredService.Importable || discoveredService.Unknown {
			unknownCount++
			reason := discoveredService.UnknownReason
			if reason == "" {
				reason = "argus_service_" + discoveredService.Ineligibility
			}
			observations = append(observations, IntegrationObservation{
				BindingID: binding.ID, Outcome: "unknown", Reason: reason,
				Message: "État de version Argus inconnu", Details: details,
			})
			continue
		}
		observedBindings = append(observedBindings, binding.ID)
		pending := !discoveredService.Skipped && discoveredService.DeployedVersion != discoveredService.LatestVersion
		outcome, reason, message := "healthy", "", "Version déployée à jour"
		if discoveredService.Skipped {
			message = "Version ignorée dans Argus"
		} else if pending {
			outcome, reason = "unhealthy", "argus_update_available"
			message = fmt.Sprintf("Version %s disponible, %s déployée", discoveredService.LatestVersion, discoveredService.DeployedVersion)
			signals = append(signals, incidents.ArgusSignal{
				TargetID: binding.TargetID, BindingID: binding.ID, ExternalService: discoveredService.ID,
				NatureKey: "software-update-available", NatureLabel: "Mise à jour logicielle disponible",
				Name: discoveredService.Name + " · " + message, Severity: incidents.SeverityWarning,
				DeployedVersion: discoveredService.DeployedVersion, LatestVersion: discoveredService.LatestVersion,
				Details: details,
			})
		}
		observations = append(observations, IntegrationObservation{
			BindingID: binding.ID, Outcome: outcome, Reason: reason, Message: message, Details: details,
		})
	}
	if err := synchronizer.store.UpdateArgusBindings(ctx, connector.ID, bindingSnapshots); err != nil {
		fail(observedAt, err)
		return
	}
	if err := synchronizer.incidents.ReconcileArgus(ctx, incidents.ReconcileArgusInput{
		ConnectorID: connector.ID, ObservedAt: observedAt,
		ObservedBindings: observedBindings, Signals: signals,
	}); err != nil {
		fail(observedAt, err)
		return
	}
	if err := synchronizer.store.RecordIntegrationObservations(ctx, observedAt, observations); err != nil {
		synchronizer.logger.Warn("record Argus observations", "connector_id", connector.ID, "error", err)
	}
	if unknownCount > 0 {
		label := "Sources Argus inconnues"
		if unknownCount == 1 {
			label = "Source Argus inconnue"
		}
		fail(observedAt, fmt.Errorf("%d %s", unknownCount, label))
		return
	}
	if err := synchronizer.store.CompleteConnectorSync(ctx, connector.ID, synchronizer.owner, observedAt); err != nil {
		synchronizer.logger.Error("complete Argus synchronization", "connector_id", connector.ID, "error", err)
		return
	}
	synchronizer.logger.Info("Argus synchronization completed", "connector_id", connector.ID, "services", len(connector.Bindings))
}

func argusMissingDetails(endpoint string, binding RuntimeBinding) map[string]any {
	details := make(map[string]any, len(binding.Metadata)+11)
	for key, value := range binding.Metadata {
		details[key] = value
	}
	details["service_id"] = binding.ExternalID
	details["service_name"] = binding.ExternalName
	details["argus_url"] = endpoint
	for key, fallback := range map[string]any{
		"deployed_version": "", "latest_version": "", "approved": false, "skipped": false,
		"last_checked": "", "latest_version_query_ok": false, "deployed_version_query_ok": false,
		"version_url": "",
	} {
		if _, exists := details[key]; !exists {
			details[key] = fallback
		}
	}
	return details
}

func argusDetails(endpoint string, service argus.Service) map[string]any {
	return map[string]any{
		"service_id": service.ID, "service_name": service.Name,
		"deployed_version": service.DeployedVersion, "latest_version": service.LatestVersion,
		"approved": service.Approved, "skipped": service.Skipped,
		"last_checked": service.LastChecked, "deployment_state": service.DeploymentState,
		"latest_version_query_ok":   service.LatestQueryOK,
		"deployed_version_query_ok": service.DeployedQueryOK,
		"argus_url":                 endpoint, "version_url": service.VersionURL,
	}
}

type UptimeKumaSynchronizer struct {
	store        RuntimeStore
	incidents    UptimeKumaIncidentReconciler
	client       UptimeKumaMonitorClient
	secrets      *secretbox.Box
	owner        string
	logger       *slog.Logger
	pollInterval time.Duration
	lease        time.Duration
	batchSize    int
	parallelism  int
	now          func() time.Time
}

func NewUptimeKumaSynchronizer(store RuntimeStore, incidentStore UptimeKumaIncidentReconciler, client UptimeKumaMonitorClient, secrets *secretbox.Box, owner string, logger *slog.Logger) *UptimeKumaSynchronizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &UptimeKumaSynchronizer{
		store: store, incidents: incidentStore, client: client, secrets: secrets,
		owner: owner, logger: logger, pollInterval: 2 * time.Second,
		lease: time.Minute, batchSize: 8, parallelism: 4, now: time.Now,
	}
}

func (synchronizer *UptimeKumaSynchronizer) Run(ctx context.Context) error {
	if err := synchronizer.tick(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(synchronizer.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := synchronizer.tick(ctx); err != nil {
				return err
			}
		}
	}
}

func (synchronizer *UptimeKumaSynchronizer) tick(ctx context.Context) error {
	connectors, err := synchronizer.store.ClaimDueConnector(ctx, "uptime_kuma", synchronizer.owner, synchronizer.batchSize, synchronizer.lease)
	if err != nil {
		return fmt.Errorf("claim due Uptime Kuma connectors: %w", err)
	}
	semaphore := make(chan struct{}, synchronizer.parallelism)
	var waitGroup sync.WaitGroup
	for _, connector := range connectors {
		connector := connector
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			synchronizer.syncOne(ctx, connector)
		}()
	}
	waitGroup.Wait()
	return nil
}

func (synchronizer *UptimeKumaSynchronizer) syncOne(ctx context.Context, connector RuntimeConnector) {
	failed := func(cause error) {
		message := strings.TrimSpace(cause.Error())
		if len(message) > 500 {
			message = message[:500]
		}
		if err := synchronizer.store.FailConnectorSync(ctx, connector.ID, synchronizer.owner, synchronizer.now().UTC(), message); err != nil {
			synchronizer.logger.Error("record Uptime Kuma synchronization failure", "connector_id", connector.ID, "error", err)
		}
		synchronizer.logger.Warn("Uptime Kuma synchronization failed", "connector_id", connector.ID, "error", cause)
	}
	credential, err := synchronizer.secrets.Open(connector.CredentialSealed, "connector:uptime_kuma:"+connector.Endpoint)
	if err != nil {
		failed(fmt.Errorf("open connector credential: %w", err))
		return
	}
	monitors, err := synchronizer.client.Monitors(ctx, connector.Endpoint, string(credential))
	if err != nil {
		failed(err)
		return
	}
	bindingByMonitor := make(map[string]RuntimeBinding, len(connector.Bindings))
	for _, binding := range connector.Bindings {
		bindingByMonitor[binding.ExternalID] = binding
	}
	signals := make([]incidents.UptimeKumaSignal, 0)
	observations := make([]IntegrationObservation, 0, len(connector.Bindings))
	for _, monitor := range monitors {
		binding, imported := bindingByMonitor[monitor.ID]
		if !imported {
			continue
		}
		observations = append(observations, uptimeKumaObservation(binding.ID, monitor))
		if monitor.Status != 0 {
			continue
		}
		signals = append(signals, incidents.UptimeKumaSignal{
			TargetID: binding.TargetID, BindingID: binding.ID,
			ExternalMonitor: monitor.ID, Name: monitor.Name, Severity: incidents.SeverityMajor,
		})
	}
	observedAt := synchronizer.now().UTC()
	if err := synchronizer.incidents.ReconcileUptimeKuma(ctx, incidents.ReconcileUptimeKumaInput{
		ConnectorID: connector.ID, ObservedAt: observedAt, Signals: signals,
	}); err != nil {
		failed(err)
		return
	}
	// La mesure ne commande pas la synchronisation : une Observation qui ne
	// s'enregistre pas laisse un trou dans la Couverture, elle ne dégrade pas
	// le Connecteur ni ne réécrit l'Incident déjà rapproché.
	if err := synchronizer.store.RecordIntegrationObservations(ctx, observedAt, observations); err != nil {
		synchronizer.logger.Warn("record Uptime Kuma observations", "connector_id", connector.ID, "error", err)
	}
	if err := synchronizer.store.CompleteConnectorSync(ctx, connector.ID, synchronizer.owner, observedAt); err != nil {
		synchronizer.logger.Error("complete Uptime Kuma synchronization", "connector_id", connector.ID, "error", err)
		return
	}
	synchronizer.logger.Info("Uptime Kuma synchronization completed", "connector_id", connector.ID, "monitors", len(monitors))
}

// uptimeKumaObservation traduit l'état d'un monitor en Observation.
//
// DOWN et UP concluent ; PENDING et MAINTENANCE restent neutres, comme partout
// ailleurs dans le Connecteur : ils ne prononcent ni défaillance ni
// rétablissement, et font seulement baisser la Couverture.
func uptimeKumaObservation(bindingID string, monitor uptimekuma.Monitor) IntegrationObservation {
	observation := IntegrationObservation{BindingID: bindingID, Outcome: "unknown"}
	switch monitor.Status {
	case 0:
		observation.Outcome = "unhealthy"
		observation.Reason = "uptime_kuma_monitor_down"
	case 1:
		observation.Outcome = "healthy"
		observation.LatencyMilliseconds = monitor.ResponseMilliseconds
	case 2:
		observation.Reason = "uptime_kuma_monitor_pending"
	default:
		observation.Reason = "uptime_kuma_monitor_maintenance"
	}
	return observation
}

func zabbixSeverity(value int) incidents.Severity {
	switch value {
	case 0, 1:
		return incidents.SeverityInformation
	case 2:
		return incidents.SeverityWarning
	case 3:
		return incidents.SeverityMajor
	default:
		return incidents.SeverityCritical
	}
}

// zabbixObservation traduit l'état d'un hôte importé en Observation.
//
// À la différence d'Uptime Kuma, Zabbix ne publie pas de temps de réponse par
// hôte : l'Observation porte la Disponibilité et la Couverture, jamais une
// latence. « unhealthy » et « healthy » concluent ; « unknown » — un problème
// supprimé, c'est-à-dire une maintenance côté Zabbix — reste neutre et fait
// seulement baisser la Couverture, comme la maintenance Uptime Kuma.
func zabbixObservation(bindingID, outcome string) IntegrationObservation {
	observation := IntegrationObservation{BindingID: bindingID, Outcome: outcome}
	switch outcome {
	case "unhealthy":
		observation.Reason = "zabbix_problem_active"
	case "unknown":
		observation.Reason = "zabbix_problem_suppressed"
	}
	return observation
}

type CredentialStore interface {
	RuntimeCredential(context.Context, string) (RuntimeCredential, error)
}

type EventAcknowledger interface {
	Acknowledge(context.Context, string, string, string, string) error
}

type Acknowledger struct {
	store   CredentialStore
	zabbix  EventAcknowledger
	secrets *secretbox.Box
}

func NewAcknowledger(store CredentialStore, client EventAcknowledger, secrets *secretbox.Box) *Acknowledger {
	return &Acknowledger{store: store, zabbix: client, secrets: secrets}
}

func (acknowledger *Acknowledger) Acknowledge(ctx context.Context, target incidents.AcknowledgementTarget, message string) error {
	credential, err := acknowledger.store.RuntimeCredential(ctx, target.ConnectorID)
	if err != nil {
		return err
	}
	if credential.Kind != target.Origin || credential.Kind != "zabbix" {
		return fmt.Errorf("unsupported acknowledgement origin %q", target.Origin)
	}
	token, err := acknowledger.secrets.Open(credential.CredentialSealed, "connector:zabbix:"+credential.Endpoint)
	if err != nil {
		return fmt.Errorf("open connector credential: %w", err)
	}
	return acknowledger.zabbix.Acknowledge(ctx, credential.Endpoint, string(token), target.ExternalEventID, message)
}
