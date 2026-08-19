package identity

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound signale un compte absent. Il ne remonte jamais à un visiteur non
// authentifié : la porte de secours répond ErrInvalidCredentials pour ne pas
// révéler quels identifiants existent.
var ErrNotFound = errors.New("user not found")

// Trois chemins mènent à un nouveau mot de passe, et ils diffèrent seulement
// par ce qu'ils exigent en preuve :
//
//   - ChangePassword     : la personne connaît le mot de passe actuel ;
//   - SetPassword        : un Administrateur agit sur le compte d'un tiers ;
//   - RecoverPassword    : personne ne peut plus se connecter, et la preuve
//     devient le Jeton d'amorçage, c'est-à-dire le contrôle du déploiement.
//
// Tous les trois révoquent les sessions ouvertes du compte concerné, dans la
// transaction qui écrit l'empreinte. Un mot de passe que l'on remplace est
// souvent un mot de passe que l'on soupçonne ; laisser vivre les sessions
// existantes viderait le geste de son sens.

// ChangePassword remplace le mot de passe d'un compte après avoir vérifié
// l'actuel. C'est le seul chemin ouvert à tous les rôles.
func (store *Store) ChangePassword(ctx context.Context, userID, current, next string) error {
	if err := validatePassword(next); err != nil {
		return err
	}
	if current == next {
		return fmt.Errorf("%w: the new password must differ from the current one", ErrInvalidInput)
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password change: %w", err)
	}
	defer tx.Rollback(ctx)

	var hash string
	err = tx.QueryRow(ctx, `
		SELECT password_hash FROM cairnops_users WHERE id = $1::uuid FOR UPDATE
	`, userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load password hash: %w", err)
	}

	valid, err := verifyPassword(current, hash)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return ErrInvalidCredentials
	}

	if err := writePassword(ctx, tx, userID, next); err != nil {
		return err
	}
	if err := revokeSessions(ctx, tx, userID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password change: %w", err)
	}
	return nil
}

// SetPassword remplace le mot de passe d'un compte sans connaître l'ancien.
// L'appelant est responsable d'exiger le rôle Administrateur.
func (store *Store) SetPassword(ctx context.Context, userID, next string) (Principal, error) {
	return store.replacePassword(ctx, `id = $1::uuid`, userID, next, ErrNotFound)
}

// RecoverPassword est la porte de secours : elle vise un compte par son
// identifiant et ne demande aucune session, car elle sert précisément quand
// plus personne ne peut ouvrir de session. Sa preuve est le Jeton d'amorçage,
// vérifié en amont par le transport.
//
// Un compte désactivé lui répond comme un compte absent : rendre son mot de
// passe à quelqu'un que l'on a retiré du service serait le contraire du geste.
// L'instance garde de toute façon un Administrateur actif, à qui cette porte
// reste ouverte.
func (store *Store) RecoverPassword(ctx context.Context, username, next string) (Principal, error) {
	return store.replacePassword(ctx, `lower(username) = $1 AND deactivated_at IS NULL`, normalizeUsername(username), next, ErrInvalidCredentials)
}

func (store *Store) replacePassword(ctx context.Context, predicate, key, next string, absent error) (Principal, error) {
	if err := validatePassword(next); err != nil {
		return Principal{}, err
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Principal{}, fmt.Errorf("begin password reset: %w", err)
	}
	defer tx.Rollback(ctx)

	var principal Principal
	err = tx.QueryRow(ctx, `
		SELECT id::text, username, display_name, role
		FROM cairnops_users WHERE `+predicate+` FOR UPDATE
	`, key).Scan(&principal.ID, &principal.Username, &principal.DisplayName, &principal.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, absent
	}
	if err != nil {
		return Principal{}, fmt.Errorf("find user: %w", err)
	}

	if err := writePassword(ctx, tx, principal.ID, next); err != nil {
		return Principal{}, err
	}
	if err := revokeSessions(ctx, tx, principal.ID); err != nil {
		return Principal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Principal{}, fmt.Errorf("commit password reset: %w", err)
	}
	return principal, nil
}

func writePassword(ctx context.Context, tx pgx.Tx, userID, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_users SET password_hash = $2, updated_at = now() WHERE id = $1::uuid
	`, userID, hash); err != nil {
		return fmt.Errorf("store password hash: %w", err)
	}
	return nil
}

func revokeSessions(ctx context.Context, tx pgx.Tx, userID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_sessions SET revoked_at = now()
		WHERE user_id = $1::uuid AND revoked_at IS NULL
	`, userID); err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	return nil
}

// Les bornes sont celles de la mise en service : un mot de passe remplacé n'a
// aucune raison d'être plus faible que le premier.
func validatePassword(password string) error {
	if !utf8.ValidString(password) || len(password) < 12 || len(password) > 128 {
		return fmt.Errorf("%w: password must contain between 12 and 128 UTF-8 bytes", ErrInvalidInput)
	}
	return nil
}
