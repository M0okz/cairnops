package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionLifetime = 12 * time.Hour

var (
	ErrAlreadyInitialized = errors.New("installation already initialized")
	ErrNotInitialized     = errors.New("installation not initialized")
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid session")
	usernamePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)
)

type Status struct {
	Initialized bool `json:"initialized"`
	// Le nom que porte cette instance. Il précède la session : la porte
	// d'entrée doit déjà dire où l'on frappe, avant toute identification.
	Name string `json:"name"`
}

type Principal struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type InitializeInput struct {
	InstanceName string `json:"instance_name"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Password     string `json:"password"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthenticatedSession struct {
	Principal Principal `json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"-"`
}

type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, now: time.Now}
}

func (store *Store) SetupStatus(ctx context.Context) (Status, error) {
	var status Status
	err := store.pool.QueryRow(ctx, `
		SELECT initialized_at IS NOT NULL, name
		FROM cairnops_installation
		WHERE singleton = true
	`).Scan(&status.Initialized, &status.Name)
	if err != nil {
		return Status{}, fmt.Errorf("read installation status: %w", err)
	}
	return status, nil
}

// RenameInstance change le nom que porte l'instance. Il ne tient à aucune
// donnée : le renommer ne déplace rien, c'est l'étiquette que les écrans
// lisent, et un Administrateur peut la corriger quand l'usage a changé.
func (store *Store) RenameInstance(ctx context.Context, name string) (Status, error) {
	name = strings.TrimSpace(name)
	if err := validateInstanceName(name); err != nil {
		return Status{}, err
	}
	var status Status
	if err := store.pool.QueryRow(ctx, `
		UPDATE cairnops_installation SET name = $1
		WHERE singleton = true
		RETURNING initialized_at IS NOT NULL, name
	`, name).Scan(&status.Initialized, &status.Name); err != nil {
		return Status{}, fmt.Errorf("rename installation: %w", err)
	}
	return status, nil
}

