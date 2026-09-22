package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/connectors/argus"
	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/proxmox"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
)

var (
	ErrEndpointConflict  = errors.New("connector endpoint already connected")
	ErrConnectionChanged = errors.New("connector connection changed; test the connection again")
	ErrConnectionBusy    = errors.New("connector synchronization is still running")
)

// Empty credential fields retain the corresponding stored value. This flow
// never provisions or revokes a remote credential, and never imports inventory.
type ConnectionInput struct {
	Name        string              `json:"name"`
	Address     string              `json:"address"`
	APIToken    string              `json:"api_token,omitempty"`
	APIKey      string              `json:"api_key,omitempty"`
	TokenKey    string              `json:"token_key,omitempty"`
	TokenSecret string              `json:"token_secret,omitempty"`
	Username    string              `json:"username,omitempty"`
	Password    string              `json:"password,omitempty"`
	Credentials proxmox.Credentials `json:"credentials,omitempty"`
}

type ConnectionTest struct {
	Name               string    `json:"name"`
	Endpoint           string    `json:"endpoint"`
	Version            string    `json:"version"`
	Compatibility      string    `json:"compatibility"`
	EncryptedTransport bool      `json:"encrypted_transport"`
	Receipt            string    `json:"receipt"`
	ExpiresAt          time.Time `json:"expires_at"`
}

type ConnectionSaveInput struct {
	Receipt string `json:"receipt"`
}

type connectionReplacement struct {
	ConnectorID        string
	Kind               string
	PreviousEndpoint   string
	PreviousCredential string
	Name               string
	Endpoint           string
	CredentialSealed   string
	Version            string
	Compatibility      string
	EncryptedTransport bool
}

type connectionReceipt struct {
	Replacement connectionReplacement
	ExpiresAt   time.Time
}

type connectionStore interface {
	RemovalCredential(context.Context, string) (RuntimeCredential, error)
	ReplaceConnection(context.Context, connectionReplacement) (Connector, error)
}

