package connectors

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/argus"
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

type managedZabbix struct {
	inspection   zabbix.Inspection
	prepared     bool
	provisioned  bool
	revoked      bool
	closed       bool
	inspectToken string
}

func (fake *managedZabbix) Inspect(_ context.Context, _ string, token string) (zabbix.Inspection, error) {
	fake.inspectToken = token
	return fake.inspection, nil
}

func (fake *managedZabbix) PrepareBootstrap(_ context.Context, _, username, password string) (zabbix.Inspection, zabbix.BootstrapSession, error) {
	if username != "installer" || password != "temporary-password" {
		return zabbix.Inspection{}, zabbix.BootstrapSession{}, errors.New("unexpected installer credentials")
	}
	fake.prepared = true
	return fake.inspection, zabbix.BootstrapSession{Endpoint: fake.inspection.Endpoint, Token: "short-session", UserID: "7"}, nil
}

func (fake *managedZabbix) Provision(_ context.Context, _ zabbix.BootstrapSession) (zabbix.ManagedCredential, error) {
	fake.provisioned = true
	return zabbix.ManagedCredential{ID: "42", Token: "generated-runtime-token"}, nil
}

func (fake *managedZabbix) Revoke(_ context.Context, _ zabbix.BootstrapSession, _ string) error {
	fake.revoked = true
	return nil
}

func (fake *managedZabbix) CloseBootstrap(_ context.Context, _ zabbix.BootstrapSession) error {
	fake.closed = true
	return nil
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
	persistedArgus    PersistArgusInput
	statusID          string
	status            string
	deletedID         string
	listed            []Connector
	credential        RuntimeCredential
	importKumaErr     error
}

func (fake *fakeStore) List(context.Context) ([]Connector, error) {
	return fake.listed, nil
}

