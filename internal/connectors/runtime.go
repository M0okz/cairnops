package connectors

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type RuntimeBinding struct {
	ID         string
	TargetID   string
	ExternalID string
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
}

type RuntimeStore interface {
	ClaimDueConnector(context.Context, string, string, int, time.Duration) ([]RuntimeConnector, error)
	CompleteConnectorSync(context.Context, string, string, time.Time) error
	FailConnectorSync(context.Context, string, string, time.Time, string) error
	RecordIntegrationObservations(context.Context, time.Time, []IntegrationObservation) error
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
