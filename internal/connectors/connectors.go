package connectors

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/secretbox"
)

const previewLifetime = 15 * time.Minute

var (
	ErrInvalidInput   = errors.New("invalid input")
	ErrConnection     = errors.New("connector connection failed")
	ErrPreviewExpired = errors.New("connector preview expired")
	ErrNotFound       = errors.New("connector not found")
)

type Connector struct {
	ID                 string    `json:"id"`
	Kind               string    `json:"kind"`
	Name               string    `json:"name"`
	Endpoint           string    `json:"endpoint"`
	Status             string    `json:"status"`
	RemoteVersion      string    `json:"remote_version"`
	Compatibility      string    `json:"compatibility"`
	EncryptedTransport bool      `json:"encrypted_transport"`
	BindingCount       int       `json:"binding_count"`
	QuarantineCount    int       `json:"quarantine_count"`
	LastCheckedAt      time.Time `json:"last_checked_at"`
	LastError          string    `json:"last_error,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Removal rend compte de ce qu'a emporté la suppression. Le décompte est celui
// du serveur, pas celui qu'affichait l'écran : c'est lui qui fait foi.
type Removal struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	Bindings          int    `json:"bindings"`
	Quarantined       int    `json:"quarantined"`
	ResolvedIncidents int    `json:"resolved_incidents"`
}

type TargetReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DiscoveredHost struct {
	ExternalID        string             `json:"external_id"`
	Name              string             `json:"name"`
	TechnicalName     string             `json:"technical_name"`
	Interfaces        []zabbix.Interface `json:"interfaces"`
	CandidateTargets  []TargetMatch      `json:"candidate_targets,omitempty"`
	SuggestedTarget   *TargetReference   `json:"suggested_target,omitempty"`
	AlreadyImportedTo *TargetReference   `json:"already_imported_to,omitempty"`
}

type ZabbixPreviewInput struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	APIToken string `json:"api_token"`
}

type ZabbixPreview struct {
	Kind               string            `json:"kind"`
	Name               string            `json:"name"`
	Endpoint           string            `json:"endpoint"`
	Version            string            `json:"version"`
	Compatibility      string            `json:"compatibility"`
	CompatibilityLabel string            `json:"compatibility_label"`
	EncryptedTransport bool              `json:"encrypted_transport"`
	Hosts              []DiscoveredHost  `json:"hosts"`
	AvailableTargets   []TargetReference `json:"available_targets"`
	Receipt            string            `json:"receipt"`
	ExpiresAt          time.Time         `json:"expires_at"`
}

type ZabbixImportInput struct {
	Receipt           string            `json:"receipt"`
	HostIDs           []string          `json:"host_ids"`
	TargetAssignments map[string]string `json:"target_assignments,omitempty"`
}

type ImportedTarget struct {
	ExternalID  string `json:"external_id"`
	TargetID    string `json:"target_id"`
	TargetName  string `json:"target_name"`
	Disposition string `json:"disposition"`
}

type ZabbixImport struct {
	Connector Connector        `json:"connector"`
	Targets   []ImportedTarget `json:"targets"`
}

type UptimeKumaMonitorPreview struct {
	ExternalID        string           `json:"external_id"`
	Name              string           `json:"name"`
	Type              string           `json:"type"`
	Address           string           `json:"address,omitempty"`
	Status            int              `json:"status"`
	CandidateTargets  []TargetMatch    `json:"candidate_targets,omitempty"`
	SuggestedTarget   *TargetReference `json:"suggested_target,omitempty"`
	AlreadyImportedTo *TargetReference `json:"already_imported_to,omitempty"`
}

type UptimeKumaPreviewInput struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	APIKey  string `json:"api_key"`
}

type UptimeKumaPreview struct {
	Kind               string                     `json:"kind"`
	Name               string                     `json:"name"`
	Endpoint           string                     `json:"endpoint"`
	Compatibility      string                     `json:"compatibility"`
	CompatibilityLabel string                     `json:"compatibility_label"`
	EncryptedTransport bool                       `json:"encrypted_transport"`
	Monitors           []UptimeKumaMonitorPreview `json:"monitors"`
	AvailableTargets   []TargetReference          `json:"available_targets"`
	Receipt            string                     `json:"receipt"`
	ExpiresAt          time.Time                  `json:"expires_at"`
}

type UptimeKumaImportInput struct {
	Receipt           string            `json:"receipt"`
	MonitorIDs        []string          `json:"monitor_ids"`
	TargetAssignments map[string]string `json:"target_assignments,omitempty"`
}

type UptimeKumaImport struct {
	Connector Connector        `json:"connector"`
	Targets   []ImportedTarget `json:"targets"`
}

type PatchMonHostPreview struct {
	ExternalID           string           `json:"external_id"`
	Name                 string           `json:"name"`
	Hostname             string           `json:"hostname"`
	IP                   string           `json:"ip"`
	OSType               string           `json:"os_type"`
	OSVersion            string           `json:"os_version"`
	ReportingState       string           `json:"reporting_state"`
	UpdateState          string           `json:"update_state"`
	UpdatesCount         int              `json:"updates_count"`
	SecurityUpdatesCount int              `json:"security_updates_count"`
	NeedsReboot          bool             `json:"needs_reboot"`
	CandidateTargets     []TargetMatch    `json:"candidate_targets,omitempty"`
	SuggestedTarget      *TargetReference `json:"suggested_target,omitempty"`
	AlreadyImportedTo    *TargetReference `json:"already_imported_to,omitempty"`
}

type PatchMonPreviewInput struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	TokenKey    string `json:"token_key"`
	TokenSecret string `json:"token_secret"`
}

type PatchMonPreview struct {
	Kind               string                `json:"kind"`
	Name               string                `json:"name"`
	Endpoint           string                `json:"endpoint"`
	Compatibility      string                `json:"compatibility"`
	CompatibilityLabel string                `json:"compatibility_label"`
	EncryptedTransport bool                  `json:"encrypted_transport"`
	Hosts              []PatchMonHostPreview `json:"hosts"`
	AvailableTargets   []TargetReference     `json:"available_targets"`
	Receipt            string                `json:"receipt"`
	ExpiresAt          time.Time             `json:"expires_at"`
}

type PatchMonImportInput struct {
	Receipt           string            `json:"receipt"`
	HostIDs           []string          `json:"host_ids"`
	TargetAssignments map[string]string `json:"target_assignments,omitempty"`
}

type PatchMonImport struct {
	Connector Connector        `json:"connector"`
	Targets   []ImportedTarget `json:"targets"`
}

type PreviewState struct {
	TargetsByName        map[string]TargetReference
	Targets              []TargetIdentity
	ImportedByExternalID map[string]TargetReference
}

type PersistZabbixInput struct {
	ActorID            string
	Name               string
	Endpoint           string
	CredentialSealed   string
	Version            string
	Compatibility      string
	EncryptedTransport bool
	Hosts              []zabbix.Host
	TargetAssignments  map[string]string
}

type PersistUptimeKumaInput struct {
	ActorID            string
	Name               string
	Endpoint           string
	CredentialSealed   string
	EncryptedTransport bool
	Monitors           []uptimekuma.Monitor
	TargetAssignments  map[string]string
}

type PersistPatchMonInput struct {
	ActorID            string
	Name               string
	Endpoint           string
	CredentialSealed   string
	EncryptedTransport bool
	Hosts              []patchmon.Host
	TargetAssignments  map[string]string
}

type Store interface {
	List(context.Context) ([]Connector, error)
	SetStatus(context.Context, string, string) (Connector, error)
	Delete(context.Context, string) (Removal, error)
	PreviewState(context.Context, string, string, []string) (PreviewState, error)
	ImportZabbix(context.Context, PersistZabbixInput) (ZabbixImport, error)
	ImportUptimeKuma(context.Context, PersistUptimeKumaInput) (UptimeKumaImport, error)
	ImportPatchMon(context.Context, PersistPatchMonInput) (PatchMonImport, error)
}

type ZabbixClient interface {
	Inspect(context.Context, string, string) (zabbix.Inspection, error)
}

type UptimeKumaClient interface {
	Inspect(context.Context, string, string) (uptimekuma.Inspection, error)
}

type PatchMonClient interface {
	Inspect(context.Context, string, patchmon.Credentials) (patchmon.Inspection, error)
}

type Service struct {
	store      Store
	zabbix     ZabbixClient
	uptimeKuma UptimeKumaClient
	patchMon   PatchMonClient
	secrets    *secretbox.Box
	now        func() time.Time
}

type zabbixReceipt struct {
	Name      string    `json:"name"`
	Endpoint  string    `json:"endpoint"`
	APIToken  string    `json:"api_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type uptimeKumaReceipt struct {
	Name      string    `json:"name"`
	Endpoint  string    `json:"endpoint"`
	APIKey    string    `json:"api_key"`
	ExpiresAt time.Time `json:"expires_at"`
}

