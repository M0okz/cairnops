package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors"
	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

type fakeConnectors struct {
	previewInput         connectors.ZabbixPreviewInput
	importActor          string
	importInput          connectors.ZabbixImportInput
	kumaPreviewInput     connectors.UptimeKumaPreviewInput
	kumaImportActor      string
	kumaImportInput      connectors.UptimeKumaImportInput
	patchMonPreviewInput connectors.PatchMonPreviewInput
	patchMonImportActor  string
	patchMonImportInput  connectors.PatchMonImportInput
	suspendedID          string
	resumedID            string
	deletedID            string
}

func (fake *fakeConnectors) PreviewPatchMon(_ context.Context, input connectors.PatchMonPreviewInput) (connectors.PatchMonPreview, error) {
	fake.patchMonPreviewInput = input
	return connectors.PatchMonPreview{Kind: "patchmon", Name: input.Name, Endpoint: "https://patchmon.example.net/api/v1/api/hosts"}, nil
}

func (fake *fakeConnectors) ImportPatchMon(_ context.Context, actorID string, input connectors.PatchMonImportInput) (connectors.PatchMonImport, error) {
	fake.patchMonImportActor = actorID
	fake.patchMonImportInput = input
	return connectors.PatchMonImport{Connector: connectors.Connector{ID: "connector-patchmon", Kind: "patchmon"}}, nil
}

func (fake *fakeConnectors) Suspend(_ context.Context, connectorID string) (connectors.Connector, error) {
	fake.suspendedID = connectorID
	return connectors.Connector{ID: connectorID, Kind: "zabbix", Status: "disabled"}, nil
}

func (fake *fakeConnectors) Resume(_ context.Context, connectorID string) (connectors.Connector, error) {
	fake.resumedID = connectorID
	return connectors.Connector{ID: connectorID, Kind: "zabbix", Status: "connected"}, nil
}

func (fake *fakeConnectors) Delete(_ context.Context, connectorID string) (connectors.Removal, error) {
	fake.deletedID = connectorID
	return connectors.Removal{ID: connectorID, Kind: "zabbix", Name: "Production", Bindings: 15, ResolvedIncidents: 2}, nil
}

func (fake *fakeConnectors) PreviewUptimeKuma(_ context.Context, input connectors.UptimeKumaPreviewInput) (connectors.UptimeKumaPreview, error) {
	fake.kumaPreviewInput = input
	return connectors.UptimeKumaPreview{
		Kind: "uptime_kuma", Name: input.Name, Endpoint: "https://kuma.example.net/metrics",
		Receipt: "opaque-kuma-preview-receipt", ExpiresAt: time.Now().Add(time.Minute),
	}, nil
}

func (fake *fakeConnectors) ImportUptimeKuma(_ context.Context, actorID string, input connectors.UptimeKumaImportInput) (connectors.UptimeKumaImport, error) {
	fake.kumaImportActor = actorID
	fake.kumaImportInput = input
	return connectors.UptimeKumaImport{Connector: connectors.Connector{ID: "connector-kuma", Kind: "uptime_kuma"}}, nil
}

func (*fakeConnectors) List(context.Context) ([]connectors.Connector, error) {
	return []connectors.Connector{{ID: "connector-one", Kind: "zabbix", Name: "Production"}}, nil
}

func (fake *fakeConnectors) PreviewZabbix(_ context.Context, input connectors.ZabbixPreviewInput) (connectors.ZabbixPreview, error) {
	fake.previewInput = input
	return connectors.ZabbixPreview{
		Kind: "zabbix", Name: input.Name, Endpoint: "https://zabbix.example.net/api_jsonrpc.php",
		Version: "7.4.2", Receipt: "opaque-preview-receipt", ExpiresAt: time.Now().Add(time.Minute),
	}, nil
}

func (fake *fakeConnectors) ImportZabbix(_ context.Context, actorID string, input connectors.ZabbixImportInput) (connectors.ZabbixImport, error) {
	fake.importActor = actorID
	fake.importInput = input
	return connectors.ZabbixImport{Connector: connectors.Connector{ID: "connector-one", Kind: "zabbix"}}, nil
}

type roleIdentity struct {
	*fakeIdentity
	role string
}

func (identity *roleIdentity) Authenticate(_ context.Context, token string) (identitymodel.Principal, error) {
	if token != testSessionToken {
		return identitymodel.Principal{}, identitymodel.ErrInvalidSession
	}
	principal := testAuthenticatedSession().Principal
	principal.Role = identity.role
	return principal, nil
}

func TestZabbixPreviewRequiresAdministratorAndPassesTokenOnlyToService(t *testing.T) {
	t.Parallel()
	fake := &fakeConnectors{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Connectors: fake})
	body, _ := json.Marshal(connectors.ZabbixPreviewInput{Name: "Production", Address: "https://zabbix.example.net", APIToken: "secret-token"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/zabbix/preview", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if fake.previewInput.APIToken != "secret-token" {
		t.Fatalf("unexpected preview input: %#v", fake.previewInput)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("secret-token")) {
		t.Fatal("API token leaked into preview response")
	}
}

