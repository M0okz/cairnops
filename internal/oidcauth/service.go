package oidcauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/secretbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	clientSecretPurpose = "cairnops-oidc-client-secret"
	refreshTokenPurpose = "cairnops-oidc-refresh-token"
	flowVerifierPurpose = "cairnops-oidc-code-verifier"
	flowLifetime        = 10 * time.Minute
	maximumLabelLength  = 80
	maximumClientLength = 255
)

type Service struct {
	pool       *pgxpool.Pool
	secrets    *secretbox.Box
	sessions   SessionIssuer
	publicURL  string
	httpClient *http.Client
	now        func() time.Time
}

func NewService(pool *pgxpool.Pool, secrets *secretbox.Box, sessions SessionIssuer, publicURL string, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Service{
		pool: pool, secrets: secrets, sessions: sessions,
		publicURL: strings.TrimSuffix(publicURL, "/"), httpClient: client, now: time.Now,
	}
}

func (service *Service) PublicStatus(ctx context.Context) (PublicStatus, error) {
	var status PublicStatus
	err := service.pool.QueryRow(ctx, `
		SELECT true, label FROM cairnops_oidc_configurations WHERE state = 'active'
	`).Scan(&status.Enabled, &status.Label)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicStatus{}, nil
	}
	if err != nil {
		return PublicStatus{}, fmt.Errorf("read active OIDC status: %w", err)
	}
	return status, nil
}

func (service *Service) Configurations(ctx context.Context) (ConfigurationSet, error) {
	rows, err := service.pool.Query(ctx, `
		SELECT id::text, state, label, issuer, client_id, client_secret_sealed,
		       groups_claim, administrator_groups, operator_groups, observer_groups,
		       tested_at, activated_at, created_at, updated_at
		FROM cairnops_oidc_configurations
		WHERE state IN ('active', 'draft')
		ORDER BY state
	`)
	if err != nil {
		return ConfigurationSet{}, fmt.Errorf("list OIDC configurations: %w", err)
	}
	defer rows.Close()

	var set ConfigurationSet
	for rows.Next() {
		configuration, err := scanConfiguration(rows)
		if err != nil {
			return ConfigurationSet{}, fmt.Errorf("scan OIDC configuration: %w", err)
		}
		if configuration.State == "active" {
			set.Active = &configuration
		} else {
			set.Draft = &configuration
		}
	}
	return set, rows.Err()
}

