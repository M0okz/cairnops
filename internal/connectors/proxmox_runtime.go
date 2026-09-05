package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/proxmox"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type ProxmoxRuntimeStore interface {
	ClaimDueConnector(context.Context, string, string, int, time.Duration) ([]RuntimeConnector, error)
	RefreshProxmox(context.Context, RuntimeConnector, string, []proxmox.Resource) ([]RuntimeBinding, error)
	RecordIntegrationObservations(context.Context, time.Time, []IntegrationObservation) error
	CompleteConnectorSync(context.Context, string, string, time.Time) error
	FailConnectorSync(context.Context, string, string, time.Time, string) error
}

type ProxmoxObserver interface {
	Resources(context.Context, string, proxmox.Credentials) ([]proxmox.Resource, error)
}
type EvidenceReconciler interface {
	ApplyEvidenceSnapshot(context.Context, incidents.EvidenceSnapshot) error
}

type ProxmoxSynchronizer struct {
	store     ProxmoxRuntimeStore
	incidents EvidenceReconciler
	client    ProxmoxObserver
	secrets   *secretbox.Box
	owner     string
	logger    *slog.Logger
	now       func() time.Time
}

func NewProxmoxSynchronizer(store ProxmoxRuntimeStore, incidentStore EvidenceReconciler, client ProxmoxObserver, secrets *secretbox.Box, owner string, logger *slog.Logger) *ProxmoxSynchronizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProxmoxSynchronizer{store: store, incidents: incidentStore, client: client, secrets: secrets, owner: owner, logger: logger, now: time.Now}
}

func (s *ProxmoxSynchronizer) Run(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.tick(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *ProxmoxSynchronizer) tick(ctx context.Context) error {
	connectors, err := s.store.ClaimDueConnector(ctx, "proxmox", s.owner, 4, 90*time.Second)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, connector := range connectors {
		wg.Add(1)
		go func() { defer wg.Done(); s.syncOne(ctx, connector) }()
	}
	wg.Wait()
	return nil
}

func (s *ProxmoxSynchronizer) syncOne(parent context.Context, connector RuntimeConnector) {
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	fail := func(cause error) {
		message := cause.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		// Preserve a short opportunity to release the lease after a timeout.
		recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer recordCancel()
		if err := s.store.FailConnectorSync(recordCtx, connector.ID, s.owner, s.now().UTC(), message); err != nil {
			s.logger.Warn("record Proxmox VE synchronization failure", "connector_id", connector.ID, "error", err)
		}
	}
	raw, err := s.secrets.Open(connector.CredentialSealed, "connector:proxmox:"+connector.Endpoint)
	if err != nil {
		fail(fmt.Errorf("open Proxmox VE credential: %w", err))
		return
	}
	var credential proxmox.Credentials
	if err := json.Unmarshal(raw, &credential); err != nil {
		fail(fmt.Errorf("decode Proxmox VE credential: %w", err))
		return
	}
	resources, err := s.client.Resources(ctx, connector.Endpoint, credential)
	if err != nil {
		fail(err)
		return
	}
	bindings, err := s.store.RefreshProxmox(ctx, connector, s.owner, resources)
	if err != nil {
		fail(err)
		return
	}
	byID := map[string]proxmox.Resource{}
	for _, r := range resources {
		byID[r.ID] = r
	}
	observedAt := s.now().UTC()
	snapshot := incidents.EvidenceSnapshot{Origin: "proxmox", ConnectorID: connector.ID, LeaseOwner: s.owner, ObservedAt: observedAt, ObservedScopes: []string{}, Facts: []incidents.EvidenceFact{}}
	observations := make([]IntegrationObservation, 0, len(bindings))
	unknown := 0
	for _, binding := range bindings {
		resource, found := byID[binding.ExternalID]
		if !found {
			unknown++
			observations = append(observations, IntegrationObservation{BindingID: binding.ID, Outcome: "unknown", Reason: "proxmox_resource_missing", Message: "Objet absent de l’inventaire Proxmox VE · à vérifier", Details: binding.Metadata})
			continue
		}
		expected, _ := binding.Metadata["expected_running"].(bool)
		outcome, reason := resource.Condition(expected)
		message := "État d’exécution publié par Proxmox VE : " + resource.Status
		details := resource.Metadata()
		details["expected_running"] = expected
		observations = append(observations, IntegrationObservation{BindingID: binding.ID, Outcome: outcome, Reason: reason, Message: message, Details: details})
		if outcome == "unknown" {
			if reason != "proxmox_guest_stop_allowed" {
				unknown++
			} else {
				snapshot.ObservedScopes = append(snapshot.ObservedScopes, binding.ID)
			}
			continue
		}
		snapshot.ObservedScopes = append(snapshot.ObservedScopes, binding.ID)
		if outcome == "unhealthy" {
			name := "Machine arrêtée dans Proxmox VE"
			if resource.Type == "node" {
				name = "Nœud hors ligne dans Proxmox VE"
			}
			snapshot.Facts = append(snapshot.Facts, incidents.EvidenceFact{Origin: "proxmox", ConnectorID: connector.ID, BindingID: binding.ID, IdentityScope: binding.ID, IdentityKey: "availability", TargetID: binding.TargetID, ExternalEventID: resource.ID, ExternalObjectID: resource.ID, Nature: incidents.CanonicalNature(incidents.NatureAvailability, incidents.NatureAvailabilityLabel), Name: name, Severity: incidents.SeverityMajor, OpenedAt: observedAt, Metadata: details})
		}
	}
	if err := s.incidents.ApplyEvidenceSnapshot(ctx, snapshot); err != nil {
		fail(err)
		return
	}
	if err := s.store.RecordIntegrationObservations(ctx, observedAt, observations); err != nil {
		fail(err)
		return
	}
	if unknown > 0 {
		fail(fmt.Errorf("Proxmox VE: %d imported resources are missing or have an unknown status", unknown))
		return
	}
	if err := s.store.CompleteConnectorSync(ctx, connector.ID, s.owner, observedAt); err != nil {
		s.logger.Warn("complete Proxmox VE synchronization", "connector_id", connector.ID, "error", err)
	}
}
