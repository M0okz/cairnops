package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/M0okz/cairnops/internal/checks"
	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrInvalidInput      = errors.New("invalid input")
	ErrHeartbeatNotFound = errors.New("heartbeat not found")
	ErrStructureBusy     = errors.New("target structure is being reconciled")
	// ErrIntegrationOwned protège ce qui appartient à une Intégration : son nom
	// et sa cadence viennent du produit distant, et prétendre les fixer ici
	// ferait diverger CairnOps au premier cycle de synchronisation.
	ErrIntegrationOwned = errors.New("source belongs to an integration")
)

type Target struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	CreatedAt           time.Time `json:"created_at"`
	ExternalSourceCount int       `json:"external_source_count"`
	Aliases             []string  `json:"aliases"`
	Sources             []Source  `json:"sources"`
}

type Source struct {
	ID                  string             `json:"id"`
	TargetID            string             `json:"target_id"`
	Name                string             `json:"name"`
	Kind                domain.SourceKind  `json:"kind"`
	Enabled             bool               `json:"enabled"`
	IntervalSeconds     int                `json:"interval_seconds"`
	TimeoutMilliseconds int                `json:"timeout_milliseconds"`
	FailureThreshold    int                `json:"failure_threshold"`
	RecoveryThreshold   int                `json:"recovery_threshold"`
	Severity            incidents.Severity `json:"severity"`
	LastSignalAt        *time.Time         `json:"last_signal_at,omitempty"`
	LastObservedAt      *time.Time         `json:"last_observed_at,omitempty"`
	LatestOutcome       *domain.Outcome    `json:"latest_outcome,omitempty"`
}

type Observation struct {
	ID                  int64          `json:"id"`
	SourceID            string         `json:"source_id"`
	SourceName          string         `json:"source_name"`
	ObservedAt          time.Time      `json:"observed_at"`
	Outcome             domain.Outcome `json:"outcome"`
	LatencyMilliseconds int            `json:"latency_milliseconds"`
	Reason              string         `json:"reason,omitempty"`
	Message             string         `json:"message,omitempty"`
	Details             map[string]any `json:"details"`
}

type CreateTargetInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type UpdateTargetInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateSourceInput modifie un Contrôle natif. Un champ absent reste inchangé,
// ce qui permet de suspendre une sonde sans réémettre toute sa configuration.
type UpdateSourceInput struct {
	Name                *string             `json:"name,omitempty"`
	Enabled             *bool               `json:"enabled,omitempty"`
	IntervalSeconds     *int                `json:"interval_seconds,omitempty"`
	TimeoutMilliseconds *int                `json:"timeout_milliseconds,omitempty"`
	FailureThreshold    *int                `json:"failure_threshold,omitempty"`
	RecoveryThreshold   *int                `json:"recovery_threshold,omitempty"`
	Severity            *incidents.Severity `json:"severity,omitempty"`
	Config              json.RawMessage     `json:"config,omitempty"`
}

type CreateSourceInput struct {
	Name                string             `json:"name"`
	Kind                domain.SourceKind  `json:"kind"`
	IntervalSeconds     int                `json:"interval_seconds"`
	TimeoutMilliseconds int                `json:"timeout_milliseconds"`
	FailureThreshold    int                `json:"failure_threshold,omitempty"`
	RecoveryThreshold   int                `json:"recovery_threshold,omitempty"`
	Severity            incidents.Severity `json:"severity,omitempty"`
	Config              json.RawMessage    `json:"config"`
}

type CreatedSource struct {
	Source         Source `json:"source"`
	HeartbeatToken string `json:"heartbeat_token,omitempty"`
	HeartbeatPath  string `json:"heartbeat_path,omitempty"`
}