func (service *Service) SaveDraft(ctx context.Context, actorID string, input ConfigurationInput) (Configuration, error) {
	normalized, err := normalizeConfiguration(input)
	if err != nil {
		return Configuration{}, err
	}

	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Configuration{}, fmt.Errorf("begin OIDC draft: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('cairnops-oidc-configuration'))`); err != nil {
		return Configuration{}, fmt.Errorf("lock OIDC configuration: %w", err)
	}
	if err := ensureIssuerCanChange(ctx, tx, normalized.Issuer); err != nil {
		return Configuration{}, err
	}

	sealedSecret := ""
	if normalized.ClientSecret != "" {
		sealedSecret, err = service.secrets.Seal([]byte(normalized.ClientSecret), clientSecretPurpose)
		if err != nil {
			return Configuration{}, fmt.Errorf("seal OIDC client secret: %w", err)
		}
	} else {
		err = tx.QueryRow(ctx, `
			SELECT client_secret_sealed
			FROM cairnops_oidc_configurations
			WHERE state IN ('draft', 'active')
			ORDER BY state = 'draft' DESC
			LIMIT 1
		`).Scan(&sealedSecret)
		if errors.Is(err, pgx.ErrNoRows) {
			return Configuration{}, fmt.Errorf("%w: le secret client est obligatoire", ErrInvalidInput)
		}
		if err != nil {
			return Configuration{}, fmt.Errorf("read prior OIDC client secret: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_oidc_configurations SET state = 'retired', updated_at = now()
		WHERE state = 'draft'
	`); err != nil {
		return Configuration{}, fmt.Errorf("retire prior OIDC draft: %w", err)
	}

	var configuration Configuration
	row := tx.QueryRow(ctx, `
		INSERT INTO cairnops_oidc_configurations (
			state, label, issuer, client_id, client_secret_sealed, groups_claim,
			administrator_groups, operator_groups, observer_groups, created_by
		) VALUES ('draft', $1, $2, $3, $4, $5, $6, $7, $8, $9::uuid)
		RETURNING id::text, state, label, issuer, client_id, client_secret_sealed,
		          groups_claim, administrator_groups, operator_groups, observer_groups,
		          tested_at, activated_at, created_at, updated_at
	`, normalized.Label, normalized.Issuer, normalized.ClientID, sealedSecret, normalized.GroupsClaim,
		normalized.Groups.Administrator, normalized.Groups.Operator, normalized.Groups.Observer, actorID)
	configuration, err = scanConfiguration(row)
	if err != nil {
		return Configuration{}, fmt.Errorf("save OIDC draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Configuration{}, fmt.Errorf("commit OIDC draft: %w", err)
	}
	return configuration, nil
}

func (service *Service) Activate(ctx context.Context) (Configuration, error) {
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Configuration{}, fmt.Errorf("begin OIDC activation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('cairnops-oidc-configuration'))`); err != nil {
		return Configuration{}, fmt.Errorf("lock OIDC configuration: %w", err)
	}

	var draftID, issuer string
	var testedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id::text, issuer, tested_at
		FROM cairnops_oidc_configurations WHERE state = 'draft' FOR UPDATE
	`).Scan(&draftID, &issuer, &testedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, ErrNotConfigured
	}
	if err != nil {
		return Configuration{}, fmt.Errorf("lock OIDC draft: %w", err)
	}
	if testedAt == nil {
		return Configuration{}, fmt.Errorf("%w: le brouillon doit réussir son test interactif", ErrConflict)
	}
	if err := ensureIssuerCanChange(ctx, tx, issuer); err != nil {
		return Configuration{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_oidc_configurations
		SET state = 'retired', activated_at = NULL, updated_at = now()
		WHERE state = 'active'
	`); err != nil {
		return Configuration{}, fmt.Errorf("retire active OIDC configuration: %w", err)
	}

	configuration, err := scanConfiguration(tx.QueryRow(ctx, `
		UPDATE cairnops_oidc_configurations
		SET state = 'active', activated_at = now(), updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, state, label, issuer, client_id, client_secret_sealed,
		          groups_claim, administrator_groups, operator_groups, observer_groups,
		          tested_at, activated_at, created_at, updated_at
	`, draftID))
	if err != nil {
		return Configuration{}, fmt.Errorf("activate OIDC configuration: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Configuration{}, fmt.Errorf("commit OIDC activation: %w", err)
	}
	return configuration, nil
}

func ensureIssuerCanChange(ctx context.Context, tx pgx.Tx, issuer string) error {
	var locked bool
	var activeIssuer string
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM cairnops_oidc_identities),
		       coalesce((SELECT issuer FROM cairnops_oidc_configurations WHERE state = 'active'), '')
	`).Scan(&locked, &activeIssuer)
	if err != nil {
		return fmt.Errorf("read OIDC issuer lock: %w", err)
	}
	if locked && activeIssuer != issuer {
		return fmt.Errorf("%w: l’issuer est immuable dès qu’un Utilisateur externe existe", ErrConflict)
	}
	return nil
}

func scanConfiguration(row interface{ Scan(...any) error }) (Configuration, error) {
	var configuration Configuration
	err := row.Scan(
		&configuration.ID, &configuration.State, &configuration.Label,
		&configuration.Issuer, &configuration.ClientID, &configuration.clientSecretSealed,
		&configuration.GroupsClaim, &configuration.Groups.Administrator,
		&configuration.Groups.Operator, &configuration.Groups.Observer,
		&configuration.TestedAt, &configuration.ActivatedAt,
		&configuration.CreatedAt, &configuration.UpdatedAt,
	)
	configuration.ClientSecretConfigured = configuration.clientSecretSealed != ""
	return configuration, err
}

func normalizeConfiguration(input ConfigurationInput) (ConfigurationInput, error) {
	input.Label = strings.TrimSpace(input.Label)
	input.Issuer = strings.TrimSpace(input.Issuer)
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.GroupsClaim = strings.TrimSpace(input.GroupsClaim)
	if input.GroupsClaim == "" {
		input.GroupsClaim = "groups"
	}
	if utf8.RuneCountInString(input.Label) < 1 || utf8.RuneCountInString(input.Label) > maximumLabelLength {
		return ConfigurationInput{}, fmt.Errorf("%w: le nom doit contenir entre 1 et 80 caractères", ErrInvalidInput)
	}
	if len(input.ClientID) < 1 || len(input.ClientID) > maximumClientLength {
		return ConfigurationInput{}, fmt.Errorf("%w: le client ID doit contenir entre 1 et 255 caractères", ErrInvalidInput)
	}
	if len(input.ClientSecret) > 4096 {
		return ConfigurationInput{}, fmt.Errorf("%w: le secret client est trop long", ErrInvalidInput)
	}
	if err := validateIssuer(input.Issuer); err != nil {
		return ConfigurationInput{}, err
	}
	if !claimNameValid(input.GroupsClaim) {
		return ConfigurationInput{}, fmt.Errorf("%w: le nom du claim de groupes est invalide", ErrInvalidInput)
	}
	var err error
	if input.Groups.Administrator, err = normalizeGroups(input.Groups.Administrator); err != nil {
		return ConfigurationInput{}, err
	}
	if input.Groups.Operator, err = normalizeGroups(input.Groups.Operator); err != nil {
		return ConfigurationInput{}, err
	}
	if input.Groups.Observer, err = normalizeGroups(input.Groups.Observer); err != nil {
		return ConfigurationInput{}, err
	}
	if len(input.Groups.Administrator)+len(input.Groups.Operator)+len(input.Groups.Observer) == 0 {
		return ConfigurationInput{}, fmt.Errorf("%w: au moins un groupe doit être associé à un rôle", ErrInvalidInput)
	}
	return input, nil
}

func validateIssuer(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return fmt.Errorf("%w: l’issuer doit être une URL absolue sans identifiants, requête ni fragment", ErrInvalidInput)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback()) {
		return nil
	}
	return fmt.Errorf("%w: l’issuer doit utiliser HTTPS, sauf sur localhost", ErrInvalidInput)
}

func claimNameValid(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' || (index > 0 && ((character >= '0' && character <= '9') || character == '.' || character == '-')) {
			continue
		}
		return false
	}
	return true
}

func normalizeGroups(groups []string) ([]string, error) {
	seen := make(map[string]struct{}, len(groups))
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		if group == "" || strings.TrimSpace(group) != group || len(group) > 512 || !utf8.ValidString(group) {
			return nil, fmt.Errorf("%w: chaque identifiant de groupe doit être une chaîne exacte non vide", ErrInvalidInput)
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}
	slices.Sort(normalized)
	return normalized, nil
}

func randomValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate OIDC random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func externalUsername(issuer, subject string) string {
	digest := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return "oidc-" + hex.EncodeToString(digest[:12])
}

func displayName(claims map[string]json.RawMessage, fallback string) string {
	for _, name := range []string{"name", "preferred_username"} {
		var value string
		if raw, exists := claims[name]; exists && json.Unmarshal(raw, &value) == nil {
			value = strings.TrimSpace(value)
			if utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 100 {
				return value
			}
		}
	}
	if value := strings.TrimSpace(fallback); utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 100 {
		return value
	}
	return "Utilisateur OIDC"
}

func groupsAndRole(claims map[string]json.RawMessage, configuration Configuration) ([]string, string, error) {
	raw, exists := claims[configuration.GroupsClaim]
	if !exists {
		return nil, "", fmt.Errorf("%w: le claim %q est absent", ErrNotAuthorized, configuration.GroupsClaim)
	}
	var groups []string
	if err := json.Unmarshal(raw, &groups); err != nil || groups == nil {
		return nil, "", fmt.Errorf("%w: le claim %q doit être un tableau de chaînes", ErrNotAuthorized, configuration.GroupsClaim)
	}
	for _, group := range groups {
		if group == "" {
			return nil, "", fmt.Errorf("%w: le claim %q contient un groupe vide", ErrNotAuthorized, configuration.GroupsClaim)
		}
	}
	role := roleForGroups(groups, configuration.Groups)
	if role == "" {
		return groups, "", fmt.Errorf("%w: aucun groupe ne donne accès à CairnOps", ErrNotAuthorized)
	}
	return groups, role, nil
}

func roleForGroups(groups []string, mappings GroupMappings) string {
	present := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		present[group] = struct{}{}
	}
	for _, candidate := range []struct {
		role   string
		groups []string
	}{
		{"administrator", mappings.Administrator},
		{"operator", mappings.Operator},
		{"observer", mappings.Observer},
	} {
		for _, group := range candidate.groups {
			if _, exists := present[group]; exists {
				return candidate.role
			}
		}
	}
	return ""
}
