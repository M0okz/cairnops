// Package indicators owns the short-lived context imported from Connectors.
// It deliberately does not expose threshold evaluation: an Indicator can
// explain a Target or an Incident, but cannot decide their health.
package indicators

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

const (
	WindowDay  = "24h"
	WindowWeek = "7d"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
)

type Capability struct {
	Key       string    `json:"key"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

type Candidate struct {
	SemanticKey string         `json:"semantic_key"`
	Label       string         `json:"label"`
	ExternalID  string         `json:"external_id"`
	Dimension   string         `json:"dimension,omitempty"`
	Unit        string         `json:"unit"`
	Recommended bool           `json:"recommended"`
	Available   bool           `json:"available"`
	Reason      string         `json:"reason,omitempty"`
	Metadata    map[string]any `json:"metadata"`
}

type Indicator struct {
	ID               string         `json:"id"`
	ConnectorID      string         `json:"connector_id"`
	BindingID        string         `json:"binding_id"`
	TargetID         string         `json:"target_id"`
	SemanticKey      string         `json:"semantic_key"`
	Label            string         `json:"label"`
	ExternalID       string         `json:"external_id"`
	Dimension        string         `json:"dimension,omitempty"`
	Unit             string         `json:"unit"`
	Enabled          bool           `json:"enabled"`
	Metadata         map[string]any `json:"metadata"`
	LastValue        *float64       `json:"last_value,omitempty"`
	LastObservedAt   *time.Time     `json:"last_observed_at,omitempty"`
	LastError        string         `json:"last_error,omitempty"`
	Pinned           bool           `json:"pinned"`
	PinPosition      *int           `json:"pin_position,omitempty"`
	OverviewPosition *int           `json:"overview_position,omitempty"`
}

type Binding struct {
	ID           string      `json:"id,omitempty"`
	TargetID     string      `json:"target_id,omitempty"`
	TargetName   string      `json:"target_name,omitempty"`
	ExternalID   string      `json:"external_id"`
	ExternalName string      `json:"external_name"`
	Enabled      bool        `json:"enabled"`
	Imported     bool        `json:"imported"`
	Indicators   []Indicator `json:"indicators"`
	Candidates   []Candidate `json:"candidates"`
}

type Profile struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Specification []ProfileEntry `json:"specification"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type ProfileEntry struct {
	SemanticKey string `json:"semantic_key"`
	Dimension   string `json:"dimension,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type Activity struct {
	ID         int64          `json:"id"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActorName  string         `json:"actor_name,omitempty"`
	Summary    string         `json:"summary"`
	Data       map[string]any `json:"data"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type Configuration struct {
	ConnectorID   string       `json:"connector_id"`
	ConnectorKind string       `json:"connector_kind"`
	ConnectorName string       `json:"connector_name"`
	Endpoint      string       `json:"endpoint"`
	GeneratedAt   time.Time    `json:"generated_at"`
	ExpiresAt     *time.Time   `json:"expires_at,omitempty"`
	Capabilities  []Capability `json:"capabilities"`
	Bindings      []Binding    `json:"bindings"`
	Profiles      []Profile    `json:"profiles"`
	Activity      []Activity   `json:"activity"`
}

type Selection struct {
	SemanticKey string         `json:"semantic_key"`
	Label       string         `json:"label"`
	ExternalID  string         `json:"external_id"`
	Dimension   string         `json:"dimension,omitempty"`
	Unit        string         `json:"unit"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type BindingInput struct {
	ID           string      `json:"id,omitempty"`
	TargetID     string      `json:"target_id,omitempty"`
	ExternalID   string      `json:"external_id"`
	ExternalName string      `json:"external_name"`
	Enabled      bool        `json:"enabled"`
	Indicators   []Selection `json:"indicators"`
}

type ProfileInput struct {
	ID            string         `json:"id,omitempty"`
	Name          string         `json:"name"`
	Specification []ProfileEntry `json:"specification"`
}

type ApplyInput struct {
	Bindings []BindingInput `json:"bindings"`
	Profiles []ProfileInput `json:"profiles"`
	Summary  string         `json:"summary"`
}

type Point struct {
	At      time.Time `json:"at"`
	Value   float64   `json:"value"`
	Minimum *float64  `json:"minimum,omitempty"`
	Maximum *float64  `json:"maximum,omitempty"`
	Samples int       `json:"samples,omitempty"`
}

type Series struct {
	Window string  `json:"window"`
	Points []Point `json:"points"`
}

type TargetProjection struct {
	TargetID    string             `json:"target_id"`
	GeneratedAt time.Time          `json:"generated_at"`
	Indicators  []Indicator        `json:"indicators"`
	Series      map[string][]Point `json:"series,omitempty"`
}

type Snapshot struct {
	IndicatorID string    `json:"indicator_id,omitempty"`
	ImpactID    string    `json:"impact_id"`
	TargetID    string    `json:"target_id"`
	TargetName  string    `json:"target_name"`
	SemanticKey string    `json:"semantic_key"`
	Label       string    `json:"label"`
	Unit        string    `json:"unit"`
	Value       float64   `json:"value"`
	ObservedAt  time.Time `json:"observed_at"`
}

type IncidentProjection struct {
	IncidentID string             `json:"incident_id"`
	TargetIDs  []string           `json:"target_ids"`
	OpenedAt   time.Time          `json:"opened_at"`
	Snapshots  []Snapshot         `json:"snapshots"`
	Indicators []Indicator        `json:"indicators"`
	Series     map[string][]Point `json:"series"`
	Disclaimer string             `json:"disclaimer"`
}

type PinInput struct {
	IndicatorIDs []string `json:"indicator_ids"`
}

type Pin struct {
	IndicatorID string `json:"indicator_id"`
	Position    int    `json:"position"`
}

type Reading struct {
	IndicatorID string
	Value       float64
	ObservedAt  time.Time
}

type RuntimeIndicator struct {
	Indicator
	BindingExternalID string
}

type RuntimeConnector struct {
	ID               string
	Kind             string
	Endpoint         string
	CredentialSealed string
	Indicators       []RuntimeIndicator
}

func validateApply(input ApplyInput) error {
	input.Summary = strings.TrimSpace(input.Summary)
	if len(input.Bindings) > 5000 || len(input.Profiles) > 100 || len(input.Summary) > 500 {
		return fmt.Errorf("%w: configuration is too large", ErrInvalidInput)
	}
	seenBindings := make(map[string]struct{}, len(input.Bindings))
	for _, binding := range input.Bindings {
		key := strings.TrimSpace(binding.ExternalID)
		externalName := strings.TrimSpace(binding.ExternalName)
		if key == "" || len(key) > 255 || externalName == "" || len(externalName) > 160 ||
			(binding.ID != "" && !uuidPattern.MatchString(binding.ID)) ||
			(binding.TargetID != "" && !uuidPattern.MatchString(binding.TargetID)) {
			return fmt.Errorf("%w: invalid binding", ErrInvalidInput)
		}
		if _, duplicate := seenBindings[key]; duplicate {
			return fmt.Errorf("%w: duplicate binding %s", ErrInvalidInput, key)
		}
		seenBindings[key] = struct{}{}
		seenSelections := make(map[string]struct{}, len(binding.Indicators))
		for _, selection := range binding.Indicators {
			label := strings.TrimSpace(selection.Label)
			if !validSemanticUnit(selection.SemanticKey, selection.Unit) || strings.TrimSpace(selection.ExternalID) == "" || len(selection.ExternalID) > 255 || label == "" || len(label) > 160 || len(selection.Dimension) > 255 {
				return fmt.Errorf("%w: invalid indicator selection", ErrInvalidInput)
			}
			selectionKey := selection.SemanticKey + "\x00" + selection.Dimension
			if _, duplicate := seenSelections[selectionKey]; duplicate {
				return fmt.Errorf("%w: duplicate indicator selection", ErrInvalidInput)
			}
			seenSelections[selectionKey] = struct{}{}
		}
	}
	seenProfiles := make(map[string]struct{}, len(input.Profiles))
	for _, profile := range input.Profiles {
		name := strings.TrimSpace(profile.Name)
		key := strings.ToLower(name)
		if name == "" || len(name) > 100 || len(profile.Specification) > 100 ||
			(profile.ID != "" && !uuidPattern.MatchString(profile.ID)) {
			return fmt.Errorf("%w: invalid profile", ErrInvalidInput)
		}
		if _, duplicate := seenProfiles[key]; duplicate {
			return fmt.Errorf("%w: duplicate profile %s", ErrInvalidInput, name)
		}
		seenProfiles[key] = struct{}{}
		seenEntries := map[string]struct{}{}
		for _, entry := range profile.Specification {
			if !validSemantic(entry.SemanticKey) || len(entry.Dimension) > 255 {
				return fmt.Errorf("%w: invalid profile entry", ErrInvalidInput)
			}
			entryKey := entry.SemanticKey + "\x00" + entry.Dimension
			if _, duplicate := seenEntries[entryKey]; duplicate {
				return fmt.Errorf("%w: duplicate profile entry", ErrInvalidInput)
			}
			seenEntries[entryKey] = struct{}{}
		}
	}
	return nil
}

func validSemantic(semantic string) bool {
	for candidate := range map[string]struct{}{
		"cpu.utilization": {}, "memory.utilization": {}, "filesystem.utilization": {},
		"network.in": {}, "network.out": {}, "response.time": {},
		"certificate.days_remaining": {}, "certificate.valid": {}, "updates.count": {},
		"security_updates.count": {}, "reboot.required": {}, "reporting.age": {},
	} {
		if semantic == candidate {
			return true
		}
	}
	return false
}

func validSemanticUnit(semantic, unit string) bool {
	expected := map[string]string{
		"cpu.utilization": "percent", "memory.utilization": "percent", "filesystem.utilization": "percent",
		"network.in": "bytes_per_second", "network.out": "bytes_per_second", "response.time": "milliseconds",
		"certificate.days_remaining": "days", "certificate.valid": "boolean", "updates.count": "count",
		"security_updates.count": "count", "reboot.required": "boolean", "reporting.age": "seconds",
	}
	return expected[semantic] == unit
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].SemanticKey != candidates[j].SemanticKey {
			return candidates[i].SemanticKey < candidates[j].SemanticKey
		}
		if candidates[i].Dimension != candidates[j].Dimension {
			return candidates[i].Dimension < candidates[j].Dimension
		}
		return candidates[i].ExternalID < candidates[j].ExternalID
	})
}
