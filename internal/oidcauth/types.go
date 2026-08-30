// Package oidcauth porte l'autorité externe OIDC sans la confondre avec les
// sessions locales de CairnOps.
package oidcauth

import (
	"context"
	"errors"
	"time"

	"github.com/M0okz/cairnops/internal/identity"
)

var (
	ErrNotConfigured = errors.New("oidc is not configured")
	ErrInvalidInput  = errors.New("invalid oidc input")
	ErrInvalidFlow   = errors.New("invalid oidc flow")
	ErrNotAuthorized = errors.New("oidc identity is not authorized")
	ErrConflict      = errors.New("oidc configuration conflict")
)

type GroupMappings struct {
	Administrator []string `json:"administrator"`
	Operator      []string `json:"operator"`
	Observer      []string `json:"observer"`
}

type ConfigurationInput struct {
	Label        string        `json:"label"`
	Issuer       string        `json:"issuer"`
	ClientID     string        `json:"client_id"`
	ClientSecret string        `json:"client_secret"`
	GroupsClaim  string        `json:"groups_claim"`
	Groups       GroupMappings `json:"groups"`
}

type Configuration struct {
	ID                     string        `json:"id"`
	State                  string        `json:"state"`
	Label                  string        `json:"label"`
	Issuer                 string        `json:"issuer"`
	ClientID               string        `json:"client_id"`
	ClientSecretConfigured bool          `json:"client_secret_configured"`
	GroupsClaim            string        `json:"groups_claim"`
	Groups                 GroupMappings `json:"groups"`
	TestedAt               *time.Time    `json:"tested_at"`
	ActivatedAt            *time.Time    `json:"activated_at"`
	CreatedAt              time.Time     `json:"created_at"`
	UpdatedAt              time.Time     `json:"updated_at"`

	clientSecretSealed string
}

type ConfigurationSet struct {
	Active *Configuration `json:"active"`
	Draft  *Configuration `json:"draft"`
}

type PublicStatus struct {
	Enabled bool   `json:"enabled"`
	Label   string `json:"label,omitempty"`
}

// Authorization garde le state disponible à la couche HTTP afin qu'elle le
// lie au navigateur dans un cookie HttpOnly. Le state stocké en base empêche
// le rejeu ; cette seconde copie empêche un callback préparé dans un autre
// navigateur d'ouvrir une session ici.
type Authorization struct {
	URL   string
	State string
}

type Completion struct {
	Purpose  string                        `json:"purpose"`
	ReturnTo string                        `json:"-"`
	Session  identity.AuthenticatedSession `json:"-"`
}

type SessionIssuer interface {
	NewSession(ctx context.Context, userID string) (identity.AuthenticatedSession, error)
}