type HeartbeatPayload struct {
	Status               string `json:"status,omitempty"`
	DurationMilliseconds *int   `json:"duration_milliseconds,omitempty"`
	Message              string `json:"message,omitempty"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func targetStructureBusy(ctx context.Context, query queryRower, targetID string) (bool, error) {
	var busy bool
	err := query.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM cairnops_target_reconciliation_operations operation
			WHERE operation.status IN ('queued', 'running')
			  AND $1::uuid IN (operation.primary_target_id, operation.secondary_target_id)
		)
	`, targetID).Scan(&busy)
	return busy, err
}

func (store *Store) ListTargets(ctx context.Context) ([]Target, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT target.id::text, target.name, target.description, target.created_at,
		       count(binding.id)::integer,
		       coalesce((SELECT array_agg(alias.alias ORDER BY lower(alias.alias), alias.id)
		                 FROM cairnops_target_aliases alias WHERE alias.target_id = target.id), ARRAY[]::text[])
		FROM cairnops_targets target
		LEFT JOIN cairnops_connector_bindings binding ON binding.target_id = target.id
		WHERE target.archived_at IS NULL
		GROUP BY target.id
		ORDER BY lower(target.name), target.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	defer rows.Close()

	targets := make([]Target, 0)
	indexes := make(map[string]int)
	for rows.Next() {
		var target Target
		if err := rows.Scan(&target.ID, &target.Name, &target.Description, &target.CreatedAt, &target.ExternalSourceCount, &target.Aliases); err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		target.Sources = make([]Source, 0)
		indexes[target.ID] = len(targets)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate targets: %w", err)
	}
	if len(targets) == 0 {
		return targets, nil
	}

	sourceRows, err := store.pool.Query(ctx, `
		SELECT source.id::text, source.target_id::text, source.name, source.kind,
		       source.enabled, source.interval_seconds, source.timeout_milliseconds,
		       source.failure_threshold, source.recovery_threshold, source.severity,
		       source.last_signal_at, source.last_observed_at, latest.outcome
		FROM cairnops_signal_sources source
		JOIN cairnops_targets target ON target.id = source.target_id
		LEFT JOIN LATERAL (
			SELECT observation.outcome
			FROM cairnops_observations observation
			WHERE observation.source_id = source.id
			ORDER BY observation.observed_at DESC, observation.id DESC
			LIMIT 1
		) latest ON true
		WHERE target.archived_at IS NULL AND source.origin = 'native'
		ORDER BY lower(source.name), source.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer sourceRows.Close()
	for sourceRows.Next() {
		source, err := scanSource(sourceRows)
		if err != nil {
			return nil, err
		}
		if index, ok := indexes[source.TargetID]; ok {
			targets[index].Sources = append(targets[index].Sources, source)
		}
	}
	if err := sourceRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sources: %w", err)
	}
	return targets, nil
}

func (store *Store) CreateTarget(ctx context.Context, input CreateTargetInput) (Target, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 160 {
		return Target{}, fmt.Errorf("%w: target name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	if utf8.RuneCountInString(input.Description) > 2000 {
		return Target{}, fmt.Errorf("%w: target description must not exceed 2000 characters", ErrInvalidInput)
	}

	target := Target{Name: input.Name, Description: input.Description, Aliases: make([]string, 0), Sources: make([]Source, 0)}
	err := store.pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name, description, identity_managed_at)
		VALUES ($1, $2, now())
		RETURNING id::text, created_at
	`, target.Name, target.Description).Scan(&target.ID, &target.CreatedAt)
	if err != nil {
		return Target{}, fmt.Errorf("create target: %w", err)
	}
	return target, nil
}

