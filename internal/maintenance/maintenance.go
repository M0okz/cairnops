package maintenance

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid maintenance input")
	ErrNotFound     = errors.New("maintenance not found")
	ErrConflict     = errors.New("maintenance conflict")
)

type Target struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Maintenance struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Reason      string     `json:"reason"`
	State       string     `json:"state"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	Targets     []Target   `json:"targets"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CreateInput struct {
	Name      string    `json:"name"`
	Reason    string    `json:"reason"`
	TargetIDs []string  `json:"target_ids"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}

type Store interface {
	List(context.Context, int) ([]Maintenance, error)
	Create(context.Context, string, CreateInput) (Maintenance, error)
	Cancel(context.Context, string, string) (Maintenance, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (service *Service) List(ctx context.Context, limit int) ([]Maintenance, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalidInput)
	}
	return service.store.List(ctx, limit)
}

func (service *Service) Create(ctx context.Context, actorID string, input CreateInput) (Maintenance, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Name)) < 3 || len([]rune(input.Name)) > 160 {
		return Maintenance{}, fmt.Errorf("%w: le nom doit contenir entre 3 et 160 caractères", ErrInvalidInput)
	}
	if len([]rune(input.Reason)) < 8 || len([]rune(input.Reason)) > 500 {
		return Maintenance{}, fmt.Errorf("%w: le motif doit contenir entre 8 et 500 caractères", ErrInvalidInput)
	}
	if len(input.TargetIDs) == 0 || len(input.TargetIDs) > 200 {
		return Maintenance{}, fmt.Errorf("%w: sélectionnez entre 1 et 200 cibles", ErrInvalidInput)
	}
	seen := make(map[string]struct{}, len(input.TargetIDs))
	targetIDs := make([]string, 0, len(input.TargetIDs))
	for _, targetID := range input.TargetIDs {
		targetID = strings.TrimSpace(targetID)
		if !validUUID(targetID) {
			return Maintenance{}, fmt.Errorf("%w: identifiant de cible invalide", ErrInvalidInput)
		}
		if _, duplicate := seen[targetID]; duplicate {
			continue
		}
		seen[targetID] = struct{}{}
		targetIDs = append(targetIDs, targetID)
	}
	input.TargetIDs = targetIDs
	now := service.now().UTC()
	if input.StartsAt.IsZero() {
		input.StartsAt = now
	} else {
		input.StartsAt = input.StartsAt.UTC()
	}
	input.EndsAt = input.EndsAt.UTC()
	if input.EndsAt.IsZero() || !input.EndsAt.After(input.StartsAt) {
		return Maintenance{}, fmt.Errorf("%w: la fin doit être postérieure au début", ErrInvalidInput)
	}
	if input.EndsAt.Sub(input.StartsAt) > 31*24*time.Hour {
		return Maintenance{}, fmt.Errorf("%w: une maintenance ne peut pas dépasser 31 jours", ErrInvalidInput)
	}
	if input.EndsAt.Before(now) {
		return Maintenance{}, fmt.Errorf("%w: la fenêtre est déjà terminée", ErrInvalidInput)
	}
	return service.store.Create(ctx, actorID, input)
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil && len(decoded) == 16
}

func (service *Service) Cancel(ctx context.Context, maintenanceID, actorID string) (Maintenance, error) {
	return service.store.Cancel(ctx, maintenanceID, actorID)
}
