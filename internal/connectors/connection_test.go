package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/argus"
	"github.com/M0okz/cairnops/internal/connectors/patchmon"
	"github.com/M0okz/cairnops/internal/connectors/proxmox"
	"github.com/M0okz/cairnops/internal/connectors/uptimekuma"
	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/secretbox"
)

const connectionTestID = "12345678-1234-4234-8234-123456789012"

type connectionMemoryStore struct {
	*fakeStore
	saved *connectionReplacement
}

func (store *connectionMemoryStore) RemovalCredential(context.Context, string) (RuntimeCredential, error) {
	return store.credential, nil
}
func (store *connectionMemoryStore) ReplaceConnection(_ context.Context, replacement connectionReplacement) (Connector, error) {
	if replacement.PreviousCredential != store.credential.CredentialSealed {
		return Connector{}, ErrConnectionChanged
	}
	store.saved = &replacement
	store.credential.CredentialSealed = replacement.CredentialSealed
	return Connector{ID: replacement.ConnectorID, Name: replacement.Name, Endpoint: replacement.Endpoint}, nil
}

type connectionProxmox struct {
	ProxmoxClient // Any provisioning call would panic: this flow must only inspect.
	credential    proxmox.Credentials
	endpoint      string
}

func (client *connectionProxmox) Inspect(_ context.Context, endpoint string, credential proxmox.Credentials) (proxmox.Inspection, error) {
	client.credential = credential
	return proxmox.Inspection{Endpoint: client.endpoint, Version: "9.0"}, nil
}

func TestConnectionRetainsOrRotatesCredentialsForEveryProduct(t *testing.T) {
	cases := []struct {
		kind, current, want string
		input               ConnectionInput
	}{
		{"zabbix", "previous-token", "previous-token", ConnectionInput{}},
		{"zabbix", "previous-token", "rotated-token", ConnectionInput{APIToken: "rotated-token"}},
		{"uptime_kuma", "uk_previous", "uk_previous", ConnectionInput{}},
		{"uptime_kuma", "uk_previous", "uk_new", ConnectionInput{APIKey: "uk_new"}},
		{"patchmon", `{"key":"key","secret":"old"}`, `{"key":"key","secret":"old"}`, ConnectionInput{}},
		{"patchmon", `{"key":"key","secret":"old"}`, `{"key":"key","secret":"new"}`, ConnectionInput{TokenSecret: "new"}},
		{"argus", `{"username":"reader","password":"old"}`, `{"username":"reader","password":"old"}`, ConnectionInput{}},
		{"argus", `{"username":"reader","password":"old"}`, `{"username":"reader","password":"new"}`, ConnectionInput{Password: "new"}},
		{"proxmox", `{"token_id":"cairnops@pve!read","secret":"old","fingerprint":"pin"}`, `{"token_id":"cairnops@pve!read","secret":"old","fingerprint":"pin"}`, ConnectionInput{}},
		{"proxmox", `{"token_id":"cairnops@pve!read","secret":"old","fingerprint":"pin"}`, `{"token_id":"cairnops@pve!read","secret":"new","fingerprint":"new-pin"}`, ConnectionInput{Credentials: proxmox.Credentials{Secret: "new", Fingerprint: "new-pin"}}},
	}
	for _, tc := range cases {
		t.Run(tc.kind+"/"+tc.want, func(t *testing.T) {
			box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32))
			oldEndpoint, _ := normalizeConnectionEndpoint(tc.kind, "https://old.example.net")
			endpoint, _ := normalizeConnectionEndpoint(tc.kind, "https://new.example.net")
			sealed, _ := box.Seal([]byte(tc.current), "connector:"+tc.kind+":"+oldEndpoint)
			store := &connectionMemoryStore{fakeStore: &fakeStore{credential: RuntimeCredential{Kind: tc.kind, Endpoint: oldEndpoint, CredentialSealed: sealed, CredentialManagement: "managed", ManagedCredentialID: "original-owned-account"}}}
			z := &managedZabbix{inspection: zabbix.Inspection{Endpoint: endpoint, Version: "7.4", Compatibility: "supported", EncryptedTransport: true}}
			k := &managedUptimeKuma{inspection: uptimekuma.Inspection{Endpoint: endpoint, EncryptedTransport: true}}
			p := &managedPatchMon{inspection: patchmon.Inspection{Endpoint: endpoint, EncryptedTransport: true}}
			a := &fakeArgus{inspection: argus.Inspection{Endpoint: endpoint, Compatibility: "supported", EncryptedTransport: true}}
			prox := &connectionProxmox{endpoint: endpoint}
			service := NewService(store, z, k, p, box, a).WithProxmox(prox)
			tc.input.Name, tc.input.Address = "Renamed connector", "https://new.example.net"
			result, err := service.TestConnection(context.Background(), connectionTestID, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if store.saved != nil || store.credential.CredentialSealed != sealed {
				t.Fatal("testing modified the stored connection")
			}
			if z.provisioned || k.provisioned || p.provisioned || z.revoked || k.revoked || p.revoked {
				t.Fatal("testing touched remote account ownership")
			}
			var used []byte
			switch tc.kind {
			case "zabbix":
				used = []byte(z.inspectToken)
			case "uptime_kuma":
				used = []byte(k.inspectKey)
			case "patchmon":
				used, _ = json.Marshal(p.credentials)
			case "argus":
				used, _ = json.Marshal(a.credentials)
			case "proxmox":
				used, _ = json.Marshal(prox.credential)
			}
			if string(used) != tc.want {
				t.Fatalf("remote used %s; want %s", used, tc.want)
			}
			connector, err := service.SaveConnection(context.Background(), connectionTestID, ConnectionSaveInput{Receipt: result.Receipt})
			if err != nil {
				t.Fatal(err)
			}
			if connector.ID != connectionTestID || connector.Endpoint != endpoint {
				t.Fatalf("changed identity: %#v", connector)
			}
			persisted, err := box.Open(store.saved.CredentialSealed, "connector:"+tc.kind+":"+endpoint)
			if err != nil || string(persisted) != tc.want {
				t.Fatalf("credential not resealed for new address: %s / %v", persisted, err)
			}
			if _, err := service.SaveConnection(context.Background(), connectionTestID, ConnectionSaveInput{Receipt: result.Receipt}); !errors.Is(err, ErrConnectionChanged) {
				t.Fatalf("replayed old receipt: %v", err)
			}
		})
	}
}

