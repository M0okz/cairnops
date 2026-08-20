package connectors

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type fakeZabbix struct {
	inspection zabbix.Inspection
	err        error
	calls      int
}

func (fake *fakeZabbix) Inspect(context.Context, string, string) (zabbix.Inspection, error) {
	fake.calls++
	return fake.inspection, fake.err
}

type fakeStore struct {
	state             PreviewState
	persisted         PersistZabbixInput
	persistedKuma     PersistUptimeKumaInput
	persistedPatchMon PersistPatchMonInput
	statusID          string
	status            string
	deletedID         string
}

func (*fakeStore) List(context.Context) ([]Connector, error) {
	return []Connector{}, nil
}

func (fake *fakeStore) SetStatus(_ context.Context, connectorID, status string) (Connector, error) {
	fake.statusID, fake.status = connectorID, status
	return Connector{ID: connectorID, Kind: "zabbix", Status: status}, nil
}

func (fake *fakeStore) Delete(_ context.Context, connectorID string) (Removal, error) {
	fake.deletedID = connectorID
	return Removal{ID: connectorID, Kind: "zabbix", Name: "Production", Bindings: 3, ResolvedIncidents: 1}, nil
}

func (fake *fakeStore) PreviewState(context.Context, string, string, []string) (PreviewState, error) {
	return fake.state, nil
}

func (fake *fakeStore) ImportUptimeKuma(_ context.Context, input PersistUptimeKumaInput) (UptimeKumaImport, error) {
	fake.persistedKuma = input
	targets := make([]ImportedTarget, 0, len(input.Monitors))
	for _, monitor := range input.Monitors {
		targets = append(targets, ImportedTarget{ExternalID: monitor.ID, TargetID: "target-" + monitor.ID, TargetName: monitor.Name, Disposition: "created"})
	}
	return UptimeKumaImport{Connector: Connector{ID: "connector-kuma", Kind: "uptime_kuma", Name: input.Name, Endpoint: input.Endpoint}, Targets: targets}, nil
}

func (fake *fakeStore) ImportPatchMon(_ context.Context, input PersistPatchMonInput) (PatchMonImport, error) {
	fake.persistedPatchMon = input
	targets := make([]ImportedTarget, 0, len(input.Hosts))
	for _, host := range input.Hosts {
		targets = append(targets, ImportedTarget{ExternalID: host.ID, TargetID: "target-" + host.ID, TargetName: host.Name(), Disposition: "created"})
	}
	return PatchMonImport{Connector: Connector{ID: "connector-patchmon", Kind: "patchmon", Name: input.Name, Endpoint: input.Endpoint}, Targets: targets}, nil
}

type fakeUptimeKuma struct {
	inspection uptimekuma.Inspection
	err        error
	calls      int
}

type fakePatchMon struct {
	inspection  patchmon.Inspection
	err         error
	calls       int
	credentials patchmon.Credentials
}

func (fake *fakePatchMon) Inspect(_ context.Context, _ string, credentials patchmon.Credentials) (patchmon.Inspection, error) {
	fake.calls++
	fake.credentials = credentials
	return fake.inspection, fake.err
}

func (fake *fakeUptimeKuma) Inspect(context.Context, string, string) (uptimekuma.Inspection, error) {
	fake.calls++
	return fake.inspection, fake.err
}

func (fake *fakeStore) ImportZabbix(_ context.Context, input PersistZabbixInput) (ZabbixImport, error) {
	fake.persisted = input
	targets := make([]ImportedTarget, 0, len(input.Hosts))
	for _, host := range input.Hosts {
		targets = append(targets, ImportedTarget{ExternalID: host.ID, TargetID: "target-" + host.ID, TargetName: host.Name, Disposition: "created"})
	}
	return ZabbixImport{Connector: Connector{ID: "connector-one", Kind: "zabbix", Name: input.Name, Endpoint: input.Endpoint}, Targets: targets}, nil
}