type patchMonReceipt struct {
	Name        string               `json:"name"`
	Endpoint    string               `json:"endpoint"`
	Credentials patchmon.Credentials `json:"credentials"`
	ExpiresAt   time.Time            `json:"expires_at"`
}

func NewService(store Store, zabbixClient ZabbixClient, uptimeKumaClient UptimeKumaClient, patchMonClient PatchMonClient, secrets *secretbox.Box) *Service {
	return &Service{store: store, zabbix: zabbixClient, uptimeKuma: uptimeKumaClient, patchMon: patchMonClient, secrets: secrets, now: time.Now}
}

func (service *Service) List(ctx context.Context) ([]Connector, error) {
	return service.store.List(ctx)
}

// Suspend arrête la lecture sans rien effacer : les liaisons, la quarantaine et
// les Incidents ouverts restent en place. C'est la réponse réversible à un
// Connecteur qui déraille, et le webhook générique la respecte aussi puisque
// Receive refuse une identité dont le Connecteur est désactivé.
func (service *Service) Suspend(ctx context.Context, connectorID string) (Connector, error) {
	return service.setStatus(ctx, connectorID, "disabled")
}

// Resume replace le Connecteur dans le cycle et le rend dû immédiatement :
// l'état annoncé redevient « connecté » sans preuve, mais le premier cycle du
// worker le corrige dans la foulée.
func (service *Service) Resume(ctx context.Context, connectorID string) (Connector, error) {
	return service.setStatus(ctx, connectorID, "connected")
}