func (fake *fakeStore) RuntimeCredential(context.Context, string) (RuntimeCredential, error) {
	return fake.credential, nil
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
	if fake.importKumaErr != nil {
		return UptimeKumaImport{}, fake.importKumaErr
	}
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

func (fake *fakeStore) ImportArgus(_ context.Context, input PersistArgusInput) (ArgusImport, error) {
	fake.persistedArgus = input
	targets := make([]ImportedTarget, 0, len(input.Services))
	for _, service := range input.Services {
		targets = append(targets, ImportedTarget{ExternalID: service.ID, TargetID: "target-" + service.ID, TargetName: service.Name, Disposition: "created"})
	}
	return ArgusImport{Connector: Connector{ID: "connector-argus", Kind: "argus", Name: input.Name, Endpoint: input.Endpoint}, Targets: targets}, nil
}

type fakeUptimeKuma struct {
	inspection uptimekuma.Inspection
	err        error
	calls      int
}

type managedUptimeKuma struct {
	inspection  uptimekuma.Inspection
	provisioned bool
	revoked     bool
	inspectKey  string
}

func (fake *managedUptimeKuma) Inspect(_ context.Context, _ string, key string) (uptimekuma.Inspection, error) {
	fake.inspectKey = key
	return fake.inspection, nil
}

func (fake *managedUptimeKuma) PrepareBootstrap(_ context.Context, _, username, password, secondFactor string) (uptimekuma.Inspection, uptimekuma.BootstrapSession, error) {
	if username != "installer" || password != "temporary-password" || secondFactor != "123456" {
		return uptimekuma.Inspection{}, uptimekuma.BootstrapSession{}, errors.New("unexpected installer credentials")
	}
	return fake.inspection, uptimekuma.BootstrapSession{Endpoint: fake.inspection.Endpoint, Token: "installer-jwt", Version: "2.3.2"}, nil
}

func (fake *managedUptimeKuma) Provision(_ context.Context, _ uptimekuma.BootstrapSession) (uptimekuma.ManagedCredential, error) {
	fake.provisioned = true
	return uptimekuma.ManagedCredential{ID: "9", APIKey: "uk9_managed"}, nil
}

func (fake *managedUptimeKuma) Revoke(_ context.Context, _ uptimekuma.BootstrapSession, _ string) error {
	fake.revoked = true
	return nil
}

type fakePatchMon struct {
	inspection  patchmon.Inspection
	err         error
	calls       int
	credentials patchmon.Credentials
}

type managedPatchMon struct {
	inspection  patchmon.Inspection
	provisioned bool
	revoked     bool
	credentials patchmon.Credentials
}

func (fake *managedPatchMon) Inspect(_ context.Context, _ string, credentials patchmon.Credentials) (patchmon.Inspection, error) {
	fake.credentials = credentials
	return fake.inspection, nil
}

func (fake *managedPatchMon) PrepareBootstrap(_ context.Context, _, username, password, secondFactor string) (patchmon.Inspection, patchmon.BootstrapSession, error) {
	if username != "installer" || password != "temporary-password" || secondFactor != "123456" {
		return patchmon.Inspection{}, patchmon.BootstrapSession{}, errors.New("unexpected installer credentials")
	}
	return fake.inspection, patchmon.BootstrapSession{Endpoint: fake.inspection.Endpoint, Token: "installer-jwt"}, nil
}

func (fake *managedPatchMon) Provision(_ context.Context, _ patchmon.BootstrapSession) (patchmon.ManagedCredential, error) {
	fake.provisioned = true
	return patchmon.ManagedCredential{ID: "token-9", Credentials: patchmon.Credentials{Key: "managed-key", Secret: "managed-secret"}}, nil
}

func (fake *managedPatchMon) Revoke(_ context.Context, _ patchmon.BootstrapSession, _ string) error {
	fake.revoked = true
	return nil
}

type fakeArgus struct {
	inspection  argus.Inspection
	err         error
	calls       int
	credentials argus.Credentials
}

func (fake *fakeArgus) Inspect(_ context.Context, _ string, credentials argus.Credentials) (argus.Inspection, error) {
	fake.calls++
	fake.credentials = credentials
	return fake.inspection, fake.err
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
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box, nil)
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

func TestManagedZabbixAccessCreatesTokenOnlyAfterConfirmation(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x39}, 32))
	remote := &managedZabbix{inspection: zabbix.Inspection{
		Endpoint: "https://zabbix.example.net/api_jsonrpc.php", Version: "7.4.2",
		Compatibility: "supported", CompatibilityLabel: "Version prise en charge", EncryptedTransport: true,
		Hosts: []zabbix.Host{{ID: "10084", Name: "Database"}},
	}}
	store := &fakeStore{state: PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}}}
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box, nil)

	preview, err := service.PreviewZabbix(context.Background(), ZabbixPreviewInput{
		Address: "https://zabbix.example.net", Username: "installer", Password: "temporary-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !remote.prepared || remote.provisioned || preview.Access.Mode != "managed" || len(preview.Access.RemoteChanges) != 1 {
		t.Fatalf("managed access was not staged before confirmation: preview=%#v remote=%#v", preview.Access, remote)
	}
	if strings.Contains(preview.Receipt, "temporary-password") || strings.Contains(preview.Receipt, "short-session") {
		t.Fatal("opaque receipt exposed bootstrap credentials")
	}

	if _, err := service.ImportZabbix(context.Background(), "administrator-one", ZabbixImportInput{
		Receipt: preview.Receipt, HostIDs: []string{"10084"},
	}); err != nil {
		t.Fatal(err)
	}
	if !remote.provisioned || remote.revoked || !remote.closed || remote.inspectToken != "generated-runtime-token" {
		t.Fatalf("managed token lifecycle was not completed: %#v", remote)
	}
	if store.persisted.CredentialManagement != "managed" || store.persisted.ManagedCredentialID != "42" {
		t.Fatalf("managed credential ownership was not persisted: %#v", store.persisted)
	}
	plaintext, err := box.Open(store.persisted.CredentialSealed, "connector:zabbix:"+remote.inspection.Endpoint)
	if err != nil || string(plaintext) != "generated-runtime-token" {
		t.Fatalf("only the generated runtime token should be stored: %q err=%v", plaintext, err)
	}
}

