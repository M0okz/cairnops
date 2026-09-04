package connectors

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

var (
	ErrWebhookUnauthorized = errors.New("webhook authentication failed")
	ErrWebhookNotFound     = errors.New("webhook resource not found")
	ErrWebhookConflict     = errors.New("webhook identity conflict")
)

type CreateGenericWebhookInput struct {
	Name string `json:"name"`
}

type GenericWebhookCreated struct {
	Connector Connector `json:"connector"`
	Endpoint  string    `json:"endpoint"`
	Token     string    `json:"token"`
}

type GenericWebhookEvent struct {
	Identity   string         `json:"identity"`
	TargetName string         `json:"target_name"`
	EventKey   string         `json:"event_key"`
	NatureKey  string         `json:"nature_key"`
	Nature     string         `json:"nature"`
	Status     string         `json:"status"`
	Severity   string         `json:"severity"`
	Summary    string         `json:"summary"`
	Details    map[string]any `json:"details,omitempty"`
}

type WebhookReceipt struct {
	Disposition  string `json:"disposition"`
	QuarantineID string `json:"quarantine_id,omitempty"`
}

type WebhookQuarantine struct {
	ID               string         `json:"id"`
	ConnectorID      string         `json:"connector_id"`
	ExternalIdentity string         `json:"external_identity"`
	TargetName       string         `json:"target_name"`
	EventKey         string         `json:"event_key"`
	NatureKey        string         `json:"nature_key"`
	Nature           string         `json:"nature"`
	Status           string         `json:"status"`
	Severity         string         `json:"severity"`
	Summary          string         `json:"summary"`
	Details          map[string]any `json:"details"`
	Occurrences      int            `json:"occurrences"`
	FirstSeenAt      time.Time      `json:"first_seen_at"`
	LastSeenAt       time.Time      `json:"last_seen_at"`
}

type ApproveWebhookIdentityInput struct {
	TargetID string `json:"target_id,omitempty"`
}

type WebhookApproval struct {
	BindingID  string              `json:"-"`
	TargetID   string              `json:"target_id"`
	TargetName string              `json:"target_name"`
	Identity   string              `json:"identity"`
	Events     []WebhookQuarantine `json:"-"`
	Replayed   int                 `json:"replayed"`
}

type WebhookCredential struct {
	ConnectorID      string
	Endpoint         string
	CredentialSealed string
	Status           string
}

type WebhookRoute struct {
	BindingID    string
	TargetID     string
	QuarantineID string
}

type WebhookStore interface {
	CreateGenericWebhook(context.Context, string, string, string, string, string, bool) (Connector, error)
	WebhookCredential(context.Context, string) (WebhookCredential, error)
	RouteWebhook(context.Context, string, GenericWebhookEvent, time.Time) (WebhookRoute, error)
	ListWebhookQuarantine(context.Context, string) ([]WebhookQuarantine, error)
	ApproveWebhookIdentity(context.Context, string, string, string, string) (WebhookApproval, error)
	CompleteWebhookApproval(context.Context, string, string, string) error
}

type WebhookIncidentStore interface {
	ApplyWebhook(context.Context, incidents.WebhookSignal) error
}

type WebhookService struct {
	store     WebhookStore
	incidents WebhookIncidentStore
	secrets   *secretbox.Box
	publicURL string
	now       func() time.Time
	random    func([]byte) (int, error)
}

func NewWebhookService(store WebhookStore, incidentStore WebhookIncidentStore, secrets *secretbox.Box, publicURL string) *WebhookService {
	return &WebhookService{
		store: store, incidents: incidentStore, secrets: secrets,
		publicURL: strings.TrimSuffix(publicURL, "/"), now: time.Now, random: rand.Read,
	}
}

