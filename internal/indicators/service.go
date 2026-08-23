package indicators

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type ZabbixClient interface {
	Inspect(context.Context, string, string) (zabbix.Inspection, error)
	Items(context.Context, string, string, []string, []string) ([]zabbix.Item, error)
}

type UptimeKumaClient interface {
	Monitors(context.Context, string, string) ([]uptimekuma.Monitor, error)
}

type PatchMonClient interface {
	Hosts(context.Context, string, patchmon.Credentials) ([]patchmon.Host, error)
}

type Service struct {
	store      *Store
	zabbix     ZabbixClient
	uptimeKuma UptimeKumaClient
	patchMon   PatchMonClient
	secrets    *secretbox.Box
	now        func() time.Time
}

func NewService(store *Store, zabbixClient ZabbixClient, uptimeKumaClient UptimeKumaClient, patchMonClient PatchMonClient, secrets *secretbox.Box) *Service {
	return &Service{store: store, zabbix: zabbixClient, uptimeKuma: uptimeKumaClient, patchMon: patchMonClient, secrets: secrets, now: time.Now}
}

func (service *Service) Configuration(ctx context.Context, connectorID string) (Configuration, error) {
	return service.store.Configuration(ctx, connectorID)
}

// Preview relit le produit d'origine. Cette projection est le seam entre le
// catalogue sémantique stable de CairnOps et les identités propres au remote.
func (service *Service) Preview(ctx context.Context, connectorID string) (Configuration, error) {
	configuration, err := service.store.Configuration(ctx, connectorID)
	if err != nil {
		return Configuration{}, err
	}
	remote, err := service.store.Remote(ctx, connectorID)
	if err != nil {
		return Configuration{}, err
	}
	byExternal := make(map[string]int, len(configuration.Bindings))
	for index := range configuration.Bindings {
		byExternal[configuration.Bindings[index].ExternalID] = index
		configuration.Bindings[index].Candidates = []Candidate{}
	}
	credential, err := service.secrets.Open(remote.CredentialSealed, "connector:"+remote.Kind+":"+remote.Endpoint)
	if err != nil {
		return Configuration{}, fmt.Errorf("open connector credential: %w", err)
	}
	switch remote.Kind {
	case "zabbix":
		inspection, inspectErr := service.zabbix.Inspect(ctx, remote.Endpoint, string(credential))
		if inspectErr != nil {
			return Configuration{}, fmt.Errorf("discover Zabbix indicators: %w", inspectErr)
		}
		hostIDs := make([]string, 0, len(inspection.Hosts))
		for _, host := range inspection.Hosts {
			hostIDs = append(hostIDs, host.ID)
		}
		items, itemsErr := service.zabbix.Items(ctx, remote.Endpoint, string(credential), hostIDs, nil)
		if itemsErr != nil {
			return Configuration{}, itemsErr
		}
		itemsByHost := map[string][]zabbix.Item{}
		for _, item := range items {
			itemsByHost[item.HostID] = append(itemsByHost[item.HostID], item)
		}
		for _, host := range inspection.Hosts {
			index := ensureBinding(&configuration, byExternal, host.ID, host.Name)
			configuration.Bindings[index].Candidates = zabbixCandidates(itemsByHost[host.ID])
		}
	case "uptime_kuma":
		monitors, monitorErr := service.uptimeKuma.Monitors(ctx, remote.Endpoint, string(credential))
		if monitorErr != nil {
			return Configuration{}, fmt.Errorf("discover Uptime Kuma indicators: %w", monitorErr)
		}
		for _, monitor := range monitors {
			index := ensureBinding(&configuration, byExternal, monitor.ID, monitor.Name)
			configuration.Bindings[index].Candidates = uptimeCandidates(monitor)
			sortCandidates(configuration.Bindings[index].Candidates)
		}
	case "patchmon":
		var credentials patchmon.Credentials
		if err := json.Unmarshal(credential, &credentials); err != nil {
			return Configuration{}, fmt.Errorf("decode PatchMon credential: %w", err)
		}
		hosts, hostErr := service.patchMon.Hosts(ctx, remote.Endpoint, credentials)
		if hostErr != nil {
			return Configuration{}, fmt.Errorf("discover PatchMon indicators: %w", hostErr)
		}
		for _, host := range hosts {
			index := ensureBinding(&configuration, byExternal, host.ID, host.Name())
			configuration.Bindings[index].Candidates = patchMonCandidates(host)
			sortCandidates(configuration.Bindings[index].Candidates)
		}
	default:
		return Configuration{}, fmt.Errorf("%w: connector does not expose contextual indicators", ErrInvalidInput)
	}
	markSelectedAvailability(&configuration)
	now := service.now().UTC()
	expires := now.Add(15 * time.Minute)
	configuration.GeneratedAt, configuration.ExpiresAt = now, &expires
	configuration.Capabilities = mergeCapability(configuration.Capabilities, Capability{Key: "indicators", Status: "available", CheckedAt: now})
	return configuration, nil
}