// TestConnection only reads the remote API. The sealed receipt binds the exact
// tested configuration to this connector and its current credential version.
func (service *Service) TestConnection(ctx context.Context, connectorID string, input ConnectionInput) (ConnectionTest, error) {
	store, ok := service.store.(connectionStore)
	if !ok || !validUUID(connectorID) {
		return ConnectionTest{}, fmt.Errorf("%w: invalid connector ID", ErrInvalidInput)
	}
	input.Name = strings.TrimSpace(input.Name)
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 160 {
		return ConnectionTest{}, fmt.Errorf("%w: connector name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	current, err := store.RemovalCredential(ctx, connectorID)
	if err != nil {
		return ConnectionTest{}, err
	}
	endpoint, err := normalizeConnectionEndpoint(current.Kind, input.Address)
	if err != nil {
		return ConnectionTest{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	items, err := service.store.List(ctx)
	if err != nil {
		return ConnectionTest{}, err
	}
	for _, item := range items {
		if item.ID != connectorID && item.Kind == current.Kind && strings.EqualFold(item.Endpoint, endpoint) {
			return ConnectionTest{}, ErrEndpointConflict
		}
	}
	raw, err := service.secrets.Open(current.CredentialSealed, "connector:"+current.Kind+":"+current.Endpoint)
	if err != nil {
		return ConnectionTest{}, fmt.Errorf("open connection credential: %w", err)
	}
	raw, err = replacementCredential(current.Kind, raw, input)
	if err != nil {
		return ConnectionTest{}, err
	}
	result, err := service.inspectConnection(ctx, current.Kind, endpoint, raw)
	if err != nil {
		return ConnectionTest{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	result.Name = input.Name
	sealed, err := service.secrets.Seal(raw, "connector:"+current.Kind+":"+result.Endpoint)
	if err != nil {
		return ConnectionTest{}, err
	}
	result.ExpiresAt = service.now().UTC().Add(previewLifetime)
	receipt := connectionReceipt{ExpiresAt: result.ExpiresAt, Replacement: connectionReplacement{
		ConnectorID: connectorID, Kind: current.Kind, PreviousEndpoint: current.Endpoint, PreviousCredential: current.CredentialSealed,
		Name: input.Name, Endpoint: result.Endpoint, CredentialSealed: sealed, Version: result.Version,
		Compatibility: result.Compatibility, EncryptedTransport: result.EncryptedTransport,
	}}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return ConnectionTest{}, err
	}
	result.Receipt, err = service.secrets.Seal(encoded, "connector-connection-v1")
	return result, err
}

func (service *Service) SaveConnection(ctx context.Context, connectorID string, input ConnectionSaveInput) (Connector, error) {
	store, ok := service.store.(connectionStore)
	if !ok || !validUUID(connectorID) || len(input.Receipt) < 32 || len(input.Receipt) > 131072 {
		return Connector{}, fmt.Errorf("%w: invalid connection receipt", ErrInvalidInput)
	}
	raw, err := service.secrets.Open(input.Receipt, "connector-connection-v1")
	if err != nil {
		return Connector{}, fmt.Errorf("%w: invalid connection receipt", ErrInvalidInput)
	}
	var receipt connectionReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil || receipt.Replacement.ConnectorID != connectorID {
		return Connector{}, fmt.Errorf("%w: invalid connection receipt", ErrInvalidInput)
	}
	if !service.now().UTC().Before(receipt.ExpiresAt) {
		return Connector{}, ErrPreviewExpired
	}
	return store.ReplaceConnection(ctx, receipt.Replacement)
}

func normalizeConnectionEndpoint(kind, address string) (string, error) {
	switch kind {
	case "zabbix":
		return zabbix.NormalizeEndpoint(address)
	case "uptime_kuma":
		return uptimekuma.NormalizeEndpoint(address)
	case "patchmon":
		return patchmon.NormalizeEndpoint(address)
	case "argus":
		return argus.NormalizeEndpoint(address)
	case "proxmox":
		return proxmox.NormalizeEndpoint(address)
	default:
		return "", fmt.Errorf("this connector does not have an editable connection")
	}
}

func replacementCredential(kind string, raw []byte, input ConnectionInput) ([]byte, error) {
	keep := func(previous, replacement string) string {
		if replacement == "" {
			return previous
		}
		return replacement
	}
	switch kind {
	case "zabbix":
		return []byte(keep(string(raw), strings.TrimSpace(input.APIToken))), nil
	case "uptime_kuma":
		return []byte(keep(string(raw), strings.TrimSpace(input.APIKey))), nil
	case "patchmon":
		var credential patchmon.Credentials
		if err := json.Unmarshal(raw, &credential); err != nil {
			return nil, err
		}
		credential.Key = keep(credential.Key, strings.TrimSpace(input.TokenKey))
		credential.Secret = keep(credential.Secret, input.TokenSecret)
		return json.Marshal(credential)
	case "argus":
		var credential argus.Credentials
		if err := json.Unmarshal(raw, &credential); err != nil {
			return nil, err
		}
		credential.Username = keep(credential.Username, strings.TrimSpace(input.Username))
		credential.Password = keep(credential.Password, input.Password)
		return json.Marshal(credential)
	case "proxmox":
		var credential proxmox.Credentials
		if err := json.Unmarshal(raw, &credential); err != nil {
			return nil, err
		}
		credential.TokenID = keep(credential.TokenID, strings.TrimSpace(input.Credentials.TokenID))
		credential.Secret = keep(credential.Secret, input.Credentials.Secret)
		credential.Fingerprint = keep(credential.Fingerprint, strings.TrimSpace(input.Credentials.Fingerprint))
		return json.Marshal(credential)
	}
	return nil, fmt.Errorf("%w: unsupported connector", ErrInvalidInput)
}

func (service *Service) inspectConnection(ctx context.Context, kind, endpoint string, raw []byte) (ConnectionTest, error) {
	result := ConnectionTest{Endpoint: endpoint, Compatibility: "supported"}
	switch kind {
	case "zabbix":
		if service.zabbix == nil {
			return result, errors.New("Zabbix client is unavailable")
		}
		inspection, err := service.zabbix.Inspect(ctx, endpoint, string(raw))
		result.Endpoint, result.Version, result.Compatibility, result.EncryptedTransport = inspection.Endpoint, inspection.Version, inspection.Compatibility, inspection.EncryptedTransport
		return result, err
	case "uptime_kuma":
		if service.uptimeKuma == nil {
			return result, errors.New("Uptime Kuma client is unavailable")
		}
		inspection, err := service.uptimeKuma.Inspect(ctx, endpoint, string(raw))
		result.Endpoint, result.EncryptedTransport = inspection.Endpoint, inspection.EncryptedTransport
		return result, err
	case "patchmon":
		if service.patchMon == nil {
			return result, errors.New("PatchMon client is unavailable")
		}
		var credential patchmon.Credentials
		if err := json.Unmarshal(raw, &credential); err != nil {
			return result, err
		}
		inspection, err := service.patchMon.Inspect(ctx, endpoint, credential)
		result.Endpoint, result.EncryptedTransport = inspection.Endpoint, inspection.EncryptedTransport
		return result, err
	case "argus":
		if service.argus == nil {
			return result, errors.New("Argus client is unavailable")
		}
		var credential argus.Credentials
		if err := json.Unmarshal(raw, &credential); err != nil {
			return result, err
		}
		inspection, err := service.argus.Inspect(ctx, endpoint, credential)
		result.Endpoint, result.Version, result.Compatibility, result.EncryptedTransport = inspection.Endpoint, inspection.Version, inspection.Compatibility, inspection.EncryptedTransport
		return result, err
	case "proxmox":
		if service.proxmox == nil {
			return result, errors.New("Proxmox VE client is unavailable")
		}
		var credential proxmox.Credentials
		if err := json.Unmarshal(raw, &credential); err != nil {
			return result, err
		}
		inspection, err := service.proxmox.Inspect(ctx, endpoint, credential)
		result.Endpoint, result.Version, result.EncryptedTransport = inspection.Endpoint, inspection.Version, true
		return result, err
	}
	return result, errors.New("unsupported connector")
}
