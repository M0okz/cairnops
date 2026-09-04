package indicators

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type Remote struct {
	ID               string
	Kind             string
	Endpoint         string
	CredentialSealed string
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

func (store *Store) Remote(ctx context.Context, connectorID string) (Remote, error) {
	var remote Remote
	if err := store.pool.QueryRow(ctx, `
		SELECT id::text, kind, endpoint, credential_sealed
		FROM cairnops_connectors WHERE id = $1::uuid AND status <> 'disabled'
	`, connectorID).Scan(&remote.ID, &remote.Kind, &remote.Endpoint, &remote.CredentialSealed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Remote{}, ErrNotFound
		}
		return Remote{}, fmt.Errorf("load indicator remote: %w", err)
	}
	return remote, nil
}

func (store *Store) Configuration(ctx context.Context, connectorID string) (Configuration, error) {
	configuration := Configuration{ConnectorID: connectorID, GeneratedAt: store.now().UTC(), Capabilities: []Capability{}, Bindings: []Binding{}, Profiles: []Profile{}, Activity: []Activity{}}
	if err := store.pool.QueryRow(ctx, `
		SELECT kind, name, endpoint FROM cairnops_connectors WHERE id = $1::uuid
	`, connectorID).Scan(&configuration.ConnectorKind, &configuration.ConnectorName, &configuration.Endpoint); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Configuration{}, ErrNotFound
		}
		return Configuration{}, fmt.Errorf("load indicator connector: %w", err)
	}
	rows, err := store.pool.Query(ctx, `
		SELECT capability, status, message, checked_at
		FROM cairnops_connector_capabilities WHERE connector_id = $1::uuid
		ORDER BY capability
	`, connectorID)
	if err != nil {
		return Configuration{}, fmt.Errorf("list connector capabilities: %w", err)
	}
	for rows.Next() {
		var capability Capability
		if err := rows.Scan(&capability.Key, &capability.Status, &capability.Message, &capability.CheckedAt); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("scan connector capability: %w", err)
		}
		configuration.Capabilities = append(configuration.Capabilities, capability)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Configuration{}, fmt.Errorf("iterate connector capabilities: %w", err)
	}
	rows.Close()

	rows, err = store.pool.Query(ctx, `
		SELECT binding.id::text, binding.target_id::text, target.name,
		       binding.external_id, binding.external_name, binding.indicators_enabled
		FROM cairnops_connector_bindings binding
		JOIN cairnops_targets target ON target.id = binding.target_id
		WHERE binding.connector_id = $1::uuid
		ORDER BY lower(binding.external_name), binding.external_id
	`, connectorID)
	if err != nil {
		return Configuration{}, fmt.Errorf("list indicator bindings: %w", err)
	}
	byBinding := make(map[string]int)
	for rows.Next() {
		var binding Binding
		if err := rows.Scan(&binding.ID, &binding.TargetID, &binding.TargetName, &binding.ExternalID, &binding.ExternalName, &binding.Enabled); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("scan indicator binding: %w", err)
		}
		binding.Imported = true
		binding.Indicators, binding.Candidates = []Indicator{}, []Candidate{}
		byBinding[binding.ID] = len(configuration.Bindings)
		configuration.Bindings = append(configuration.Bindings, binding)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Configuration{}, fmt.Errorf("iterate indicator bindings: %w", err)
	}
	rows.Close()

	rows, err = store.pool.Query(ctx, `
		SELECT id::text, connector_binding_id::text, target_id::text, semantic_key,
		       label, external_id, dimension, unit, enabled, metadata,
		       last_value, last_observed_at, last_error
		FROM cairnops_context_indicators
		WHERE connector_id = $1::uuid
		ORDER BY semantic_key, dimension, label
	`, connectorID)
	if err != nil {
		return Configuration{}, fmt.Errorf("list configured indicators: %w", err)
	}
	for rows.Next() {
		var indicator Indicator
		var metadata []byte
		if err := rows.Scan(&indicator.ID, &indicator.BindingID, &indicator.TargetID, &indicator.SemanticKey, &indicator.Label, &indicator.ExternalID, &indicator.Dimension, &indicator.Unit, &indicator.Enabled, &metadata, &indicator.LastValue, &indicator.LastObservedAt, &indicator.LastError); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("scan configured indicator: %w", err)
		}
		if err := json.Unmarshal(metadata, &indicator.Metadata); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("decode indicator metadata: %w", err)
		}
		if index, known := byBinding[indicator.BindingID]; known {
			configuration.Bindings[index].Indicators = append(configuration.Bindings[index].Indicators, indicator)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Configuration{}, fmt.Errorf("iterate configured indicators: %w", err)
	}
	rows.Close()

	rows, err = store.pool.Query(ctx, `
		SELECT id::text, name, specification, created_at, updated_at
		FROM cairnops_indicator_profiles WHERE connector_id = $1::uuid
		ORDER BY lower(name), id
	`, connectorID)
	if err != nil {
		return Configuration{}, fmt.Errorf("list indicator profiles: %w", err)
	}
	for rows.Next() {
		var profile Profile
		var specification []byte
		if err := rows.Scan(&profile.ID, &profile.Name, &specification, &profile.CreatedAt, &profile.UpdatedAt); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("scan indicator profile: %w", err)
		}
		if err := json.Unmarshal(specification, &profile.Specification); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("decode indicator profile: %w", err)
		}
		configuration.Profiles = append(configuration.Profiles, profile)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Configuration{}, fmt.Errorf("iterate indicator profiles: %w", err)
	}
	rows.Close()

	rows, err = store.pool.Query(ctx, `
		SELECT activity.id, coalesce(activity.actor_id::text, ''), coalesce(account.display_name, ''),
		       activity.summary, activity.data, activity.occurred_at
		FROM cairnops_connector_configuration_activity activity
		LEFT JOIN cairnops_users account ON account.id = activity.actor_id
		WHERE activity.connector_id = $1::uuid
		ORDER BY activity.occurred_at DESC, activity.id DESC LIMIT 30
	`, connectorID)
	if err != nil {
		return Configuration{}, fmt.Errorf("list connector configuration activity: %w", err)
	}
	for rows.Next() {
		var activity Activity
		var data []byte
		if err := rows.Scan(&activity.ID, &activity.ActorID, &activity.ActorName, &activity.Summary, &data, &activity.OccurredAt); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("scan connector configuration activity: %w", err)
		}
		if err := json.Unmarshal(data, &activity.Data); err != nil {
			rows.Close()
			return Configuration{}, fmt.Errorf("decode connector configuration activity: %w", err)
		}
		configuration.Activity = append(configuration.Activity, activity)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Configuration{}, fmt.Errorf("iterate connector configuration activity: %w", err)
	}
	rows.Close()
	return configuration, nil
}

