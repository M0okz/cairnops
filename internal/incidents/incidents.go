package incidents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid incident input")
	ErrNotFound     = errors.New("incident not found")
	ErrConflict     = errors.New("incident conflict")
	// ErrTargetArchived signale un signal visant une Cible sortie de l'Espace
	// opérationnel. Ce n'est pas une panne : le signal est simplement ignoré.
	ErrTargetArchived = errors.New("target is archived")
)

type Severity string

const (
	SeverityInformation Severity = "information"
	SeverityWarning     Severity = "warning"
	SeverityMajor       Severity = "major"
	SeverityCritical    Severity = "critical"
)

type Signal struct {
	ID                   string     `json:"id"`
	Origin               string     `json:"origin"`
	ConnectorID          string     `json:"connector_id,omitempty"`
	ConnectorName        string     `json:"connector_name,omitempty"`
	ExternalEventID      string     `json:"external_event_id,omitempty"`
	ExternalObjectID     string     `json:"external_object_id,omitempty"`
	Name                 string     `json:"name"`
	Active               bool       `json:"active"`
	Severity             Severity   `json:"severity"`
	OpenedAt             time.Time  `json:"opened_at"`
	ResolvedAt           *time.Time `json:"resolved_at,omitempty"`
	UpstreamAcknowledged bool       `json:"upstream_acknowledged"`
	InvalidatedAt        *time.Time `json:"invalidated_at,omitempty"`
	InvalidatedBy        string     `json:"invalidated_by,omitempty"`
	InvalidationReason   string     `json:"invalidation_reason,omitempty"`
	RearmedAt            *time.Time `json:"rearmed_at,omitempty"`
}

type Activity struct {
	ID         int64          `json:"id"`
	Kind       string         `json:"kind"`
	Origin     string         `json:"origin"`
	ActorName  string         `json:"actor_name,omitempty"`
	Message    string         `json:"message"`
	Data       map[string]any `json:"data"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type Incident struct {
	ID                        string     `json:"id"`
	TargetID                  string     `json:"target_id"`
	TargetName                string     `json:"target_name"`
	NatureKey                 string     `json:"nature_key"`
	NatureLabel               string     `json:"nature_label"`
	Status                    string     `json:"status"`
	SourceSeverity            Severity   `json:"source_severity"`
	EffectiveSeverity         Severity   `json:"effective_severity"`
	OpenedAt                  time.Time  `json:"opened_at"`
	ResolvedAt                *time.Time `json:"resolved_at,omitempty"`
	AcknowledgedAt            *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy            string     `json:"acknowledged_by,omitempty"`
	AcknowledgementOrigin     string     `json:"acknowledgement_origin,omitempty"`
	AcknowledgementSyncStatus string     `json:"acknowledgement_sync_status"`
	AcknowledgementSyncError  string     `json:"acknowledgement_sync_error,omitempty"`
	MaintenanceActive         bool       `json:"maintenance_active"`
	MaintenanceEndsAt         *time.Time `json:"maintenance_ends_at,omitempty"`
	Signals                   []Signal   `json:"signals"`
	Activity                  []Activity `json:"activity"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type ZabbixSignal struct {
	TargetID             string
	BindingID            string
	ExternalEventID      string
	ExternalObjectID     string
	Name                 string
	Severity             Severity
	OpenedAt             time.Time
	UpstreamAcknowledged bool
	Suppressed           bool
}

type ReconcileZabbixInput struct {
	ConnectorID string
	ObservedAt  time.Time
	Signals     []ZabbixSignal
}

type UptimeKumaSignal struct {
	TargetID        string
	BindingID       string
	ExternalMonitor string
	Name            string
	Severity        Severity
}

type ReconcileUptimeKumaInput struct {
	ConnectorID string
	ObservedAt  time.Time
	Signals     []UptimeKumaSignal
}

type WebhookSignal struct {
	ConnectorID      string
	BindingID        string
	TargetID         string
	ExternalEventKey string
	NatureKey        string
	NatureLabel      string
	Summary          string
	Status           string
	Severity         Severity
	ObservedAt       time.Time
	Details          map[string]any
}

type AcknowledgementTarget struct {
	Origin          string
	ConnectorID     string
	ExternalEventID string
}

type AcknowledgementPlan struct {
	Incident Incident
	Targets  []AcknowledgementTarget
}

type Store interface {
	List(context.Context, string, int) ([]Incident, error)
	Get(context.Context, string) (Incident, error)
	AcknowledgeLocal(context.Context, string, string, string) (AcknowledgementPlan, error)
	CompleteAcknowledgement(context.Context, string, string, string) (Incident, error)
	InvalidateSignal(context.Context, string, string, string, string, string) (Incident, error)
}

type ExternalAcknowledger interface {
	Acknowledge(context.Context, AcknowledgementTarget, string) error
}

type Service struct {
	store        Store
	acknowledger ExternalAcknowledger
}

func NewService(store Store, acknowledger ExternalAcknowledger) *Service {
	return &Service{store: store, acknowledger: acknowledger}
}

func (service *Service) List(ctx context.Context, status string, limit int) ([]Incident, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "resolved" && status != "all" {
		return nil, fmt.Errorf("%w: status must be active, resolved, or all", ErrInvalidInput)
	}
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 500", ErrInvalidInput)
	}
	return service.store.List(ctx, status, limit)
}

func (service *Service) Get(ctx context.Context, incidentID string) (Incident, error) {
	return service.store.Get(ctx, incidentID)
}

func (service *Service) InvalidateSignal(ctx context.Context, incidentID, signalID, actorID, actorName, reason string) (Incident, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 8 || len([]rune(reason)) > 500 {
		return Incident{}, fmt.Errorf("%w: le motif doit contenir entre 8 et 500 caractères", ErrInvalidInput)
	}
	return service.store.InvalidateSignal(ctx, incidentID, signalID, actorID, actorName, reason)
}

func (service *Service) Acknowledge(ctx context.Context, incidentID, actorID, actorName string) (Incident, error) {
	plan, err := service.store.AcknowledgeLocal(ctx, incidentID, actorID, actorName)
	if err != nil {
		return Incident{}, err
	}
	if len(plan.Targets) == 0 {
		return plan.Incident, nil
	}

	errorsByTarget := make([]string, 0)
	seen := make(map[string]struct{}, len(plan.Targets))
	for _, target := range plan.Targets {
		key := target.Origin + ":" + target.ConnectorID + ":" + target.ExternalEventID
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		if service.acknowledger == nil {
			errorsByTarget = append(errorsByTarget, "aucun adaptateur de synchronisation n’est disponible")
			continue
		}
		message := "Acquitté depuis CairnOps"
		if strings.TrimSpace(actorName) != "" {
			message += " par " + strings.TrimSpace(actorName)
		}
		if err := service.acknowledger.Acknowledge(ctx, target, message); err != nil {
			errorsByTarget = append(errorsByTarget, err.Error())
		}
	}

	status, syncError := "synchronized", ""
	if len(errorsByTarget) > 0 {
		status = "failed"
		sort.Strings(errorsByTarget)
		syncError = strings.Join(errorsByTarget, "; ")
		if len(syncError) > 500 {
			syncError = syncError[:500]
		}
	}
	return service.store.CompleteAcknowledgement(ctx, incidentID, status, syncError)
}