func TestGuidedAccessRejectsMixedOrIncompleteCredentials(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x40}, 32))
	service := NewService(&fakeStore{}, &managedZabbix{}, &managedUptimeKuma{}, &managedPatchMon{}, box, nil)
	ctx := context.Background()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "Zabbix mixed modes",
			run: func() error {
				_, err := service.PreviewZabbix(ctx, ZabbixPreviewInput{Address: "https://zabbix.example", Username: "installer", Password: "password", APIToken: "token"})
				return err
			},
		},
		{
			name: "Uptime Kuma missing password",
			run: func() error {
				_, err := service.PreviewUptimeKuma(ctx, UptimeKumaPreviewInput{Address: "https://kuma.example", Username: "installer"})
				return err
			},
		},
		{
			name: "PatchMon incomplete manual token",
			run: func() error {
				_, err := service.PreviewPatchMon(ctx, PatchMonPreviewInput{Address: "https://patchmon.example", TokenKey: "key"})
				return err
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}

func TestPreviewExistingConnectorReusesSealedCredentialWithoutExposingIt(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(bytes.Repeat([]byte{0x38}, 32))
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "https://zabbix.example.net/api_jsonrpc.php"
	sealed, err := box.Seal([]byte("stored-secret-token"), "connector:zabbix:"+endpoint)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{
		listed: []Connector{{ID: "connector-one", Kind: "zabbix", Name: "Production", Endpoint: endpoint}},
		credential: RuntimeCredential{
			Kind: "zabbix", Endpoint: endpoint, CredentialSealed: sealed,
			CredentialManagement: "managed", ManagedCredentialID: "42",
		},
		state: PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}},
	}
	remote := &fakeZabbix{inspection: zabbix.Inspection{
		Endpoint: endpoint, Version: "7.4.2", Compatibility: "supported",
		EncryptedTransport: true, Hosts: []zabbix.Host{{ID: "10084", Name: "Authentik"}},
	}}
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box, nil)

	result, err := service.PreviewExisting(context.Background(), "connector-one")
	if err != nil {
		t.Fatalf("preview existing connector: %v", err)
	}
	preview, ok := result.(ZabbixPreview)
	if !ok || remote.calls != 1 || preview.Name != "Production" || len(preview.Hosts) != 1 || preview.Receipt == "" || strings.Contains(preview.Receipt, "stored-secret-token") {
		t.Fatalf("stored connector inventory was not reopened safely: %#v calls=%d", result, remote.calls)
	}
	if preview.Access.Mode != "managed" || preview.Access.WillProvision || len(preview.Access.RemoteChanges) != 0 {
		t.Fatalf("managed ownership should be shown without provisioning another token: %#v", preview.Access)
	}
	if _, err := service.ImportZabbix(context.Background(), "administrator-one", ZabbixImportInput{
		Receipt: preview.Receipt, HostIDs: []string{"10084"},
	}); err != nil {
		t.Fatal(err)
	}
	if store.persisted.CredentialManagement != "managed" || store.persisted.ManagedCredentialID != "42" {
		t.Fatalf("reopening inventory lost managed credential ownership: %#v", store.persisted)
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
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box, nil)
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
	service := NewService(store, &fakeZabbix{}, remote, &fakePatchMon{}, box, nil)
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

