package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/proxmox"
)

type ProxmoxClient interface {
	Inspect(context.Context, string, proxmox.Credentials) (proxmox.Inspection, error)
	Resources(context.Context, string, proxmox.Credentials) ([]proxmox.Resource, error)
	ProbeCertificate(context.Context, string) (proxmox.Certificate, error)
	CheckProvisioning(context.Context, string, proxmox.Credentials) error
	Provision(context.Context, string, proxmox.Credentials) (proxmox.ManagedCredential, error)
	RemoveManaged(context.Context, string, proxmox.Credentials, string) error
}

type ProxmoxPreviewInput struct {
	Name        string              `json:"name"`
	Address     string              `json:"address"`
	Mode        string              `json:"mode"`
	Credentials proxmox.Credentials `json:"credentials"`
}

type ProxmoxResourcePreview struct {
	proxmox.Resource
	ExternalID        string           `json:"external_id"`
	Importable        bool             `json:"importable"`
	ExpectedRunning   bool             `json:"expected_running"`
	AlreadyImported   bool             `json:"already_imported"`
	AlreadyImportedTo *TargetReference `json:"already_imported_to,omitempty"`
	SuggestedTarget   *TargetReference `json:"suggested_target,omitempty"`
	Matches           []TargetMatch    `json:"candidate_targets"`
}

type ProxmoxPreview struct {
	Kind               string                   `json:"kind"`
	Name               string                   `json:"name"`
	Endpoint           string                   `json:"endpoint"`
	Version            string                   `json:"version"`
	EncryptedTransport bool                     `json:"encrypted_transport"`
	ImportableCount    int                      `json:"importable_count"`
	Resources          []ProxmoxResourcePreview `json:"resources"`
	AvailableTargets   []TargetReference        `json:"available_targets"`
	Access             AccessPreview            `json:"access"`
	Receipt            string                   `json:"receipt"`
	ExpiresAt          time.Time                `json:"expires_at"`
}

type ProxmoxImportInput struct {
	Receipt            string            `json:"receipt"`
	ResourceIDs        []string          `json:"resource_ids"`
	ExpectedRunningIDs []string          `json:"expected_running_ids"`
	TargetAssignments  map[string]string `json:"target_assignments,omitempty"`
}

type ProxmoxImport struct {
	Connector Connector        `json:"connector"`
	Targets   []ImportedTarget `json:"targets"`
}

type PersistProxmoxInput struct {
	ActorID, Name, Endpoint, CredentialSealed, Version string
	ExistingID, ManagedUserID                          string
	Resources                                          []proxmox.Resource
	Inventory                                          []proxmox.Resource
	ExpectedRunning                                    map[string]bool
	TargetAssignments                                  map[string]string
}

type proxmoxStore interface {
	ImportProxmox(context.Context, PersistProxmoxInput) (ProxmoxImport, error)
	ProxmoxSettings(context.Context, string) (map[string]bool, error)
	RemovalCredential(context.Context, string) (RuntimeCredential, error)
	ReplaceProxmoxCredential(context.Context, string, string, string) error
}

