package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openIdentityPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testsupport.Pool(t)
}

// Crée un compte directement en base : Initialize ne sert qu'au tout premier,
// et ces scénarios en demandent plusieurs.
func seedUser(t *testing.T, pool *pgxpool.Pool, role, password string) (id, username string) {
	t.Helper()
	username = "compte-" + role
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Compte de test', $2, $3)
		RETURNING id::text
	`, username, hash, role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id, username
}

func TestPostgresChangePasswordRequiresTheCurrentOne(t *testing.T) {
	pool := openIdentityPool(t)
	ctx := context.Background()
	store := NewStore(pool)
	const before, after = "ancienne-phrase-2026", "nouvelle-phrase-2026"
	_, username := seedUser(t, pool, "operator", before)

	// Une session ouverte avant le changement doit tomber avec lui.
	session, err := store.Login(ctx, LoginInput{Username: username, Password: before})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.ChangePassword(ctx, session.Principal.ID, "mauvais-mot-de-passe", after); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a wrong current password was accepted or failed for another reason: %v", err)
	}
	if err := store.ChangePassword(ctx, session.Principal.ID, before, "court"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a too-short password was accepted: %v", err)
	}
	if err := store.ChangePassword(ctx, session.Principal.ID, before, before); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("reusing the current password was accepted")
	}

	if err := store.ChangePassword(ctx, session.Principal.ID, before, after); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Login(ctx, LoginInput{Username: username, Password: before}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("the former password still opens a session")
	}
	if _, err := store.Login(ctx, LoginInput{Username: username, Password: after}); err != nil {
		t.Fatalf("the new password does not open a session: %v", err)
	}
	if _, err := store.Authenticate(ctx, session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("a session opened before the change survived it")
	}
}

func TestPostgresAdministratorResetsAnotherAccount(t *testing.T) {
	pool := openIdentityPool(t)
	ctx := context.Background()
	store := NewStore(pool)
	const before, after = "phrase-du-collegue-1", "phrase-du-collegue-2"
	userID, username := seedUser(t, pool, "observer", before)

	session, err := store.Login(ctx, LoginInput{Username: username, Password: before})
	if err != nil {
		t.Fatal(err)
	}

	principal, err := store.SetPassword(ctx, userID, after)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Username != username {
		t.Fatalf("reset returned %q instead of %q", principal.Username, username)
	}
	if _, err := store.Login(ctx, LoginInput{Username: username, Password: after}); err != nil {
		t.Fatalf("the reset password does not open a session: %v", err)
	}
	if _, err := store.Authenticate(ctx, session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("the account kept a live session through the reset")
	}

	if _, err := store.SetPassword(ctx, "00000000-0000-0000-0000-000000000000", after); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resetting an absent account did not report it: %v", err)
	}
}

// La porte de secours ne dit pas qui existe : un identifiant inconnu répond
// comme un jeton refusé, sinon elle deviendrait un oracle sur les comptes.
func TestPostgresRecoveryDoesNotRevealWhichAccountsExist(t *testing.T) {
	pool := openIdentityPool(t)
	ctx := context.Background()
	store := NewStore(pool)
	const before, after = "phrase-verrouillee-1", "phrase-verrouillee-2"
	_, username := seedUser(t, pool, "administrator", before)

	if _, err := store.RecoverPassword(ctx, "compte-qui-n-existe-pas", after); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("an unknown account produced %v instead of invalid credentials", err)
	}

	// L'identifiant est normalisé comme à la connexion.
	if _, err := store.RecoverPassword(ctx, "  "+username+"  ", after); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Login(ctx, LoginInput{Username: username, Password: after}); err != nil {
		t.Fatalf("the recovered password does not open a session: %v", err)
	}
}