func TestManagedUptimeKumaAccessStoresOnlyCreatedAPIKey(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x64}, 32))
	remote := &managedUptimeKuma{inspection: uptimekuma.Inspection{
		Endpoint: "https://kuma.example.net/metrics", EncryptedTransport: true,
		Monitors: []uptimekuma.Monitor{{ID: "12", Name: "Database", Type: "tcp", Status: 1}},
	}}
	store := &fakeStore{state: PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}}}
	service := NewService(store, &fakeZabbix{}, remote, &fakePatchMon{}, box, nil)
	preview, err := service.PreviewUptimeKuma(context.Background(), UptimeKumaPreviewInput{
		Address: "https://kuma.example.net", Username: "installer", Password: "temporary-password", SecondFactor: "123456",
	})
	if err != nil || preview.Access.Mode != "managed" || remote.provisioned {
		t.Fatalf("unexpected managed preview: %#v err=%v", preview, err)
	}
	if _, err := service.ImportUptimeKuma(context.Background(), "administrator-one", UptimeKumaImportInput{
		Receipt: preview.Receipt, MonitorIDs: []string{"12"},
	}); err != nil {
		t.Fatal(err)
	}
	if !remote.provisioned || remote.revoked || remote.inspectKey != "uk9_managed" || store.persistedKuma.ManagedCredentialID != "9" {
		t.Fatalf("unexpected managed key lifecycle: remote=%#v persisted=%#v", remote, store.persistedKuma)
	}
	plaintext, err := box.Open(store.persistedKuma.CredentialSealed, "connector:uptime_kuma:"+remote.inspection.Endpoint)
	if err != nil || string(plaintext) != "uk9_managed" || strings.Contains(string(plaintext), "temporary-password") {
		t.Fatalf("only the generated API key should be stored: %q err=%v", plaintext, err)
	}
}

func TestManagedCredentialIsRevokedWhenAtomicImportFails(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x65}, 32))
	remote := &managedUptimeKuma{inspection: uptimekuma.Inspection{
		Endpoint: "https://kuma.example.net/metrics", EncryptedTransport: true,
		Monitors: []uptimekuma.Monitor{{ID: "12", Name: "Database", Status: 1}},
	}}
	store := &fakeStore{
		state:         PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}},
		importKumaErr: errors.New("transaction rolled back"),
	}
	service := NewService(store, &fakeZabbix{}, remote, &fakePatchMon{}, box, nil)
	preview, err := service.PreviewUptimeKuma(context.Background(), UptimeKumaPreviewInput{
		Address: "https://kuma.example.net", Username: "installer", Password: "temporary-password", SecondFactor: "123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ImportUptimeKuma(context.Background(), "administrator-one", UptimeKumaImportInput{
		Receipt: preview.Receipt, MonitorIDs: []string{"12"},
	})
	if err == nil || !remote.revoked {
		t.Fatalf("failed import must revoke the newly created remote key: err=%v remote=%#v", err, remote)
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
	service := NewService(store, &fakeZabbix{}, &fakeUptimeKuma{}, remote, box, nil)
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

func TestManagedPatchMonAccessStoresOnlyScopedCredential(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x75}, 32))
	remote := &managedPatchMon{inspection: patchmon.Inspection{
		Endpoint: "https://patchmon.example.net/api/v1/api/hosts", EncryptedTransport: true,
		Hosts: []patchmon.Host{{ID: "host-12", FriendlyName: "Database", Hostname: "db"}},
	}}
	store := &fakeStore{state: PreviewState{TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{}}}
	service := NewService(store, &fakeZabbix{}, &fakeUptimeKuma{}, remote, box, nil)
	preview, err := service.PreviewPatchMon(context.Background(), PatchMonPreviewInput{
		Address: "https://patchmon.example.net", Username: "installer", Password: "temporary-password", SecondFactor: "123456",
	})
	if err != nil || preview.Access.Mode != "managed" || remote.provisioned {
		t.Fatalf("unexpected managed preview: %#v err=%v", preview, err)
	}
	if _, err := service.ImportPatchMon(context.Background(), "administrator-one", PatchMonImportInput{
		Receipt: preview.Receipt, HostIDs: []string{"host-12"},
	}); err != nil {
		t.Fatal(err)
	}
	if !remote.provisioned || remote.revoked || remote.credentials.Secret != "managed-secret" || store.persistedPatchMon.ManagedCredentialID != "token-9" {
		t.Fatalf("unexpected managed token lifecycle: remote=%#v persisted=%#v", remote, store.persistedPatchMon)
	}
	plaintext, err := box.Open(store.persistedPatchMon.CredentialSealed, "connector:patchmon:"+remote.inspection.Endpoint)
	if err != nil || strings.Contains(string(plaintext), "temporary-password") || !strings.Contains(string(plaintext), "managed-secret") {
		t.Fatalf("only the generated scoped credential should be stored: %q err=%v", plaintext, err)
	}
}