func TestZabbixPreviewRejectsOperator(t *testing.T) {
	t.Parallel()
	fake := &fakeConnectors{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Connectors: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/zabbix/preview", bytes.NewBufferString(`{"name":"Production","address":"https://zabbix.example.net","api_token":"token"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", response.Code, response.Body.String())
	}
	if fake.previewInput.APIToken != "" {
		t.Fatal("operator unexpectedly reached connector service")
	}
}

func TestZabbixImportUsesAuthenticatedActor(t *testing.T) {
	t.Parallel()
	fake := &fakeConnectors{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Connectors: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/zabbix/import", bytes.NewBufferString(`{"receipt":"opaque-preview-receipt","host_ids":["10084"],"target_assignments":{"10084":"12345678-1234-4234-8234-123456789012"}}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || fake.importActor != "user-id" || fake.importInput.TargetAssignments["10084"] != "12345678-1234-4234-8234-123456789012" {
		t.Fatalf("expected authenticated import, status=%d actor=%q body=%s", response.Code, fake.importActor, response.Body.String())
	}
}

func TestUptimeKumaPreviewAndImportRequireAdministrator(t *testing.T) {
	t.Parallel()
	fake := &fakeConnectors{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Connectors: fake})
	body, _ := json.Marshal(connectors.UptimeKumaPreviewInput{Name: "Kuma", Address: "https://kuma.example.net", APIKey: "uk2-secret"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/uptime-kuma/preview", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.kumaPreviewInput.APIKey != "uk2-secret" || bytes.Contains(response.Body.Bytes(), []byte("uk2-secret")) {
		t.Fatalf("unexpected Kuma preview response status=%d body=%s input=%#v", response.Code, response.Body.String(), fake.kumaPreviewInput)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/uptime-kuma/import", bytes.NewBufferString(`{"receipt":"opaque-kuma-preview-receipt","monitor_ids":["12"],"target_assignments":{"12":"12345678-1234-4234-8234-123456789012"}}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || fake.kumaImportActor != "user-id" || fake.kumaImportInput.TargetAssignments["12"] != "12345678-1234-4234-8234-123456789012" {
		t.Fatalf("expected authenticated Kuma import, status=%d actor=%q body=%s", response.Code, fake.kumaImportActor, response.Body.String())
	}
}

func TestPatchMonPreviewAndImportKeepCredentialsOutOfResponses(t *testing.T) {
	t.Parallel()
	fake := &fakeConnectors{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Connectors: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/patchmon/preview", bytes.NewBufferString(`{"name":"Patch posture","address":"https://patchmon.example.net","token_key":"patchmon_key","token_secret":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.patchMonPreviewInput.TokenSecret != "secret" || bytes.Contains(response.Body.Bytes(), []byte("secret")) {
		t.Fatalf("unexpected PatchMon preview status=%d body=%s input=%#v", response.Code, response.Body.String(), fake.patchMonPreviewInput)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/patchmon/import", bytes.NewBufferString(`{"receipt":"opaque-patchmon-receipt-with-enough-characters","host_ids":["host-12"],"target_assignments":{"host-12":"12345678-1234-4234-8234-123456789012"}}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || fake.patchMonImportActor != "user-id" || fake.patchMonImportInput.TargetAssignments["host-12"] == "" {
		t.Fatalf("unexpected PatchMon import status=%d actor=%q body=%s", response.Code, fake.patchMonImportActor, response.Body.String())
	}
}

func TestConnectorSuspensionAndRemovalRequireAdministrator(t *testing.T) {
	t.Parallel()
	const connectorID = "6f1d4d4e-0f2c-4a3a-9f1a-6f9c1f2b3c4d"
	fake := &fakeConnectors{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Connectors: fake})

	for _, transition := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/connectors/" + connectorID + "/suspension"},
		{http.MethodDelete, "/api/v1/connectors/" + connectorID + "/suspension"},
		{http.MethodDelete, "/api/v1/connectors/" + connectorID},
	} {
		request := httptest.NewRequest(transition.method, transition.path, nil)
		request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s %s, got %d: %s", transition.method, transition.path, response.Code, response.Body.String())
		}
	}
	if fake.suspendedID != connectorID || fake.resumedID != connectorID || fake.deletedID != connectorID {
		t.Fatalf("unexpected connector transitions: %#v", fake)
	}
}

func TestConnectorRemovalRejectsOperatorAndMalformedIdentity(t *testing.T) {
	t.Parallel()
	fake := &fakeConnectors{}
	operatorServer := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Connectors: fake})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/connectors/6f1d4d4e-0f2c-4a3a-9f1a-6f9c1f2b3c4d", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	operatorServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || fake.deletedID != "" {
		t.Fatalf("operator unexpectedly removed a connector: status=%d deleted=%q", response.Code, fake.deletedID)
	}

	adminServer := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Connectors: fake})
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/connectors/not-a-uuid", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	adminServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || fake.deletedID != "" {
		t.Fatalf("malformed identity unexpectedly reached the service: status=%d deleted=%q", response.Code, fake.deletedID)
	}
}