func (store *Store) Initialize(ctx context.Context, input InitializeInput) (AuthenticatedSession, error) {
	input.Username = normalizeUsername(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.InstanceName = strings.TrimSpace(input.InstanceName)
	if err := validateInstanceName(input.InstanceName); err != nil {
		return AuthenticatedSession{}, err
	}
	if err := validateIdentityInput(input.Username, input.DisplayName, input.Password); err != nil {
		return AuthenticatedSession{}, err
	}
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return AuthenticatedSession{}, err
	}
	token, digest, err := newSessionToken()
	if err != nil {
		return AuthenticatedSession{}, err
	}
	expiresAt := store.now().UTC().Add(sessionLifetime)

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AuthenticatedSession{}, fmt.Errorf("begin initialization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var initialized bool
	if err := tx.QueryRow(ctx, `
		SELECT initialized_at IS NOT NULL
		FROM cairnops_installation
		WHERE singleton = true
		FOR UPDATE
	`).Scan(&initialized); err != nil {
		return AuthenticatedSession{}, fmt.Errorf("lock installation: %w", err)
	}
	if initialized {
		return AuthenticatedSession{}, ErrAlreadyInitialized
	}

	principal := Principal{Username: input.Username, DisplayName: input.DisplayName, Role: "administrator"}
	if err := tx.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text
	`, principal.Username, principal.DisplayName, passwordHash, principal.Role).Scan(&principal.ID); err != nil {
		return AuthenticatedSession{}, fmt.Errorf("create administrator: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_sessions (user_id, token_digest, expires_at)
		VALUES ($1::uuid, $2, $3)
	`, principal.ID, digest[:], expiresAt); err != nil {
		return AuthenticatedSession{}, fmt.Errorf("create initial session: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_installation SET initialized_at = now(), name = $1 WHERE singleton = true
	`, input.InstanceName); err != nil {
		return AuthenticatedSession{}, fmt.Errorf("complete initialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AuthenticatedSession{}, fmt.Errorf("commit initialization: %w", err)
	}
	return AuthenticatedSession{Principal: principal, ExpiresAt: expiresAt, Token: token}, nil
}

func (store *Store) Login(ctx context.Context, input LoginInput) (AuthenticatedSession, error) {
	input.Username = normalizeUsername(input.Username)
	if !usernamePattern.MatchString(input.Username) || !utf8.ValidString(input.Password) || len(input.Password) < 1 || len(input.Password) > 128 {
		_, _ = verifyPassword(input.Password, dummyPasswordHash)
		return AuthenticatedSession{}, ErrInvalidCredentials
	}
	var principal Principal
	var passwordHash string
	// Un compte désactivé n'ouvre plus de session, et le vérifier ici plutôt
	// qu'après la comparaison lui vaut la même réponse qu'un compte absent.
	err := store.pool.QueryRow(ctx, `
		SELECT id::text, username, display_name, role, password_hash
		FROM cairnops_users
		WHERE lower(username) = $1 AND deactivated_at IS NULL
	`, input.Username).Scan(&principal.ID, &principal.Username, &principal.DisplayName, &principal.Role, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = verifyPassword(input.Password, dummyPasswordHash)
		return AuthenticatedSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthenticatedSession{}, fmt.Errorf("find user: %w", err)
	}
	valid, err := verifyPassword(input.Password, passwordHash)
	if err != nil {
		return AuthenticatedSession{}, fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return AuthenticatedSession{}, ErrInvalidCredentials
	}
	token, digest, err := newSessionToken()
	if err != nil {
		return AuthenticatedSession{}, err
	}
	expiresAt := store.now().UTC().Add(sessionLifetime)
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO cairnops_sessions (user_id, token_digest, expires_at)
		VALUES ($1::uuid, $2, $3)
	`, principal.ID, digest[:], expiresAt); err != nil {
		return AuthenticatedSession{}, fmt.Errorf("create session: %w", err)
	}
	return AuthenticatedSession{Principal: principal, ExpiresAt: expiresAt, Token: token}, nil
}

func (store *Store) Authenticate(ctx context.Context, token string) (Principal, error) {
	digest, err := sessionTokenDigest(token)
	if err != nil {
		return Principal{}, ErrInvalidSession
	}
	var principal Principal
	err = store.pool.QueryRow(ctx, `
		SELECT users.id::text, users.username, users.display_name, users.role
		FROM cairnops_sessions sessions
		JOIN cairnops_users users ON users.id = sessions.user_id
		WHERE sessions.token_digest = $1
		  AND sessions.revoked_at IS NULL
		  AND sessions.expires_at > now()
		  AND users.deactivated_at IS NULL
	`, digest[:]).Scan(&principal.ID, &principal.Username, &principal.DisplayName, &principal.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrInvalidSession
	}
	if err != nil {
		return Principal{}, fmt.Errorf("authenticate session: %w", err)
	}
	_, _ = store.pool.Exec(ctx, `
		UPDATE cairnops_sessions
		SET last_seen_at = now()
		WHERE token_digest = $1 AND last_seen_at < now() - interval '5 minutes'
	`, digest[:])
	return principal, nil
}

func (store *Store) Logout(ctx context.Context, token string) error {
	digest, err := sessionTokenDigest(token)
	if err != nil {
		return ErrInvalidSession
	}
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_sessions
		SET revoked_at = now()
		WHERE token_digest = $1 AND revoked_at IS NULL
	`, digest[:])
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrInvalidSession
	}
	return nil
}

func validateIdentityInput(username, displayName, password string) error {
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("%w: username must contain 3 to 64 lowercase letters, digits, dots, underscores or hyphens", ErrInvalidInput)
	}
	if err := validateDisplayName(displayName); err != nil {
		return err
	}
	return validatePassword(password)
}

// Le nom de l'instance est obligatoire à la mise en service : on ne pose pas
// une instance sans savoir laquelle on pose. Les installations antérieures,
// elles, gardent un nom vide jusqu'à ce qu'on les nomme.
func validateInstanceName(name string) error {
	if count := utf8.RuneCountInString(name); count < 1 || count > 80 {
		return fmt.Errorf("%w: instance name must contain between 1 and 80 characters", ErrInvalidInput)
	}
	return nil
}

func validateDisplayName(displayName string) error {
	if count := utf8.RuneCountInString(displayName); count < 1 || count > 100 {
		return fmt.Errorf("%w: display name must contain between 1 and 100 characters", ErrInvalidInput)
	}
	return nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func newSessionToken() (string, [sha256.Size]byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", [sha256.Size]byte{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	return token, sha256.Sum256([]byte(token)), nil
}

func sessionTokenDigest(token string) ([sha256.Size]byte, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return [sha256.Size]byte{}, ErrInvalidSession
	}
	return sha256.Sum256([]byte(token)), nil
}