func (service *WebhookService) Create(ctx context.Context, actorID string, input CreateGenericWebhookInput) (GenericWebhookCreated, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Webhook générique"
	}
	if strings.TrimSpace(actorID) == "" || !validText(name, 160) {
		return GenericWebhookCreated{}, fmt.Errorf("%w: administrator identity and a valid connector name are required", ErrInvalidInput)
	}
	publicIDBytes := make([]byte, 16)
	tokenBytes := make([]byte, 32)
	count, err := service.random(publicIDBytes)
	if err != nil {
		return GenericWebhookCreated{}, fmt.Errorf("generate webhook identity: %w", err)
	}
	if count != len(publicIDBytes) {
		return GenericWebhookCreated{}, fmt.Errorf("generate webhook identity: short random read")
	}
	count, err = service.random(tokenBytes)
	if err != nil {
		return GenericWebhookCreated{}, fmt.Errorf("generate webhook token: %w", err)
	}
	if count != len(tokenBytes) {
		return GenericWebhookCreated{}, fmt.Errorf("generate webhook token: short random read")
	}
	publicID := hex.EncodeToString(publicIDBytes)
	token := hex.EncodeToString(tokenBytes)
	endpoint := service.publicURL + "/api/v1/webhooks/" + publicID
	credential, err := service.secrets.Seal([]byte(token), "connector:generic_webhook:"+endpoint)
	if err != nil {
		return GenericWebhookCreated{}, fmt.Errorf("seal webhook credential: %w", err)
	}
	connector, err := service.store.CreateGenericWebhook(ctx, actorID, name, endpoint, publicID, credential, strings.HasPrefix(strings.ToLower(endpoint), "https://"))
	if err != nil {
		return GenericWebhookCreated{}, err
	}
	connector.Endpoint = endpoint
	return GenericWebhookCreated{Connector: connector, Endpoint: endpoint, Token: token}, nil
}

func (service *WebhookService) Receive(ctx context.Context, publicID, authorization string, event GenericWebhookEvent) (WebhookReceipt, error) {
	credential, err := service.store.WebhookCredential(ctx, strings.TrimSpace(publicID))
	if err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			return WebhookReceipt{}, ErrWebhookUnauthorized
		}
		return WebhookReceipt{}, err
	}
	if credential.Status == "disabled" {
		return WebhookReceipt{}, ErrWebhookUnauthorized
	}
	plaintext, err := service.secrets.Open(credential.CredentialSealed, "connector:generic_webhook:"+credential.Endpoint)
	if err != nil || !matchesBearer(authorization, string(plaintext)) {
		return WebhookReceipt{}, ErrWebhookUnauthorized
	}
	event, err = validateWebhookEvent(event)
	if err != nil {
		return WebhookReceipt{}, err
	}
	now := service.now().UTC()
	route, err := service.store.RouteWebhook(ctx, credential.ConnectorID, event, now)
	if err != nil {
		return WebhookReceipt{}, err
	}
	if route.QuarantineID != "" {
		return WebhookReceipt{Disposition: "quarantined", QuarantineID: route.QuarantineID}, nil
	}
	if err := service.incidents.ApplyWebhook(ctx, webhookIncidentEvidence(credential.ConnectorID, route.BindingID, route.TargetID, event, now)); err != nil {
		return WebhookReceipt{}, fmt.Errorf("apply authorized webhook: %w", err)
	}
	return WebhookReceipt{Disposition: "accepted"}, nil
}

func (service *WebhookService) Quarantine(ctx context.Context, connectorID string) ([]WebhookQuarantine, error) {
	if strings.TrimSpace(connectorID) == "" {
		return nil, fmt.Errorf("%w: connector identity is required", ErrInvalidInput)
	}
	return service.store.ListWebhookQuarantine(ctx, connectorID)
}