func (store *Store) Apply(ctx context.Context, actorID, connectorID string, input ApplyInput) error {
	if err := validateApply(input); err != nil {
		return err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin indicator configuration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var connectorExists int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM cairnops_connectors WHERE id = $1::uuid FOR UPDATE`, connectorID).Scan(&connectorExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock indicator connector: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE cairnops_connector_bindings SET indicators_enabled = false, updated_at = now() WHERE connector_id = $1::uuid`, connectorID); err != nil {
		return fmt.Errorf("stage indicator configuration: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE cairnops_context_indicators SET enabled = false, updated_at = now() WHERE connector_id = $1::uuid`, connectorID); err != nil {
		return fmt.Errorf("stage indicator selections: %w", err)
	}

	for _, binding := range input.Bindings {
		bindingID, targetID, err := upsertBinding(ctx, tx, connectorID, binding)
		if err != nil {
			return err
		}
		for _, selection := range binding.Indicators {
			metadata, err := json.Marshal(selection.Metadata)
			if err != nil {
				return fmt.Errorf("encode indicator metadata: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO cairnops_context_indicators (
					connector_id, connector_binding_id, target_id, semantic_key, label,
					external_id, dimension, unit, enabled, metadata
				) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10::jsonb)
				ON CONFLICT (connector_binding_id, semantic_key, dimension) DO UPDATE SET
					target_id = EXCLUDED.target_id, label = EXCLUDED.label,
					external_id = EXCLUDED.external_id, unit = EXCLUDED.unit,
					enabled = EXCLUDED.enabled, metadata = EXCLUDED.metadata,
					last_error = CASE WHEN cairnops_context_indicators.external_id = EXCLUDED.external_id THEN cairnops_context_indicators.last_error ELSE '' END,
					last_value = CASE WHEN cairnops_context_indicators.external_id = EXCLUDED.external_id THEN cairnops_context_indicators.last_value ELSE NULL END,
					last_observed_at = CASE WHEN cairnops_context_indicators.external_id = EXCLUDED.external_id THEN cairnops_context_indicators.last_observed_at ELSE NULL END,
					updated_at = now()
			`, connectorID, bindingID, targetID, selection.SemanticKey, strings.TrimSpace(selection.Label), strings.TrimSpace(selection.ExternalID), strings.TrimSpace(selection.Dimension), selection.Unit, binding.Enabled, metadata); err != nil {
				return fmt.Errorf("save indicator selection: %w", err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE cairnops_indicator_profiles SET name = id::text WHERE connector_id = $1::uuid`, connectorID); err != nil {
		return fmt.Errorf("stage indicator profiles: %w", err)
	}
	keptProfileIDs := make([]string, 0, len(input.Profiles))
	for _, profile := range input.Profiles {
		name := strings.TrimSpace(profile.Name)
		specification, err := json.Marshal(profile.Specification)
		if err != nil {
			return fmt.Errorf("encode indicator profile: %w", err)
		}
		profileID := strings.TrimSpace(profile.ID)
		if profileID == "" {
			if err := tx.QueryRow(ctx, `INSERT INTO cairnops_indicator_profiles (connector_id, name, specification) VALUES ($1::uuid, $2, $3::jsonb) RETURNING id::text`, connectorID, name, specification).Scan(&profileID); err != nil {
				return fmt.Errorf("create indicator profile: %w", err)
			}
		} else {
			result, err := tx.Exec(ctx, `UPDATE cairnops_indicator_profiles SET name = $3, specification = $4::jsonb, updated_at = now() WHERE connector_id = $1::uuid AND id = $2::uuid`, connectorID, profileID, name, specification)
			if err != nil || result.RowsAffected() != 1 {
				return fmt.Errorf("%w: indicator profile is unavailable", ErrInvalidInput)
			}
		}
		keptProfileIDs = append(keptProfileIDs, profileID)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cairnops_indicator_profiles WHERE connector_id = $1::uuid AND NOT (id = ANY($2::uuid[]))`, connectorID, keptProfileIDs); err != nil {
		return fmt.Errorf("remove indicator profiles: %w", err)
	}
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Configuration enregistrée · %d périmètre(s), %d profil(s)", len(input.Bindings), len(input.Profiles))
	}
	data, _ := json.Marshal(map[string]any{"bindings": len(input.Bindings), "profiles": len(input.Profiles), "indicators": countSelections(input.Bindings)})
	if _, err := tx.Exec(ctx, `INSERT INTO cairnops_connector_configuration_activity (connector_id, actor_id, summary, data) VALUES ($1::uuid, $2::uuid, $3, $4::jsonb)`, connectorID, actorID, summary, data); err != nil {
		return fmt.Errorf("record connector configuration activity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit indicator configuration: %w", err)
	}
	return nil
}

func upsertBinding(ctx context.Context, tx pgx.Tx, connectorID string, binding BindingInput) (string, string, error) {
	targetID := strings.TrimSpace(binding.TargetID)
	var existingID, existingTargetID string
	err := tx.QueryRow(ctx, `SELECT id::text, target_id::text FROM cairnops_connector_bindings WHERE connector_id = $1::uuid AND external_id = $2 FOR UPDATE`, connectorID, binding.ExternalID).Scan(&existingID, &existingTargetID)
	if err == nil {
		if targetID != "" && targetID != existingTargetID {
			return "", "", fmt.Errorf("%w: the operational target mapping cannot be changed from indicator configuration", ErrInvalidInput)
		}
		if _, err := tx.Exec(ctx, `UPDATE cairnops_connector_bindings SET external_name = $2, indicators_enabled = $3, updated_at = now() WHERE id = $1::uuid`, existingID, strings.TrimSpace(binding.ExternalName), binding.Enabled); err != nil {
			return "", "", fmt.Errorf("update indicator binding: %w", err)
		}
		return existingID, existingTargetID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("resolve indicator binding: %w", err)
	}
	if targetID == "" {
		return "", "", fmt.Errorf("%w: a target is required for new binding %s", ErrInvalidInput, binding.ExternalID)
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT archived_at IS NULL FROM cairnops_targets WHERE id = $1::uuid`, targetID).Scan(&active); err != nil || !active {
		if errors.Is(err, pgx.ErrNoRows) || !active {
			return "", "", fmt.Errorf("%w: target is unavailable", ErrInvalidInput)
		}
		return "", "", fmt.Errorf("validate indicator target: %w", err)
	}
	var bindingID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (
			connector_id, target_id, external_id, external_name,
			integration_enabled, indicators_enabled
		)
		VALUES ($1::uuid, $2::uuid, $3, $4, false, $5)
		ON CONFLICT (connector_id, external_id) DO UPDATE SET
			external_name = EXCLUDED.external_name,
			indicators_enabled = EXCLUDED.indicators_enabled, updated_at = now()
		RETURNING id::text
	`, connectorID, targetID, strings.TrimSpace(binding.ExternalID), strings.TrimSpace(binding.ExternalName), binding.Enabled).Scan(&bindingID); err != nil {
		return "", "", fmt.Errorf("save indicator binding: %w", err)
	}
	return bindingID, targetID, nil
}

func countSelections(bindings []BindingInput) int {
	count := 0
	for _, binding := range bindings {
		count += len(binding.Indicators)
	}
	return count
}

func (store *Store) RuntimeConnectors(ctx context.Context) ([]RuntimeConnector, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT connector.id::text, connector.kind, connector.endpoint, connector.credential_sealed,
		       indicator.id::text, indicator.connector_binding_id::text, indicator.target_id::text,
		       indicator.semantic_key, indicator.label, indicator.external_id, indicator.dimension,
		       indicator.unit, indicator.metadata, indicator.last_value, indicator.last_observed_at,
		       binding.external_id
		FROM cairnops_context_indicators indicator
		JOIN cairnops_connector_bindings binding ON binding.id = indicator.connector_binding_id AND binding.indicators_enabled
		JOIN cairnops_connectors connector ON connector.id = indicator.connector_id AND connector.status <> 'disabled'
		WHERE indicator.enabled
		ORDER BY connector.id, binding.external_id, indicator.semantic_key, indicator.dimension
	`)
	if err != nil {
		return nil, fmt.Errorf("list runtime indicators: %w", err)
	}
	defer rows.Close()
	connectors := []RuntimeConnector{}
	byConnector := map[string]int{}
	for rows.Next() {
		var runtime RuntimeConnector
		var indicator RuntimeIndicator
		var metadata []byte
		if err := rows.Scan(&runtime.ID, &runtime.Kind, &runtime.Endpoint, &runtime.CredentialSealed, &indicator.ID, &indicator.BindingID, &indicator.TargetID, &indicator.SemanticKey, &indicator.Label, &indicator.ExternalID, &indicator.Dimension, &indicator.Unit, &metadata, &indicator.LastValue, &indicator.LastObservedAt, &indicator.BindingExternalID); err != nil {
			return nil, fmt.Errorf("scan runtime indicator: %w", err)
		}
		if err := json.Unmarshal(metadata, &indicator.Metadata); err != nil {
			return nil, fmt.Errorf("decode runtime indicator metadata: %w", err)
		}
		index, known := byConnector[runtime.ID]
		if !known {
			index = len(connectors)
			byConnector[runtime.ID] = index
			runtime.Indicators = []RuntimeIndicator{}
			connectors = append(connectors, runtime)
		}
		connectors[index].Indicators = append(connectors[index].Indicators, indicator)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime indicators: %w", err)
	}
	return connectors, nil
}

func (store *Store) Record(ctx context.Context, connectorID string, at time.Time, readings []Reading, missing map[string]string) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin indicator readings: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, reading := range readings {
		if !finite(reading.Value) {
			continue
		}
		observed := reading.ObservedAt.UTC()
		if observed.IsZero() {
			observed = at.UTC()
		}
		minute := observed.Truncate(time.Minute)
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_indicator_samples (indicator_id, observed_at, value)
			VALUES ($1::uuid, $2::timestamptz, $3)
			ON CONFLICT (indicator_id, observed_at) DO UPDATE SET value = EXCLUDED.value
		`, reading.IndicatorID, minute, reading.Value); err != nil {
			return fmt.Errorf("save indicator sample: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE cairnops_context_indicators SET last_value = $2, last_observed_at = $3, last_error = '', updated_at = now() WHERE id = $1::uuid AND connector_id = $4::uuid AND enabled`, reading.IndicatorID, reading.Value, observed, connectorID); err != nil {
			return fmt.Errorf("update indicator value: %w", err)
		}
	}
	for indicatorID, message := range missing {
		message = strings.TrimSpace(message)
		if len(message) > 500 {
			message = message[:500]
		}
		if _, err := tx.Exec(ctx, `UPDATE cairnops_context_indicators SET last_error = $2, updated_at = now() WHERE id = $1::uuid AND connector_id = $3::uuid AND enabled`, indicatorID, message, connectorID); err != nil {
			return fmt.Errorf("mark indicator unavailable: %w", err)
		}
	}
	status, message := "available", ""
	if len(missing) > 0 {
		status, message = "degraded", fmt.Sprintf("%d indicateur(s) indisponible(s)", len(missing))
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_connector_capabilities (connector_id, capability, status, message, checked_at)
		VALUES ($1::uuid, 'indicators', $2, $3, $4)
		ON CONFLICT (connector_id, capability) DO UPDATE SET status = EXCLUDED.status, message = EXCLUDED.message, checked_at = EXCLUDED.checked_at
	`, connectorID, status, message, at.UTC()); err != nil {
		return fmt.Errorf("update indicator capability: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit indicator readings: %w", err)
	}
	return nil
}

func (store *Store) Fail(ctx context.Context, connectorID, message string, at time.Time) error {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := store.pool.Exec(ctx, `
		INSERT INTO cairnops_connector_capabilities (connector_id, capability, status, message, checked_at)
		VALUES ($1::uuid, 'indicators', 'unavailable', $2, $3)
		ON CONFLICT (connector_id, capability) DO UPDATE SET status = EXCLUDED.status, message = EXCLUDED.message, checked_at = EXCLUDED.checked_at
	`, connectorID, message, at.UTC())
	if err != nil {
		return fmt.Errorf("fail indicator capability: %w", err)
	}
	return nil
}

func (store *Store) Consolidate(ctx context.Context, now time.Time) error {
	cutoff := now.UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	week := now.UTC().Add(-7 * 24 * time.Hour).Truncate(time.Hour)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin indicator retention: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_indicator_hours (indicator_id, hour, minimum, maximum, average, latest, samples, consolidated_at)
		SELECT sample.indicator_id, date_trunc('hour', sample.observed_at), min(sample.value), max(sample.value), avg(sample.value),
		       (array_agg(sample.value ORDER BY sample.observed_at DESC))[1], count(*)::integer, now()
		FROM cairnops_indicator_samples sample
		WHERE sample.observed_at < $1 GROUP BY sample.indicator_id, date_trunc('hour', sample.observed_at)
		ON CONFLICT (indicator_id, hour) DO UPDATE SET minimum = EXCLUDED.minimum, maximum = EXCLUDED.maximum,
		    average = EXCLUDED.average, latest = EXCLUDED.latest, samples = EXCLUDED.samples, consolidated_at = now()
	`, cutoff); err != nil {
		return fmt.Errorf("consolidate indicator hours: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cairnops_indicator_samples WHERE observed_at < $1`, cutoff); err != nil {
		return fmt.Errorf("expire indicator samples: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cairnops_indicator_hours WHERE hour < $1`, week); err != nil {
		return fmt.Errorf("expire indicator hours: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit indicator retention: %w", err)
	}
	return nil
}

func (store *Store) SetPins(ctx context.Context, userID string, indicatorIDs []string) ([]Pin, error) {
	if len(indicatorIDs) > 4 {
		return nil, fmt.Errorf("%w: at most four indicators can be pinned", ErrInvalidInput)
	}
	seen := map[string]struct{}{}
	for _, id := range indicatorIDs {
		if !uuidPattern.MatchString(id) {
			return nil, fmt.Errorf("%w: invalid indicator", ErrInvalidInput)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate indicator", ErrInvalidInput)
		}
		seen[id] = struct{}{}
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin indicator pins: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM cairnops_user_indicator_pins WHERE user_id = $1::uuid`, userID); err != nil {
		return nil, fmt.Errorf("replace indicator pins: %w", err)
	}
	pins := make([]Pin, 0, len(indicatorIDs))
	for position, indicatorID := range indicatorIDs {
		result, err := tx.Exec(ctx, `
			INSERT INTO cairnops_user_indicator_pins (user_id, indicator_id, position)
			SELECT $1::uuid, indicator.id, $3 FROM cairnops_context_indicators indicator
			JOIN cairnops_connector_bindings binding ON binding.id = indicator.connector_binding_id
			JOIN cairnops_connectors connector ON connector.id = indicator.connector_id
			JOIN cairnops_targets target ON target.id = indicator.target_id
			WHERE indicator.id = $2::uuid AND indicator.enabled AND binding.indicators_enabled AND connector.status <> 'disabled' AND target.archived_at IS NULL
		`, userID, indicatorID, position)
		if err != nil {
			return nil, fmt.Errorf("save indicator pin: %w", err)
		}
		if result.RowsAffected() != 1 {
			return nil, fmt.Errorf("%w: indicator is unavailable", ErrInvalidInput)
		}
		pins = append(pins, Pin{IndicatorID: indicatorID, Position: position})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit indicator pins: %w", err)
	}
	return pins, nil
}

func (store *Store) Pins(ctx context.Context, userID string) ([]Pin, error) {
	rows, err := store.pool.Query(ctx, `SELECT indicator_id::text, position FROM cairnops_user_indicator_pins WHERE user_id = $1::uuid ORDER BY position`, userID)
	if err != nil {
		return nil, fmt.Errorf("list indicator pins: %w", err)
	}
	defer rows.Close()
	pins := []Pin{}
	for rows.Next() {
		var pin Pin
		if err := rows.Scan(&pin.IndicatorID, &pin.Position); err != nil {
			return nil, fmt.Errorf("scan indicator pin: %w", err)
		}
		pins = append(pins, pin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indicator pins: %w", err)
	}
	return pins, nil
}

func (store *Store) Target(ctx context.Context, userID, targetID, window string) (TargetProjection, error) {
	if window != WindowDay && window != WindowWeek {
		return TargetProjection{}, fmt.Errorf("%w: invalid window", ErrInvalidInput)
	}
	var exists bool
	if err := store.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cairnops_targets WHERE id = $1::uuid AND archived_at IS NULL)`, targetID).Scan(&exists); err != nil {
		return TargetProjection{}, fmt.Errorf("find indicator target: %w", err)
	}
	if !exists {
		return TargetProjection{}, ErrNotFound
	}
	projection := TargetProjection{TargetID: targetID, GeneratedAt: store.now().UTC(), Indicators: []Indicator{}, Series: map[string][]Point{}}
	indicators, err := store.listIndicators(ctx, userID, targetID, "")
	if err != nil {
		return TargetProjection{}, err
	}
	projection.Indicators = indicators
	for _, indicator := range indicators {
		points, err := store.series(ctx, indicator.ID, window, time.Time{}, time.Time{})
		if err != nil {
			return TargetProjection{}, err
		}
		projection.Series[indicator.ID] = points
	}
	return projection, nil
}

func (store *Store) Overview(ctx context.Context, userID string) ([]TargetProjection, error) {
	all, err := store.listIndicators(ctx, userID, "", "")
	if err != nil {
		return nil, err
	}
	selected := selectOverviewIndicators(all, 4)
	for index := range selected {
		position := index
		selected[index].OverviewPosition = &position
	}
	return store.overviewProjections(ctx, selected, true)
}

func (store *Store) Catalog(ctx context.Context, userID string) ([]TargetProjection, error) {
	all, err := store.listIndicators(ctx, userID, "", "")
	if err != nil {
		return nil, err
	}
	return store.overviewProjections(ctx, all, false)
}

func (store *Store) overviewProjections(ctx context.Context, indicators []Indicator, withSeries bool) ([]TargetProjection, error) {
	projections := []TargetProjection{}
	byTarget := map[string]int{}
	for _, indicator := range indicators {
		index, known := byTarget[indicator.TargetID]
		if !known {
			index = len(projections)
			byTarget[indicator.TargetID] = index
			projections = append(projections, TargetProjection{TargetID: indicator.TargetID, GeneratedAt: store.now().UTC(), Indicators: []Indicator{}, Series: map[string][]Point{}})
		}
		projections[index].Indicators = append(projections[index].Indicators, indicator)
		if !withSeries {
			continue
		}
		points, err := store.series(ctx, indicator.ID, WindowDay, time.Time{}, time.Time{})
		if err != nil {
			return nil, err
		}
		projections[index].Series[indicator.ID] = points
	}
	return projections, nil
}

func (store *Store) Incident(ctx context.Context, userID, incidentID string) (IncidentProjection, error) {
	projection := IncidentProjection{IncidentID: incidentID, TargetIDs: []string{}, Snapshots: []Snapshot{}, Indicators: []Indicator{}, Series: map[string][]Point{}, Disclaimer: "Corrélation temporelle uniquement — ces Indicateurs ne prouvent pas la cause de l’Incident."}
	if err := store.pool.QueryRow(ctx, `
		SELECT incident.opened_at,
		       array_agg(impact.target_id::text ORDER BY impact.opened_at, impact.id)
		FROM cairnops_incidents incident
		JOIN cairnops_incident_impacts impact ON impact.incident_id = incident.id
		WHERE incident.id = $1::uuid
		GROUP BY incident.id, incident.opened_at
	`, incidentID).Scan(&projection.OpenedAt, &projection.TargetIDs); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IncidentProjection{}, ErrNotFound
		}
		return IncidentProjection{}, fmt.Errorf("load indicator incident: %w", err)
	}
	rows, err := store.pool.Query(ctx, `
		SELECT coalesce(snapshot.indicator_id::text, ''), snapshot.impact_id::text,
		       snapshot.target_id::text, target.name, snapshot.semantic_key,
		       snapshot.label, snapshot.unit, snapshot.value, snapshot.observed_at
		FROM cairnops_incident_indicator_snapshots snapshot
		JOIN cairnops_targets target ON target.id = snapshot.target_id
		WHERE snapshot.incident_id = $1::uuid
		ORDER BY target.name, snapshot.semantic_key, snapshot.label
	`, incidentID)
	if err != nil {
		return IncidentProjection{}, fmt.Errorf("list incident indicator snapshots: %w", err)
	}
	for rows.Next() {
		var snapshot Snapshot
		if err := rows.Scan(
			&snapshot.IndicatorID, &snapshot.ImpactID, &snapshot.TargetID,
			&snapshot.TargetName, &snapshot.SemanticKey, &snapshot.Label,
			&snapshot.Unit, &snapshot.Value, &snapshot.ObservedAt,
		); err != nil {
			rows.Close()
			return IncidentProjection{}, fmt.Errorf("scan incident indicator snapshot: %w", err)
		}
		projection.Snapshots = append(projection.Snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return IncidentProjection{}, fmt.Errorf("iterate incident indicator snapshots: %w", err)
	}
	rows.Close()
	projection.Indicators, err = store.listIndicatorsForTargets(ctx, userID, projection.TargetIDs, "")
	if err != nil {
		return IncidentProjection{}, err
	}
	from, to := projection.OpenedAt.Add(-2*time.Hour), projection.OpenedAt.Add(2*time.Hour)
	for _, indicator := range projection.Indicators {
		points, seriesErr := store.series(ctx, indicator.ID, WindowDay, from, to)
		if seriesErr != nil {
			return IncidentProjection{}, seriesErr
		}
		projection.Series[indicator.ID] = points
	}
	return projection, nil
}