func ensureBinding(configuration *Configuration, indexes map[string]int, externalID, externalName string) int {
	if index, known := indexes[externalID]; known {
		return index
	}
	index := len(configuration.Bindings)
	indexes[externalID] = index
	configuration.Bindings = append(configuration.Bindings, Binding{ExternalID: externalID, ExternalName: externalName, Enabled: false, Imported: false, Indicators: []Indicator{}, Candidates: []Candidate{}})
	return index
}

func markSelectedAvailability(configuration *Configuration) {
	for bindingIndex := range configuration.Bindings {
		available := map[string]Candidate{}
		for _, candidate := range configuration.Bindings[bindingIndex].Candidates {
			available[candidate.ExternalID+"\x00"+candidate.SemanticKey+"\x00"+candidate.Dimension] = candidate
		}
		for indicatorIndex := range configuration.Bindings[bindingIndex].Indicators {
			indicator := &configuration.Bindings[bindingIndex].Indicators[indicatorIndex]
			if !indicator.Enabled {
				continue
			}
			key := indicator.ExternalID + "\x00" + indicator.SemanticKey + "\x00" + indicator.Dimension
			if candidate, exists := available[key]; !exists || !candidate.Available {
				indicator.LastError = "À vérifier · l’élément externe exact n’est plus disponible"
			}
		}
	}
}

func mergeCapability(capabilities []Capability, next Capability) []Capability {
	for index := range capabilities {
		if capabilities[index].Key == next.Key {
			capabilities[index] = next
			return capabilities
		}
	}
	return append(capabilities, next)
}

func (service *Service) Apply(ctx context.Context, actorID, connectorID string, input ApplyInput) (Configuration, error) {
	if err := validateApply(input); err != nil {
		return Configuration{}, err
	}
	preview, err := service.Preview(ctx, connectorID)
	if err != nil {
		return Configuration{}, err
	}
	allowed := map[string]map[string]Candidate{}
	externalNames := map[string]string{}
	for _, binding := range preview.Bindings {
		allowed[binding.ExternalID] = map[string]Candidate{}
		externalNames[binding.ExternalID] = binding.ExternalName
		for _, candidate := range binding.Candidates {
			allowed[binding.ExternalID][candidate.ExternalID+"\x00"+candidate.SemanticKey+"\x00"+candidate.Dimension] = candidate
		}
	}
	for bindingIndex := range input.Bindings {
		binding := &input.Bindings[bindingIndex]
		candidates, known := allowed[binding.ExternalID]
		if !known {
			return Configuration{}, fmt.Errorf("%w: external target %s is no longer available", ErrInvalidInput, binding.ExternalID)
		}
		binding.ExternalName = externalNames[binding.ExternalID]
		for selectionIndex := range binding.Indicators {
			selection := &binding.Indicators[selectionIndex]
			candidate, known := candidates[selection.ExternalID+"\x00"+selection.SemanticKey+"\x00"+selection.Dimension]
			if !known || candidate.Unit != selection.Unit || !candidate.Available {
				return Configuration{}, fmt.Errorf("%w: indicator %s must be confirmed again", ErrInvalidInput, selection.Label)
			}
			selection.Label = candidate.Label
			selection.Metadata = candidate.Metadata
		}
	}
	if err := service.store.Apply(ctx, actorID, connectorID, input); err != nil {
		return Configuration{}, err
	}
	return service.store.Configuration(ctx, connectorID)
}

func (service *Service) Overview(ctx context.Context, userID string) ([]TargetProjection, error) {
	return service.store.Overview(ctx, userID)
}
func (service *Service) Target(ctx context.Context, userID, targetID, window string) (TargetProjection, error) {
	return service.store.Target(ctx, userID, targetID, window)
}
func (service *Service) Incident(ctx context.Context, userID, incidentID string) (IncidentProjection, error) {
	return service.store.Incident(ctx, userID, incidentID)
}
func (service *Service) Pins(ctx context.Context, userID string) ([]Pin, error) {
	return service.store.Pins(ctx, userID)
}
func (service *Service) SetPins(ctx context.Context, userID string, input PinInput) ([]Pin, error) {
	return service.store.SetPins(ctx, userID, input.IndicatorIDs)
}

func selectionScale(metadata map[string]any) float64 {
	value, ok := metadata["scale"].(float64)
	if !ok || value <= 0 {
		return 1
	}
	return value
}

func externalSuffix(externalID string) string {
	if index := strings.LastIndex(externalID, ":"); index >= 0 {
		return externalID[index+1:]
	}
	return externalID
}
