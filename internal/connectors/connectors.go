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

	"github.com/M0okz/cairnops/internal/connectors/argus"
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
	ErrStructureBusy  = errors.New("target reconciliation is in progress")
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

type AccessPreview struct {
	Mode          string   `json:"mode"`
	WillProvision bool     `json:"will_provision"`
	RemoteChanges []string `json:"remote_changes,omitempty"`
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
	Name                 string `json:"name"`
	Address              string `json:"address"`
	Username             string `json:"username,omitempty"`
	Password             string `json:"password,omitempty"`
	APIToken             string `json:"api_token,omitempty"`
	credentialManagement string
	managedCredentialID  string
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
	Access             AccessPreview     `json:"access"`
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
	Name                 string `json:"name"`
	Address              string `json:"address"`
	Username             string `json:"username,omitempty"`
	Password             string `json:"password,omitempty"`
	SecondFactor         string `json:"second_factor,omitempty"`
	APIKey               string `json:"api_key,omitempty"`
	credentialManagement string
	managedCredentialID  string
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
	Access             AccessPreview              `json:"access"`
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
	Name                 string `json:"name"`
	Address              string `json:"address"`
	Username             string `json:"username,omitempty"`
	Password             string `json:"password,omitempty"`
	SecondFactor         string `json:"second_factor,omitempty"`
	TokenKey             string `json:"token_key,omitempty"`
	TokenSecret          string `json:"token_secret,omitempty"`
	credentialManagement string
	managedCredentialID  string
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
	Access             AccessPreview         `json:"access"`
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

type ArgusServicePreview struct {
	ExternalID        string                `json:"external_id"`
	Name              string                `json:"name"`
	Active            bool                  `json:"active"`
	Importable        bool                  `json:"importable"`
	Ineligibility     string                `json:"ineligibility,omitempty"`
	DeployedVersion   string                `json:"deployed_version,omitempty"`
	LatestVersion     string                `json:"latest_version,omitempty"`
	LastChecked       string                `json:"last_checked,omitempty"`
	Approved          bool                  `json:"approved"`
	Skipped           bool                  `json:"skipped"`
	Unknown           bool                  `json:"unknown"`
	UnknownReason     string                `json:"unknown_reason,omitempty"`
	DeploymentState   argus.DeploymentState `json:"deployment_state"`
	VersionURL        string                `json:"version_url,omitempty"`
	CandidateTargets  []TargetMatch         `json:"candidate_targets,omitempty"`
	SuggestedTarget   *TargetReference      `json:"suggested_target,omitempty"`
	AlreadyImportedTo *TargetReference      `json:"already_imported_to,omitempty"`
}

type ArgusPreviewInput struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ArgusPreview struct {
	Kind               string                `json:"kind"`
	Name               string                `json:"name"`
	Endpoint           string                `json:"endpoint"`
	Version            string                `json:"version"`
	Compatibility      string                `json:"compatibility"`
	CompatibilityLabel string                `json:"compatibility_label"`
	EncryptedTransport bool                  `json:"encrypted_transport"`
	ImportableCount    int                   `json:"importable_count"`
	PendingUpdateCount int                   `json:"pending_update_count"`
	Services           []ArgusServicePreview `json:"services"`
	AvailableTargets   []TargetReference     `json:"available_targets"`
	Receipt            string                `json:"receipt"`
	ExpiresAt          time.Time             `json:"expires_at"`
}

type ArgusImportInput struct {
	Receipt           string            `json:"receipt"`
	ServiceIDs        []string          `json:"service_ids"`
	TargetAssignments map[string]string `json:"target_assignments,omitempty"`
}

type ArgusImport struct {
	Connector Connector        `json:"connector"`
	Targets   []ImportedTarget `json:"targets"`
}

type PreviewState struct {
	TargetsByName        map[string]TargetReference
	Targets              []TargetIdentity
	ImportedByExternalID map[string]TargetReference
}

type PersistZabbixInput struct {
	ActorID              string
	Name                 string
	Endpoint             string
	CredentialSealed     string
	Version              string
	Compatibility        string
	EncryptedTransport   bool
	Hosts                []zabbix.Host
	TargetAssignments    map[string]string
	CredentialManagement string
	ManagedCredentialID  string
}

type PersistUptimeKumaInput struct {
	ActorID              string
	Name                 string
	Endpoint             string
	CredentialSealed     string
	EncryptedTransport   bool
	Monitors             []uptimekuma.Monitor
	TargetAssignments    map[string]string
	CredentialManagement string
	ManagedCredentialID  string
}

type PersistPatchMonInput struct {
	ActorID              string
	Name                 string
	Endpoint             string
	CredentialSealed     string
	EncryptedTransport   bool
	Hosts                []patchmon.Host
	TargetAssignments    map[string]string
	CredentialManagement string
	ManagedCredentialID  string
}

type PersistArgusInput struct {
	ActorID            string
	Name               string
	Endpoint           string
	CredentialSealed   string
	Version            string
	EncryptedTransport bool
	Services           []argus.Service
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
	ImportArgus(context.Context, PersistArgusInput) (ArgusImport, error)
}

type ZabbixClient interface {
	Inspect(context.Context, string, string) (zabbix.Inspection, error)
}

type ZabbixBootstrapper interface {
	PrepareBootstrap(context.Context, string, string, string) (zabbix.Inspection, zabbix.BootstrapSession, error)
	Provision(context.Context, zabbix.BootstrapSession) (zabbix.ManagedCredential, error)
	Revoke(context.Context, zabbix.BootstrapSession, string) error
	CloseBootstrap(context.Context, zabbix.BootstrapSession) error
}

type UptimeKumaClient interface {
	Inspect(context.Context, string, string) (uptimekuma.Inspection, error)
}

type UptimeKumaBootstrapper interface {
	PrepareBootstrap(context.Context, string, string, string, string) (uptimekuma.Inspection, uptimekuma.BootstrapSession, error)
	Provision(context.Context, uptimekuma.BootstrapSession) (uptimekuma.ManagedCredential, error)
	Revoke(context.Context, uptimekuma.BootstrapSession, string) error
}

type PatchMonClient interface {
	Inspect(context.Context, string, patchmon.Credentials) (patchmon.Inspection, error)
}

type PatchMonBootstrapper interface {
	PrepareBootstrap(context.Context, string, string, string, string) (patchmon.Inspection, patchmon.BootstrapSession, error)
	Provision(context.Context, patchmon.BootstrapSession) (patchmon.ManagedCredential, error)
	Revoke(context.Context, patchmon.BootstrapSession, string) error
}

type ArgusClient interface {
	Inspect(context.Context, string, argus.Credentials) (argus.Inspection, error)
}

type Service struct {
	store      Store
	zabbix     ZabbixClient
	uptimeKuma UptimeKumaClient
	patchMon   PatchMonClient
	argus      ArgusClient
	secrets    *secretbox.Box
	now        func() time.Time
}

type zabbixReceipt struct {
	Name                string                   `json:"name"`
	Endpoint            string                   `json:"endpoint"`
	Mode                string                   `json:"mode"`
	APIToken            string                   `json:"api_token,omitempty"`
	ManagedCredentialID string                   `json:"managed_credential_id,omitempty"`
	Bootstrap           *zabbix.BootstrapSession `json:"bootstrap,omitempty"`
	ExpiresAt           time.Time                `json:"expires_at"`
}

type uptimeKumaReceipt struct {
	Name                string                       `json:"name"`
	Endpoint            string                       `json:"endpoint"`
	Mode                string                       `json:"mode"`
	APIKey              string                       `json:"api_key,omitempty"`
	ManagedCredentialID string                       `json:"managed_credential_id,omitempty"`
	Bootstrap           *uptimekuma.BootstrapSession `json:"bootstrap,omitempty"`
	ExpiresAt           time.Time                    `json:"expires_at"`
}

type patchMonReceipt struct {
	Name                string                     `json:"name"`
	Endpoint            string                     `json:"endpoint"`
	Mode                string                     `json:"mode"`
	Credentials         patchmon.Credentials       `json:"credentials,omitempty"`
	ManagedCredentialID string                     `json:"managed_credential_id,omitempty"`
	Bootstrap           *patchmon.BootstrapSession `json:"bootstrap,omitempty"`
	ExpiresAt           time.Time                  `json:"expires_at"`
}

type argusReceipt struct {
	Name        string            `json:"name"`
	Endpoint    string            `json:"endpoint"`
	Credentials argus.Credentials `json:"credentials"`
	ExpiresAt   time.Time         `json:"expires_at"`
}

func NewService(store Store, zabbixClient ZabbixClient, uptimeKumaClient UptimeKumaClient, patchMonClient PatchMonClient, secrets *secretbox.Box, argusClient ArgusClient) *Service {
	return &Service{
		store: store, zabbix: zabbixClient, uptimeKuma: uptimeKumaClient,
		patchMon: patchMonClient, argus: argusClient, secrets: secrets, now: time.Now,
	}
}

func (service *Service) List(ctx context.Context) ([]Connector, error) {
	return service.store.List(ctx)
}

type runtimeCredentialStore interface {
	RuntimeCredential(context.Context, string) (RuntimeCredential, error)
}

// PreviewExisting rouvre l'inventaire avec le secret déjà scellé. Il produit
// exactement le même reçu court que le parcours de connexion : l'import reste
// donc explicite et ne reçoit jamais le secret persistant dans le navigateur.
func (service *Service) PreviewExisting(ctx context.Context, connectorID string) (any, error) {
	connectorID = strings.TrimSpace(connectorID)
	credentialStore, ok := service.store.(runtimeCredentialStore)
	if connectorID == "" || !ok {
		return nil, fmt.Errorf("%w: connector identity is required", ErrInvalidInput)
	}
	credential, err := credentialStore.RuntimeCredential(ctx, connectorID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	name := ""
	items, err := service.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connector for inventory: %w", err)
	}
	for _, item := range items {
		if item.ID == connectorID {
			name = item.Name
			break
		}
	}
	if name == "" {
		return nil, ErrNotFound
	}
	plaintext, err := service.secrets.Open(credential.CredentialSealed, "connector:"+credential.Kind+":"+credential.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("open connector inventory credential: %w", err)
	}
	switch credential.Kind {
	case "zabbix":
		return service.PreviewZabbix(ctx, ZabbixPreviewInput{
			Name: name, Address: credential.Endpoint, APIToken: string(plaintext),
			credentialManagement: credential.CredentialManagement,
			managedCredentialID:  credential.ManagedCredentialID,
		})
	case "uptime_kuma":
		return service.PreviewUptimeKuma(ctx, UptimeKumaPreviewInput{
			Name: name, Address: credential.Endpoint, APIKey: string(plaintext),
			credentialManagement: credential.CredentialManagement,
			managedCredentialID:  credential.ManagedCredentialID,
		})
	case "patchmon":
		var patchCredentials patchmon.Credentials
		if err := json.Unmarshal(plaintext, &patchCredentials); err != nil {
			return nil, fmt.Errorf("decode PatchMon inventory credential: %w", err)
		}
		return service.PreviewPatchMon(ctx, PatchMonPreviewInput{
			Name: name, Address: credential.Endpoint,
			TokenKey: patchCredentials.Key, TokenSecret: patchCredentials.Secret,
			credentialManagement: credential.CredentialManagement,
			managedCredentialID:  credential.ManagedCredentialID,
		})
	case "argus":
		var credentials argus.Credentials
		if err := json.Unmarshal(plaintext, &credentials); err != nil {
			return nil, fmt.Errorf("decode Argus inventory credential: %w", err)
		}
		return service.PreviewArgus(ctx, ArgusPreviewInput{
			Name: name, Address: credential.Endpoint,
			Username: credentials.Username, Password: credentials.Password,
		})
	default:
		return nil, fmt.Errorf("%w: connector does not expose a discoverable inventory", ErrInvalidInput)
	}
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
	mode := input.credentialManagement
	if mode != "managed" {
		mode = "provided"
	}
	access := AccessPreview{Mode: mode}
	var bootstrap *zabbix.BootstrapSession
	var inspection zabbix.Inspection
	var err error
	manualAccess := strings.TrimSpace(input.APIToken) != ""
	bootstrapAccess := strings.TrimSpace(input.Username) != "" || input.Password != ""
	if manualAccess && bootstrapAccess {
		return ZabbixPreview{}, fmt.Errorf("%w: choose automatic setup or an existing Zabbix token, not both", ErrInvalidInput)
	}
	if manualAccess {
		inspection, err = service.zabbix.Inspect(ctx, input.Address, input.APIToken)
	} else {
		if strings.TrimSpace(input.Username) == "" || input.Password == "" {
			return ZabbixPreview{}, fmt.Errorf("%w: username and password are required for automatic Zabbix setup", ErrInvalidInput)
		}
		provisioner, ok := service.zabbix.(ZabbixBootstrapper)
		if !ok {
			return ZabbixPreview{}, fmt.Errorf("%w: automatic Zabbix token setup is unavailable", ErrConnection)
		}
		var session zabbix.BootstrapSession
		inspection, session, err = provisioner.PrepareBootstrap(ctx, input.Address, input.Username, input.Password)
		bootstrap = &session
		mode = "managed"
		access = AccessPreview{
			Mode: mode, WillProvision: true, RemoteChanges: []string{"create_zabbix_api_token"},
		}
	}
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
		if discovered.SuggestedTarget == nil && len(discovered.CandidateTargets) == 0 {
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
		Mode: mode, APIToken: strings.TrimSpace(input.APIToken),
		ManagedCredentialID: input.managedCredentialID, Bootstrap: bootstrap, ExpiresAt: expiresAt,
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
		Access:  access,
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
	mode := input.credentialManagement
	if mode != "managed" {
		mode = "provided"
	}
	access := AccessPreview{Mode: mode}
	var bootstrap *uptimekuma.BootstrapSession
	var inspection uptimekuma.Inspection
	var err error
	manualAccess := strings.TrimSpace(input.APIKey) != ""
	bootstrapAccess := strings.TrimSpace(input.Username) != "" || input.Password != "" || strings.TrimSpace(input.SecondFactor) != ""
	if manualAccess && bootstrapAccess {
		return UptimeKumaPreview{}, fmt.Errorf("%w: choose automatic setup or an existing Uptime Kuma API key, not both", ErrInvalidInput)
	}
	if manualAccess {
		inspection, err = service.uptimeKuma.Inspect(ctx, input.Address, input.APIKey)
	} else {
		if strings.TrimSpace(input.Username) == "" || input.Password == "" {
			return UptimeKumaPreview{}, fmt.Errorf("%w: username and password are required for automatic Uptime Kuma setup", ErrInvalidInput)
		}
		provisioner, ok := service.uptimeKuma.(UptimeKumaBootstrapper)
		if !ok {
			return UptimeKumaPreview{}, fmt.Errorf("%w: automatic Uptime Kuma API-key setup is unavailable", ErrConnection)
		}
		var session uptimekuma.BootstrapSession
		inspection, session, err = provisioner.PrepareBootstrap(ctx, input.Address, input.Username, input.Password, input.SecondFactor)
		bootstrap = &session
		mode = "managed"
		access = AccessPreview{
			Mode: mode, WillProvision: true, RemoteChanges: []string{"create_uptime_kuma_api_key"},
		}
	}
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
		if discovered.SuggestedTarget == nil && len(discovered.CandidateTargets) == 0 {
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
		Mode: mode, APIKey: strings.TrimSpace(input.APIKey),
		ManagedCredentialID: input.managedCredentialID, Bootstrap: bootstrap, ExpiresAt: expiresAt,
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
		AvailableTargets: availableTargets(state.Targets), Access: access,
		Receipt: receipt, ExpiresAt: expiresAt,
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
	mode := input.credentialManagement
	if mode != "managed" {
		mode = "provided"
	}
	access := AccessPreview{Mode: mode}
	credentials := patchmon.Credentials{Key: input.TokenKey, Secret: input.TokenSecret}
	var bootstrap *patchmon.BootstrapSession
	var inspection patchmon.Inspection
	var err error
	manualAccess := strings.TrimSpace(input.TokenKey) != "" || strings.TrimSpace(input.TokenSecret) != ""
	bootstrapAccess := strings.TrimSpace(input.Username) != "" || input.Password != "" || strings.TrimSpace(input.SecondFactor) != ""
	if manualAccess && bootstrapAccess {
		return PatchMonPreview{}, fmt.Errorf("%w: choose automatic setup or an existing PatchMon token, not both", ErrInvalidInput)
	}
	if manualAccess {
		if strings.TrimSpace(input.TokenKey) == "" || strings.TrimSpace(input.TokenSecret) == "" {
			return PatchMonPreview{}, fmt.Errorf("%w: both PatchMon token key and secret are required", ErrInvalidInput)
		}
		inspection, err = service.patchMon.Inspect(ctx, input.Address, credentials)
	} else {
		if strings.TrimSpace(input.Username) == "" || input.Password == "" {
			return PatchMonPreview{}, fmt.Errorf("%w: username and password are required for automatic PatchMon setup", ErrInvalidInput)
		}
		provisioner, ok := service.patchMon.(PatchMonBootstrapper)
		if !ok {
			return PatchMonPreview{}, fmt.Errorf("%w: automatic PatchMon API-token setup is unavailable", ErrConnection)
		}
		var session patchmon.BootstrapSession
		inspection, session, err = provisioner.PrepareBootstrap(ctx, input.Address, input.Username, input.Password, input.SecondFactor)
		bootstrap = &session
		mode = "managed"
		access = AccessPreview{
			Mode: mode, WillProvision: true, RemoteChanges: []string{"create_patchmon_host_read_token"},
		}
	}
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
		if discovered.SuggestedTarget == nil && len(discovered.CandidateTargets) == 0 {
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
		Mode:                mode,
		Credentials:         patchmon.Credentials{Key: strings.TrimSpace(input.TokenKey), Secret: strings.TrimSpace(input.TokenSecret)},
		ManagedCredentialID: input.managedCredentialID,
		Bootstrap:           bootstrap,
		ExpiresAt:           expiresAt,
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
		AvailableTargets: availableTargets(state.Targets), Access: access,
		Receipt: receipt, ExpiresAt: expiresAt,
	}, nil
}

func (service *Service) PreviewArgus(ctx context.Context, input ArgusPreviewInput) (ArgusPreview, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "Argus"
	}
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 160 {
		return ArgusPreview{}, fmt.Errorf("%w: connector name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	if service.argus == nil {
		return ArgusPreview{}, fmt.Errorf("%w: Argus client is unavailable", ErrConnection)
	}
	credentials := argus.Credentials{Username: input.Username, Password: input.Password}
	inspection, err := service.argus.Inspect(ctx, input.Address, credentials)
	if err != nil {
		return ArgusPreview{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	names := make([]string, 0, len(inspection.Services))
	for _, discoveredService := range inspection.Services {
		names = append(names, discoveredService.Name)
	}
	state, err := service.store.PreviewState(ctx, "argus", inspection.Endpoint, names)
	if err != nil {
		return ArgusPreview{}, fmt.Errorf("prepare Argus preview: %w", err)
	}
	services := make([]ArgusServicePreview, 0, len(inspection.Services))
	importableCount, pendingUpdateCount := 0, 0
	for _, discoveredService := range inspection.Services {
		discovered := ArgusServicePreview{
			ExternalID: discoveredService.ID, Name: discoveredService.Name,
			Active: discoveredService.Active, Importable: discoveredService.Importable,
			Ineligibility:   discoveredService.Ineligibility,
			DeployedVersion: discoveredService.DeployedVersion, LatestVersion: discoveredService.LatestVersion,
			LastChecked: discoveredService.LastChecked, Approved: discoveredService.Approved,
			Skipped: discoveredService.Skipped, Unknown: discoveredService.Unknown,
			UnknownReason: discoveredService.UnknownReason, DeploymentState: discoveredService.DeploymentState,
			VersionURL: discoveredService.VersionURL,
		}
		discovered.CandidateTargets = matchTargets(DiscoveredIdentity{Names: []string{discoveredService.Name}}, state.Targets)
		discovered.SuggestedTarget = suggestedTarget(discovered.CandidateTargets)
		if discovered.SuggestedTarget == nil && len(discovered.CandidateTargets) == 0 {
			if target, ok := state.TargetsByName[normalizeName(discoveredService.Name)]; ok {
				targetCopy := target
				discovered.SuggestedTarget = &targetCopy
			}
		}
		if target, ok := state.ImportedByExternalID[discoveredService.ID]; ok {
			targetCopy := target
			discovered.AlreadyImportedTo = &targetCopy
		}
		if discoveredService.Importable {
			importableCount++
			if !discoveredService.Unknown && !discoveredService.Skipped && discoveredService.DeployedVersion != discoveredService.LatestVersion {
				pendingUpdateCount++
			}
		}
		services = append(services, discovered)
	}
	expiresAt := service.now().UTC().Add(previewLifetime)
	receiptPayload, err := json.Marshal(argusReceipt{
		Name: input.Name, Endpoint: inspection.Endpoint,
		Credentials: argus.Credentials{Username: strings.TrimSpace(input.Username), Password: input.Password},
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return ArgusPreview{}, fmt.Errorf("encode Argus preview: %w", err)
	}
	receipt, err := service.secrets.Seal(receiptPayload, "argus-preview-v1")
	if err != nil {
		return ArgusPreview{}, fmt.Errorf("seal Argus preview: %w", err)
	}
	return ArgusPreview{
		Kind: "argus", Name: input.Name, Endpoint: inspection.Endpoint,
		Version: inspection.Version, Compatibility: inspection.Compatibility,
		CompatibilityLabel: "Argus " + inspection.Version + " · API en lecture seule",
		EncryptedTransport: inspection.EncryptedTransport,
		ImportableCount:    importableCount, PendingUpdateCount: pendingUpdateCount,
		Services: services, AvailableTargets: availableTargets(state.Targets),
		Receipt: receipt, ExpiresAt: expiresAt,
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
	mode := receipt.Mode
	if mode == "" {
		mode = "provided"
	}
	apiToken := receipt.APIToken
	managedCredentialID := receipt.ManagedCredentialID
	var provisioner ZabbixBootstrapper
	persisted := false
	if mode == "managed" {
		if receipt.Bootstrap == nil {
			if strings.TrimSpace(apiToken) == "" || strings.TrimSpace(managedCredentialID) == "" {
				return ZabbixImport{}, fmt.Errorf("%w: invalid managed preview receipt", ErrInvalidInput)
			}
		} else {
			var ok bool
			provisioner, ok = service.zabbix.(ZabbixBootstrapper)
			if !ok {
				return ZabbixImport{}, fmt.Errorf("%w: automatic Zabbix token setup is unavailable", ErrConnection)
			}
			managed, provisionErr := provisioner.Provision(ctx, *receipt.Bootstrap)
			if provisionErr != nil {
				return ZabbixImport{}, fmt.Errorf("%w: %v", ErrConnection, provisionErr)
			}
			apiToken, managedCredentialID = managed.Token, managed.ID
			defer func() {
				cleanupCtx, cancel := connectorCleanupContext(ctx)
				defer cancel()
				if !persisted {
					_ = provisioner.Revoke(cleanupCtx, *receipt.Bootstrap, managedCredentialID)
				}
				_ = provisioner.CloseBootstrap(cleanupCtx, *receipt.Bootstrap)
			}()
		}
	}
	inspection, err := service.zabbix.Inspect(ctx, receipt.Endpoint, apiToken)
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
	credential, err := service.secrets.Seal([]byte(apiToken), "connector:zabbix:"+inspection.Endpoint)
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("seal Zabbix credential: %w", err)
	}
	result, err := service.store.ImportZabbix(ctx, PersistZabbixInput{
		ActorID: actorID, Name: receipt.Name, Endpoint: inspection.Endpoint,
		CredentialSealed: credential, Version: inspection.Version,
		Compatibility: inspection.Compatibility, EncryptedTransport: inspection.EncryptedTransport,
		Hosts: selected, TargetAssignments: assignments,
		CredentialManagement: mode, ManagedCredentialID: managedCredentialID,
	})
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("import Zabbix hosts: %w", err)
	}
	persisted = true
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
	mode := receipt.Mode
	if mode == "" {
		mode = "provided"
	}
	apiKey := receipt.APIKey
	managedCredentialID := receipt.ManagedCredentialID
	var provisioner UptimeKumaBootstrapper
	persisted := false
	if mode == "managed" {
		if receipt.Bootstrap == nil {
			if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(managedCredentialID) == "" {
				return UptimeKumaImport{}, fmt.Errorf("%w: invalid managed preview receipt", ErrInvalidInput)
			}
		} else {
			var ok bool
			provisioner, ok = service.uptimeKuma.(UptimeKumaBootstrapper)
			if !ok {
				return UptimeKumaImport{}, fmt.Errorf("%w: automatic Uptime Kuma API-key setup is unavailable", ErrConnection)
			}
			managed, provisionErr := provisioner.Provision(ctx, *receipt.Bootstrap)
			if provisionErr != nil {
				return UptimeKumaImport{}, fmt.Errorf("%w: %v", ErrConnection, provisionErr)
			}
			apiKey, managedCredentialID = managed.APIKey, managed.ID
			defer func() {
				cleanupCtx, cancel := connectorCleanupContext(ctx)
				defer cancel()
				if !persisted {
					_ = provisioner.Revoke(cleanupCtx, *receipt.Bootstrap, managedCredentialID)
				}
			}()
		}
	}
	inspection, err := service.uptimeKuma.Inspect(ctx, receipt.Endpoint, apiKey)
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
	credential, err := service.secrets.Seal([]byte(apiKey), "connector:uptime_kuma:"+inspection.Endpoint)
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("seal Uptime Kuma credential: %w", err)
	}
	result, err := service.store.ImportUptimeKuma(ctx, PersistUptimeKumaInput{
		ActorID: actorID, Name: receipt.Name, Endpoint: inspection.Endpoint,
		CredentialSealed: credential, EncryptedTransport: inspection.EncryptedTransport,
		Monitors: selected, TargetAssignments: assignments,
		CredentialManagement: mode, ManagedCredentialID: managedCredentialID,
	})
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("import Uptime Kuma monitors: %w", err)
	}
	persisted = true
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
	mode := receipt.Mode
	if mode == "" {
		mode = "provided"
	}
	credentials := receipt.Credentials
	managedCredentialID := receipt.ManagedCredentialID
	var provisioner PatchMonBootstrapper
	persisted := false
	if mode == "managed" {
		if receipt.Bootstrap == nil {
			if strings.TrimSpace(credentials.Key) == "" || strings.TrimSpace(credentials.Secret) == "" || strings.TrimSpace(managedCredentialID) == "" {
				return PatchMonImport{}, fmt.Errorf("%w: invalid managed preview receipt", ErrInvalidInput)
			}
		} else {
			var ok bool
			provisioner, ok = service.patchMon.(PatchMonBootstrapper)
			if !ok {
				return PatchMonImport{}, fmt.Errorf("%w: automatic PatchMon API-token setup is unavailable", ErrConnection)
			}
			managed, provisionErr := provisioner.Provision(ctx, *receipt.Bootstrap)
			if provisionErr != nil {
				return PatchMonImport{}, fmt.Errorf("%w: %v", ErrConnection, provisionErr)
			}
			credentials, managedCredentialID = managed.Credentials, managed.ID
			defer func() {
				cleanupCtx, cancel := connectorCleanupContext(ctx)
				defer cancel()
				if !persisted {
					_ = provisioner.Revoke(cleanupCtx, *receipt.Bootstrap, managedCredentialID)
				}
			}()
		}
	}
	inspection, err := service.patchMon.Inspect(ctx, receipt.Endpoint, credentials)
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
	encodedCredential, err := json.Marshal(credentials)
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
		CredentialManagement: mode, ManagedCredentialID: managedCredentialID,
	})
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("import PatchMon hosts: %w", err)
	}
	persisted = true
	return result, nil
}

