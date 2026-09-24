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
		{"VM inventory", []categorySignal{{Kind: "proxmox", Metadata: map[string]any{"resource_type": "qemu"}}}, CategoryVirtualMachine},
		{"container inventory", []categorySignal{{Kind: "proxmox", Metadata: map[string]any{"resource_type": "lxc"}}}, CategoryContainer},
		{"node inventory", []categorySignal{{Kind: "proxmox", Metadata: map[string]any{"resource_type": "node"}}}, CategoryVirtualHost},
		{"storage inventory", []categorySignal{{Kind: "proxmox", Metadata: map[string]any{"resource_type": "storage"}}}, CategoryStorage},
		{"VM with additional checks", []categorySignal{{Kind: "proxmox", Metadata: map[string]any{"resource_type": "qemu"}}, {Kind: "icmp"}, {Kind: "zabbix", Metadata: map[string]any{"interfaces": []any{"eth0"}}}}, CategoryVirtualMachine},
		{"conflicting inventories", []categorySignal{{Kind: "proxmox", Metadata: map[string]any{"resource_type": "qemu"}}, {Kind: "proxmox", Metadata: map[string]any{"resource_type": "lxc"}}}, CategoryUnclassified},
		{"PatchMon host", []categorySignal{{Kind: "patchmon", Metadata: map[string]any{"machine_id": "abc"}}}, CategoryHost},
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