func TestConnectionFailureExpiryAndIdentityLeaveWorkingAccessUntouched(t *testing.T) {
	box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32))
	endpoint := "https://zabbix.example.net/api_jsonrpc.php"
	sealed, _ := box.Seal([]byte("working-token"), "connector:zabbix:"+endpoint)
	store := &connectionMemoryStore{fakeStore: &fakeStore{credential: RuntimeCredential{Kind: "zabbix", Endpoint: endpoint, CredentialSealed: sealed}}}
	remote := &fakeZabbix{err: errors.New("authorization refused")}
	service := NewService(store, remote, nil, nil, box, nil)
	input := ConnectionInput{Name: "Production", Address: endpoint, APIToken: "bad-token"}
	if _, err := service.TestConnection(context.Background(), connectionTestID, input); !errors.Is(err, ErrConnection) {
		t.Fatalf("expected rejected credentials: %v", err)
	}
	if store.credential.CredentialSealed != sealed || store.saved != nil {
		t.Fatal("failure changed working credential")
	}
	remote.err = nil
	remote.inspection = zabbix.Inspection{Endpoint: endpoint, Compatibility: "supported", EncryptedTransport: true}
	tested, err := service.TestConnection(context.Background(), connectionTestID, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveConnection(context.Background(), "12345678-1234-4234-8234-123456789013", ConnectionSaveInput{Receipt: tested.Receipt}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("receipt accepted for another connector: %v", err)
	}
	service.now = func() time.Time { return tested.ExpiresAt }
	if _, err := service.SaveConnection(context.Background(), connectionTestID, ConnectionSaveInput{Receipt: tested.Receipt}); !errors.Is(err, ErrPreviewExpired) {
		t.Fatalf("expired receipt accepted: %v", err)
	}
	if store.saved != nil {
		t.Fatal("invalid receipt changed working access")
	}
	store.listed = []Connector{{ID: "other", Kind: "zabbix", Endpoint: strings.ToUpper(endpoint)}}
	calls := remote.calls
	if _, err := service.TestConnection(context.Background(), connectionTestID, input); !errors.Is(err, ErrEndpointConflict) {
		t.Fatalf("duplicate endpoint accepted: %v", err)
	}
	if remote.calls != calls {
		t.Fatal("duplicate endpoint received a credential")
	}
}

type connectionCleanupStore struct {
	*connectionMemoryStore
}

func (*connectionCleanupStore) ImportProxmox(context.Context, PersistProxmoxInput) (ProxmoxImport, error) {
	panic("unexpected import")
}
func (*connectionCleanupStore) ProxmoxSettings(context.Context, string) (map[string]bool, error) {
	panic("unexpected settings read")
}
func (*connectionCleanupStore) ReplaceProxmoxCredential(context.Context, string, string, string) error {
	panic("unexpected certificate replacement")
}

type cleanupProxmox struct {
	ProxmoxClient
	endpoint   string
	credential proxmox.Credentials
	account    string
}

func (client *cleanupProxmox) RemoveManaged(_ context.Context, endpoint string, credential proxmox.Credentials, account string) error {
	client.endpoint, client.credential, client.account = endpoint, credential, account
	return nil
}

func TestManagedProxmoxCleanupKeepsOriginalEndpointAndCertificateAfterConnectionEdit(t *testing.T) {
	box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32))
	origin, current := "https://original.example.net", "https://moved.example.net"
	previous, _ := box.Seal([]byte(`{"token_id":"owned@pve!read","secret":"original","fingerprint":"original-pin"}`), "connector:proxmox:"+origin)
	replacement, _ := box.Seal([]byte(`{"token_id":"other@pve!read","secret":"replacement","fingerprint":"new-pin"}`), "connector:proxmox:"+current)
	store := &connectionCleanupStore{&connectionMemoryStore{fakeStore: &fakeStore{credential: RuntimeCredential{
		Kind: "proxmox", Endpoint: current, CredentialSealed: replacement, CredentialManagement: "managed", ManagedCredentialID: "owned@pve", ManagedCleanupEndpoint: origin, ManagedCleanupCredentialSealed: previous,
	}}}}
	remote := &cleanupProxmox{}
	service := NewService(store, nil, nil, nil, box, nil).WithProxmox(remote)
	if _, err := service.RemoveProxmox(context.Background(), connectionTestID, proxmox.Credentials{TokenID: "installer@pve!cleanup", Secret: "temporary"}); err != nil {
		t.Fatal(err)
	}
	if remote.endpoint != origin || remote.credential.Fingerprint != "original-pin" || remote.account != "owned@pve" {
		t.Fatalf("cleanup was redirected to another instance or account: %#v", remote)
	}
	if store.deletedID != connectionTestID {
		t.Fatal("successful cleanup did not finish local removal")
	}
}