func (service *Service) ImportArgus(ctx context.Context, actorID string, input ArgusImportInput) (ArgusImport, error) {
	if strings.TrimSpace(actorID) == "" {
		return ArgusImport{}, fmt.Errorf("%w: administrator identity is required", ErrInvalidInput)
	}
	if len(input.Receipt) < 32 || len(input.Receipt) > 32768 {
		return ArgusImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if len(input.ServiceIDs) < 1 || len(input.ServiceIDs) > 5000 {
		return ArgusImport{}, fmt.Errorf("%w: select between 1 and 5000 services", ErrInvalidInput)
	}
	selection := make(map[string]struct{}, len(input.ServiceIDs))
	for _, id := range input.ServiceIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return ArgusImport{}, fmt.Errorf("%w: selected service identity must not be empty", ErrInvalidInput)
		}
		if _, duplicate := selection[id]; duplicate {
			return ArgusImport{}, fmt.Errorf("%w: selected services must be unique", ErrInvalidInput)
		}
		selection[id] = struct{}{}
	}
	assignments, err := validateTargetAssignments(selection, input.TargetAssignments)
	if err != nil {
		return ArgusImport{}, err
	}
	plaintext, err := service.secrets.Open(input.Receipt, "argus-preview-v1")
	if err != nil {
		return ArgusImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	var receipt argusReceipt
	if err := json.Unmarshal(plaintext, &receipt); err != nil {
		return ArgusImport{}, fmt.Errorf("%w: invalid preview receipt", ErrInvalidInput)
	}
	if !service.now().UTC().Before(receipt.ExpiresAt) {
		return ArgusImport{}, ErrPreviewExpired
	}
	if service.argus == nil {
		return ArgusImport{}, fmt.Errorf("%w: Argus client is unavailable", ErrConnection)
	}
	inspection, err := service.argus.Inspect(ctx, receipt.Endpoint, receipt.Credentials)
	if err != nil {
		return ArgusImport{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	selected := make([]argus.Service, 0, len(selection))
	for _, discoveredService := range inspection.Services {
		if _, ok := selection[discoveredService.ID]; !ok {
			continue
		}
		if !discoveredService.Importable {
			return ArgusImport{}, fmt.Errorf("%w: Argus service %s is no longer eligible for import", ErrInvalidInput, discoveredService.ID)
		}
		selected = append(selected, discoveredService)
		delete(selection, discoveredService.ID)
	}
	if len(selection) != 0 {
		return ArgusImport{}, fmt.Errorf("%w: one or more selected services are no longer available", ErrInvalidInput)
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	encodedCredential, err := json.Marshal(receipt.Credentials)
	if err != nil {
		return ArgusImport{}, fmt.Errorf("encode Argus credential: %w", err)
	}
	credential, err := service.secrets.Seal(encodedCredential, "connector:argus:"+inspection.Endpoint)
	if err != nil {
		return ArgusImport{}, fmt.Errorf("seal Argus credential: %w", err)
	}
	result, err := service.store.ImportArgus(ctx, PersistArgusInput{
		ActorID: actorID, Name: receipt.Name, Endpoint: inspection.Endpoint,
		CredentialSealed: credential, Version: inspection.Version,
		EncryptedTransport: inspection.EncryptedTransport,
		Services:           selected, TargetAssignments: assignments,
	})
	if err != nil {
		return ArgusImport{}, fmt.Errorf("import Argus services: %w", err)
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

func connectorCleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	_, err := hex.DecodeString(compact)
	return err == nil
}