type proxmoxReceipt struct {
	ProxmoxPreviewInput
	ExistingID    string    `json:"existing_id,omitempty"`
	ManagedUserID string    `json:"managed_user_id,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (service *Service) WithProxmox(client ProxmoxClient) *Service {
	service.proxmox = client
	return service
}

func (service *Service) ProxmoxCertificate(ctx context.Context, address string) (proxmox.Certificate, error) {
	if service.proxmox == nil {
		return proxmox.Certificate{}, fmt.Errorf("%w: Proxmox VE client is unavailable", ErrConnection)
	}
	certificate, err := service.proxmox.ProbeCertificate(ctx, address)
	if err != nil {
		return certificate, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	return certificate, nil
}

func (service *Service) PreviewProxmox(ctx context.Context, input ProxmoxPreviewInput) (ProxmoxPreview, error) {
	return service.previewProxmox(ctx, proxmoxReceipt{ProxmoxPreviewInput: input})
}

func (service *Service) previewProxmox(ctx context.Context, receipt proxmoxReceipt) (ProxmoxPreview, error) {
	input := &receipt.ProxmoxPreviewInput
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 160 {
		return ProxmoxPreview{}, fmt.Errorf("%w: connector name must contain 1 to 160 characters", ErrInvalidInput)
	}
	if input.Mode == "" {
		input.Mode = "automatic"
	}
	if input.Mode != "automatic" && input.Mode != "provided" {
		return ProxmoxPreview{}, fmt.Errorf("%w: unsupported authorization mode", ErrInvalidInput)
	}
	store, ok := service.store.(proxmoxStore)
	if service.proxmox == nil || !ok {
		return ProxmoxPreview{}, fmt.Errorf("%w: Proxmox VE is unavailable", ErrConnection)
	}
	inspection, err := service.proxmox.Inspect(ctx, input.Address, input.Credentials)
	if err != nil {
		return ProxmoxPreview{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	input.Address = inspection.Endpoint
	if input.Mode == "automatic" {
		if err := service.proxmox.CheckProvisioning(ctx, inspection.Endpoint, input.Credentials); err != nil {
			return ProxmoxPreview{}, fmt.Errorf("%w: %v", ErrConnection, err)
		}
	}
	connectors, err := service.store.List(ctx)
	if err != nil {
		return ProxmoxPreview{}, err
	}
	for _, connector := range connectors {
		if connector.Kind == "proxmox" && strings.EqualFold(connector.Endpoint, inspection.Endpoint) && connector.ID != receipt.ExistingID {
			return ProxmoxPreview{}, fmt.Errorf("%w: this Proxmox VE endpoint is already connected; open its inventory to add targets", ErrInvalidInput)
		}
	}
	names := make([]string, 0, len(inspection.Resources))
	for _, r := range inspection.Resources {
		names = append(names, r.Name)
	}
	state, err := service.store.PreviewState(ctx, "proxmox", inspection.Endpoint, names)
	if err != nil {
		return ProxmoxPreview{}, err
	}
	settings, err := store.ProxmoxSettings(ctx, inspection.Endpoint)
	if err != nil {
		return ProxmoxPreview{}, err
	}
	resources := make([]ProxmoxResourcePreview, 0, len(inspection.Resources))
	count := 0
	for _, r := range inspection.Resources {
		matches := matchTargets(DiscoveredIdentity{Names: []string{r.Name}}, state.Targets)
		discovered := ProxmoxResourcePreview{Resource: r, ExternalID: r.ID, Importable: r.Importable(), ExpectedRunning: settings[r.ID], Matches: matches, SuggestedTarget: suggestedTarget(matches)}
		if target, found := state.ImportedByExternalID[r.ID]; found {
			discovered.AlreadyImported, discovered.AlreadyImportedTo = true, &target
		}
		if r.Importable() {
			count++
		}
		resources = append(resources, discovered)
	}
	receipt.ExpiresAt = service.now().UTC().Add(previewLifetime)
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return ProxmoxPreview{}, err
	}
	sealed, err := service.secrets.Seal(encoded, "proxmox-preview-v1")
	if err != nil {
		return ProxmoxPreview{}, err
	}
	access := AccessPreview{Mode: input.Mode, RemoteChanges: []string{}}
	if input.Mode == "automatic" {
		access.WillProvision = true
		access.RemoteChanges = []string{"proxmox_technical_user", "proxmox_auditor_token", "proxmox_cleanup_authorization"}
	}
	return ProxmoxPreview{Kind: "proxmox", Name: input.Name, Endpoint: inspection.Endpoint, Version: inspection.Version, EncryptedTransport: true, ImportableCount: count, Resources: resources, AvailableTargets: availableTargets(state.Targets), Access: access, Receipt: sealed, ExpiresAt: receipt.ExpiresAt}, nil
}

func (service *Service) ImportProxmox(ctx context.Context, actorID string, input ProxmoxImportInput) (ProxmoxImport, error) {
	if !validUUID(actorID) || len(input.Receipt) < 32 || len(input.Receipt) > 32768 || len(input.ResourceIDs) < 1 || len(input.ResourceIDs) > 5000 {
		return ProxmoxImport{}, fmt.Errorf("%w: select between 1 and 5000 resources with a valid preview", ErrInvalidInput)
	}
	selection := map[string]struct{}{}
	for _, id := range input.ResourceIDs {
		if _, duplicate := selection[id]; duplicate || id == "" {
			return ProxmoxImport{}, fmt.Errorf("%w: selected resource identities must be unique", ErrInvalidInput)
		}
		selection[id] = struct{}{}
	}
	assignments, err := validateTargetAssignments(selection, input.TargetAssignments)
	if err != nil {
		return ProxmoxImport{}, err
	}
	alerts := map[string]bool{}
	for _, id := range input.ExpectedRunningIDs {
		if _, selected := selection[id]; !selected || alerts[id] {
			return ProxmoxImport{}, fmt.Errorf("%w: stop alerts must refer to unique selected guests", ErrInvalidInput)
		}
		alerts[id] = true
	}
	encoded, err := service.secrets.Open(input.Receipt, "proxmox-preview-v1")
	if err != nil {
		return ProxmoxImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	var receipt proxmoxReceipt
	if err := json.Unmarshal(encoded, &receipt); err != nil {
		return ProxmoxImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if !service.now().UTC().Before(receipt.ExpiresAt) {
		return ProxmoxImport{}, ErrPreviewExpired
	}
	store, ok := service.store.(proxmoxStore)
	if !ok || service.proxmox == nil {
		return ProxmoxImport{}, fmt.Errorf("%w: Proxmox VE is unavailable", ErrConnection)
	}
	inspection, err := service.proxmox.Inspect(ctx, receipt.Address, receipt.Credentials)
	if err != nil {
		return ProxmoxImport{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	selected := make([]proxmox.Resource, 0, len(selection))
	for _, resource := range inspection.Resources {
		if _, found := selection[resource.ID]; !found {
			continue
		}
		if !resource.Importable() || alerts[resource.ID] && !resource.Guest() {
			return ProxmoxImport{}, fmt.Errorf("%w: selected resource is no longer eligible", ErrInvalidInput)
		}
		selected = append(selected, resource)
		delete(selection, resource.ID)
	}
	if len(selection) != 0 {
		return ProxmoxImport{}, fmt.Errorf("%w: a selected resource disappeared; refresh the inventory", ErrInvalidInput)
	}
	credentials := receipt.Credentials
	managedID := receipt.ManagedUserID
	if receipt.Mode == "automatic" {
		managed, provisionErr := service.proxmox.Provision(ctx, inspection.Endpoint, receipt.Credentials)
		if provisionErr != nil {
			return ProxmoxImport{}, fmt.Errorf("%w: %v", ErrConnection, provisionErr)
		}
		credentials, managedID = managed.Credentials, managed.UserID
	}
	cleanup := func(cause error) (ProxmoxImport, error) {
		if receipt.Mode == "automatic" {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
			defer cancel()
			if cleanupErr := service.proxmox.RemoveManaged(cleanupCtx, inspection.Endpoint, receipt.Credentials, managedID); cleanupErr != nil {
				return ProxmoxImport{}, fmt.Errorf("%w; remove the unused Proxmox VE account %s manually: %v", cause, managedID, cleanupErr)
			}
		}
		return ProxmoxImport{}, cause
	}
	encoded, err = json.Marshal(credentials)
	if err != nil {
		return cleanup(err)
	}
	sealed, err := service.secrets.Seal(encoded, "connector:proxmox:"+inspection.Endpoint)
	if err != nil {
		return cleanup(err)
	}
	result, err := store.ImportProxmox(ctx, PersistProxmoxInput{ActorID: actorID, Name: receipt.Name, Endpoint: inspection.Endpoint, CredentialSealed: sealed, Version: inspection.Version, ExistingID: receipt.ExistingID, ManagedUserID: managedID, Resources: selected, Inventory: inspection.Resources, ExpectedRunning: alerts, TargetAssignments: assignments})
	if err != nil {
		return cleanup(err)
	}
	return result, nil
}

func (service *Service) RemoveProxmox(ctx context.Context, connectorID string, installer proxmox.Credentials) (Removal, error) {
	store, ok := service.store.(proxmoxStore)
	if !ok || service.proxmox == nil || !validUUID(connectorID) {
		return Removal{}, fmt.Errorf("%w: invalid Proxmox VE connector", ErrInvalidInput)
	}
	credential, err := store.RemovalCredential(ctx, connectorID)
	if err != nil {
		return Removal{}, err
	}
	if credential.Kind != "proxmox" {
		return Removal{}, fmt.Errorf("%w: connector is not Proxmox VE", ErrInvalidInput)
	}
	if _, err := service.Suspend(ctx, connectorID); err != nil {
		return Removal{}, err
	}
	if credential.CredentialManagement == "managed" {
		encoded, err := service.secrets.Open(credential.CredentialSealed, "connector:proxmox:"+credential.Endpoint)
		if err != nil {
			return Removal{}, err
		}
		var runtime proxmox.Credentials
		if err := json.Unmarshal(encoded, &runtime); err != nil {
			return Removal{}, err
		}
		installer.Fingerprint = runtime.Fingerprint
		if err := service.proxmox.RemoveManaged(ctx, credential.Endpoint, installer, credential.ManagedCredentialID); err != nil {
			return Removal{}, fmt.Errorf("%w: connector suspended; remote cleanup is pending: %v", ErrConnection, err)
		}
	}
	return service.store.Delete(ctx, connectorID)
}

// ReapproveProxmoxCertificate verifies the existing credential against the
// explicitly approved certificate before atomically replacing the stored pin.
func (service *Service) ReapproveProxmoxCertificate(ctx context.Context, connectorID, fingerprint string) error {
	store, ok := service.store.(proxmoxStore)
	if !ok || service.proxmox == nil || !validUUID(connectorID) || len(fingerprint) != 64 {
		return fmt.Errorf("%w: invalid certificate approval", ErrInvalidInput)
	}
	credential, err := store.RemovalCredential(ctx, connectorID)
	if err != nil {
		return err
	}
	if credential.Kind != "proxmox" {
		return fmt.Errorf("%w: connector is not Proxmox VE", ErrInvalidInput)
	}
	raw, err := service.secrets.Open(credential.CredentialSealed, "connector:proxmox:"+credential.Endpoint)
	if err != nil {
		return err
	}
	var runtime proxmox.Credentials
	if err := json.Unmarshal(raw, &runtime); err != nil {
		return err
	}
	runtime.Fingerprint = fingerprint
	if _, err := service.proxmox.Inspect(ctx, credential.Endpoint, runtime); err != nil {
		return fmt.Errorf("%w: %v", ErrConnection, err)
	}
	raw, err = json.Marshal(runtime)
	if err != nil {
		return err
	}
	sealed, err := service.secrets.Seal(raw, "connector:proxmox:"+credential.Endpoint)
	if err != nil {
		return err
	}
	return store.ReplaceProxmoxCredential(ctx, connectorID, credential.CredentialSealed, sealed)
}