func TestPreviewAndImportZabbixUseSealedShortLivedReceipt(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(bytes.Repeat([]byte{0x37}, 32))
	if err != nil {
		t.Fatal(err)
	}
	remote := &fakeZabbix{inspection: zabbix.Inspection{
		Endpoint: "https://zabbix.example.net/api_jsonrpc.php", Version: "7.4.2",
		Compatibility: "supported", CompatibilityLabel: "Version prise en charge", EncryptedTransport: true,
		Hosts: []zabbix.Host{{ID: "10084", Name: "Zabbix Server"}, {ID: "10085", Name: "Database"}},
	}}
	store := &fakeStore{state: PreviewState{
		TargetsByName:        map[string]TargetReference{"database": {ID: "target-db", Name: "Database"}},
		ImportedByExternalID: map[string]TargetReference{},
	}}
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box)
	service.now = func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) }

	preview, err := service.PreviewZabbix(context.Background(), ZabbixPreviewInput{
		Name: "Production", Address: "https://zabbix.example.net", APIToken: "secret-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Receipt == "" || preview.Receipt == "secret-token" || preview.ExpiresAt.Sub(service.now()) != previewLifetime {
		t.Fatalf("unexpected sealed preview: %#v", preview)
	}
	if preview.Hosts[1].SuggestedTarget == nil || preview.Hosts[1].SuggestedTarget.ID != "target-db" {
		t.Fatalf("expected exact target suggestion, got %#v", preview.Hosts[1])
	}

	result, err := service.ImportZabbix(context.Background(), "administrator-one", ZabbixImportInput{
		Receipt: preview.Receipt, HostIDs: []string{"10085"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if remote.calls != 2 || len(store.persisted.Hosts) != 1 || store.persisted.Hosts[0].ID != "10085" {
		t.Fatalf("expected a fresh selected discovery, calls=%d persisted=%#v", remote.calls, store.persisted.Hosts)
	}
	if store.persisted.CredentialSealed == "" || store.persisted.CredentialSealed == "secret-token" {
		t.Fatal("expected the stored credential to be sealed")
	}
	if len(result.Targets) != 1 || result.Targets[0].ExternalID != "10085" {
		t.Fatalf("unexpected import result: %#v", result)
	}
}

func TestImportRejectsExpiredOrTamperedPreview(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x21}, 32))
	remote := &fakeZabbix{inspection: zabbix.Inspection{
		Endpoint: "https://zabbix.example.net/api_jsonrpc.php", Version: "7.4.2",
		Compatibility: "supported", Hosts: []zabbix.Host{{ID: "1", Name: "One"}},
	}}
	store := &fakeStore{state: PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}}}
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	preview, err := service.PreviewZabbix(context.Background(), ZabbixPreviewInput{Address: "https://zabbix.example.net", APIToken: "token"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.ImportZabbix(context.Background(), "admin", ZabbixImportInput{Receipt: preview.Receipt + "x", HostIDs: []string{"1"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected tampered receipt error, got %v", err)
	}
	now = now.Add(previewLifetime)
	if _, err := service.ImportZabbix(context.Background(), "admin", ZabbixImportInput{Receipt: preview.Receipt, HostIDs: []string{"1"}}); !errors.Is(err, ErrPreviewExpired) {
		t.Fatalf("expected expired preview error, got %v", err)
	}
}

func TestPreviewAndImportUptimeKumaUseMetricsAPIKeyReceipt(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(bytes.Repeat([]byte{0x63}, 32))
	if err != nil {
		t.Fatal(err)
	}
	remote := &fakeUptimeKuma{inspection: uptimekuma.Inspection{
		Endpoint: "https://kuma.example.net/metrics", EncryptedTransport: true,
		Monitors: []uptimekuma.Monitor{{ID: "12", Name: "Database", Type: "tcp", Hostname: "db.internal", Port: "5432", Status: 0}},
	}}
	store := &fakeStore{state: PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}}}
	service := NewService(store, &fakeZabbix{}, remote, &fakePatchMon{}, box)
	service.now = func() time.Time { return time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC) }

	preview, err := service.PreviewUptimeKuma(context.Background(), UptimeKumaPreviewInput{
		Name: "Kuma production", Address: "https://kuma.example.net", APIKey: "uk2-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Receipt == "" || preview.Receipt == "uk2-secret" || len(preview.Monitors) != 1 || preview.Monitors[0].Address != "db.internal:5432" {
		t.Fatalf("unexpected Uptime Kuma preview: %#v", preview)
	}
	result, err := service.ImportUptimeKuma(context.Background(), "administrator-one", UptimeKumaImportInput{
		Receipt: preview.Receipt, MonitorIDs: []string{"12"},
		TargetAssignments: map[string]string{"12": "12345678-1234-4234-8234-123456789012"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if remote.calls != 2 || len(store.persistedKuma.Monitors) != 1 || store.persistedKuma.CredentialSealed == "uk2-secret" {
		t.Fatalf("expected a fresh selected discovery with a sealed credential: calls=%d persisted=%#v", remote.calls, store.persistedKuma)
	}
	if len(result.Targets) != 1 || result.Targets[0].ExternalID != "12" {
		t.Fatalf("unexpected Uptime Kuma import result: %#v", result)
	}
	if store.persistedKuma.TargetAssignments["12"] != "12345678-1234-4234-8234-123456789012" {
		t.Fatalf("explicit target assignment was lost: %#v", store.persistedKuma.TargetAssignments)
	}
}

func TestPreviewAndImportPatchMonUseReadOnlyScopedReceipt(t *testing.T) {
	t.Parallel()
	const targetID = "12345678-1234-4234-8234-123456789012"
	box, err := secretbox.New(bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	remote := &fakePatchMon{inspection: patchmon.Inspection{
		Endpoint: "https://patchmon.example.net/api/v1/api/hosts", EncryptedTransport: true,
		Hosts: []patchmon.Host{{ID: "host-12", MachineID: "machine-db", FriendlyName: "Database", Hostname: "db.internal", IP: "192.0.2.12", Status: "active", SecurityUpdatesCount: 3}},
	}}
	store := &fakeStore{state: PreviewState{
		Targets:       []TargetIdentity{{TargetReference: TargetReference{ID: targetID, Name: "Database"}, Identifiers: []string{"machine-db"}}},
		TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{},
	}}
	service := NewService(store, &fakeZabbix{}, &fakeUptimeKuma{}, remote, box)
	service.now = func() time.Time { return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC) }

	preview, err := service.PreviewPatchMon(context.Background(), PatchMonPreviewInput{
		Name: "Patch posture", Address: "https://patchmon.example.net", TokenKey: "patchmon_key", TokenSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Receipt == "" || preview.Receipt == "secret" || len(preview.Hosts) != 1 || preview.Hosts[0].SuggestedTarget == nil || preview.Hosts[0].SuggestedTarget.ID != targetID {
		t.Fatalf("unexpected PatchMon preview: %#v", preview)
	}
	result, err := service.ImportPatchMon(context.Background(), "administrator-one", PatchMonImportInput{
		Receipt: preview.Receipt, HostIDs: []string{"host-12"}, TargetAssignments: map[string]string{"host-12": targetID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if remote.calls != 2 || remote.credentials.Secret != "secret" || len(store.persistedPatchMon.Hosts) != 1 || store.persistedPatchMon.CredentialSealed == "secret" {
		t.Fatalf("expected fresh PatchMon discovery with sealed credentials: calls=%d persisted=%#v", remote.calls, store.persistedPatchMon)
	}
	if len(result.Targets) != 1 || result.Targets[0].ExternalID != "host-12" {
		t.Fatalf("unexpected PatchMon import: %#v", result)
	}
}

func TestPreviewSuggestsCrossConnectorTargetWithEvidence(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	target := TargetIdentity{
		TargetReference: TargetReference{ID: "12345678-1234-4234-8234-123456789012", Name: "Passerelle"},
		Names:           []string{"Passerelle"}, Addresses: []string{"192.0.2.42"},
	}
	store := &fakeStore{state: PreviewState{
		TargetsByName: map[string]TargetReference{}, Targets: []TargetIdentity{target},
		ImportedByExternalID: map[string]TargetReference{},
	}}
	remote := &fakeZabbix{inspection: zabbix.Inspection{
		Endpoint: "https://zabbix.example.net/api_jsonrpc.php", Version: "7.4.2", Compatibility: "supported",
		Hosts: []zabbix.Host{{ID: "42", Name: "gw-prod", Interfaces: []zabbix.Interface{{Address: "192.0.2.42", Main: true}}}},
	}}
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box)

	preview, err := service.PreviewZabbix(context.Background(), ZabbixPreviewInput{Address: "https://zabbix.example.net", APIToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.AvailableTargets) != 1 || preview.Hosts[0].SuggestedTarget == nil || preview.Hosts[0].SuggestedTarget.ID != target.ID {
		t.Fatalf("expected an IP suggestion and the active target list, got %#v", preview)
	}
	if len(preview.Hosts[0].CandidateTargets) != 1 || preview.Hosts[0].CandidateTargets[0].Evidence[0].Kind != "same_ip" {
		t.Fatalf("expected explainable matching evidence, got %#v", preview.Hosts[0].CandidateTargets)
	}
}

func TestImportRejectsAssignmentForUnselectedObject(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x74}, 32))
	remote := &fakeZabbix{inspection: zabbix.Inspection{
		Endpoint: "https://zabbix.example.net/api_jsonrpc.php", Version: "7.4.2", Compatibility: "supported",
		Hosts: []zabbix.Host{{ID: "1", Name: "One"}, {ID: "2", Name: "Two"}},
	}}
	store := &fakeStore{state: PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}}}
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box)
	preview, err := service.PreviewZabbix(context.Background(), ZabbixPreviewInput{Address: "https://zabbix.example.net", APIToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ImportZabbix(context.Background(), "admin", ZabbixImportInput{
		Receipt: preview.Receipt, HostIDs: []string{"1"},
		TargetAssignments: map[string]string{"2": "12345678-1234-4234-8234-123456789012"},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected an invalid assignment error, got %v", err)
	}
}