func (store *Store) listIndicators(ctx context.Context, userID, targetID, mode string) ([]Indicator, error) {
	targetIDs := []string{}
	if targetID != "" {
		targetIDs = append(targetIDs, targetID)
	}
	return store.listIndicatorsForTargets(ctx, userID, targetIDs, mode)
}

func (store *Store) listIndicatorsForTargets(ctx context.Context, userID string, targetIDs []string, mode string) ([]Indicator, error) {
	if targetIDs == nil {
		targetIDs = []string{}
	}
	rows, err := store.pool.Query(ctx, `
		SELECT indicator.id::text, indicator.connector_id::text, indicator.connector_binding_id::text,
		       indicator.target_id::text, indicator.semantic_key, indicator.label, indicator.external_id,
		       indicator.dimension, indicator.unit, indicator.enabled, indicator.metadata,
		       indicator.last_value, indicator.last_observed_at, indicator.last_error,
		       pin.position
		FROM cairnops_context_indicators indicator
		JOIN cairnops_connector_bindings binding ON binding.id = indicator.connector_binding_id AND binding.indicators_enabled
		JOIN cairnops_connectors connector ON connector.id = indicator.connector_id AND connector.status <> 'disabled'
		LEFT JOIN cairnops_user_indicator_pins pin ON pin.indicator_id = indicator.id AND pin.user_id = $1::uuid
		WHERE indicator.enabled AND (cardinality($2::uuid[]) = 0 OR indicator.target_id = ANY($2::uuid[]))
		  AND ($3 <> 'pinned' OR pin.position IS NOT NULL)
		ORDER BY pin.position NULLS LAST, indicator.semantic_key, indicator.dimension, indicator.id
	`, userID, targetIDs, mode)
	if err != nil {
		return nil, fmt.Errorf("list target indicators: %w", err)
	}
	defer rows.Close()
	indicators := []Indicator{}
	for rows.Next() {
		var indicator Indicator
		var metadata []byte
		var position *int
		if err := rows.Scan(&indicator.ID, &indicator.ConnectorID, &indicator.BindingID, &indicator.TargetID, &indicator.SemanticKey, &indicator.Label, &indicator.ExternalID, &indicator.Dimension, &indicator.Unit, &indicator.Enabled, &metadata, &indicator.LastValue, &indicator.LastObservedAt, &indicator.LastError, &position); err != nil {
			return nil, fmt.Errorf("scan target indicator: %w", err)
		}
		if err := json.Unmarshal(metadata, &indicator.Metadata); err != nil {
			return nil, fmt.Errorf("decode target indicator metadata: %w", err)
		}
		indicator.PinPosition = position
		indicator.Pinned = position != nil
		indicators = append(indicators, indicator)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate target indicators: %w", err)
	}
	return indicators, nil
}

