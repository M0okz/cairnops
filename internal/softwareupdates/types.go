// Package softwareupdates enrichit les versions observées par Argus sans piloter l'infrastructure.
package softwareupdates

import (
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid software update configuration")
var ErrNotFound = errors.New("software service not found")

type Source struct {
	Kind     string `json:"kind,omitempty"`
	URL      string `json:"url,omitempty"`
	Software string `json:"software,omitempty"`
}
type Note struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Body    string `json:"body"`
	Missing bool   `json:"missing"`
}
type Collection struct {
	Installed  string `json:"installed_version"`
	Target     string `json:"target_version"`
	Notes      []Note `json:"notes"`
	Incomplete bool   `json:"incomplete"`
}
type Point struct {
	Category string `json:"category"`
	Text     string `json:"text"`
	Version  string `json:"version"`
	Quote    string `json:"quote"`
}
type Summary struct {
	Overview []Point `json:"overview"`
	Details  []Point `json:"details"`
}
type Analysis struct {
	Notes     []Note    `json:"notes"`
	ID        int64     `json:"id"`
	Revision  int64     `json:"revision"`
	Installed string    `json:"installed_version"`
	Target    string    `json:"target_version"`
	Source    Source    `json:"source"`
	Hash      string    `json:"-"`
	Result    Summary   `json:"result"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Current   bool      `json:"current"`
}
type History struct {
	Installed  string    `json:"installed_version"`
	Target     string    `json:"target_version"`
	ObservedAt time.Time `json:"observed_at"`
}
type Service struct {
	ContentHash        string      `json:"-"`
	ID                 string      `json:"id"`
	TargetID           string      `json:"target_id"`
	ResourceName       string      `json:"resource_name"`
	Name               string      `json:"name"`
	Installed          string      `json:"installed_version"`
	Target             string      `json:"target_version"`
	ObservedAt         *time.Time  `json:"observed_at"`
	Known              bool        `json:"known"`
	Situation          string      `json:"situation"`
	Level              string      `json:"level,omitempty"`
	Group              string      `json:"group"`
	VerificationIssue  string      `json:"verification_issue,omitempty"`
	Approved           bool        `json:"approved"`
	Skipped            bool        `json:"skipped"`
	SecurityMentioned  bool        `json:"security_mentioned"`
	Source             Source      `json:"source"`
	SourceOrigin       string      `json:"source_origin"`
	Suggested          *Source     `json:"suggested_source"`
	ConfirmedAt        *time.Time  `json:"confirmed_at"`
	Revision           int64       `json:"revision"`
	State              string      `json:"state"`
	LastError          string      `json:"last_error"`
	CheckedAt          *time.Time  `json:"checked_at"`
	NextCheckAt        *time.Time  `json:"next_check_at"`
	Collection         *Collection `json:"collection"`
	CollectionRevision *int64      `json:"collection_revision"`
	Analyses           []Analysis  `json:"analyses"`
	History            []History   `json:"history"`
	Events             []Event     `json:"events"`
}
type AIConfig struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Model         string `json:"model"`
	KeyConfigured bool   `json:"key_configured"`
	APIKey        string `json:"api_key,omitempty"`
}
