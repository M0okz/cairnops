package notifications

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

const mattermostSecretPurpose = "mattermost-webhook-v1"

var (
	ErrInvalidInput = errors.New("invalid notification input")
	ErrConnection   = errors.New("notification connection failed")
	ErrNoDelivery   = errors.New("no notification delivery due")
)

type Channel struct {
	ID                 string               `json:"id"`
	Kind               string               `json:"kind"`
	Name               string               `json:"name"`
	Endpoint           string               `json:"endpoint"`
	Severities         []incidents.Severity `json:"severities"`
	Enabled            bool                 `json:"enabled"`
	Status             string               `json:"status"`
	EncryptedTransport bool                 `json:"encrypted_transport"`
	LastCheckedAt      time.Time            `json:"last_checked_at"`
	LastError          string               `json:"last_error,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

type CreateMattermostInput struct {
	Name       string               `json:"name"`
	WebhookURL string               `json:"webhook_url"`
	Severities []incidents.Severity `json:"severities"`
}

type PersistMattermostInput struct {
	ActorID            string
	Name               string
	Endpoint           string
	CredentialSealed   string
	Severities         []incidents.Severity
	EncryptedTransport bool
}

type Store interface {
	List(context.Context) ([]Channel, error)
	CreateMattermost(context.Context, PersistMattermostInput) (Channel, error)
	Inbox(ctx context.Context, userID string, limit int) (Inbox, error)
	MarkRead(ctx context.Context, userID string, ids []int64) (int, error)
	Dismiss(ctx context.Context, userID string) (int, error)
}

type Mattermost interface {
	Test(context.Context, string) error
}

type Service struct {
	store   Store
	client  Mattermost
	secrets *secretbox.Box
}

func NewService(store Store, client Mattermost, secrets *secretbox.Box) *Service {
	return &Service{store: store, client: client, secrets: secrets}
}

func (service *Service) List(ctx context.Context) ([]Channel, error) {
	return service.store.List(ctx)
}

// Inbox, MarkRead et Dismiss ne portent que sur l'appelant : l'identifiant
// vient de la session, jamais de la requête, si bien qu'aucun compte ne peut
// lire ni modifier la boîte d'un autre.
func (service *Service) Inbox(ctx context.Context, userID string, limit int) (Inbox, error) {
	return service.store.Inbox(ctx, userID, limit)
}

func (service *Service) MarkRead(ctx context.Context, userID string, ids []int64) (int, error) {
	return service.store.MarkRead(ctx, userID, ids)
}

func (service *Service) Dismiss(ctx context.Context, userID string) (int, error) {
	return service.store.Dismiss(ctx, userID)
}

func (service *Service) CreateMattermost(ctx context.Context, actorID string, input CreateMattermostInput) (Channel, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "Mattermost"
	}
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 160 {
		return Channel{}, fmt.Errorf("%w: le nom doit contenir entre 1 et 160 caractères", ErrInvalidInput)
	}
	webhookURL := strings.TrimSpace(input.WebhookURL)
	parsed, err := url.Parse(webhookURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return Channel{}, fmt.Errorf("%w: utilisez une URL de webhook Mattermost HTTPS valide", ErrInvalidInput)
	}
	severities, err := normalizeSeverities(input.Severities)
	if err != nil {
		return Channel{}, err
	}
	if service.client == nil {
		return Channel{}, fmt.Errorf("%w: client Mattermost indisponible", ErrConnection)
	}
	if err := service.client.Test(ctx, webhookURL); err != nil {
		return Channel{}, fmt.Errorf("%w: %v", ErrConnection, err)
	}
	sealed, err := service.secrets.Seal([]byte(webhookURL), mattermostSecretPurpose)
	if err != nil {
		return Channel{}, fmt.Errorf("seal Mattermost webhook: %w", err)
	}
	endpoint := parsed.Scheme + "://" + parsed.Host
	return service.store.CreateMattermost(ctx, PersistMattermostInput{
		ActorID: actorID, Name: input.Name, Endpoint: endpoint,
		CredentialSealed: sealed, Severities: severities, EncryptedTransport: true,
	})
}

func normalizeSeverities(values []incidents.Severity) ([]incidents.Severity, error) {
	if len(values) == 0 {
		values = []incidents.Severity{incidents.SeverityWarning, incidents.SeverityMajor, incidents.SeverityCritical}
	}
	allowed := map[incidents.Severity]int{
		incidents.SeverityInformation: 0,
		incidents.SeverityWarning:     1,
		incidents.SeverityMajor:       2,
		incidents.SeverityCritical:    3,
	}
	seen := make(map[incidents.Severity]struct{}, len(values))
	result := make([]incidents.Severity, 0, len(values))
	for _, severity := range values {
		if _, ok := allowed[severity]; !ok {
			return nil, fmt.Errorf("%w: gravité inconnue", ErrInvalidInput)
		}
		if _, duplicate := seen[severity]; duplicate {
			continue
		}
		seen[severity] = struct{}{}
		result = append(result, severity)
	}
	sort.Slice(result, func(i, j int) bool { return allowed[result[i]] < allowed[result[j]] })
	return result, nil
}

// KindInApp désigne le Canal intégré : celui qui ne sort pas de l'instance.
const KindInApp = "in_app"

type Delivery struct {
	ID                int64
	IncidentID        string
	IncidentRevision  int
	ChannelID         string
	ChannelKind       string
	EventKind         string
	Presentation      string
	TargetName        string
	NatureLabel       string
	Severity          incidents.Severity
	ImpactCount       int
	AffectedTargets   int
	MaxAffected       int
	PropagationStatus string
	Extended          bool
	OpenedAt          time.Time
	ResolvedAt        *time.Time
	CredentialSealed  string
}

type DeliveryStore interface {
	Schedule(context.Context) error
	Claim(context.Context, string) (Delivery, error)
	Complete(context.Context, int64, string) error
	Fail(context.Context, int64, string, string) error
	// Deliver dépose une entrée par destinataire pour une livraison intégrée,
	// et retourne combien de personnes l'ont reçue.
	Deliver(context.Context, Delivery) (int, error)
}

type Sender interface {
	Send(context.Context, string, Message) error
}

type Message struct {
	EventKind         string
	IncidentID        string
	TargetName        string
	NatureLabel       string
	Severity          incidents.Severity
	ImpactCount       int
	AffectedTargets   int
	MaxAffected       int
	PropagationStatus string
	Extended          bool
	OpenedAt          time.Time
	ResolvedAt        *time.Time
	PublicURL         string
}

type Dispatcher struct {
	store     DeliveryStore
	sender    Sender
	secrets   *secretbox.Box
	workerID  string
	publicURL string
	interval  time.Duration
}

func NewDispatcher(store DeliveryStore, sender Sender, secrets *secretbox.Box, workerID, publicURL string) *Dispatcher {
	return &Dispatcher{
		store: store, sender: sender, secrets: secrets, workerID: workerID,
		publicURL: strings.TrimSuffix(publicURL, "/"), interval: 2 * time.Second,
	}
}

func (dispatcher *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(dispatcher.interval)
	defer ticker.Stop()
	for {
		if err := dispatcher.Process(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (dispatcher *Dispatcher) Process(ctx context.Context) error {
	if err := dispatcher.store.Schedule(ctx); err != nil {
		return fmt.Errorf("schedule notifications: %w", err)
	}
	for processed := 0; processed < 20; processed++ {
		delivery, err := dispatcher.store.Claim(ctx, dispatcher.workerID)
		if errors.Is(err, ErrNoDelivery) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim notification: %w", err)
		}
		err = dispatcher.deliver(ctx, delivery)
		if err != nil {
			if failErr := dispatcher.store.Fail(ctx, delivery.ID, dispatcher.workerID, err.Error()); failErr != nil {
				return fmt.Errorf("record notification failure: %w", failErr)
			}
			continue
		}
		if err := dispatcher.store.Complete(ctx, delivery.ID, dispatcher.workerID); err != nil {
			return fmt.Errorf("complete notification: %w", err)
		}
	}
	return nil
}

// deliver porte une livraison jusqu'à sa destination. Le Canal intégré ne sort
// pas de l'instance : il n'a ni secret à ouvrir ni appel à émettre, et sa
// livraison est l'écriture elle-même.
func (dispatcher *Dispatcher) deliver(ctx context.Context, delivery Delivery) error {
	if delivery.ChannelKind == KindInApp {
		if _, err := dispatcher.store.Deliver(ctx, delivery); err != nil {
			return err
		}
		return nil
	}
	webhook, err := dispatcher.secrets.Open(delivery.CredentialSealed, mattermostSecretPurpose)
	if err != nil {
		return err
	}
	return dispatcher.sender.Send(ctx, string(webhook), Message{
		EventKind: delivery.EventKind, IncidentID: delivery.IncidentID,
		TargetName: delivery.TargetName, NatureLabel: delivery.NatureLabel,
		Severity: delivery.Severity, ImpactCount: delivery.ImpactCount,
		AffectedTargets: delivery.AffectedTargets, MaxAffected: delivery.MaxAffected,
		PropagationStatus: delivery.PropagationStatus, Extended: delivery.Extended,
		OpenedAt: delivery.OpenedAt, ResolvedAt: delivery.ResolvedAt, PublicURL: dispatcher.publicURL,
	})
}