// UpdateTarget corrige le nom et la description d'une Cible. Son identité, son
// historique et ses Sources ne bougent pas : c'est la même chose qu'on nomme
// autrement.
func (store *Store) UpdateTarget(ctx context.Context, targetID string, input UpdateTargetInput) (Target, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 160 {
		return Target{}, fmt.Errorf("%w: target name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	if utf8.RuneCountInString(input.Description) > 2000 {
		return Target{}, fmt.Errorf("%w: target description must not exceed 2000 characters", ErrInvalidInput)
	}
	if busy, err := targetStructureBusy(ctx, store.pool, targetID); err != nil {
		return Target{}, fmt.Errorf("check target reconciliation: %w", err)
	} else if busy {
		return Target{}, ErrStructureBusy
	}

	target := Target{ID: targetID, Name: input.Name, Description: input.Description, Aliases: make([]string, 0), Sources: make([]Source, 0)}
	err := store.pool.QueryRow(ctx, `
		UPDATE cairnops_targets
		SET name = $2, description = $3, identity_managed_at = now(), updated_at = now()
		WHERE id = $1::uuid AND archived_at IS NULL
		RETURNING created_at
	`, targetID, target.Name, target.Description).Scan(&target.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Target{}, ErrNotFound
	}
	if err != nil {
		return Target{}, fmt.Errorf("update target: %w", err)
	}
	return target, nil
}

// ArchiveTarget retire une Cible de l'Espace opérationnel sans effacer son
// passé. Ses Preuves actives sont refermées avec leur raison au Journal ; leur
// Incident suit ensuite sa règle normale de fermeture de Propagation. Plus
// aucun signal ne ressuscite la Cible tant qu'elle n'est pas restaurée.
func (store *Store) ArchiveTarget(ctx context.Context, targetID string) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin target archival: %w", err)
	}
	defer tx.Rollback(ctx)
	if busy, err := targetStructureBusy(ctx, tx, targetID); err != nil {
		return fmt.Errorf("check target reconciliation: %w", err)
	} else if busy {
		return ErrStructureBusy
	}

	var name string
	err = tx.QueryRow(ctx, `
		UPDATE cairnops_targets
		SET archived_at = now(), updated_at = now()
		WHERE id = $1::uuid AND archived_at IS NULL
		RETURNING name
	`, targetID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive target: %w", err)
	}
	if err := incidents.ResolveForArchivedTarget(ctx, tx, targetID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit target archival: %w", err)
	}
	return nil
}

// RestoreTarget remet une Cible en service. Ses Contrôles redeviennent dus et
// ses Sources d'Intégration reprennent au cycle suivant ; aucun Incident n'est
// rouvert pour autant, faute de preuve fraîche.
func (store *Store) RestoreTarget(ctx context.Context, targetID string) (Target, error) {
	var target Target
	err := store.pool.QueryRow(ctx, `
		UPDATE cairnops_targets
		SET archived_at = NULL, updated_at = now()
		WHERE id = $1::uuid AND archived_at IS NOT NULL AND reconciled_into_target_id IS NULL
		RETURNING id::text, name, description, created_at
	`, targetID).Scan(&target.ID, &target.Name, &target.Description, &target.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Target{}, ErrNotFound
	}
	if err != nil {
		return Target{}, fmt.Errorf("restore target: %w", err)
	}
	target.Sources = make([]Source, 0)
	target.Aliases = make([]string, 0)
	return target, nil
}