func (service *Service) setStatus(ctx context.Context, connectorID, status string) (Connector, error) {
	connectorID = strings.TrimSpace(connectorID)
	if connectorID == "" {
		return Connector{}, fmt.Errorf("%w: connector identity is required", ErrInvalidInput)
	}
	return service.store.SetStatus(ctx, connectorID, status)
}

func (service *Service) Delete(ctx context.Context, connectorID string) (Removal, error) {
	connectorID = strings.TrimSpace(connectorID)
	if connectorID == "" {
		return Removal{}, fmt.Errorf("%w: connector identity is required", ErrInvalidInput)
	}
	return service.store.Delete(ctx, connectorID)
}

func (service *Service) PreviewZabbix(ctx context.Context, input ZabbixPreviewInput) (ZabbixPreview, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "Zabbix"
	}
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 160 {
		return ZabbixPreview{}, fmt.Errorf("%w: connector name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	inspection, err := service.zabbix.Inspect(ctx, input.Address, input.APIToken)
	if err != nil {
		return ZabbixPreview{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	names := make([]string, 0, len(inspection.Hosts))
	for _, host := range inspection.Hosts {
		names = append(names, host.Name)
	}
	state, err := service.store.PreviewState(ctx, "zabbix", inspection.Endpoint, names)
	if err != nil {
		return ZabbixPreview{}, fmt.Errorf("prepare Zabbix preview: %w", err)
	}
	hosts := make([]DiscoveredHost, 0, len(inspection.Hosts))
	for _, host := range inspection.Hosts {
		discovered := DiscoveredHost{
			ExternalID: host.ID, Name: host.Name, TechnicalName: host.Technical,
			Interfaces: host.Interfaces,
		}
		discovered.CandidateTargets = matchTargets(identityForZabbix(host), state.Targets)
		discovered.SuggestedTarget = suggestedTarget(discovered.CandidateTargets)
		if discovered.SuggestedTarget == nil {
			if target, ok := state.TargetsByName[normalizeName(host.Name)]; ok {
				targetCopy := target
				discovered.SuggestedTarget = &targetCopy
			}
		}
		if target, ok := state.ImportedByExternalID[host.ID]; ok {
			targetCopy := target
			discovered.AlreadyImportedTo = &targetCopy
		}
		hosts = append(hosts, discovered)
	}
	expiresAt := service.now().UTC().Add(previewLifetime)
	receiptPayload, err := json.Marshal(zabbixReceipt{
		Name: input.Name, Endpoint: inspection.Endpoint,
		APIToken: strings.TrimSpace(input.APIToken), ExpiresAt: expiresAt,
	})
	if err != nil {
		return ZabbixPreview{}, fmt.Errorf("encode Zabbix preview: %w", err)
	}
	receipt, err := service.secrets.Seal(receiptPayload, "zabbix-preview-v1")
	if err != nil {
		return ZabbixPreview{}, fmt.Errorf("seal Zabbix preview: %w", err)
	}
	return ZabbixPreview{
		Kind: "zabbix", Name: input.Name, Endpoint: inspection.Endpoint,
		Version: inspection.Version, Compatibility: inspection.Compatibility,
		CompatibilityLabel: inspection.CompatibilityLabel,
		EncryptedTransport: inspection.EncryptedTransport,
		Hosts:              hosts, AvailableTargets: availableTargets(state.Targets),
		Receipt: receipt, ExpiresAt: expiresAt,
	}, nil
}

func (service *Service) PreviewUptimeKuma(ctx context.Context, input UptimeKumaPreviewInput) (UptimeKumaPreview, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "Uptime Kuma"
	}
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 160 {
		return UptimeKumaPreview{}, fmt.Errorf("%w: connector name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	inspection, err := service.uptimeKuma.Inspect(ctx, input.Address, input.APIKey)
	if err != nil {
		return UptimeKumaPreview{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	names := make([]string, 0, len(inspection.Monitors))
	for _, monitor := range inspection.Monitors {
		names = append(names, monitor.Name)
	}
	state, err := service.store.PreviewState(ctx, "uptime_kuma", inspection.Endpoint, names)
	if err != nil {
		return UptimeKumaPreview{}, fmt.Errorf("prepare Uptime Kuma preview: %w", err)
	}
	monitors := make([]UptimeKumaMonitorPreview, 0, len(inspection.Monitors))
	for _, monitor := range inspection.Monitors {
		discovered := UptimeKumaMonitorPreview{
			ExternalID: monitor.ID, Name: monitor.Name, Type: monitor.Type,
			Address: monitor.Address(), Status: monitor.Status,
		}
		discovered.CandidateTargets = matchTargets(identityForUptimeKuma(monitor), state.Targets)
		discovered.SuggestedTarget = suggestedTarget(discovered.CandidateTargets)
		if discovered.SuggestedTarget == nil {
			if target, ok := state.TargetsByName[normalizeName(monitor.Name)]; ok {
				targetCopy := target
				discovered.SuggestedTarget = &targetCopy
			}
		}
		if target, ok := state.ImportedByExternalID[monitor.ID]; ok {
			targetCopy := target
			discovered.AlreadyImportedTo = &targetCopy
		}
		monitors = append(monitors, discovered)
	}
	expiresAt := service.now().UTC().Add(previewLifetime)
	receiptPayload, err := json.Marshal(uptimeKumaReceipt{
		Name: input.Name, Endpoint: inspection.Endpoint,
		APIKey: strings.TrimSpace(input.APIKey), ExpiresAt: expiresAt,
	})
	if err != nil {
		return UptimeKumaPreview{}, fmt.Errorf("encode Uptime Kuma preview: %w", err)
	}
	receipt, err := service.secrets.Seal(receiptPayload, "uptime-kuma-preview-v1")
	if err != nil {
		return UptimeKumaPreview{}, fmt.Errorf("seal Uptime Kuma preview: %w", err)
	}
	return UptimeKumaPreview{
		Kind: "uptime_kuma", Name: input.Name, Endpoint: inspection.Endpoint,
		Compatibility: "supported", CompatibilityLabel: "API métriques officielle",
		EncryptedTransport: inspection.EncryptedTransport, Monitors: monitors,
		AvailableTargets: availableTargets(state.Targets), Receipt: receipt, ExpiresAt: expiresAt,
	}, nil
}

func (service *Service) PreviewPatchMon(ctx context.Context, input PatchMonPreviewInput) (PatchMonPreview, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "PatchMon"
	}
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 160 {
		return PatchMonPreview{}, fmt.Errorf("%w: connector name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	credentials := patchmon.Credentials{Key: input.TokenKey, Secret: input.TokenSecret}
	inspection, err := service.patchMon.Inspect(ctx, input.Address, credentials)
	if err != nil {
		return PatchMonPreview{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	names := make([]string, 0, len(inspection.Hosts))
	for _, host := range inspection.Hosts {
		names = append(names, host.Name())
	}
	state, err := service.store.PreviewState(ctx, "patchmon", inspection.Endpoint, names)
	if err != nil {
		return PatchMonPreview{}, fmt.Errorf("prepare PatchMon preview: %w", err)
	}
	hosts := make([]PatchMonHostPreview, 0, len(inspection.Hosts))
	for _, host := range inspection.Hosts {
		discovered := PatchMonHostPreview{
			ExternalID: host.ID, Name: host.Name(), Hostname: host.Hostname, IP: host.IP,
			OSType: host.OSType, OSVersion: host.OSVersion,
			ReportingState: host.ReportingState, UpdateState: host.UpdateState,
			UpdatesCount: host.UpdatesCount, SecurityUpdatesCount: host.SecurityUpdatesCount,
			NeedsReboot: host.NeedsReboot,
		}
		discovered.CandidateTargets = matchTargets(identityForPatchMon(host), state.Targets)
		discovered.SuggestedTarget = suggestedTarget(discovered.CandidateTargets)
		if discovered.SuggestedTarget == nil {
			if target, ok := state.TargetsByName[normalizeName(host.Name())]; ok {
				targetCopy := target
				discovered.SuggestedTarget = &targetCopy
			}
		}
		if target, ok := state.ImportedByExternalID[host.ID]; ok {
			targetCopy := target
			discovered.AlreadyImportedTo = &targetCopy
		}
		hosts = append(hosts, discovered)
	}
	expiresAt := service.now().UTC().Add(previewLifetime)
	receiptPayload, err := json.Marshal(patchMonReceipt{
		Name: input.Name, Endpoint: inspection.Endpoint,
		Credentials: patchmon.Credentials{Key: strings.TrimSpace(input.TokenKey), Secret: strings.TrimSpace(input.TokenSecret)},
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return PatchMonPreview{}, fmt.Errorf("encode PatchMon preview: %w", err)
	}
	receipt, err := service.secrets.Seal(receiptPayload, "patchmon-preview-v1")
	if err != nil {
		return PatchMonPreview{}, fmt.Errorf("seal PatchMon preview: %w", err)
	}
	return PatchMonPreview{
		Kind: "patchmon", Name: input.Name, Endpoint: inspection.Endpoint,
		Compatibility: "supported", CompatibilityLabel: "API d’intégration en lecture seule",
		EncryptedTransport: inspection.EncryptedTransport, Hosts: hosts,
		AvailableTargets: availableTargets(state.Targets), Receipt: receipt, ExpiresAt: expiresAt,
	}, nil
}

func (service *Service) ImportZabbix(ctx context.Context, actorID string, input ZabbixImportInput) (ZabbixImport, error) {
	if strings.TrimSpace(actorID) == "" {
		return ZabbixImport{}, fmt.Errorf("%w: administrator identity is required", ErrInvalidInput)
	}
	if len(input.Receipt) < 32 || len(input.Receipt) > 32768 {
		return ZabbixImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if len(input.HostIDs) < 1 || len(input.HostIDs) > 5000 {
		return ZabbixImport{}, fmt.Errorf("%w: select between 1 and 5000 hosts", ErrInvalidInput)
	}
	selection := make(map[string]struct{}, len(input.HostIDs))
	for _, id := range input.HostIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return ZabbixImport{}, fmt.Errorf("%w: selected host identity must not be empty", ErrInvalidInput)
		}
		if _, duplicate := selection[id]; duplicate {
			return ZabbixImport{}, fmt.Errorf("%w: selected hosts must be unique", ErrInvalidInput)
		}
		selection[id] = struct{}{}
	}
	assignments, err := validateTargetAssignments(selection, input.TargetAssignments)
	if err != nil {
		return ZabbixImport{}, err
	}
	plaintext, err := service.secrets.Open(input.Receipt, "zabbix-preview-v1")
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	var receipt zabbixReceipt
	if err := json.Unmarshal(plaintext, &receipt); err != nil {
		return ZabbixImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if !service.now().UTC().Before(receipt.ExpiresAt) {
		return ZabbixImport{}, ErrPreviewExpired
	}
	inspection, err := service.zabbix.Inspect(ctx, receipt.Endpoint, receipt.APIToken)
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	selected := make([]zabbix.Host, 0, len(selection))
	for _, host := range inspection.Hosts {
		if _, ok := selection[host.ID]; ok {
			selected = append(selected, host)
			delete(selection, host.ID)
		}
	}
	if len(selection) != 0 {
		return ZabbixImport{}, fmt.Errorf("%w: one or more selected hosts are no longer available", ErrInvalidInput)
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	credential, err := service.secrets.Seal([]byte(receipt.APIToken), "connector:zabbix:"+inspection.Endpoint)
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("seal Zabbix credential: %w", err)
	}
	result, err := service.store.ImportZabbix(ctx, PersistZabbixInput{
		ActorID: actorID, Name: receipt.Name, Endpoint: inspection.Endpoint,
		CredentialSealed: credential, Version: inspection.Version,
		Compatibility: inspection.Compatibility, EncryptedTransport: inspection.EncryptedTransport,
		Hosts: selected, TargetAssignments: assignments,
	})
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("import Zabbix hosts: %w", err)
	}
	return result, nil
}

func (service *Service) ImportUptimeKuma(ctx context.Context, actorID string, input UptimeKumaImportInput) (UptimeKumaImport, error) {
	if strings.TrimSpace(actorID) == "" {
		return UptimeKumaImport{}, fmt.Errorf("%w: administrator identity is required", ErrInvalidInput)
	}
	if len(input.Receipt) < 32 || len(input.Receipt) > 32768 {
		return UptimeKumaImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if len(input.MonitorIDs) < 1 || len(input.MonitorIDs) > 5000 {
		return UptimeKumaImport{}, fmt.Errorf("%w: select between 1 and 5000 monitors", ErrInvalidInput)
	}
	selection := make(map[string]struct{}, len(input.MonitorIDs))
	for _, id := range input.MonitorIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return UptimeKumaImport{}, fmt.Errorf("%w: selected monitor identity must not be empty", ErrInvalidInput)
		}
		if _, duplicate := selection[id]; duplicate {
			return UptimeKumaImport{}, fmt.Errorf("%w: selected monitors must be unique", ErrInvalidInput)
		}
		selection[id] = struct{}{}
	}
	assignments, err := validateTargetAssignments(selection, input.TargetAssignments)
	if err != nil {
		return UptimeKumaImport{}, err
	}
	plaintext, err := service.secrets.Open(input.Receipt, "uptime-kuma-preview-v1")
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	var receipt uptimeKumaReceipt
	if err := json.Unmarshal(plaintext, &receipt); err != nil {
		return UptimeKumaImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if !service.now().UTC().Before(receipt.ExpiresAt) {
		return UptimeKumaImport{}, ErrPreviewExpired
	}
	inspection, err := service.uptimeKuma.Inspect(ctx, receipt.Endpoint, receipt.APIKey)
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	selected := make([]uptimekuma.Monitor, 0, len(selection))
	for _, monitor := range inspection.Monitors {
		if _, ok := selection[monitor.ID]; ok {
			selected = append(selected, monitor)
			delete(selection, monitor.ID)
		}
	}
	if len(selection) != 0 {
		return UptimeKumaImport{}, fmt.Errorf("%w: one or more selected monitors are no longer available", ErrInvalidInput)
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	credential, err := service.secrets.Seal([]byte(receipt.APIKey), "connector:uptime_kuma:"+inspection.Endpoint)
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("seal Uptime Kuma credential: %w", err)
	}
	result, err := service.store.ImportUptimeKuma(ctx, PersistUptimeKumaInput{
		ActorID: actorID, Name: receipt.Name, Endpoint: inspection.Endpoint,
		CredentialSealed: credential, EncryptedTransport: inspection.EncryptedTransport,
		Monitors: selected, TargetAssignments: assignments,
	})
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("import Uptime Kuma monitors: %w", err)
	}
	return result, nil
}

func (service *Service) ImportPatchMon(ctx context.Context, actorID string, input PatchMonImportInput) (PatchMonImport, error) {
	if strings.TrimSpace(actorID) == "" {
		return PatchMonImport{}, fmt.Errorf("%w: administrator identity is required", ErrInvalidInput)
	}
	if len(input.Receipt) < 32 || len(input.Receipt) > 32768 {
		return PatchMonImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if len(input.HostIDs) < 1 || len(input.HostIDs) > 5000 {
		return PatchMonImport{}, fmt.Errorf("%w: select between 1 and 5000 hosts", ErrInvalidInput)
	}
	selection := make(map[string]struct{}, len(input.HostIDs))
	for _, id := range input.HostIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return PatchMonImport{}, fmt.Errorf("%w: selected host identity must not be empty", ErrInvalidInput)
		}
		if _, duplicate := selection[id]; duplicate {
			return PatchMonImport{}, fmt.Errorf("%w: selected hosts must be unique", ErrInvalidInput)
		}
		selection[id] = struct{}{}
	}
	assignments, err := validateTargetAssignments(selection, input.TargetAssignments)
	if err != nil {
		return PatchMonImport{}, err
	}
	plaintext, err := service.secrets.Open(input.Receipt, "patchmon-preview-v1")
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	var receipt patchMonReceipt
	if err := json.Unmarshal(plaintext, &receipt); err != nil {
		return PatchMonImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if !service.now().UTC().Before(receipt.ExpiresAt) {
		return PatchMonImport{}, ErrPreviewExpired
	}
	inspection, err := service.patchMon.Inspect(ctx, receipt.Endpoint, receipt.Credentials)
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	selected := make([]patchmon.Host, 0, len(selection))
	for _, host := range inspection.Hosts {
		if _, ok := selection[host.ID]; ok {
			selected = append(selected, host)
			delete(selection, host.ID)
		}
	}
	if len(selection) != 0 {
		return PatchMonImport{}, fmt.Errorf("%w: one or more selected hosts are no longer available", ErrInvalidInput)
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	encodedCredential, err := json.Marshal(receipt.Credentials)
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("encode PatchMon credential: %w", err)
	}
	credential, err := service.secrets.Seal(encodedCredential, "connector:patchmon:"+inspection.Endpoint)
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("seal PatchMon credential: %w", err)
	}
	result, err := service.store.ImportPatchMon(ctx, PersistPatchMonInput{
		ActorID: actorID, Name: receipt.Name, Endpoint: inspection.Endpoint,
		CredentialSealed: credential, EncryptedTransport: inspection.EncryptedTransport,
		Hosts: selected, TargetAssignments: assignments,
	})
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("import PatchMon hosts: %w", err)
	}
	return result, nil
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func availableTargets(identities []TargetIdentity) []TargetReference {
	seen := make(map[string]struct{}, len(identities))
	targets := make([]TargetReference, 0, len(identities))
	for _, identity := range identities {
		if identity.ID == "" {
			continue
		}
		if _, exists := seen[identity.ID]; exists {
			continue
		}
		seen[identity.ID] = struct{}{}
		targets = append(targets, identity.TargetReference)
	}
	sort.Slice(targets, func(i, j int) bool {
		if normalizeName(targets[i].Name) != normalizeName(targets[j].Name) {
			return normalizeName(targets[i].Name) < normalizeName(targets[j].Name)
		}
		return targets[i].ID < targets[j].ID
	})
	return targets
}

func validateTargetAssignments(selected map[string]struct{}, assignments map[string]string) (map[string]string, error) {
	validated := make(map[string]string, len(assignments))
	for externalID, targetID := range assignments {
		externalID = strings.TrimSpace(externalID)
		targetID = strings.TrimSpace(targetID)
		if _, ok := selected[externalID]; !ok {
			return nil, fmt.Errorf("%w: a target assignment refers to an unselected object", ErrInvalidInput)
		}
		if !validUUID(targetID) {
			return nil, fmt.Errorf("%w: target assignments must contain valid target identities", ErrInvalidInput)
		}
		validated[externalID] = targetID
	}
	return validated, nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	_, err := hex.DecodeString(compact)
	return err == nil
}