func (store *Store) series(ctx context.Context, indicatorID, window string, from, to time.Time) ([]Point, error) {
	points := []Point{}
	var rows pgx.Rows
	var err error
	if window == WindowWeek {
		rows, err = store.pool.Query(ctx, `
			WITH compact AS (
				SELECT hour AS at, latest AS value, minimum, maximum, samples
				FROM cairnops_indicator_hours
				WHERE indicator_id = $1::uuid AND hour >= now() - interval '7 days'
				UNION ALL
				SELECT date_trunc('hour', observed_at),
				       (array_agg(value ORDER BY observed_at DESC))[1], min(value), max(value), count(*)::integer
				FROM cairnops_indicator_samples
				WHERE indicator_id = $1::uuid
				  AND observed_at >= date_trunc('hour', now() - interval '24 hours')
				GROUP BY date_trunc('hour', observed_at)
			)
			SELECT at, value, minimum, maximum, samples FROM compact ORDER BY at
		`, indicatorID)
	} else if !from.IsZero() && !to.IsZero() {
		rows, err = store.pool.Query(ctx, `SELECT observed_at, value, NULL::double precision, NULL::double precision, 0 FROM cairnops_indicator_samples WHERE indicator_id = $1::uuid AND observed_at BETWEEN $2 AND $3 ORDER BY observed_at`, indicatorID, from.UTC(), to.UTC())
	} else {
		rows, err = store.pool.Query(ctx, `SELECT observed_at, value, NULL::double precision, NULL::double precision, 0 FROM cairnops_indicator_samples WHERE indicator_id = $1::uuid AND observed_at >= now() - interval '24 hours' ORDER BY observed_at`, indicatorID)
	}
	if err != nil {
		return nil, fmt.Errorf("read indicator series: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var point Point
		if err := rows.Scan(&point.At, &point.Value, &point.Minimum, &point.Maximum, &point.Samples); err != nil {
			return nil, fmt.Errorf("scan indicator series: %w", err)
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indicator series: %w", err)
	}
	return points, nil
}

func sortedRuntimeIDs(indicators []RuntimeIndicator) []string {
	ids := make([]string, 0, len(indicators))
	for _, indicator := range indicators {
		ids = append(ids, indicator.ID)
	}
	sort.Strings(ids)
	return ids
}
