package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrConflict signale un geste que l'état de l'instance interdit : un
// identifiant déjà pris, le dernier Administrateur que l'on voudrait retirer,
// ou un Administrateur qui agirait sur son propre compte.
var ErrConflict = errors.New("account conflict")

const uniqueViolation = "23505"

// Les trois rôles de la V1. Ils sont globaux : un compte porte le même partout
// dans l'instance, et la base les contraint déjà.
var roles = []string{"administrator", "operator", "observer"}

// Account est un compte vu depuis l'écran qui les administre. Il ajoute au
// Principal ce qui n'intéresse que cet écran : depuis quand le compte existe,
// et depuis quand il ne sert plus.
type Account struct {
	Principal
	CreatedAt                time.Time  `json:"created_at"`
	DeactivatedAt            *time.Time `json:"deactivated_at"`
	ExternalSuspendedAt      *time.Time `json:"external_suspended_at"`
	ExternalSuspensionReason string     `json:"external_suspension_reason"`
	// La présence tient en deux nombres : combien de sessions le compte tient
	// ouvertes, et quand il s'est manifesté pour la dernière fois. La dernière
	// activité regarde toutes les sessions, même révoquées : un compte
	// désactivé garde la trace de son dernier passage.
	ActiveSessions int        `json:"active_sessions"`
	LastSeenAt     *time.Time `json:"last_seen_at"`
}

type CreateAccountInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}

// UpdateAccountInput ne porte que ce que la requête a nommé : un champ absent
// reste inchangé, ce qui distingue « ne touchez pas au rôle » de « mettez-le à
// vide ».
type UpdateAccountInput struct {
	DisplayName *string `json:"display_name"`
	Role        *string `json:"role"`
}

const accountColumns = `account.id::text, account.username, account.display_name, account.role,
	account.authorization_regime, account.created_at, account.deactivated_at,
	account.external_suspended_at, account.external_suspension_reason,
	presence.active_sessions, presence.last_seen_at`

// Toute lecture d'un compte passe par la même source : la ligne du compte et,
// à côté, ce que ses sessions racontent de lui.
const accountSource = `
	FROM cairnops_users AS account
	LEFT JOIN LATERAL (
		SELECT count(*) FILTER (
			WHERE session.revoked_at IS NULL AND session.expires_at > now()
		) AS active_sessions,
		max(session.last_seen_at) AS last_seen_at
		FROM cairnops_sessions AS session
		WHERE session.user_id = account.id
	) AS presence ON true`

// ListAccounts énumère les comptes de l'instance, actifs d'abord. Aucune
// empreinte n'en sort.
func (store *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT `+accountColumns+accountSource+`
		ORDER BY account.deactivated_at IS NOT NULL, account.created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0)
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// CountActiveSessions dit combien de sessions un compte tient ouvertes. Son
// propre écran de Réglages le demande pour lui-même : la liste des comptes, qui
// porte déjà le nombre, n'est lisible que par les Administrateurs.
func (store *Store) CountActiveSessions(ctx context.Context, userID string) (int, error) {
	var sessions int
	err := store.pool.QueryRow(ctx, `
		SELECT count(*) FROM cairnops_sessions
		WHERE user_id = $1::uuid AND revoked_at IS NULL AND expires_at > now()
	`, userID).Scan(&sessions)
	if err != nil {
		return 0, fmt.Errorf("count active sessions: %w", err)
	}
	return sessions, nil
}

