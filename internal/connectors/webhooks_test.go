package connectors

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type webhookFakeStore struct {
	connector         Connector
	credential        WebhookCredential
	storedSealed      string
	route             WebhookRoute
	quarantine        []WebhookQuarantine
	approval          WebhookApproval
	completedIdentity string
}

func (store *webhookFakeStore) CreateGenericWebhook(_ context.Context, _ string, name, endpoint, _ string, sealed string, _ bool) (Connector, error) {
	store.storedSealed = sealed
	store.connector = Connector{ID: "connector-one", Kind: "generic_webhook", Name: name, Endpoint: endpoint, Status: "connected"}
	store.credential = WebhookCredential{ConnectorID: store.connector.ID, Endpoint: endpoint, CredentialSealed: sealed, Status: "connected"}
	return store.connector, nil
}

func (store *webhookFakeStore) WebhookCredential(context.Context, string) (WebhookCredential, error) {
	if store.credential.ConnectorID == "" {
		return WebhookCredential{}, ErrWebhookNotFound
	}
	return store.credential, nil
}

func (store *webhookFakeStore) RouteWebhook(context.Context, string, GenericWebhookEvent, time.Time) (WebhookRoute, error) {
	return store.route, nil
}

func (store *webhookFakeStore) ListWebhookQuarantine(context.Context, string) ([]WebhookQuarantine, error) {
	return store.quarantine, nil
}

func (store *webhookFakeStore) ApproveWebhookIdentity(context.Context, string, string, string, string) (WebhookApproval, error) {
	return store.approval, nil
}

func (store *webhookFakeStore) CompleteWebhookApproval(_ context.Context, _ string, identity, _ string) error {
	store.completedIdentity = identity
	return nil
}

type webhookIncidentFake struct {
	signals []incidents.WebhookSignal
}

func (fake *webhookIncidentFake) ApplyWebhook(_ context.Context, signal incidents.WebhookSignal) error {
	fake.signals = append(fake.signals, signal)
	return nil
}

func TestGenericWebhookCreationSealsOneTimeToken(t *testing.T) {
	t.Parallel()
	box, err := secretbox.New(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	store := &webhookFakeStore{}
	service := NewWebhookService(store, &webhookIncidentFake{}, box, "https://cairnops.example.net/")
	service.random = func(destination []byte) (int, error) {
		for index := range destination {
			destination[index] = byte(index + 1)
		}
		return len(destination), nil
	}

	created, err := service.Create(context.Background(), "administrator-one", CreateGenericWebhookInput{Name: "Automations"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.Endpoint != "https://cairnops.example.net/api/v1/webhooks/0102030405060708090a0b0c0d0e0f10" {
		t.Fatalf("unexpected generated webhook: %#v", created)
	}
	if store.storedSealed == "" || store.storedSealed == created.Token {
		t.Fatal("webhook token must be sealed before persistence")
	}
	plaintext, err := box.Open(store.storedSealed, "connector:generic_webhook:"+created.Endpoint)
	if err != nil || string(plaintext) != created.Token {
		t.Fatalf("stored webhook credential cannot be opened: %v", err)
	}
}

func TestGenericWebhookQuarantinesUnknownIdentityAndAcceptsBoundIdentity(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x31}, 32))
	sealed, _ := box.Seal([]byte("token-one"), "connector:generic_webhook:https://cairnops.example/hook")
	store := &webhookFakeStore{credential: WebhookCredential{
		ConnectorID: "connector-one", Endpoint: "https://cairnops.example/hook",
		CredentialSealed: sealed, Status: "connected",
	}, route: WebhookRoute{QuarantineID: "quarantine-one"}}
	incidentStore := &webhookIncidentFake{}
	service := NewWebhookService(store, incidentStore, box, "https://cairnops.example")
	event := GenericWebhookEvent{
		Identity: "worker/api", TargetName: "Public API", EventKey: "latency",
		Status: "firing", Severity: "critical", Summary: "Latency above 2 s",
	}

	if _, err := service.Receive(context.Background(), "public", "Bearer wrong", event); !errors.Is(err, ErrWebhookUnauthorized) {
		t.Fatalf("expected uniform authentication failure, got %v", err)
	}
	receipt, err := service.Receive(context.Background(), "public", "bearer token-one", event)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != "quarantined" || receipt.QuarantineID != "quarantine-one" || len(incidentStore.signals) != 0 {
		t.Fatalf("unknown identity bypassed quarantine: %#v", receipt)
	}

	store.route = WebhookRoute{BindingID: "binding-one", TargetID: "target-one"}
	receipt, err = service.Receive(context.Background(), "public", "Bearer token-one", event)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != "accepted" || len(incidentStore.signals) != 1 {
		t.Fatalf("bound identity was not projected: receipt=%#v signals=%#v", receipt, incidentStore.signals)
	}
	if signal := incidentStore.signals[0]; signal.NatureKey != "availability" || signal.NatureLabel != "Indisponibilité" || signal.Severity != incidents.SeverityCritical {
		t.Fatalf("unexpected incident projection: %#v", signal)
	}
}

func TestWebhookApprovalReplaysLatestQuarantinedStates(t *testing.T) {
	t.Parallel()
	box, _ := secretbox.New(bytes.Repeat([]byte{0x51}, 32))
	store := &webhookFakeStore{approval: WebhookApproval{
		BindingID: "binding-one", TargetID: "target-one", TargetName: "Database", Identity: "db-primary",
		Events: []WebhookQuarantine{{
			ExternalIdentity: "db-primary", TargetName: "Database", EventKey: "availability",
			NatureKey: "availability", Nature: "Indisponibilité", Status: "firing",
			Severity: "major", Summary: "Database unreachable", LastSeenAt: time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC),
		}},
	}}
	incidentStore := &webhookIncidentFake{}
	service := NewWebhookService(store, incidentStore, box, "https://cairnops.example")
	approval, err := service.Approve(context.Background(), "admin-one", "connector-one", "quarantine-one", ApproveWebhookIdentityInput{})
	if err != nil {
		t.Fatal(err)
	}
	if approval.Replayed != 1 || store.completedIdentity != "db-primary" || len(incidentStore.signals) != 1 {
		t.Fatalf("unexpected approval replay: approval=%#v signals=%#v", approval, incidentStore.signals)
	}
}
