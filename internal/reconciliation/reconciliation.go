package reconciliation

import (
	"context"
	"errors"
	"time"

	"github.com/M0okz/cairnops/internal/connectors"
)

var (
	ErrInvalidInput = errors.New("invalid reconciliation input")
	ErrNotFound     = errors.New("reconciliation not found")
	ErrConflict     = errors.New("reconciliation conflict")
)

type TargetSummary struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	CreatedAt           time.Time `json:"created_at"`
	SourceCount         int       `json:"source_count"`
	IncidentCount       int       `json:"incident_count"`
	ActiveIncidentCount int       `json:"active_incident_count"`
	ObservationCount    int64     `json:"observation_count"`
	MaintenanceCount    int       `json:"maintenance_count"`
	IndicatorCount      int       `json:"indicator_count"`
	RichnessScore       int       `json:"richness_score"`
	HumanManaged        bool      `json:"human_managed"`
}

type IncidentConflict struct {
	NatureKey     string `json:"nature_key"`
	NatureLabel   string `json:"nature_label"`
	LeftIncident  string `json:"left_incident_id"`
	RightIncident string `json:"right_incident_id"`
}

type Preview struct {
	Kind                string             `json:"kind"`
	Primary             TargetSummary      `json:"primary"`
	Secondary           TargetSummary      `json:"secondary"`
	SuggestedPrimaryID  string             `json:"suggested_primary_id"`
	Conflicts           []IncidentConflict `json:"incident_conflicts"`
	CombinedSourceCount int                `json:"combined_source_count"`
	Warnings            []string           `json:"warnings"`
	Source              *SourceSummary     `json:"source,omitempty"`
}

type SourceSummary struct {
	ID       string `json:"id"`
	TargetID string `json:"target_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Origin   string `json:"origin"`
}

type Suggestion struct {
	ID             string                     `json:"id"`
	Kind           string                     `json:"kind"`
	Left           TargetSummary              `json:"left"`
	Right          TargetSummary              `json:"right"`
	Source         *SourceSummary             `json:"source,omitempty"`
	Confidence     string                     `json:"confidence"`
	Score          int                        `json:"score"`
	Evidence       []connectors.MatchEvidence `json:"evidence"`
	Contradictions []connectors.MatchEvidence `json:"contradictions"`
	Status         string                     `json:"status"`
	SnoozedUntil   *time.Time                 `json:"snoozed_until,omitempty"`
	DecisionReason string                     `json:"decision_reason,omitempty"`
	LastDetectedAt time.Time                  `json:"last_detected_at"`
	CreatedAt      time.Time                  `json:"created_at"`
	UpdatedAt      time.Time                  `json:"updated_at"`
}

type Operation struct {
	ID                  string         `json:"id"`
	Kind                string         `json:"kind"`
	PrimaryTargetID     string         `json:"primary_target_id"`
	PrimaryTargetName   string         `json:"primary_target_name"`
	SecondaryTargetID   string         `json:"secondary_target_id"`
	SecondaryTargetName string         `json:"secondary_target_name"`
	SourceID            string         `json:"source_id,omitempty"`
	SuggestionID        string         `json:"suggestion_id,omitempty"`
	ArchiveOrigin       bool           `json:"archive_origin"`
	Reason              string         `json:"reason"`
	Status              string         `json:"status"`
	Stage               string         `json:"stage"`
	Preview             map[string]any `json:"preview"`
	Result              map[string]any `json:"result"`
	LastError           string         `json:"last_error,omitempty"`
	Attempts            int            `json:"attempts"`
	RequestedBy         string         `json:"requested_by,omitempty"`
	StartedAt           *time.Time     `json:"started_at,omitempty"`
	CompletedAt         *time.Time     `json:"completed_at,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type EnqueueInput struct {
	Kind              string `json:"kind"`
	PrimaryTargetID   string `json:"primary_target_id"`
	SecondaryTargetID string `json:"secondary_target_id"`
	SourceID          string `json:"source_id,omitempty"`
	SuggestionID      string `json:"suggestion_id,omitempty"`
	ArchiveOrigin     bool   `json:"archive_origin,omitempty"`
	Reason            string `json:"reason"`
	Confirmation      string `json:"confirmation"`
}

type SnoozeInput struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason,omitempty"`
}

type RejectInput struct {
	Reason string `json:"reason,omitempty"`
}

type TargetActivity struct {
	ID         int64          `json:"id"`
	TargetID   string         `json:"target_id"`
	Kind       string         `json:"kind"`
	ActorName  string         `json:"actor_name,omitempty"`
	Message    string         `json:"message"`
	Data       map[string]any `json:"data"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type Service interface {
	ListSuggestions(context.Context, string) ([]Suggestion, error)
	PreviewTargets(context.Context, string, string) (Preview, error)
	PreviewSourceMove(context.Context, string, string) (Preview, error)
	ListOperations(context.Context, int) ([]Operation, error)
	Enqueue(context.Context, string, EnqueueInput) (Operation, error)
	RejectSuggestion(context.Context, string, string, string) (Suggestion, error)
	SnoozeSuggestion(context.Context, string, string, SnoozeInput) (Suggestion, error)
	ResolveTarget(context.Context, string) (string, error)
	ListTargetActivity(context.Context, string, int) ([]TargetActivity, error)
}