// CreateAccount ouvre un compte avec son premier mot de passe. CairnOps n'envoie
// pas de courrier en V1 : l'Administrateur choisit ce mot de passe et le
// transmet lui-même, exactement comme une réinitialisation.
func (store *Store) CreateAccount(ctx context.Context, input CreateAccountInput) (Account, error) {
	input.Username = normalizeUsername(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if err := validateIdentityInput(input.Username, input.DisplayName, input.Password); err != nil {
		return Account{}, err
	}
	if err := validateRole(input.Role); err != nil {
		return Account{}, err
	}
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return Account{}, err
	}

	// L'unicité est celle de l'index : la demander à l'avance laisserait passer
	// deux créations simultanées du même identifiant.
	row := store.pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, username, display_name, role, authorization_regime,
		          created_at, deactivated_at, external_suspended_at,
		          external_suspension_reason, 0, NULL::timestamptz`,
		input.Username, input.DisplayName, passwordHash, input.Role)
	account, err := scanAccount(row)
	var violation *pgconn.PgError
	if errors.As(err, &violation) && violation.Code == uniqueViolation {
		return Account{}, fmt.Errorf("%w: cet identifiant est déjà pris", ErrConflict)
	}
	if err != nil {
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	return account, nil
}

// UpdateAccount corrige un nom d'affichage, ou change un rôle. L'identifiant,
// lui, ne bouge pas : il désigne la personne dans les sessions et dans le
// Journal, et le renommer brouillerait ce que l'on relit.
func (store *Store) UpdateAccount(ctx context.Context, actorID, userID string, input UpdateAccountInput) (Account, error) {
	if input.DisplayName == nil && input.Role == nil {
		return Account{}, fmt.Errorf("%w: rien à modifier", ErrInvalidInput)
	}
	if input.DisplayName != nil {
		trimmed := strings.TrimSpace(*input.DisplayName)
		if err := validateDisplayName(trimmed); err != nil {
			return Account{}, err
		}
		input.DisplayName = &trimmed
	}
	if input.Role != nil {
		if err := validateRole(*input.Role); err != nil {
			return Account{}, err
		}
		// Se retirer soi-même l'Administration, c'est se fermer la porte
		// derrière soi sans que personne n'ait demandé à la rouvrir.
		if actorID == userID {
			return Account{}, fmt.Errorf("%w: changez le rôle d'un autre compte que le vôtre", ErrConflict)
		}
	}

	return store.mutateAccount(ctx, userID, func(ctx context.Context, tx pgx.Tx) error {
		if input.Role != nil {
			var regime string
			if err := tx.QueryRow(ctx, `SELECT authorization_regime FROM cairnops_users WHERE id = $1::uuid`, userID).Scan(&regime); err != nil {
				return fmt.Errorf("read authorization regime: %w", err)
			}
			if regime == "external" {
				return fmt.Errorf("%w: le rôle d'un Utilisateur externe vient des groupes OIDC", ErrConflict)
			}
		}
		_, err := tx.Exec(ctx, `
			UPDATE cairnops_users
			SET display_name = COALESCE($2, display_name),
			    role = COALESCE($3, role),
			    updated_at = now()
			WHERE id = $1::uuid
		`, userID, input.DisplayName, input.Role)
		if err != nil {
			return fmt.Errorf("update account: %w", err)
		}
		// Rétrograder ferme des portes que la session ouverte tenait encore.
		if input.Role != nil {
			return revokeSessions(ctx, tx, userID)
		}
		return nil
	})
}

// SetAccountActivation retire un compte du service, ou l'y remet. Un compte
// désactivé ne peut plus ouvrir de session et ses sessions en cours tombent,
// mais tout ce qu'il a décidé reste à son nom.
func (store *Store) SetAccountActivation(ctx context.Context, actorID, userID string, active bool) (Account, error) {
	if !active && actorID == userID {
		return Account{}, fmt.Errorf("%w: désactivez un autre compte que le vôtre", ErrConflict)
	}

	return store.mutateAccount(ctx, userID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_users
			SET deactivated_at = CASE WHEN $2 THEN NULL ELSE COALESCE(deactivated_at, now()) END,
			    updated_at = now()
			WHERE id = $1::uuid
		`, userID, active); err != nil {
			return fmt.Errorf("set account activation: %w", err)
		}
		if active {
			return nil
		}
		return revokeSessions(ctx, tx, userID)
	})
}

// mutateAccount tient la même garde autour de tous les gestes qui touchent un
// compte : verrouiller la ligne visée, appliquer le geste, puis vérifier que
// l'instance a toujours un Administrateur actif. La vérification est dans la
// transaction parce que deux Administrateurs qui se rétrogradent l'un l'autre
// au même instant liraient tous deux un état encore rassurant.
func (store *Store) mutateAccount(ctx context.Context, userID string, apply func(context.Context, pgx.Tx) error) (Account, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("begin account change: %w", err)
	}
	defer tx.Rollback(ctx)

	var locked string
	err = tx.QueryRow(ctx, `SELECT id::text FROM cairnops_users WHERE id = $1::uuid FOR UPDATE`, userID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("lock account: %w", err)
	}

	if err := apply(ctx, tx); err != nil {
		return Account{}, err
	}

	var administrators int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM cairnops_users
		WHERE role = 'administrator' AND deactivated_at IS NULL
		  AND authorization_regime = 'local' AND password_hash IS NOT NULL
	`).Scan(&administrators); err != nil {
		return Account{}, fmt.Errorf("count administrators: %w", err)
	}
	if administrators == 0 {
		return Account{}, fmt.Errorf("%w: l'instance doit garder un Administrateur actif", ErrConflict)
	}

	account, err := scanAccount(tx.QueryRow(ctx,
		`SELECT `+accountColumns+accountSource+` WHERE account.id = $1::uuid`, userID))
	if err != nil {
		return Account{}, fmt.Errorf("read account: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, fmt.Errorf("commit account change: %w", err)
	}
	return account, nil
}

type scanner interface {
	Scan(...any) error
}

func scanAccount(row scanner) (Account, error) {
	var account Account
	err := row.Scan(&account.ID, &account.Username, &account.DisplayName, &account.Role,
		&account.AuthorizationRegime, &account.CreatedAt, &account.DeactivatedAt,
		&account.ExternalSuspendedAt, &account.ExternalSuspensionReason,
		&account.ActiveSessions, &account.LastSeenAt)
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func validateRole(role string) error {
	for _, known := range roles {
		if role == known {
			return nil
		}
	}
	return fmt.Errorf("%w: le rôle doit être %s", ErrInvalidInput, strings.Join(roles, ", "))
}
