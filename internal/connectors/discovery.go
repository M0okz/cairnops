package connectors

import (
	"github.com/M0okz/cairnops/internal/connectors/argus"
	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
)

// DiscoveryObject is an identity from a complete, authenticated inventory.
// Matching proposes a target; it never authorizes an automatic association.
type DiscoveryObject struct {
	ExternalID string
	Name       string
	Eligible   bool
	Identity   DiscoveredIdentity
	Metadata   map[string]any
}

func zabbixDiscoveries(hosts []zabbix.Host) []DiscoveryObject {
	objects := make([]DiscoveryObject, 0, len(hosts))
	for _, h := range hosts {
		objects = append(objects, DiscoveryObject{h.ID, h.Name, true, identityForZabbix(h), map[string]any{"technical_name": h.Technical, "interfaces": h.Interfaces}})
	}
	return objects
}
func kumaDiscoveries(monitors []uptimekuma.Monitor) []DiscoveryObject {
	objects := make([]DiscoveryObject, 0, len(monitors))
	for _, m := range monitors {
		objects = append(objects, DiscoveryObject{m.ID, m.Name, true, identityForUptimeKuma(m), map[string]any{"type": m.Type, "address": m.Address(), "url": m.URL, "hostname": m.Hostname, "port": m.Port}})
	}
	return objects
}
func patchMonDiscoveries(hosts []patchmon.Host) []DiscoveryObject {
	objects := make([]DiscoveryObject, 0, len(hosts))
	for _, h := range hosts {
		objects = append(objects, DiscoveryObject{h.ID, h.Name(), true, identityForPatchMon(h), map[string]any{"machine_id": h.MachineID, "hostname": h.Hostname, "address": h.IP, "os_type": h.OSType, "os_version": h.OSVersion, "host_groups": h.HostGroups}})
	}
	return objects
}
func argusDiscoveries(endpoint string, services []argus.Service) []DiscoveryObject {
	objects := make([]DiscoveryObject, 0, len(services))
	for _, s := range services {
		objects = append(objects, DiscoveryObject{s.ID, s.Name, s.Importable, DiscoveredIdentity{Names: []string{s.Name}}, argusDetails(endpoint, s)})
	}
	return objects
}
