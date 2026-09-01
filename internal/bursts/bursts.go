package bursts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
)

var (
	ErrInvalidInput = errors.New("invalid incident burst input")
	ErrNotFound     = errors.New("incident burst not found")
	ErrConflict     = errors.New("incident burst conflict")
)

type Member struct {
	IncidentID        string             `json:"incident_id"`
	TargetID          string             `json:"target_id"`
	TargetName        string             `json:"target_name"`
	Status            string             `json:"status"`
	EffectiveSeverity incidents.Severity `json:"effective_severity"`
	OpenedAt          time.Time          `json:"opened_at"`
	ResolvedAt        *time.Time         `json:"resolved_at,omitempty"`
	AcknowledgedAt    *time.Time         `json:"acknowledged_at,omitempty"`
	MaintenanceActive bool               `json:"maintenance_active"`
	JoinedAt          time.Time          `json:"joined_at"`
}

type Activity struct {
	ID         int64          `json:"id"`
	Kind       string         `json:"kind"`
	ActorName  string         `json:"actor_name,omitempty"`
	Message    string         `json:"message"`
	Data       map[string]any `json:"data"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type Burst struct {
	ID                       string             `json:"id"`
	AnchorIncidentID         string             `json:"anchor_incident_id"`
	NatureScope              string             `json:"nature_scope"`
	NatureNamespace          string             `json:"nature_namespace"`
	NatureFingerprint        string             `json:"nature_fingerprint"`
	NatureLabel              string             `json:"nature_label"`
	Status                   string             `json:"status"`
	Severity                 incidents.Severity `json:"severity"`
	OpenedAt                 time.Time          `json:"opened_at"`
	LastJoinedAt             time.Time          `json:"last_joined_at"`
	PropagationWindowSeconds int                `json:"propagation_window_seconds"`
	PropagationEndsAt        time.Time          `json:"propagation_ends_at"`
	SealedAt                 *time.Time         `json:"sealed_at,omitempty"`
	ResolvedAt               *time.Time         `json:"resolved_at,omitempty"`
	AcknowledgedAt           *time.Time         `json:"acknowledged_at,omitempty"`
	AcknowledgedBy           string             `json:"acknowledged_by,omitempty"`
	Extended                 bool               `json:"extended"`
	ActiveIncidentCount      int                `json:"active_incident_count"`
	IncidentCount            int                `json:"incident_count"`
	AffectedTargetCount      int                `json:"affected_target_count"`
	TargetCount              int                `json:"target_count"`
	MaxAffectedTargets       int                `json:"max_affected_targets"`
	Revision                 int                `json:"revision"`
	Explanation              string             `json:"explanation"`
	Members                  []Member           `json:"members"`
	Activity                 []Activity         `json:"activity"`
	CreatedAt                time.Time          `json:"created_at"`
	UpdatedAt                time.Time          `json:"updated_at"`
}

type Store interface {
	List(context.Context, string, int) ([]Burst, error)
	Get(context.Context, string) (Burst, error)
	Acknowledge(context.Context, string, string, string) ([]string, error)
}

type IncidentAcknowledger interface {
	Acknowledge(context.Context, string, string, string) (incidents.Incident, error)
}

type Service struct {
	store     Store
	incidents IncidentAcknowledger
}

func NewService(store Store, incidentAcknowledger IncidentAcknowledger) *Service {
	return &Service{store: store, incidents: incidentAcknowledger}
}

func (service *Service) List(ctx context.Context, status string, limit int) ([]Burst, error) {
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

func (service *Service) Get(ctx context.Context, burstID string) (Burst, error) {
	return service.store.Get(ctx, burstID)
}

func (service *Service) Acknowledge(ctx context.Context, burstID, actorID, actorName string) (Burst, error) {
	memberIDs, err := service.store.Acknowledge(ctx, burstID, actorID, actorName)
	if err != nil {
		return Burst{}, err
	}
	// Chaque Incident garde sa propre tentative et son propre résultat de
	// synchronisation amont. Un échec n'annule jamais les autres acquittements.
	if service.incidents != nil {
		for _, incidentID := range memberIDs {
			if ctx.Err() != nil {
				return Burst{}, ctx.Err()
			}
			_, _ = service.incidents.Acknowledge(ctx, incidentID, actorID, actorName)
		}
	}
	return service.store.Get(ctx, burstID)
}