func TestPreviewAndImportArgusPreparesEligibleServicesAndSealsBasicAuth(t *testing.T) {
	t.Parallel()
	const targetID = "12345678-1234-4234-8234-123456789012"
	box, err := secretbox.New(bytes.Repeat([]byte{0x72}, 32))
	if err != nil {
		t.Fatal(err)
	}
	remote := &fakeArgus{inspection: argus.Inspection{
		Endpoint: "https://argus.example.net/argus", EncryptedTransport: true,
		Version: "0.35.0", Compatibility: "supported",
		Services: []argus.Service{
			{ID: "api", Name: "Public API", Active: true, Importable: true, DeployedVersion: "1.2.2", LatestVersion: "1.2.3", Approved: true, DeploymentState: argus.DeploymentStateApproved, LatestQueryOK: true, DeployedQueryOK: true, VersionURL: "https://releases.example/1.2.3"},
			{ID: "disabled", Name: "Disabled", Active: false, Importable: false, Ineligibility: argus.IneligibilityInactive},
		},
	}}
	store := &fakeStore{state: PreviewState{
		Targets:       []TargetIdentity{{TargetReference: TargetReference{ID: targetID, Name: "Public API"}, Names: []string{"Public API"}}},
		TargetsByName: map[string]TargetReference{}, ImportedByExternalID: map[string]TargetReference{},
	}}
	service := NewService(store, &fakeZabbix{}, &fakeUptimeKuma{}, &fakePatchMon{}, box, remote)
	service.now = func() time.Time { return time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC) }

	preview, err := service.PreviewArgus(context.Background(), ArgusPreviewInput{
		Name: "Versions", Address: "https://argus.example.net/argus", Username: "reader", Password: " secret ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Receipt == "" || preview.Receipt == "secret" || preview.Version != "0.35.0" || preview.ImportableCount != 1 || preview.PendingUpdateCount != 1 || len(preview.Services) != 2 {
		t.Fatalf("unexpected Argus preview: %#v", preview)
	}
	if preview.Services[0].SuggestedTarget == nil || preview.Services[0].SuggestedTarget.ID != targetID || !preview.Services[0].Importable {
		t.Fatalf("expected explicit target suggestion for importable service: %#v", preview.Services[0])
	}
	if preview.Services[1].Importable || preview.Services[1].Ineligibility != argus.IneligibilityInactive {
		t.Fatalf("expected disabled service to remain visible and ineligible: %#v", preview.Services[1])
	}

	result, err := service.ImportArgus(context.Background(), "administrator-one", ArgusImportInput{
		Receipt: preview.Receipt, ServiceIDs: []string{"api"}, TargetAssignments: map[string]string{"api": targetID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if remote.calls != 2 || remote.credentials.Password != " secret " || len(store.persistedArgus.Services) != 1 || store.persistedArgus.CredentialSealed == " secret " {
		t.Fatalf("expected fresh Argus discovery with sealed credentials: calls=%d persisted=%#v", remote.calls, store.persistedArgus)
	}
	if store.persistedArgus.Version != "0.35.0" || len(result.Targets) != 1 || result.Targets[0].ExternalID != "api" {
		t.Fatalf("unexpected Argus import: %#v", result)
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
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box, nil)

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
	service := NewService(store, remote, &fakeUptimeKuma{}, &fakePatchMon{}, box, nil)
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
