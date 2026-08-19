package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/M0okz/cairnops/internal/connectors"
)

type fakeWebhooks struct {
	createdActor  string
	receivedToken string
	receivedEvent connectors.GenericWebhookEvent
	approvedActor string
}

func (fake *fakeWebhooks) Create(_ context.Context, actorID string, input connectors.CreateGenericWebhookInput) (connectors.GenericWebhookCreated, error) {
	fake.createdActor = actorID
	return connectors.GenericWebhookCreated{
		Connector: connectors.Connector{ID: "connector-one", Kind: "generic_webhook", Name: input.Name},
		Endpoint:  "https://cairnops.example/api/v1/webhooks/public-one", Token: "one-time-token",
	}, nil
}

func (fake *fakeWebhooks) Receive(_ context.Context, _ string, authorization string, event connectors.GenericWebhookEvent) (connectors.WebhookReceipt, error) {
	fake.receivedToken = authorization
	fake.receivedEvent = event
	return connectors.WebhookReceipt{Disposition: "quarantined", QuarantineID: "quarantine-one"}, nil
}

func (*fakeWebhooks) Quarantine(context.Context, string) ([]connectors.WebhookQuarantine, error) {
	return []connectors.WebhookQuarantine{{ID: "quarantine-one", ExternalIdentity: "worker/api"}}, nil
}

func (fake *fakeWebhooks) Approve(_ context.Context, actorID, _, _ string, _ connectors.ApproveWebhookIdentityInput) (connectors.WebhookApproval, error) {
	fake.approvedActor = actorID
	return connectors.WebhookApproval{TargetID: "target-one", TargetName: "Public API", Identity: "worker/api", Replayed: 1}, nil
}

func TestGenericWebhookCreationIsAdminOnlyAndTokenIsReturnedOnce(t *testing.T) {
	t.Parallel()
	fake := &fakeWebhooks{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Webhooks: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/generic-webhook", bytes.NewBufferString(`{"name":"Automations"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || fake.createdActor != "user-id" || !bytes.Contains(response.Body.Bytes(), []byte("one-time-token")) {
		t.Fatalf("unexpected webhook creation status=%d actor=%q body=%s", response.Code, fake.createdActor, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("one-time webhook token response must not be cached")
	}

	fake = &fakeWebhooks{}
	server = NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Webhooks: fake})
	request = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/generic-webhook", bytes.NewBufferString(`{"name":"Automations"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || fake.createdActor != "" {
		t.Fatalf("operator reached webhook creation: status=%d actor=%q", response.Code, fake.createdActor)
	}
}

func TestGenericWebhookReceiverIsPublicBoundedAndForwardsAuthorization(t *testing.T) {
	t.Parallel()
	fake := &fakeWebhooks{}
	server := NewServer(ServerOptions{Webhooks: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/00112233445566778899aabbccddeeff", bytes.NewBufferString(`{
		"identity":"worker/api","target_name":"Public API","event_key":"availability",
		"status":"firing","severity":"major","summary":"API unreachable"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer inbound-secret")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || fake.receivedToken != "Bearer inbound-secret" || fake.receivedEvent.Identity != "worker/api" {
		t.Fatalf("unexpected webhook reception status=%d token=%q event=%#v body=%s", response.Code, fake.receivedToken, fake.receivedEvent, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("webhook receipt must not be cached")
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/00112233445566778899aabbccddeeff", bytes.NewReader(bytes.Repeat([]byte("x"), maximumWebhookBody+1)))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized webhook body was accepted: %d", response.Code)
	}
}

func TestWebhookQuarantineApprovalUsesAuthenticatedAdministrator(t *testing.T) {
	t.Parallel()
	fake := &fakeWebhooks{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Webhooks: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/10000000-0000-0000-0000-000000000001/quarantine/20000000-0000-0000-0000-000000000002/approve", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.approvedActor != "user-id" {
		t.Fatalf("unexpected approval status=%d actor=%q body=%s", response.Code, fake.approvedActor, response.Body.String())
	}
}
