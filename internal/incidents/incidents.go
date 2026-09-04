package incidents

import (
	"context"
	"errors"
	"fmt"
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

type Evidence struct {
	ID                        string     `json:"id"`
	ImpactID                  string     `json:"impact_id"`
	TargetID                  string     `json:"target_id"`
	Origin                    string     `json:"origin"`
	ConnectorID               string     `json:"connector_id,omitempty"`
	ConnectorName             string     `json:"connector_name,omitempty"`
	ExternalEventID           string     `json:"external_event_id,omitempty"`
	ExternalObjectID          string     `json:"external_object_id,omitempty"`
	Name                      string     `json:"name"`
	Active                    bool       `json:"active"`
	Severity                  Severity   `json:"severity"`
	OpenedAt                  time.Time  `json:"opened_at"`
	ResolvedAt                *time.Time `json:"resolved_at,omitempty"`
	UpstreamAcknowledged      bool       `json:"upstream_acknowledged"`
	AcknowledgementSyncStatus string     `json:"acknowledgement_sync_status"`
	AcknowledgementSyncError  string     `json:"acknowledgement_sync_error,omitempty"`
	AcknowledgementSyncedAt   *time.Time `json:"acknowledgement_synced_at,omitempty"`
	InvalidatedAt             *time.Time `json:"invalidated_at,omitempty"`
	InvalidatedBy             string     `json:"invalidated_by,omitempty"`
	InvalidationReason        string     `json:"invalidation_reason,omitempty"`
	RearmedAt                 *time.Time `json:"rearmed_at,omitempty"`
}

type Impact struct {
	ID                string     `json:"id"`
	TargetID          string     `json:"target_id"`
	TargetName        string     `json:"target_name"`
	Status            string     `json:"status"`
	SourceSeverity    Severity   `json:"source_severity"`
	EffectiveSeverity Severity   `json:"effective_severity"`
	OpenedAt          time.Time  `json:"opened_at"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	MaintenanceActive bool       `json:"maintenance_active"`
	MaintenanceEndsAt *time.Time `json:"maintenance_ends_at,omitempty"`
	Evidence          []Evidence `json:"evidence"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Activity struct {
	ID         int64          `json:"id"`
	ImpactID   string         `json:"impact_id,omitempty"`
	EvidenceID string         `json:"evidence_id,omitempty"`
	Kind       string         `json:"kind"`
	Origin     string         `json:"origin"`
	ActorName  string         `json:"actor_name,omitempty"`
	Message    string         `json:"message"`
	Data       map[string]any `json:"data"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type Incident struct {
	ID                        string     `json:"id"`
	NatureKey                 string     `json:"nature_key"`
	NatureLabel               string     `json:"nature_label"`
	NatureScope               string     `json:"nature_scope"`
	NatureNamespace           string     `json:"nature_namespace"`
	NatureFingerprint         string     `json:"nature_fingerprint"`
	PropagationEligible       bool       `json:"propagation_eligible"`
	Status                    string     `json:"status"`
	PropagationStatus         string     `json:"propagation_status"`
	Severity                  Severity   `json:"severity"`
	OpenedAt                  time.Time  `json:"opened_at"`
	LastImpactAt              time.Time  `json:"last_impact_at"`
	PropagationWindowSeconds  int        `json:"propagation_window_seconds"`
	PropagationEndsAt         time.Time  `json:"propagation_ends_at"`
	PropagationClosedAt       *time.Time `json:"propagation_closed_at,omitempty"`
	ResolvedAt                *time.Time `json:"resolved_at,omitempty"`
	AcknowledgedAt            *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy            string     `json:"acknowledged_by,omitempty"`
	AcknowledgementOrigin     string     `json:"acknowledgement_origin,omitempty"`
	AcknowledgementSyncStatus string     `json:"acknowledgement_sync_status"`
	AcknowledgementSyncError  string     `json:"acknowledgement_sync_error,omitempty"`
	Extended                  bool       `json:"extended"`
	ActiveImpactCount         int        `json:"active_impact_count"`
	ImpactCount               int        `json:"impact_count"`
	AffectedTargetCount       int        `json:"affected_target_count"`
	MaxAffectedTargets        int        `json:"max_affected_targets"`
	Revision                  int        `json:"revision"`
	Impacts                   []Impact   `json:"impacts"`
	Activity                  []Activity `json:"activity"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type ZabbixSignal struct {
	TargetID             string
	BindingID            string
	ExternalEventID      string
	ExternalObjectID     string
	NatureFingerprint    string
	CanonicalNature      string
	Name                 string
	Severity             Severity
	OpenedAt             time.Time
	UpstreamAcknowledged bool
	Suppressed           bool
}

// NatureIdentity est la preuve technique utilisée pour rapprocher des
// Incidents. Key continue d'identifier l'Incident sur une Cible ; Scope,
// Namespace et Fingerprint identifient la Nature indépendamment de la Cible.
// Eligible reste faux lorsque le Connecteur ne peut pas fournir cette preuve.
type NatureIdentity struct {
	Key         string
	Label       string
	Scope       string
	Namespace   string
	Fingerprint string
	Eligible    bool
}

func CanonicalNature(key, label string) NatureIdentity {
	return NatureIdentity{
		Key: key, Label: label, Scope: "canonical", Namespace: "cairnops",
		Fingerprint: key, Eligible: true,
	}
}

func ConnectorNature(connectorID, key, label, fingerprint string) NatureIdentity {
	return NatureIdentity{
		Key: key, Label: label, Scope: "connector", Namespace: connectorID,
		Fingerprint: fingerprint, Eligible: strings.TrimSpace(fingerprint) != "",
	}
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
	ConnectorID      string
	ObservedAt       time.Time
	ObservedBindings []string
	Signals          []UptimeKumaSignal
}

type PatchMonSignal struct {
	TargetID     string
	BindingID    string
	ExternalHost string
	ConditionKey string
	NatureKey    string
	NatureLabel  string
	Name         string
	Severity     Severity
	Details      map[string]any
}

type ReconcilePatchMonInput struct {
	ConnectorID      string
	ObservedAt       time.Time
	ObservedBindings []string
	Signals          []PatchMonSignal
}

type ArgusSignal struct {
	TargetID        string
	BindingID       string
	ExternalService string
	NatureKey       string
	NatureLabel     string
	Name            string
	Severity        Severity
	DeployedVersion string
	LatestVersion   string
	Details         map[string]any
}

type ReconcileArgusInput struct {
	ConnectorID      string
	ObservedAt       time.Time
	ObservedBindings []string
	Signals          []ArgusSignal
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
	EvidenceID      string
	Origin          string
	ConnectorID     string
	ExternalEventID string
}

type AcknowledgementResult struct {
	EvidenceID string
	Error      string
}

type AcknowledgementPlan struct {
	Incident Incident
	Targets  []AcknowledgementTarget
}

// OpenedDay porte le nombre d'Incidents ouverts un jour donné. Le jour est daté en
// UTC, comme les agrégats horaires : c'est l'instance qui découpe le temps,
// pas le client qui la lit.
type OpenedDay struct {
	Day    time.Time `json:"day"`
	Opened int       `json:"opened"`
}

type Store interface {
	List(context.Context, string, int) ([]Incident, error)
	ListForTarget(context.Context, string, string, int) ([]Incident, error)
	Get(context.Context, string) (Incident, error)
	OpenedByDay(context.Context, int) ([]OpenedDay, error)
	AcknowledgeLocal(context.Context, string, string, string) (AcknowledgementPlan, error)
	CompleteAcknowledgement(context.Context, string, []AcknowledgementResult) (Incident, error)
	InvalidateEvidence(context.Context, string, string, string, string, string) (Incident, error)
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
	status, err := validateListInput(status, limit)
	if err != nil {
		return nil, err
	}
	return service.store.List(ctx, status, limit)
}

func (service *Service) ListForTarget(ctx context.Context, status, targetID string, limit int) ([]Incident, error) {
	status, err := validateListInput(status, limit)
	if err != nil {
		return nil, err
	}
	return service.store.ListForTarget(ctx, status, targetID, limit)
}

func validateListInput(status string, limit int) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "resolved" && status != "all" {
		return "", fmt.Errorf("%w: status must be active, resolved, or all", ErrInvalidInput)
	}
	if limit < 1 || limit > 500 {
		return "", fmt.Errorf("%w: limit must be between 1 and 500", ErrInvalidInput)
	}
	return status, nil
}

func (service *Service) Get(ctx context.Context, incidentID string) (Incident, error) {
	return service.store.Get(ctx, incidentID)
}

// OpenedByDay rend le nombre d'Incidents ouverts par jour sur la fenêtre
// demandée, du plus ancien au plus récent. La Vue d'ensemble s'en sert pour
// situer le compte du moment dans les jours qui le précèdent : un zéro se lit
// autrement selon qu'il succède au calme ou à une semaine agitée.
func (service *Service) OpenedByDay(ctx context.Context, days int) ([]OpenedDay, error) {
	if days < 1 || days > 90 {
		return nil, fmt.Errorf("%w: days must be between 1 and 90", ErrInvalidInput)
	}
	return service.store.OpenedByDay(ctx, days)
}

func (service *Service) InvalidateEvidence(ctx context.Context, incidentID, evidenceID, actorID, actorName, reason string) (Incident, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 8 || len([]rune(reason)) > 500 {
		return Incident{}, fmt.Errorf("%w: le motif doit contenir entre 8 et 500 caractères", ErrInvalidInput)
	}
	return service.store.InvalidateEvidence(ctx, incidentID, evidenceID, actorID, actorName, reason)
}

func (service *Service) Acknowledge(ctx context.Context, incidentID, actorID, actorName string) (Incident, error) {
	plan, err := service.store.AcknowledgeLocal(ctx, incidentID, actorID, actorName)
	if err != nil {
		return Incident{}, err
	}
	if len(plan.Targets) == 0 {
		return plan.Incident, nil
	}

	results := make([]AcknowledgementResult, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		if service.acknowledger == nil {
			results = append(results, AcknowledgementResult{
				EvidenceID: target.EvidenceID,
				Error:      "aucun adaptateur de synchronisation n’est disponible",
			})
			continue
		}
		message := "Acquitté depuis CairnOps"
		if strings.TrimSpace(actorName) != "" {
			message += " par " + strings.TrimSpace(actorName)
		}
		if err := service.acknowledger.Acknowledge(ctx, target, message); err != nil {
			results = append(results, AcknowledgementResult{EvidenceID: target.EvidenceID, Error: err.Error()})
			continue
		}
		results = append(results, AcknowledgementResult{EvidenceID: target.EvidenceID})
	}
	return service.store.CompleteAcknowledgement(ctx, incidentID, results)
}
