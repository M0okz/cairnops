package identity

import (
	"context"
	"errors"
	"testing"
)

// Crée l'Administrateur fondateur et retourne le Store prêt à l'emploi. Presque
// tous les scénarios de comptes en ont besoin : les gestes qu'ils vérifient
// s'exercent au nom de quelqu'un.
func seedInstallation(t *testing.T) (*Store, Principal) {
	t.Helper()
	store := NewStore(openIdentityPool(t))
	session, err := store.Initialize(context.Background(), InitializeInput{
		InstanceName: "Instance d'essai",
		Username:     "fondatrice", DisplayName: "Fondatrice", Password: "phrase-de-fondation-2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, session.Principal
}

func TestPostgresListAccountsOmitsPasswordHashes(t *testing.T) {
	store, _ := seedInstallation(t)
	ctx := context.Background()
	_, username := seedUser(t, store.pool, "operator", "phrase-de-liste-2026")

	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, account := range accounts {
		if account.Username == username {
			found = true
			if account.Role != "operator" || account.ID == "" || account.CreatedAt.IsZero() {
				t.Fatalf("account is listed without its role, identifier or creation date: %+v", account)
			}
			if account.DeactivatedAt != nil {
				t.Fatalf("a fresh account is listed as deactivated: %+v", account)
			}
		}
	}
	if !found {
		t.Fatal("the created account is missing from the listing")
	}
}

func TestPostgresCreateAccountOpensASessionAndRefusesADuplicate(t *testing.T) {
	store, _ := seedInstallation(t)
	ctx := context.Background()
	const password = "phrase-du-nouveau-compte-2026"

	account, err := store.CreateAccount(ctx, CreateAccountInput{
		Username: "  Marguerite  ", DisplayName: "  Marguerite  ", Role: "operator", Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	if account.Username != "marguerite" || account.DisplayName != "Marguerite" {
		t.Fatalf("the identifier and the display name were not normalized: %+v", account)
	}

	session, err := store.Login(ctx, LoginInput{Username: "marguerite", Password: password})
	if err != nil {
		t.Fatalf("the new account cannot open a session: %v", err)
	}
	if session.Principal.Role != "operator" {
		t.Fatalf("the account did not keep its role: %+v", session.Principal)
	}

	// La casse ne fait pas deux comptes : l'unicité se lit sur l'identifiant
	// normalisé, et c'est l'index qui l'affirme.
	_, err = store.CreateAccount(ctx, CreateAccountInput{
		Username: "MARGUERITE", DisplayName: "Autre", Role: "observer", Password: password,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("a duplicate identifier was accepted: %v", err)
	}
}

func TestPostgresCreateAccountRefusesAnUnknownRole(t *testing.T) {
	store, _ := seedInstallation(t)

	_, err := store.CreateAccount(context.Background(), CreateAccountInput{
		Username: "camille", DisplayName: "Camille", Role: "superviseur", Password: "phrase-de-camille-2026",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("an unknown role was accepted: %v", err)
	}
}

func TestPostgresUpdateAccountChangesRoleAndClosesTheSessionsItOpened(t *testing.T) {
	store, founder := seedInstallation(t)
	ctx := context.Background()
	const password = "phrase-de-camille-2026"

	account, err := store.CreateAccount(ctx, CreateAccountInput{
		Username: "camille", DisplayName: "Camile", Role: "observer", Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Login(ctx, LoginInput{Username: "camille", Password: password})
	if err != nil {
		t.Fatal(err)
	}

	name, role := "Camille", "operator"
	updated, err := store.UpdateAccount(ctx, founder.ID, account.ID, UpdateAccountInput{DisplayName: &name, Role: &role})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Camille" || updated.Role != "operator" {
		t.Fatalf("the correction was not applied: %+v", updated)
	}

	// Le rôle a changé sous la session ouverte : la laisser vivre, c'est laisser
	// des portes ouvertes que le nouveau rôle ne franchit plus.
	if _, err := store.Authenticate(ctx, session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("the session survived the role change: %v", err)
	}

	// Corriger le seul nom d'affichage ne coupe personne.
	other, err := store.Login(ctx, LoginInput{Username: "camille", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	name = "Camille D."
	if _, err := store.UpdateAccount(ctx, founder.ID, account.ID, UpdateAccountInput{DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, other.Token); err != nil {
		t.Fatalf("a display name correction closed a session: %v", err)
	}
}

func TestPostgresAccountChangesRefuseToLeaveTheInstanceWithoutAnAdministrator(t *testing.T) {
	store, founder := seedInstallation(t)
	ctx := context.Background()
	role := "observer"

	// Un Administrateur n'agit pas sur son propre rôle ni sur sa propre
	// activité : c'est la première barrière, et la seule que l'interface montre.
	if _, err := store.UpdateAccount(ctx, founder.ID, founder.ID, UpdateAccountInput{Role: &role}); !errors.Is(err, ErrConflict) {
		t.Fatalf("an administrator demoted themselves: %v", err)
	}
	if _, err := store.SetAccountActivation(ctx, founder.ID, founder.ID, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("an administrator deactivated themselves: %v", err)
	}

	// La seconde barrière tient en transaction, et vise le cas que la première
	// ne voit pas : deux Administrateurs qui se retirent l'un l'autre.
	second, err := store.CreateAccount(ctx, CreateAccountInput{
		Username: "second", DisplayName: "Second", Role: "administrator", Password: "phrase-du-second-2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetAccountActivation(ctx, second.ID, founder.ID, false); err != nil {
		t.Fatalf("an administrator could not be deactivated while another remained: %v", err)
	}
	if _, err := store.SetAccountActivation(ctx, founder.ID, second.ID, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("the last administrator was deactivated: %v", err)
	}
	if _, err := store.UpdateAccount(ctx, founder.ID, second.ID, UpdateAccountInput{Role: &role}); !errors.Is(err, ErrConflict) {
		t.Fatalf("the last administrator was demoted: %v", err)
	}
}

func TestPostgresDeactivationClosesEveryDoorAndReactivationReopensThem(t *testing.T) {
	store, founder := seedInstallation(t)
	ctx := context.Background()
	const password = "phrase-de-sacha-2026"

	account, err := store.CreateAccount(ctx, CreateAccountInput{
		Username: "sacha", DisplayName: "Sacha", Role: "operator", Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Login(ctx, LoginInput{Username: "sacha", Password: password})
	if err != nil {
		t.Fatal(err)
	}

	deactivated, err := store.SetAccountActivation(ctx, founder.ID, account.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if deactivated.DeactivatedAt == nil {
		t.Fatalf("the account is not marked as deactivated: %+v", deactivated)
	}
	if _, err := store.Authenticate(ctx, session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("the open session survived the deactivation: %v", err)
	}
	if _, err := store.Login(ctx, LoginInput{Username: "sacha", Password: password}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a deactivated account opened a session: %v", err)
	}
	// La porte de secours n'est pas une porte dérobée : elle ne rend pas non
	// plus son mot de passe à un compte retiré du service.
	if _, err := store.RecoverPassword(ctx, "sacha", "autre-phrase-de-sacha-2026"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("the recovery door reopened a deactivated account: %v", err)
	}

	reactivated, err := store.SetAccountActivation(ctx, founder.ID, account.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if reactivated.DeactivatedAt != nil {
		t.Fatalf("the account stayed deactivated: %+v", reactivated)
	}
	if _, err := store.Login(ctx, LoginInput{Username: "sacha", Password: password}); err != nil {
		t.Fatalf("the reactivated account cannot open a session again: %v", err)
	}
}

func TestPostgresAccountChangesReportAnAbsentAccount(t *testing.T) {
	store, founder := seedInstallation(t)
	ctx := context.Background()
	const absent = "00000000-0000-0000-0000-000000000000"
	name := "Personne"

	if _, err := store.UpdateAccount(ctx, founder.ID, absent, UpdateAccountInput{DisplayName: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an absent account was corrected: %v", err)
	}
	if _, err := store.SetAccountActivation(ctx, founder.ID, absent, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an absent account was deactivated: %v", err)
	}
	if _, err := store.UpdateAccount(ctx, founder.ID, absent, UpdateAccountInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("an empty correction was accepted: %v", err)
	}
}
