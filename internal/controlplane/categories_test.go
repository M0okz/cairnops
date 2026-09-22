package controlplane

import "testing"

func TestSuggestCategoryUsesFactsNotConnectorNames(t *testing.T) {
	tests := []struct {
		name    string
		signals []categorySignal
		want    Category
	}{
		{"unknown zabbix", []categorySignal{{Kind: "zabbix"}}, CategoryUnclassified},
		{"HTTP service", []categorySignal{{Kind: "uptime_kuma", Metadata: map[string]any{"type": "http"}}}, CategoryService},
		{"VM inventory", []categorySignal{{Kind: "proxmox", Metadata: map[string]any{"resource_type": "qemu"}}}, CategoryInfrastructure},
		{"software only", []categorySignal{{Kind: "argus", Metadata: map[string]any{"deployed_version": "1"}}}, CategorySoftware},
		{"service with version", []categorySignal{{Kind: "http"}, {Kind: "argus", Metadata: map[string]any{"deployed_version": "1"}}}, CategoryService},
		{"ambiguous mixed", []categorySignal{{Kind: "http"}, {Kind: "icmp"}}, CategoryUnclassified},
		{"heartbeat is not always a scheduled task", []categorySignal{{Kind: "heartbeat"}}, CategoryUnclassified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestCategory(tt.signals); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
