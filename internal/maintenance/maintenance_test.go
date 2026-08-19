package maintenance

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct{ input CreateInput }

func (*fakeStore) List(context.Context, int) ([]Maintenance, error) { return nil, nil }
func (store *fakeStore) Create(_ context.Context, _ string, input CreateInput) (Maintenance, error) {
	store.input = input
	return Maintenance{Name: input.Name, StartsAt: input.StartsAt, EndsAt: input.EndsAt}, nil
}
func (*fakeStore) Cancel(context.Context, string, string) (Maintenance, error) {
	return Maintenance{}, nil
}

func TestImmediateMaintenanceUsesServerTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 14, 21, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	service := NewService(store)
	service.now = func() time.Time { return now }
	_, err := service.Create(context.Background(), "actor", CreateInput{
		Name: "Mise à jour stockage", Reason: "Remplacement préventif du volume",
		TargetIDs: []string{"10000000-0000-0000-0000-000000000001"}, EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.input.StartsAt.Equal(now) {
		t.Fatalf("expected server time %s, got %s", now, store.input.StartsAt)
	}
}

func TestMaintenanceRejectsAnEmptyTargetSet(t *testing.T) {
	t.Parallel()
	service := NewService(&fakeStore{})
	service.now = func() time.Time { return time.Date(2026, 8, 14, 21, 0, 0, 0, time.UTC) }
	_, err := service.Create(context.Background(), "actor", CreateInput{
		Name: "Mise à jour stockage", Reason: "Remplacement préventif du volume",
		EndsAt: service.now().Add(time.Hour),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
