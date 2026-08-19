package identity

import (
	"context"
	"errors"
	"testing"
)

// Le nom se pose avec le premier Administrateur, puis se relit sans session :
// la porte d'entrée doit dire où l'on frappe avant toute identification.
func TestPostgresInstanceNameIsSetAtSetupAndReadWithoutSession(t *testing.T) {
	store := NewStore(openIdentityPool(t))
	ctx := context.Background()

	before, err := store.SetupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Initialized || before.Name != "" {
		t.Fatalf("expected an unnamed installation before setup, got %#v", before)
	}

	if _, err := store.Initialize(ctx, InitializeInput{
		InstanceName: "  Astreinte Nord  ",
		Username:     "fondatrice", DisplayName: "Fondatrice", Password: "phrase-de-fondation-2026",
	}); err != nil {
		t.Fatal(err)
	}

	after, err := store.SetupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Initialized || after.Name != "Astreinte Nord" {
		t.Fatalf("expected the trimmed name to survive setup, got %#v", after)
	}
}

func TestPostgresInstanceRenameKeepsANameAtAllTimes(t *testing.T) {
	store, _ := seedInstallation(t)
	ctx := context.Background()

	renamed, err := store.RenameInstance(ctx, " Production Sud ")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Production Sud" || !renamed.Initialized {
		t.Fatalf("unexpected status after rename: %#v", renamed)
	}

	if _, err := store.RenameInstance(ctx, "   "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected an empty name to be refused, got %v", err)
	}
	status, err := store.SetupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Name != "Production Sud" {
		t.Fatalf("expected the refused rename to leave the name alone, got %q", status.Name)
	}
}