func (store *Store) CreateSource(ctx context.Context, targetID string, input CreateSourceInput) (CreatedSource, error) {
	input.Name = strings.TrimSpace(input.Name)
	if utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 160 {
		return CreatedSource{}, fmt.Errorf("%w: source name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	if !input.Kind.Valid() {
		return CreatedSource{}, fmt.Errorf("%w: unsupported source kind %q", ErrInvalidInput, input.Kind)
	}
	if input.IntervalSeconds < int(domain.MinimumInterval.Seconds()) || input.IntervalSeconds > int(domain.MaximumInterval.Seconds()) {
		return CreatedSource{}, fmt.Errorf("%w: interval must be between 20 and 86400 seconds", ErrInvalidInput)
	}
	if input.TimeoutMilliseconds < 100 || input.TimeoutMilliseconds > 60000 || input.TimeoutMilliseconds > input.IntervalSeconds*1000 {
		return CreatedSource{}, fmt.Errorf("%w: timeout must be between 100 and 60000 milliseconds and no longer than the interval", ErrInvalidInput)
	}
	if input.FailureThreshold == 0 {
		input.FailureThreshold = domain.DefaultFailureThreshold
	}
	if input.RecoveryThreshold == 0 {
		input.RecoveryThreshold = domain.DefaultRecoveryThreshold
	}
	policy := domain.TriggerPolicy{FailureThreshold: input.FailureThreshold, RecoveryThreshold: input.RecoveryThreshold}
	if err := policy.Validate(); err != nil {
		return CreatedSource{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if input.Severity == "" {
		input.Severity = incidents.SeverityMajor
	}
	if !validSeverity(input.Severity) {
		return CreatedSource{}, fmt.Errorf("%w: severity must be information, warning, major, or critical", ErrInvalidInput)
	}
	if err := checks.ValidateConfig(input.Kind, input.Config); err != nil {
		return CreatedSource{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if input.Kind == domain.SourceHeartbeat {
		var config checks.HeartbeatConfig
		if err := json.Unmarshal(input.Config, &config); err != nil {
			return CreatedSource{}, fmt.Errorf("%w: invalid heartbeat config", ErrInvalidInput)
		}
		config.Activated = false
		input.Config, _ = json.Marshal(config)
	}

	var exists bool
	if err := store.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM cairnops_targets WHERE id = $1::uuid AND archived_at IS NULL)", targetID).Scan(&exists); err != nil {
		return CreatedSource{}, fmt.Errorf("find target: %w", err)
	}
	if !exists {
		return CreatedSource{}, ErrNotFound
	}
	if busy, err := targetStructureBusy(ctx, store.pool, targetID); err != nil {
		return CreatedSource{}, fmt.Errorf("check target reconciliation: %w", err)
	} else if busy {
		return CreatedSource{}, ErrStructureBusy
	}

	created := CreatedSource{Source: Source{
		TargetID: targetID, Name: input.Name, Kind: input.Kind, Enabled: true,
		IntervalSeconds: input.IntervalSeconds, TimeoutMilliseconds: input.TimeoutMilliseconds,
		FailureThreshold: input.FailureThreshold, RecoveryThreshold: input.RecoveryThreshold,
		Severity: input.Severity,
	}}
	var digest []byte
	if input.Kind == domain.SourceHeartbeat {
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			return CreatedSource{}, fmt.Errorf("generate heartbeat token: %w", err)
		}
		created.HeartbeatToken = base64.RawURLEncoding.EncodeToString(tokenBytes)
		created.HeartbeatPath = "/api/v1/heartbeat/" + created.HeartbeatToken
		hashed := sha256.Sum256([]byte(created.HeartbeatToken))
		digest = hashed[:]
	}

	err := store.pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, interval_seconds, timeout_milliseconds,
			config, heartbeat_token_digest,
			failure_threshold, recovery_threshold, severity
		) VALUES ($1::uuid, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)
		RETURNING id::text
	`, targetID, input.Name, input.Kind, input.IntervalSeconds,
		input.TimeoutMilliseconds, input.Config, digest,
		input.FailureThreshold, input.RecoveryThreshold, input.Severity).Scan(&created.Source.ID)
	if err != nil {
		return CreatedSource{}, fmt.Errorf("create source: %w", err)
	}
	return created, nil
}

// UpdateSource modifie un Contrôle natif, configuration comprise : corriger une
// URL ne doit obliger ni à recréer la Source ni à perdre ses Observations.
//
// Changer la cadence change du même coup ce que la Couverture attend, sans
// réécrire les heures déjà consolidées. Une Source d'Intégration est refusée :
// elle appartient au produit distant.
func (store *Store) UpdateSource(ctx context.Context, sourceID string, input UpdateSourceInput) (Source, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Source{}, fmt.Errorf("begin source update: %w", err)
	}
	defer tx.Rollback(ctx)

	var current Source
	var kind, origin string
	var config []byte
	err = tx.QueryRow(ctx, `
		SELECT source.id::text, source.target_id::text, source.name, source.kind, source.origin,
		       source.enabled, source.interval_seconds, source.timeout_milliseconds,
		       source.failure_threshold, source.recovery_threshold, source.severity, source.config
		FROM cairnops_signal_sources source
		JOIN cairnops_targets target ON target.id = source.target_id
		WHERE source.id = $1::uuid AND target.archived_at IS NULL
		FOR UPDATE OF source
	`, sourceID).Scan(
		&current.ID, &current.TargetID, &current.Name, &kind, &origin,
		&current.Enabled, &current.IntervalSeconds, &current.TimeoutMilliseconds,
		&current.FailureThreshold, &current.RecoveryThreshold, &current.Severity, &config,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	if err != nil {
		return Source{}, fmt.Errorf("load source to update: %w", err)
	}
	if origin != "native" {
		return Source{}, ErrIntegrationOwned
	}
	if busy, err := targetStructureBusy(ctx, tx, current.TargetID); err != nil {
		return Source{}, fmt.Errorf("check target reconciliation: %w", err)
	} else if busy {
		return Source{}, ErrStructureBusy
	}
	current.Kind = domain.SourceKind(kind)

	if input.Name != nil {
		current.Name = strings.TrimSpace(*input.Name)
	}
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	if input.IntervalSeconds != nil {
		current.IntervalSeconds = *input.IntervalSeconds
	}
	if input.TimeoutMilliseconds != nil {
		current.TimeoutMilliseconds = *input.TimeoutMilliseconds
	}
	if input.FailureThreshold != nil {
		current.FailureThreshold = *input.FailureThreshold
	}
	if input.RecoveryThreshold != nil {
		current.RecoveryThreshold = *input.RecoveryThreshold
	}
	if input.Severity != nil {
		current.Severity = *input.Severity
	}
	stored := config
	if input.Config != nil {
		config = input.Config
	}

	if utf8.RuneCountInString(current.Name) < 1 || utf8.RuneCountInString(current.Name) > 160 {
		return Source{}, fmt.Errorf("%w: source name must contain between 1 and 160 characters", ErrInvalidInput)
	}
	if current.IntervalSeconds < int(domain.MinimumInterval.Seconds()) || current.IntervalSeconds > int(domain.MaximumInterval.Seconds()) {
		return Source{}, fmt.Errorf("%w: interval must be between 20 and 86400 seconds", ErrInvalidInput)
	}
	if current.TimeoutMilliseconds < 100 || current.TimeoutMilliseconds > 60000 || current.TimeoutMilliseconds > current.IntervalSeconds*1000 {
		return Source{}, fmt.Errorf("%w: timeout must be between 100 and 60000 milliseconds and no longer than the interval", ErrInvalidInput)
	}
	policy := domain.TriggerPolicy{FailureThreshold: current.FailureThreshold, RecoveryThreshold: current.RecoveryThreshold}
	if err := policy.Validate(); err != nil {
		return Source{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if !validSeverity(current.Severity) {
		return Source{}, fmt.Errorf("%w: severity must be information, warning, major, or critical", ErrInvalidInput)
	}
	if err := checks.ValidateConfig(current.Kind, config); err != nil {
		return Source{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if current.Kind == domain.SourceHeartbeat {
		// L'activation d'un Heartbeat se gagne au premier signal reçu ; une
		// modification ne la décrète pas.
		var edited checks.HeartbeatConfig
		if err := json.Unmarshal(config, &edited); err != nil {
			return Source{}, fmt.Errorf("%w: invalid heartbeat config", ErrInvalidInput)
		}
		var previous checks.HeartbeatConfig
		if err := json.Unmarshal(stored, &previous); err == nil {
			edited.Activated = previous.Activated
		}
		if config, err = json.Marshal(edited); err != nil {
			return Source{}, fmt.Errorf("encode heartbeat config: %w", err)
		}
	}

	// Une cadence raccourcie doit s'appliquer tout de suite : la prochaine
	// échéance se recalcule depuis la dernière Observation, jamais depuis
	// l'ancienne échéance.
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_signal_sources
		SET name = $2, enabled = $3, interval_seconds = $4, timeout_milliseconds = $5,
		    failure_threshold = $6, recovery_threshold = $7, severity = $8, config = $9::jsonb,
		    next_run_at = least(next_run_at, coalesce(last_observed_at, now()) + make_interval(secs => $10)),
		    updated_at = now()
		WHERE id = $1::uuid
	`, sourceID, current.Name, current.Enabled, current.IntervalSeconds, current.TimeoutMilliseconds,
		current.FailureThreshold, current.RecoveryThreshold, current.Severity, config,
		float64(current.IntervalSeconds)); err != nil {
		return Source{}, fmt.Errorf("update source: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Source{}, fmt.Errorf("commit source update: %w", err)
	}
	return current, nil
}

// DeleteSource retire un Contrôle natif et les Observations qu'il portait : une
// preuve sans Source qui la porte ne s'interprète plus. Si cette suppression
// retire la dernière preuve active d'un Incident, celui-ci doit être résolu
// dans la même transaction plutôt que de rester affiché avec 0/0 Source.
func (store *Store) DeleteSource(ctx context.Context, sourceID string) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin source deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var origin, targetID string
	err = tx.QueryRow(ctx, `
		SELECT origin, target_id::text
		FROM cairnops_signal_sources
		WHERE id = $1::uuid
		FOR UPDATE
	`, sourceID).Scan(&origin, &targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find source to delete: %w", err)
	}
	if origin != "native" {
		return ErrIntegrationOwned
	}
	if busy, err := targetStructureBusy(ctx, tx, targetID); err != nil {
		return fmt.Errorf("check target reconciliation: %w", err)
	} else if busy {
		return ErrStructureBusy
	}

	if err := incidents.ResolveForSourceRemoval(ctx, tx, sourceID); err != nil {
		return fmt.Errorf("resolve evidence fed by source: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM cairnops_signal_sources WHERE id = $1::uuid`, sourceID); err != nil {
		return fmt.Errorf("delete source: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit source deletion: %w", err)
	}
	return nil
}

func (store *Store) ListObservations(ctx context.Context, targetID string, limit int) ([]Observation, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT observation.id, observation.source_id::text, source.name,
		       observation.observed_at, observation.outcome,
		       observation.latency_milliseconds, observation.reason,
		       observation.message, observation.details
		FROM cairnops_observations observation
		JOIN cairnops_signal_sources source ON source.id = observation.source_id
		WHERE observation.target_id = $1::uuid
		ORDER BY observation.observed_at DESC, observation.id DESC
		LIMIT $2
	`, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("list observations: %w", err)
	}
	defer rows.Close()

	observations := make([]Observation, 0)
	for rows.Next() {
		var observation Observation
		var details []byte
		if err := rows.Scan(
			&observation.ID, &observation.SourceID, &observation.SourceName,
			&observation.ObservedAt, &observation.Outcome,
			&observation.LatencyMilliseconds, &observation.Reason,
			&observation.Message, &details,
		); err != nil {
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		if err := json.Unmarshal(details, &observation.Details); err != nil {
			return nil, fmt.Errorf("decode observation details: %w", err)
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observations: %w", err)
	}
	return observations, nil
}

func (store *Store) ReceiveHeartbeat(ctx context.Context, token string, payload HeartbeatPayload) (Observation, error) {
	if err := validateHeartbeatPayload(payload); err != nil {
		return Observation{}, err
	}
	digest := sha256.Sum256([]byte(token))
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Observation{}, fmt.Errorf("begin heartbeat transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var sourceID, targetID, sourceName string
	var observedAt time.Time
	err = tx.QueryRow(ctx, `
		UPDATE cairnops_signal_sources
		SET last_signal_at = now(),
		    last_signal_outcome = $2,
		    last_observed_at = now(),
		    next_run_at = now() + make_interval(secs => interval_seconds),
		    config = jsonb_set(config, '{activated}', 'true'::jsonb, true),
		    updated_at = now()
		WHERE heartbeat_token_digest = $1 AND enabled AND kind = 'heartbeat'
		RETURNING id::text, target_id::text, name, last_signal_at
	`, digest[:], heartbeatOutcome(payload.Status)).Scan(&sourceID, &targetID, &sourceName, &observedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Observation{}, ErrHeartbeatNotFound
	}
	if err != nil {
		return Observation{}, fmt.Errorf("record heartbeat signal: %w", err)
	}

	observation := Observation{
		SourceID: sourceID, SourceName: sourceName, ObservedAt: observedAt,
		Outcome: domain.OutcomeHealthy, Details: make(map[string]any),
	}
	if payload.Status != "" {
		observation.Details["status"] = payload.Status
	}
	if payload.DurationMilliseconds != nil {
		observation.Details["duration_milliseconds"] = *payload.DurationMilliseconds
	}
	if payload.Status == "failed" {
		observation.Outcome = domain.OutcomeUnhealthy
		observation.Reason = "heartbeat_reported_failure"
	}
	observation.Message = payload.Message
	details, _ := json.Marshal(observation.Details)
	err = tx.QueryRow(ctx, `
		INSERT INTO cairnops_observations (
			source_id, target_id, observed_at, outcome,
			latency_milliseconds, reason, message, details
		) VALUES ($1::uuid, $2::uuid, $3, $4, 0, $5, $6, $7::jsonb)
		RETURNING id
	`, sourceID, targetID, observedAt, observation.Outcome,
		observation.Reason, observation.Message, details).Scan(&observation.ID)
	if err != nil {
		return Observation{}, fmt.Errorf("insert heartbeat observation: %w", err)
	}
	if err := incidents.ApplyNativeObservation(ctx, tx, incidents.NativeObservation{
		SourceID: sourceID, TargetID: targetID, SourceName: sourceName,
		Outcome: observation.Outcome, ObservedAt: observedAt,
		Reason: observation.Reason, Message: observation.Message,
	}); err != nil {
		return Observation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Observation{}, fmt.Errorf("commit heartbeat: %w", err)
	}
	return observation, nil
}

func validSeverity(severity incidents.Severity) bool {
	switch severity {
	case incidents.SeverityInformation, incidents.SeverityWarning,
		incidents.SeverityMajor, incidents.SeverityCritical:
		return true
	default:
		return false
	}
}

func heartbeatOutcome(status string) domain.Outcome {
	if status == "failed" {
		return domain.OutcomeUnhealthy
	}
	return domain.OutcomeHealthy
}

func validateHeartbeatPayload(payload HeartbeatPayload) error {
	if payload.Status != "" && payload.Status != "ok" && payload.Status != "failed" {
		return fmt.Errorf("%w: heartbeat status must be ok or failed", ErrInvalidInput)
	}
	if payload.DurationMilliseconds != nil && (*payload.DurationMilliseconds < 0 || *payload.DurationMilliseconds > 86400000) {
		return fmt.Errorf("%w: heartbeat duration must be between 0 and 86400000 milliseconds", ErrInvalidInput)
	}
	if utf8.RuneCountInString(payload.Message) > 500 {
		return fmt.Errorf("%w: heartbeat message must not exceed 500 characters", ErrInvalidInput)
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanSource(row rowScanner) (Source, error) {
	var source Source
	var kind string
	var outcome *string
	if err := row.Scan(
		&source.ID, &source.TargetID, &source.Name, &kind,
		&source.Enabled, &source.IntervalSeconds, &source.TimeoutMilliseconds,
		&source.FailureThreshold, &source.RecoveryThreshold, &source.Severity,
		&source.LastSignalAt, &source.LastObservedAt, &outcome,
	); err != nil {
		return Source{}, fmt.Errorf("scan source: %w", err)
	}
	source.Kind = domain.SourceKind(kind)
	if outcome != nil {
		value := domain.Outcome(*outcome)
		source.LatestOutcome = &value
	}
	return source, nil
}