func (service *WebhookService) Approve(ctx context.Context, actorID, connectorID, quarantineID string, input ApproveWebhookIdentityInput) (WebhookApproval, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(connectorID) == "" || strings.TrimSpace(quarantineID) == "" {
		return WebhookApproval{}, fmt.Errorf("%w: administrator, connector and quarantine identities are required", ErrInvalidInput)
	}
	approval, err := service.store.ApproveWebhookIdentity(ctx, actorID, connectorID, quarantineID, strings.TrimSpace(input.TargetID))
	if err != nil {
		return WebhookApproval{}, err
	}
	for _, event := range approval.Events {
		input := GenericWebhookEvent{
			Identity: event.ExternalIdentity, TargetName: event.TargetName,
			EventKey: event.EventKey, NatureKey: event.NatureKey, Nature: event.Nature,
			Status: event.Status, Severity: event.Severity, Summary: event.Summary, Details: event.Details,
		}
		if err := service.incidents.ApplyWebhook(ctx, webhookIncidentEvidence(connectorID, approval.BindingID, approval.TargetID, input, event.LastSeenAt)); err != nil {
			return WebhookApproval{}, fmt.Errorf("replay quarantined webhook: %w", err)
		}
	}
	if err := service.store.CompleteWebhookApproval(ctx, connectorID, approval.Identity, actorID); err != nil {
		return WebhookApproval{}, err
	}
	approval.Replayed = len(approval.Events)
	approval.Events = nil
	return approval, nil
}

func webhookIncidentEvidence(connectorID, bindingID, targetID string, event GenericWebhookEvent, observedAt time.Time) incidents.WebhookSignal {
	return incidents.WebhookSignal{
		ConnectorID: connectorID, BindingID: bindingID, TargetID: targetID,
		ExternalEventKey: event.EventKey, NatureKey: event.NatureKey, NatureLabel: event.Nature,
		Summary: event.Summary, Status: event.Status, Severity: incidents.Severity(event.Severity),
		ObservedAt: observedAt, Details: event.Details,
	}
}

func validateWebhookEvent(event GenericWebhookEvent) (GenericWebhookEvent, error) {
	event.Identity = strings.TrimSpace(event.Identity)
	event.TargetName = strings.TrimSpace(event.TargetName)
	event.EventKey = strings.TrimSpace(event.EventKey)
	event.NatureKey = strings.TrimSpace(event.NatureKey)
	event.Nature = strings.TrimSpace(event.Nature)
	event.Status = strings.ToLower(strings.TrimSpace(event.Status))
	event.Severity = strings.ToLower(strings.TrimSpace(event.Severity))
	event.Summary = strings.TrimSpace(event.Summary)
	if event.NatureKey == "" {
		event.NatureKey = "availability"
	}
	if event.Nature == "" {
		event.Nature = "Indisponibilité"
	}
	if event.Severity == "" {
		event.Severity = string(incidents.SeverityMajor)
	}
	if !validText(event.Identity, 255) || !validText(event.TargetName, 160) ||
		!validText(event.EventKey, 255) || !validText(event.NatureKey, 255) ||
		!validText(event.Nature, 512) || !validText(event.Summary, 512) {
		return GenericWebhookEvent{}, fmt.Errorf("%w: webhook identity, target, event, nature and summary are required and must fit their limits", ErrInvalidInput)
	}
	if event.Status != "firing" && event.Status != "resolved" {
		return GenericWebhookEvent{}, fmt.Errorf("%w: webhook status must be firing or resolved", ErrInvalidInput)
	}
	switch incidents.Severity(event.Severity) {
	case incidents.SeverityInformation, incidents.SeverityWarning, incidents.SeverityMajor, incidents.SeverityCritical:
	default:
		return GenericWebhookEvent{}, fmt.Errorf("%w: webhook severity is invalid", ErrInvalidInput)
	}
	if event.Details == nil {
		event.Details = map[string]any{}
	}
	encoded, err := json.Marshal(event.Details)
	if err != nil || len(encoded) > 32*1024 {
		return GenericWebhookEvent{}, fmt.Errorf("%w: webhook details must be a JSON object smaller than 32 KiB", ErrInvalidInput)
	}
	return event, nil
}

func validText(value string, limit int) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit
}

func matchesBearer(header, expected string) bool {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) == 1
}
