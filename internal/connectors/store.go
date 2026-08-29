package connectors

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

func enableIntegrationBinding(ctx context.Context, tx pgx.Tx, connectorID, externalID string) (string, error) {
	var bindingID string
	if err := tx.QueryRow(ctx, `
		UPDATE cairnops_connector_bindings
		SET integration_enabled = true, updated_at = now()
		WHERE connector_id = $1::uuid AND external_id = $2
		RETURNING id::text
	`, connectorID, externalID).Scan(&bindingID); err != nil {
		return "", fmt.Errorf("enable integration binding: %w", err)
	}
	return bindingID, nil
}

// ensureIntegrationSource fait exister la Source de signal d'une liaison.
//
// L'Intégration en conserve la propriété : le nom et la cadence suivent le
// produit distant, et la Source disparaît avec la liaison. C'est cette Source
// qui portera les Observations dont se déduisent Disponibilité, Couverture et
// latence — sans elle, une Cible importée resterait à jamais sans mesure.
func ensureIntegrationSource(ctx context.Context, tx pgx.Tx, bindingID string, measuresAvailability bool) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, origin, connector_binding_id, enabled,
			interval_seconds, timeout_milliseconds, config, measures_availability
		)
		SELECT binding.target_id,
		       left(coalesce(nullif(btrim(binding.external_name), ''), 'Source importée'), 160),
		       connector.kind, 'integration', binding.id, connector.status <> 'disabled',
		       connector.sync_interval_seconds, 1000, '{}'::jsonb, $2
		FROM cairnops_connector_bindings binding
		JOIN cairnops_connectors connector ON connector.id = binding.connector_id
		WHERE binding.id = $1::uuid
		ON CONFLICT (connector_binding_id) WHERE connector_binding_id IS NOT NULL
		DO UPDATE SET
			target_id = excluded.target_id,
			name = excluded.name,
			enabled = excluded.enabled,
			interval_seconds = excluded.interval_seconds,
			measures_availability = excluded.measures_availability,
			updated_at = now()
	`, bindingID, measuresAvailability); err != nil {
		return fmt.Errorf("ensure integration signal source: %w", err)
	}
	return nil
}

type PostgresStore struct {
	pool *pgxpool.Pool
}

func addBindingIdentity(identity *TargetIdentity, kind string, encoded []byte) error {
	if kind == "" || len(encoded) == 0 {
		return nil
	}
	var metadata struct {
		TechnicalName string `json:"technical_name"`
		Address       string `json:"address"`
		URL           string `json:"url"`
		Hostname      string `json:"hostname"`
		MachineID     string `json:"machine_id"`
		Interfaces    []struct {
			Address string `json:"address"`
		} `json:"interfaces"`
	}
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return err
	}
	if metadata.TechnicalName != "" {
		identity.Names = append(identity.Names, metadata.TechnicalName)
		identity.Addresses = append(identity.Addresses, metadata.TechnicalName)
	}
	identity.Addresses = append(identity.Addresses, metadata.Address, metadata.URL, metadata.Hostname)
	identity.Identifiers = append(identity.Identifiers, metadata.MachineID)
	for _, item := range metadata.Interfaces {
		identity.Addresses = append(identity.Addresses, item.Address)
	}
	return nil
}

func selectAssignedTarget(ctx context.Context, tx pgx.Tx, assignedTargetID string, targetID, targetName *string) error {
	return tx.QueryRow(ctx, `
		SELECT id::text, name
		FROM cairnops_targets
		WHERE id = $1::uuid AND archived_at IS NULL
	`, assignedTargetID).Scan(targetID, targetName)
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (store *PostgresStore) List(ctx context.Context) ([]Connector, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT connector.id::text, connector.kind, connector.name, connector.endpoint,
		       connector.status, connector.remote_version, connector.compatibility,
		       connector.encrypted_transport, count(binding.id) FILTER (WHERE binding.integration_enabled)::integer,
		       (SELECT count(*)::integer FROM cairnops_webhook_quarantine quarantine
		        WHERE quarantine.connector_id = connector.id AND quarantine.approved_at IS NULL),
		       connector.last_checked_at, connector.last_error,
		       connector.created_at, connector.updated_at
		FROM cairnops_connectors connector
		LEFT JOIN cairnops_connector_bindings binding ON binding.connector_id = connector.id
		GROUP BY connector.id
		ORDER BY lower(connector.name), connector.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}
	defer rows.Close()

	connectors := make([]Connector, 0)
	for rows.Next() {
		connector, err := scanConnector(rows)
		if err != nil {
			return nil, err
		}
		connectors = append(connectors, connector)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connectors: %w", err)
	}
	return connectors, nil
}

// SetStatus suspend ou reprend un Connecteur. Le bail est relâché dans les deux
// sens : un cycle en vol se terminerait sinon par un CompleteConnectorSync qui
// remet `status = 'connected'`, ressuscitant un Connecteur qu'on vient de
// suspendre. Sans bail, sa garde `lease_owner = $2` n'écrit plus rien.
//
// Les Sources d'Intégration suivent leur Connecteur : suspendre n'efface rien,
// mais plus rien ne doit se présenter comme actif alors que plus rien n'est lu.
func (store *PostgresStore) SetStatus(ctx context.Context, connectorID, status string) (Connector, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Connector{}, fmt.Errorf("begin connector status change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	connector, err := scanConnector(tx.QueryRow(ctx, `
		UPDATE cairnops_connectors SET
			status = $2,
			last_error = CASE WHEN $2 = 'disabled' THEN last_error ELSE '' END,
			next_sync_at = CASE WHEN $2 = 'disabled' THEN next_sync_at ELSE now() END,
			lease_owner = NULL,
			lease_until = NULL,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, kind, name, endpoint, status, remote_version,
		          compatibility, encrypted_transport,
		          (SELECT count(*)::integer FROM cairnops_connector_bindings binding
		           WHERE binding.connector_id = cairnops_connectors.id),
		          (SELECT count(*)::integer FROM cairnops_webhook_quarantine quarantine
		           WHERE quarantine.connector_id = cairnops_connectors.id AND quarantine.approved_at IS NULL),
		          last_checked_at, last_error, created_at, updated_at
	`, connectorID, status))
	if errors.Is(err, pgx.ErrNoRows) {
		return Connector{}, ErrNotFound
	}
	if err != nil {
		return Connector{}, fmt.Errorf("set connector status: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_signal_sources source
		SET enabled = ($2 <> 'disabled' AND binding.integration_enabled), updated_at = now()
		FROM cairnops_connector_bindings binding
		WHERE binding.id = source.connector_binding_id
		  AND binding.connector_id = $1::uuid
		  AND source.origin = 'integration'
	`, connectorID, status); err != nil {
		return Connector{}, fmt.Errorf("follow connector status on its sources: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Connector{}, fmt.Errorf("commit connector status change: %w", err)
	}
	return connector, nil
}

// Delete retire un Connecteur et tout ce qu'il a produit. Les liaisons, la
// quarantaine et les preuves partent en cascade ; les Cibles restent, elles
// n'appartiennent pas au Connecteur.
//
// Les Incidents, eux, ne cascadent pas : sans ce qui suit, un Incident nourri
// par ce seul Connecteur resterait « actif » sans plus aucune preuve, à jamais,
// en occupant l'index unique (target_id, nature_key) qui interdit alors tout
// nouvel Incident de même nature sur la Cible. On relève donc les Incidents
// concernés avant la suppression, puis on clôt ceux que la cascade a vidés.
func (store *PostgresStore) Delete(ctx context.Context, connectorID string) (Removal, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Removal{}, fmt.Errorf("begin connector removal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var removal Removal
	err = tx.QueryRow(ctx, `
		SELECT id::text, kind, name,
		       (SELECT count(*)::integer FROM cairnops_connector_bindings binding
		        WHERE binding.connector_id = connector.id),
		       (SELECT count(*)::integer FROM cairnops_webhook_quarantine quarantine
		        WHERE quarantine.connector_id = connector.id AND quarantine.approved_at IS NULL)
		FROM cairnops_connectors connector
		WHERE id = $1::uuid
		FOR UPDATE
	`, connectorID).Scan(&removal.ID, &removal.Kind, &removal.Name, &removal.Bindings, &removal.Quarantined)
	if errors.Is(err, pgx.ErrNoRows) {
		return Removal{}, ErrNotFound
	}
	if err != nil {
		return Removal{}, fmt.Errorf("load connector to remove: %w", err)
	}

	var reconciliationBusy bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM cairnops_connector_bindings binding
			JOIN cairnops_signal_sources source ON source.connector_binding_id = binding.id
			JOIN cairnops_target_reconciliation_operations operation
			  ON operation.status IN ('queued', 'running')
			 AND (
				operation.source_id = source.id
				OR source.target_id IN (operation.primary_target_id, operation.secondary_target_id)
			 )
			WHERE binding.connector_id = $1::uuid
		)
	`, connectorID).Scan(&reconciliationBusy); err != nil {
		return Removal{}, fmt.Errorf("check connector reconciliation: %w", err)
	}
	if reconciliationBusy {
		return Removal{}, ErrStructureBusy
	}

	exposed, err := collectIncidentIDs(ctx, tx, `
		SELECT DISTINCT incident.id::text
		FROM cairnops_incidents incident
		JOIN cairnops_incident_signals signal ON signal.incident_id = incident.id
		WHERE incident.status = 'active' AND signal.connector_id = $1::uuid
	`, connectorID)
	if err != nil {
		return Removal{}, fmt.Errorf("list incidents fed by connector: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM cairnops_connectors WHERE id = $1::uuid`, connectorID); err != nil {
		return Removal{}, fmt.Errorf("remove connector: %w", err)
	}

	orphaned, err := collectIncidentIDs(ctx, tx, `
		UPDATE cairnops_incidents incident
		SET status = 'resolved', resolved_at = now(), updated_at = now()
		WHERE incident.id::text = ANY($1) AND incident.status = 'active'
		  AND NOT EXISTS (
		      SELECT 1 FROM cairnops_incident_signals signal
		      WHERE signal.incident_id = incident.id AND signal.active
		  )
		RETURNING incident.id::text
	`, exposed)
	if err != nil {
		return Removal{}, fmt.Errorf("close incidents left without evidence: %w", err)
	}
	removal.ResolvedIncidents = len(orphaned)

	if len(orphaned) > 0 {
		note, err := json.Marshal(map[string]any{"connector": removal.Name, "kind": removal.Kind})
		if err != nil {
			return Removal{}, fmt.Errorf("encode connector removal note: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_activity (incident_id, kind, origin, message, data)
			SELECT id::uuid, 'resolved', 'cairnops', $2, $3::jsonb
			FROM unnest($1::text[]) AS removed(id)
		`, orphaned, fmt.Sprintf("Le Connecteur « %s » a été supprimé : plus aucune preuve n'appuie cet Incident", removal.Name), note); err != nil {
			return Removal{}, fmt.Errorf("record connector removal on incidents: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Removal{}, fmt.Errorf("commit connector removal: %w", err)
	}
	return removal, nil
}

func collectIncidentIDs(ctx context.Context, tx pgx.Tx, query string, argument any) ([]string, error) {
	rows, err := tx.Query(ctx, query, argument)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	identifiers := make([]string, 0)
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			return nil, err
		}
		identifiers = append(identifiers, identifier)
	}
	return identifiers, rows.Err()
}

func (store *PostgresStore) CreateGenericWebhook(ctx context.Context, actorID, name, endpoint, publicID, credentialSealed string, encryptedTransport bool) (Connector, error) {
	connector, err := scanConnector(store.pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, webhook_public_id, credential_sealed, status,
			remote_version, compatibility, encrypted_transport,
			last_checked_at, last_error, created_by
		) VALUES ('generic_webhook', $1, $2, $3, $4, 'connected',
		          'JSON v1', 'supported', $5, now(), '', $6::uuid)
		RETURNING id::text, kind, name, endpoint, status, remote_version,
		          compatibility, encrypted_transport, 0, 0,
		          last_checked_at, last_error, created_at, updated_at
	`, name, endpoint, publicID, credentialSealed, encryptedTransport, actorID))
	if err != nil {
		return Connector{}, fmt.Errorf("create generic webhook connector: %w", err)
	}
	return connector, nil
}

func (store *PostgresStore) WebhookCredential(ctx context.Context, publicID string) (WebhookCredential, error) {
	var credential WebhookCredential
	if err := store.pool.QueryRow(ctx, `
		SELECT id::text, endpoint, credential_sealed, status
		FROM cairnops_connectors
		WHERE kind = 'generic_webhook' AND webhook_public_id = $1
	`, publicID).Scan(&credential.ConnectorID, &credential.Endpoint, &credential.CredentialSealed, &credential.Status); errors.Is(err, pgx.ErrNoRows) {
		return WebhookCredential{}, ErrWebhookNotFound
	} else if err != nil {
		return WebhookCredential{}, fmt.Errorf("load webhook credential: %w", err)
	}
	return credential, nil
}

func (store *PostgresStore) RouteWebhook(ctx context.Context, connectorID string, event GenericWebhookEvent, observedAt time.Time) (WebhookRoute, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WebhookRoute{}, fmt.Errorf("begin webhook routing: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, connectorID+":"+event.Identity); err != nil {
		return WebhookRoute{}, fmt.Errorf("lock webhook identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_connectors
		SET last_checked_at = $2, updated_at = now()
		WHERE id = $1::uuid AND kind = 'generic_webhook'
	`, connectorID, observedAt); err != nil {
		return WebhookRoute{}, fmt.Errorf("record webhook reception: %w", err)
	}
	var route WebhookRoute
	err = tx.QueryRow(ctx, `
		SELECT id::text, target_id::text
		FROM cairnops_connector_bindings
		WHERE connector_id = $1::uuid AND external_id = $2
	`, connectorID, event.Identity).Scan(&route.BindingID, &route.TargetID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return WebhookRoute{}, fmt.Errorf("commit webhook routing: %w", err)
		}
		return route, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return WebhookRoute{}, fmt.Errorf("find webhook identity binding: %w", err)
	}
	details, err := json.Marshal(event.Details)
	if err != nil {
		return WebhookRoute{}, fmt.Errorf("encode quarantined webhook details: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO cairnops_webhook_quarantine (
			connector_id, external_identity, target_name, external_event_key,
			nature_key, nature_label, status, severity, summary, details,
			first_seen_at, last_seen_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $11)
		ON CONFLICT (connector_id, external_identity, external_event_key) DO UPDATE SET
			target_name = EXCLUDED.target_name, nature_key = EXCLUDED.nature_key,
			nature_label = EXCLUDED.nature_label, status = EXCLUDED.status,
			severity = EXCLUDED.severity, summary = EXCLUDED.summary,
			details = EXCLUDED.details, occurrences = cairnops_webhook_quarantine.occurrences + 1,
			last_seen_at = EXCLUDED.last_seen_at, approved_at = NULL, approved_by = NULL
		RETURNING id::text
	`, connectorID, event.Identity, event.TargetName, event.EventKey,
		event.NatureKey, event.Nature, event.Status, event.Severity, event.Summary, details, observedAt).Scan(&route.QuarantineID); err != nil {
		return WebhookRoute{}, fmt.Errorf("quarantine unknown webhook identity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return WebhookRoute{}, fmt.Errorf("commit webhook quarantine: %w", err)
	}
	return route, nil
}

func (store *PostgresStore) ListWebhookQuarantine(ctx context.Context, connectorID string) ([]WebhookQuarantine, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT quarantine.id::text, quarantine.connector_id::text,
		       quarantine.external_identity, quarantine.target_name,
		       quarantine.external_event_key, quarantine.nature_key,
		       quarantine.nature_label, quarantine.status, quarantine.severity,
		       quarantine.summary, quarantine.details, quarantine.occurrences,
		       quarantine.first_seen_at, quarantine.last_seen_at
		FROM cairnops_webhook_quarantine quarantine
		JOIN cairnops_connectors connector ON connector.id = quarantine.connector_id
		WHERE quarantine.connector_id = $1::uuid AND connector.kind = 'generic_webhook'
		  AND quarantine.approved_at IS NULL
		ORDER BY quarantine.last_seen_at DESC, quarantine.id
	`, connectorID)
	if err != nil {
		return nil, fmt.Errorf("list webhook quarantine: %w", err)
	}
	defer rows.Close()
	items := make([]WebhookQuarantine, 0)
	for rows.Next() {
		item, err := scanWebhookQuarantine(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhook quarantine: %w", err)
	}
	return items, nil
}

func (store *PostgresStore) ApproveWebhookIdentity(ctx context.Context, actorID, connectorID, quarantineID, requestedTargetID string) (WebhookApproval, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WebhookApproval{}, fmt.Errorf("begin webhook approval: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var identity, targetName string
	if err := tx.QueryRow(ctx, `
		SELECT quarantine.external_identity, quarantine.target_name
		FROM cairnops_webhook_quarantine quarantine
		JOIN cairnops_connectors connector ON connector.id = quarantine.connector_id
		WHERE quarantine.id = $1::uuid AND quarantine.connector_id = $2::uuid
		  AND quarantine.approved_at IS NULL AND connector.kind = 'generic_webhook'
		FOR UPDATE OF quarantine
	`, quarantineID, connectorID).Scan(&identity, &targetName); errors.Is(err, pgx.ErrNoRows) {
		return WebhookApproval{}, ErrWebhookNotFound
	} else if err != nil {
		return WebhookApproval{}, fmt.Errorf("load quarantined webhook identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, connectorID+":"+identity); err != nil {
		return WebhookApproval{}, fmt.Errorf("lock webhook approval identity: %w", err)
	}

	var targetID, resolvedTargetName string
	if requestedTargetID != "" {
		err = tx.QueryRow(ctx, `
			SELECT id::text, name FROM cairnops_targets
			WHERE id = $1::uuid AND archived_at IS NULL
		`, requestedTargetID).Scan(&targetID, &resolvedTargetName)
		if errors.Is(err, pgx.ErrNoRows) {
			return WebhookApproval{}, ErrWebhookNotFound
		}
	} else {
		err = tx.QueryRow(ctx, `
			SELECT id::text, name FROM cairnops_targets
			WHERE archived_at IS NULL AND lower(btrim(name)) = lower(btrim($1))
			ORDER BY created_at, id LIMIT 1
		`, targetName).Scan(&targetID, &resolvedTargetName)
		if errors.Is(err, pgx.ErrNoRows) {
			description := "Identité autorisée depuis un webhook générique."
			err = tx.QueryRow(ctx, `
				INSERT INTO cairnops_targets (name, description) VALUES ($1, $2)
				RETURNING id::text, name
			`, targetName, description).Scan(&targetID, &resolvedTargetName)
		}
	}
	if err != nil {
		return WebhookApproval{}, fmt.Errorf("resolve webhook target: %w", err)
	}

	var bindingID, existingTargetID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, target_id::text FROM cairnops_connector_bindings
		WHERE connector_id = $1::uuid AND external_id = $2
	`, connectorID, identity).Scan(&bindingID, &existingTargetID)
	if err == nil && existingTargetID != targetID {
		return WebhookApproval{}, ErrWebhookConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		metadata, _ := json.Marshal(map[string]any{"approved_from_quarantine": quarantineID})
		err = tx.QueryRow(ctx, `
			INSERT INTO cairnops_connector_bindings (
				connector_id, target_id, external_id, external_name, metadata
			) VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
			RETURNING id::text
		`, connectorID, targetID, identity, targetName, metadata).Scan(&bindingID)
	}
	if err != nil {
		return WebhookApproval{}, fmt.Errorf("bind approved webhook identity: %w", err)
	}
	if err := ensureIntegrationSource(ctx, tx, bindingID, true); err != nil {
		return WebhookApproval{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, connector_id::text, external_identity, target_name,
		       external_event_key, nature_key, nature_label, status, severity,
		       summary, details, occurrences, first_seen_at, last_seen_at
		FROM cairnops_webhook_quarantine
		WHERE connector_id = $1::uuid AND external_identity = $2 AND approved_at IS NULL
		ORDER BY first_seen_at, id
	`, connectorID, identity)
	if err != nil {
		return WebhookApproval{}, fmt.Errorf("load webhook events for approval: %w", err)
	}
	events := make([]WebhookQuarantine, 0)
	for rows.Next() {
		event, err := scanWebhookQuarantine(rows)
		if err != nil {
			rows.Close()
			return WebhookApproval{}, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return WebhookApproval{}, fmt.Errorf("iterate webhook events for approval: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return WebhookApproval{}, fmt.Errorf("commit webhook identity approval: %w", err)
	}
	return WebhookApproval{
		BindingID: bindingID, TargetID: targetID, TargetName: resolvedTargetName,
		Identity: identity, Events: events,
	}, nil
}

func (store *PostgresStore) CompleteWebhookApproval(ctx context.Context, connectorID, identity, actorID string) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE cairnops_webhook_quarantine
		SET approved_at = now(), approved_by = $3::uuid
		WHERE connector_id = $1::uuid AND external_identity = $2 AND approved_at IS NULL
	`, connectorID, identity, actorID)
	if err != nil {
		return fmt.Errorf("complete webhook identity approval: %w", err)
	}
	return nil
}

func scanWebhookQuarantine(row scanner) (WebhookQuarantine, error) {
	var item WebhookQuarantine
	var details []byte
	if err := row.Scan(
		&item.ID, &item.ConnectorID, &item.ExternalIdentity, &item.TargetName,
		&item.EventKey, &item.NatureKey, &item.Nature, &item.Status, &item.Severity,
		&item.Summary, &details, &item.Occurrences, &item.FirstSeenAt, &item.LastSeenAt,
	); err != nil {
		return WebhookQuarantine{}, fmt.Errorf("scan webhook quarantine: %w", err)
	}
	if err := json.Unmarshal(details, &item.Details); err != nil {
		return WebhookQuarantine{}, fmt.Errorf("decode webhook quarantine details: %w", err)
	}
	return item, nil
}

func (store *PostgresStore) PreviewState(ctx context.Context, kind, endpoint string, names []string) (PreviewState, error) {
	state := PreviewState{
		TargetsByName:        make(map[string]TargetReference),
		ImportedByExternalID: make(map[string]TargetReference),
	}
	normalized := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = normalizeName(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	if len(normalized) > 0 {
		rows, err := store.pool.Query(ctx, `
			SELECT id::text, name
			FROM cairnops_targets
			WHERE archived_at IS NULL AND lower(btrim(name)) = ANY($1::text[])
			ORDER BY created_at, id
		`, normalized)
		if err != nil {
			return PreviewState{}, fmt.Errorf("match existing targets: %w", err)
		}
		for rows.Next() {
			var target TargetReference
			if err := rows.Scan(&target.ID, &target.Name); err != nil {
				rows.Close()
				return PreviewState{}, fmt.Errorf("scan existing target: %w", err)
			}
			key := normalizeName(target.Name)
			if _, alreadySelected := state.TargetsByName[key]; !alreadySelected {
				state.TargetsByName[key] = target
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return PreviewState{}, fmt.Errorf("iterate existing targets: %w", err)
		}
		rows.Close()
	}

	identityRows, err := store.pool.Query(ctx, `
		SELECT target.id::text, target.name,
		       coalesce(binding.external_name, ''), coalesce(connector.kind, ''),
		       coalesce(binding.metadata, '{}'::jsonb)
		FROM cairnops_targets target
		LEFT JOIN cairnops_connector_bindings binding ON binding.target_id = target.id
		LEFT JOIN cairnops_connectors connector ON connector.id = binding.connector_id
		WHERE target.archived_at IS NULL
		ORDER BY lower(target.name), target.id, binding.created_at, binding.id
	`)
	if err != nil {
		return PreviewState{}, fmt.Errorf("list target identities: %w", err)
	}
	identities := make(map[string]TargetIdentity)
	for identityRows.Next() {
		var targetID, targetName, externalName, connectorKind string
		var metadata []byte
		if err := identityRows.Scan(&targetID, &targetName, &externalName, &connectorKind, &metadata); err != nil {
			identityRows.Close()
			return PreviewState{}, fmt.Errorf("scan target identity: %w", err)
		}
		identity, exists := identities[targetID]
		if !exists {
			identity = TargetIdentity{
				TargetReference: TargetReference{ID: targetID, Name: targetName},
				Names:           []string{targetName},
			}
		}
		if externalName != "" {
			identity.Names = append(identity.Names, externalName)
		}
		if err := addBindingIdentity(&identity, connectorKind, metadata); err != nil {
			identityRows.Close()
			return PreviewState{}, fmt.Errorf("decode identity metadata for target %s: %w", targetID, err)
		}
		identities[targetID] = identity
	}
	if err := identityRows.Err(); err != nil {
		identityRows.Close()
		return PreviewState{}, fmt.Errorf("iterate target identities: %w", err)
	}
	identityRows.Close()
	state.Targets = make([]TargetIdentity, 0, len(identities))
	for _, identity := range identities {
		state.Targets = append(state.Targets, identity)
	}
	sort.Slice(state.Targets, func(i, j int) bool {
		if normalizeName(state.Targets[i].Name) != normalizeName(state.Targets[j].Name) {
			return normalizeName(state.Targets[i].Name) < normalizeName(state.Targets[j].Name)
		}
		return state.Targets[i].ID < state.Targets[j].ID
	})

	rows, err := store.pool.Query(ctx, `
		SELECT binding.external_id, target.id::text, target.name
		FROM cairnops_connectors connector
		JOIN cairnops_connector_bindings binding ON binding.connector_id = connector.id
		JOIN cairnops_targets target ON target.id = binding.target_id
		WHERE connector.kind = $1
		  AND lower(connector.endpoint) = lower($2)
		  AND target.archived_at IS NULL
	`, kind, endpoint)
	if err != nil {
		return PreviewState{}, fmt.Errorf("match imported %s objects: %w", kind, err)
	}
	defer rows.Close()
	for rows.Next() {
		var externalID string
		var target TargetReference
		if err := rows.Scan(&externalID, &target.ID, &target.Name); err != nil {
			return PreviewState{}, fmt.Errorf("scan imported %s object: %w", kind, err)
		}
		state.ImportedByExternalID[externalID] = target
	}
	if err := rows.Err(); err != nil {
		return PreviewState{}, fmt.Errorf("iterate imported %s objects: %w", kind, err)
	}
	return state, nil
}

func (store *PostgresStore) ImportUptimeKuma(ctx context.Context, input PersistUptimeKumaInput) (UptimeKumaImport, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("begin Uptime Kuma import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	connector, err := scanConnector(tx.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			remote_version, compatibility, encrypted_transport,
			last_checked_at, last_error, created_by
		) VALUES ('uptime_kuma', $1, $2, $3, 'connected', '', 'supported', $4, now(), '', $5::uuid)
		ON CONFLICT (kind, (lower(endpoint))) DO UPDATE SET
			name = EXCLUDED.name,
			credential_sealed = EXCLUDED.credential_sealed,
			status = 'connected',
			compatibility = 'supported',
			encrypted_transport = EXCLUDED.encrypted_transport,
			last_checked_at = now(),
			last_error = '',
			next_sync_at = now(),
			lease_owner = NULL,
			lease_until = NULL,
			updated_at = now()
		RETURNING id::text, kind, name, endpoint, status, remote_version,
		          compatibility, encrypted_transport,
		          (SELECT count(*)::integer FROM cairnops_connector_bindings WHERE connector_id = cairnops_connectors.id), 0,
		          last_checked_at, last_error, created_at, updated_at
	`, input.Name, input.Endpoint, input.CredentialSealed, input.EncryptedTransport, input.ActorID))
	if err != nil {
		return UptimeKumaImport{}, fmt.Errorf("save Uptime Kuma connector: %w", err)
	}

	result := UptimeKumaImport{Connector: connector, Targets: make([]ImportedTarget, 0, len(input.Monitors))}
	for _, monitor := range input.Monitors {
		var targetID, targetName string
		assignedTargetID := input.TargetAssignments[monitor.ID]
		err := tx.QueryRow(ctx, `
			SELECT target.id::text, target.name
			FROM cairnops_connector_bindings binding
			JOIN cairnops_targets target ON target.id = binding.target_id
			WHERE binding.connector_id = $1::uuid AND binding.external_id = $2
		`, connector.ID, monitor.ID).Scan(&targetID, &targetName)
		disposition := "already_imported"
		if err == nil && assignedTargetID != "" && assignedTargetID != targetID {
			return UptimeKumaImport{}, fmt.Errorf("%w: Uptime Kuma monitor %s is already linked to another target", ErrInvalidInput, monitor.ID)
		}
		if err == nil {
			bindingID, enableErr := enableIntegrationBinding(ctx, tx, connector.ID, monitor.ID)
			if enableErr != nil {
				return UptimeKumaImport{}, enableErr
			}
			if sourceErr := ensureIntegrationSource(ctx, tx, bindingID, true); sourceErr != nil {
				return UptimeKumaImport{}, sourceErr
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			disposition = "reused"
			if assignedTargetID != "" {
				err = selectAssignedTarget(ctx, tx, assignedTargetID, &targetID, &targetName)
			} else {
				err = tx.QueryRow(ctx, `
					SELECT id::text, name
					FROM cairnops_targets
					WHERE archived_at IS NULL AND lower(btrim(name)) = lower(btrim($1))
					ORDER BY created_at, id
					LIMIT 1
				`, monitor.Name).Scan(&targetID, &targetName)
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID == "" {
				description := fmt.Sprintf("Découvert par le Connecteur %s.", input.Name)
				err = tx.QueryRow(ctx, `
					INSERT INTO cairnops_targets (name, description)
					VALUES ($1, $2)
					RETURNING id::text, name
				`, monitor.Name, description).Scan(&targetID, &targetName)
				disposition = "created"
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID != "" {
				return UptimeKumaImport{}, fmt.Errorf("%w: assigned target does not exist or is archived", ErrInvalidInput)
			}
			if err != nil {
				return UptimeKumaImport{}, fmt.Errorf("resolve target for Uptime Kuma monitor %s: %w", monitor.ID, err)
			}
			metadata, err := json.Marshal(map[string]any{
				"type": monitor.Type, "address": monitor.Address(),
				"url": monitor.URL, "hostname": monitor.Hostname, "port": monitor.Port,
			})
			if err != nil {
				return UptimeKumaImport{}, fmt.Errorf("encode Uptime Kuma monitor %s metadata: %w", monitor.ID, err)
			}
			var bindingID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO cairnops_connector_bindings (
					connector_id, target_id, external_id, external_name, metadata
				) VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
				ON CONFLICT (connector_id, external_id) DO UPDATE SET
					target_id = EXCLUDED.target_id,
					external_name = EXCLUDED.external_name,
					metadata = EXCLUDED.metadata,
					integration_enabled = true,
					updated_at = now()
				RETURNING id::text
			`, connector.ID, targetID, monitor.ID, monitor.Name, metadata).Scan(&bindingID); err != nil {
				return UptimeKumaImport{}, fmt.Errorf("bind Uptime Kuma monitor %s: %w", monitor.ID, err)
			}
			if err := ensureIntegrationSource(ctx, tx, bindingID, true); err != nil {
				return UptimeKumaImport{}, err
			}
		} else if err != nil {
			return UptimeKumaImport{}, fmt.Errorf("find existing Uptime Kuma monitor %s: %w", monitor.ID, err)
		}
		result.Targets = append(result.Targets, ImportedTarget{
			ExternalID: monitor.ID, TargetID: targetID, TargetName: targetName, Disposition: disposition,
		})
	}
	result.Connector.BindingCount += countNewBindings(result.Targets)
	if err := tx.Commit(ctx); err != nil {
		return UptimeKumaImport{}, fmt.Errorf("commit Uptime Kuma import: %w", err)
	}
	return result, nil
}

func (store *PostgresStore) ImportPatchMon(ctx context.Context, input PersistPatchMonInput) (PatchMonImport, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("begin PatchMon import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	connector, err := scanConnector(tx.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			remote_version, compatibility, encrypted_transport,
			last_checked_at, last_error, created_by, sync_interval_seconds
		) VALUES ('patchmon', $1, $2, $3, 'connected', '', 'supported', $4, now(), '', $5::uuid, 300)
		ON CONFLICT (kind, (lower(endpoint))) DO UPDATE SET
			name = EXCLUDED.name,
			credential_sealed = EXCLUDED.credential_sealed,
			status = 'connected',
			compatibility = 'supported',
			encrypted_transport = EXCLUDED.encrypted_transport,
			last_checked_at = now(),
			last_error = '',
			next_sync_at = now(),
			lease_owner = NULL,
			lease_until = NULL,
			updated_at = now()
		RETURNING id::text, kind, name, endpoint, status, remote_version,
		          compatibility, encrypted_transport,
		          (SELECT count(*)::integer FROM cairnops_connector_bindings WHERE connector_id = cairnops_connectors.id), 0,
		          last_checked_at, last_error, created_at, updated_at
	`, input.Name, input.Endpoint, input.CredentialSealed, input.EncryptedTransport, input.ActorID))
	if err != nil {
		return PatchMonImport{}, fmt.Errorf("save PatchMon connector: %w", err)
	}

	result := PatchMonImport{Connector: connector, Targets: make([]ImportedTarget, 0, len(input.Hosts))}
	for _, host := range input.Hosts {
		var targetID, targetName string
		assignedTargetID := input.TargetAssignments[host.ID]
		err := tx.QueryRow(ctx, `
			SELECT target.id::text, target.name
			FROM cairnops_connector_bindings binding
			JOIN cairnops_targets target ON target.id = binding.target_id
			WHERE binding.connector_id = $1::uuid AND binding.external_id = $2
		`, connector.ID, host.ID).Scan(&targetID, &targetName)
		disposition := "already_imported"
		if err == nil && assignedTargetID != "" && assignedTargetID != targetID {
			return PatchMonImport{}, fmt.Errorf("%w: PatchMon host %s is already linked to another target", ErrInvalidInput, host.ID)
		}
		if err == nil {
			bindingID, enableErr := enableIntegrationBinding(ctx, tx, connector.ID, host.ID)
			if enableErr != nil {
				return PatchMonImport{}, enableErr
			}
			if sourceErr := ensureIntegrationSource(ctx, tx, bindingID, false); sourceErr != nil {
				return PatchMonImport{}, sourceErr
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			disposition = "reused"
			if assignedTargetID != "" {
				err = selectAssignedTarget(ctx, tx, assignedTargetID, &targetID, &targetName)
			} else {
				err = tx.QueryRow(ctx, `
					SELECT id::text, name
					FROM cairnops_targets
					WHERE archived_at IS NULL AND lower(btrim(name)) = lower(btrim($1))
					ORDER BY created_at, id
					LIMIT 1
				`, host.Name()).Scan(&targetID, &targetName)
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID == "" {
				description := fmt.Sprintf("Découvert par le Connecteur %s.", input.Name)
				err = tx.QueryRow(ctx, `
					INSERT INTO cairnops_targets (name, description)
					VALUES ($1, $2)
					RETURNING id::text, name
				`, host.Name(), description).Scan(&targetID, &targetName)
				disposition = "created"
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID != "" {
				return PatchMonImport{}, fmt.Errorf("%w: assigned target does not exist or is archived", ErrInvalidInput)
			}
			if err != nil {
				return PatchMonImport{}, fmt.Errorf("resolve target for PatchMon host %s: %w", host.ID, err)
			}
			metadata, err := json.Marshal(map[string]any{
				"machine_id": host.MachineID, "hostname": host.Hostname, "address": host.IP,
				"os_type": host.OSType, "os_version": host.OSVersion, "host_groups": host.HostGroups,
			})
			if err != nil {
				return PatchMonImport{}, fmt.Errorf("encode PatchMon host %s metadata: %w", host.ID, err)
			}
			var bindingID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO cairnops_connector_bindings (
					connector_id, target_id, external_id, external_name, metadata
				) VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
				ON CONFLICT (connector_id, external_id) DO UPDATE SET
					target_id = EXCLUDED.target_id,
					external_name = EXCLUDED.external_name,
					metadata = EXCLUDED.metadata,
					integration_enabled = true,
					updated_at = now()
				RETURNING id::text
			`, connector.ID, targetID, host.ID, host.Name(), metadata).Scan(&bindingID); err != nil {
				return PatchMonImport{}, fmt.Errorf("bind PatchMon host %s: %w", host.ID, err)
			}
			if err := ensureIntegrationSource(ctx, tx, bindingID, false); err != nil {
				return PatchMonImport{}, err
			}
		} else if err != nil {
			return PatchMonImport{}, fmt.Errorf("find existing PatchMon host %s: %w", host.ID, err)
		}
		result.Targets = append(result.Targets, ImportedTarget{
			ExternalID: host.ID, TargetID: targetID, TargetName: targetName, Disposition: disposition,
		})
	}
	result.Connector.BindingCount += countNewBindings(result.Targets)
	if err := tx.Commit(ctx); err != nil {
		return PatchMonImport{}, fmt.Errorf("commit PatchMon import: %w", err)
	}
	return result, nil
}

func (store *PostgresStore) ImportArgus(ctx context.Context, input PersistArgusInput) (ArgusImport, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ArgusImport{}, fmt.Errorf("begin Argus import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	connector, err := scanConnector(tx.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			remote_version, compatibility, encrypted_transport,
			last_checked_at, last_error, created_by, sync_interval_seconds
		) VALUES ('argus', $1, $2, $3, 'connected', $4, 'supported', $5, now(), '', $6::uuid, 300)
		ON CONFLICT (kind, (lower(endpoint))) DO UPDATE SET
			name = EXCLUDED.name,
			credential_sealed = EXCLUDED.credential_sealed,
			status = 'connected',
			remote_version = EXCLUDED.remote_version,
			compatibility = 'supported',
			encrypted_transport = EXCLUDED.encrypted_transport,
			last_checked_at = now(),
			last_error = '',
			next_sync_at = now(),
			lease_owner = NULL,
			lease_until = NULL,
			updated_at = now()
		RETURNING id::text, kind, name, endpoint, status, remote_version,
		          compatibility, encrypted_transport,
		          (SELECT count(*)::integer FROM cairnops_connector_bindings WHERE connector_id = cairnops_connectors.id), 0,
		          last_checked_at, last_error, created_at, updated_at
	`, input.Name, input.Endpoint, input.CredentialSealed, input.Version, input.EncryptedTransport, input.ActorID))
	if err != nil {
		return ArgusImport{}, fmt.Errorf("save Argus connector: %w", err)
	}

	result := ArgusImport{Connector: connector, Targets: make([]ImportedTarget, 0, len(input.Services))}
	for _, service := range input.Services {
		var targetID, targetName string
		assignedTargetID := input.TargetAssignments[service.ID]
		err := tx.QueryRow(ctx, `
			SELECT target.id::text, target.name
			FROM cairnops_connector_bindings binding
			JOIN cairnops_targets target ON target.id = binding.target_id
			WHERE binding.connector_id = $1::uuid AND binding.external_id = $2
		`, connector.ID, service.ID).Scan(&targetID, &targetName)
		disposition := "already_imported"
		if err == nil && assignedTargetID != "" && assignedTargetID != targetID {
			return ArgusImport{}, fmt.Errorf("%w: Argus service %s is already linked to another target", ErrInvalidInput, service.ID)
		}
		if err == nil {
			bindingID, enableErr := enableIntegrationBinding(ctx, tx, connector.ID, service.ID)
			if enableErr != nil {
				return ArgusImport{}, enableErr
			}
			if sourceErr := ensureIntegrationSource(ctx, tx, bindingID, false); sourceErr != nil {
				return ArgusImport{}, sourceErr
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			disposition = "reused"
			if assignedTargetID != "" {
				err = selectAssignedTarget(ctx, tx, assignedTargetID, &targetID, &targetName)
			} else {
				err = tx.QueryRow(ctx, `
					SELECT id::text, name
					FROM cairnops_targets
					WHERE archived_at IS NULL AND lower(btrim(name)) = lower(btrim($1))
					ORDER BY created_at, id
					LIMIT 1
				`, service.Name).Scan(&targetID, &targetName)
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID == "" {
				description := fmt.Sprintf("Découvert par le Connecteur %s.", input.Name)
				err = tx.QueryRow(ctx, `
					INSERT INTO cairnops_targets (name, description)
					VALUES ($1, $2)
					RETURNING id::text, name
				`, service.Name, description).Scan(&targetID, &targetName)
				disposition = "created"
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID != "" {
				return ArgusImport{}, fmt.Errorf("%w: assigned target does not exist or is archived", ErrInvalidInput)
			}
			if err != nil {
				return ArgusImport{}, fmt.Errorf("resolve target for Argus service %s: %w", service.ID, err)
			}
			metadata, err := json.Marshal(map[string]any{
				"deployed_version": service.DeployedVersion, "latest_version": service.LatestVersion,
				"approved": service.Approved, "skipped": service.Skipped,
				"last_checked": service.LastChecked, "version_url": service.VersionURL,
			})
			if err != nil {
				return ArgusImport{}, fmt.Errorf("encode Argus service %s metadata: %w", service.ID, err)
			}
			var bindingID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO cairnops_connector_bindings (
					connector_id, target_id, external_id, external_name, metadata
				) VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
				ON CONFLICT (connector_id, external_id) DO UPDATE SET
					target_id = EXCLUDED.target_id,
					external_name = EXCLUDED.external_name,
					metadata = EXCLUDED.metadata,
					integration_enabled = true,
					updated_at = now()
				RETURNING id::text
			`, connector.ID, targetID, service.ID, service.Name, metadata).Scan(&bindingID); err != nil {
				return ArgusImport{}, fmt.Errorf("bind Argus service %s: %w", service.ID, err)
			}
			if err := ensureIntegrationSource(ctx, tx, bindingID, false); err != nil {
				return ArgusImport{}, err
			}
		} else if err != nil {
			return ArgusImport{}, fmt.Errorf("find existing Argus service %s: %w", service.ID, err)
		}
		result.Targets = append(result.Targets, ImportedTarget{
			ExternalID: service.ID, TargetID: targetID, TargetName: targetName, Disposition: disposition,
		})
	}
	result.Connector.BindingCount += countNewBindings(result.Targets)
	if err := tx.Commit(ctx); err != nil {
		return ArgusImport{}, fmt.Errorf("commit Argus import: %w", err)
	}
	return result, nil
}

func (store *PostgresStore) ImportZabbix(ctx context.Context, input PersistZabbixInput) (ZabbixImport, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("begin connector import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	connector, err := scanConnector(tx.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status,
			remote_version, compatibility, encrypted_transport,
			last_checked_at, last_error, created_by
		) VALUES ('zabbix', $1, $2, $3, 'connected', $4, $5, $6, now(), '', $7::uuid)
		ON CONFLICT (kind, (lower(endpoint))) DO UPDATE SET
			name = EXCLUDED.name,
			credential_sealed = EXCLUDED.credential_sealed,
			status = 'connected',
			remote_version = EXCLUDED.remote_version,
			compatibility = EXCLUDED.compatibility,
			encrypted_transport = EXCLUDED.encrypted_transport,
			last_checked_at = now(),
			last_error = '',
			next_sync_at = now(),
			lease_owner = NULL,
			lease_until = NULL,
			updated_at = now()
		RETURNING id::text, kind, name, endpoint, status, remote_version,
		          compatibility, encrypted_transport,
		          (SELECT count(*)::integer FROM cairnops_connector_bindings WHERE connector_id = cairnops_connectors.id), 0,
		          last_checked_at, last_error, created_at, updated_at
	`, input.Name, input.Endpoint, input.CredentialSealed, input.Version,
		input.Compatibility, input.EncryptedTransport, input.ActorID))
	if err != nil {
		return ZabbixImport{}, fmt.Errorf("save Zabbix connector: %w", err)
	}

	result := ZabbixImport{Connector: connector, Targets: make([]ImportedTarget, 0, len(input.Hosts))}
	for _, host := range input.Hosts {
		var targetID, targetName string
		assignedTargetID := input.TargetAssignments[host.ID]
		err := tx.QueryRow(ctx, `
			SELECT target.id::text, target.name
			FROM cairnops_connector_bindings binding
			JOIN cairnops_targets target ON target.id = binding.target_id
			WHERE binding.connector_id = $1::uuid AND binding.external_id = $2
		`, connector.ID, host.ID).Scan(&targetID, &targetName)
		disposition := "already_imported"
		if err == nil && assignedTargetID != "" && assignedTargetID != targetID {
			return ZabbixImport{}, fmt.Errorf("%w: Zabbix host %s is already linked to another target", ErrInvalidInput, host.ID)
		}
		if err == nil {
			bindingID, enableErr := enableIntegrationBinding(ctx, tx, connector.ID, host.ID)
			if enableErr != nil {
				return ZabbixImport{}, enableErr
			}
			if sourceErr := ensureIntegrationSource(ctx, tx, bindingID, true); sourceErr != nil {
				return ZabbixImport{}, sourceErr
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			disposition = "reused"
			if assignedTargetID != "" {
				err = selectAssignedTarget(ctx, tx, assignedTargetID, &targetID, &targetName)
			} else {
				err = tx.QueryRow(ctx, `
					SELECT id::text, name
					FROM cairnops_targets
					WHERE archived_at IS NULL AND lower(btrim(name)) = lower(btrim($1))
					ORDER BY created_at, id
					LIMIT 1
				`, host.Name).Scan(&targetID, &targetName)
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID == "" {
				description := fmt.Sprintf("Découvert par le Connecteur %s.", input.Name)
				err = tx.QueryRow(ctx, `
					INSERT INTO cairnops_targets (name, description)
					VALUES ($1, $2)
					RETURNING id::text, name
				`, host.Name, description).Scan(&targetID, &targetName)
				disposition = "created"
			}
			if errors.Is(err, pgx.ErrNoRows) && assignedTargetID != "" {
				return ZabbixImport{}, fmt.Errorf("%w: assigned target does not exist or is archived", ErrInvalidInput)
			}
			if err != nil {
				return ZabbixImport{}, fmt.Errorf("resolve target for Zabbix host %s: %w", host.ID, err)
			}
			metadata, err := json.Marshal(map[string]any{
				"technical_name": host.Technical,
				"interfaces":     host.Interfaces,
			})
			if err != nil {
				return ZabbixImport{}, fmt.Errorf("encode Zabbix host %s metadata: %w", host.ID, err)
			}
			var bindingID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO cairnops_connector_bindings (
					connector_id, target_id, external_id, external_name, metadata
				) VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
				ON CONFLICT (connector_id, external_id) DO UPDATE SET
					target_id = EXCLUDED.target_id,
					external_name = EXCLUDED.external_name,
					metadata = EXCLUDED.metadata,
					integration_enabled = true,
					updated_at = now()
				RETURNING id::text
			`, connector.ID, targetID, host.ID, host.Name, metadata).Scan(&bindingID); err != nil {
				return ZabbixImport{}, fmt.Errorf("bind Zabbix host %s: %w", host.ID, err)
			}
			if err := ensureIntegrationSource(ctx, tx, bindingID, true); err != nil {
				return ZabbixImport{}, err
			}
		} else if err != nil {
			return ZabbixImport{}, fmt.Errorf("find existing Zabbix host %s: %w", host.ID, err)
		}
		result.Targets = append(result.Targets, ImportedTarget{
			ExternalID: host.ID, TargetID: targetID, TargetName: targetName, Disposition: disposition,
		})
	}
	result.Connector.BindingCount += countNewBindings(result.Targets)
	if err := tx.Commit(ctx); err != nil {
		return ZabbixImport{}, fmt.Errorf("commit Zabbix import: %w", err)
	}
	return result, nil
}

func (store *PostgresStore) ClaimDueConnector(ctx context.Context, kind, owner string, limit int, lease time.Duration) ([]RuntimeConnector, error) {
	leaseUntil := time.Now().UTC().Add(lease)
	rows, err := store.pool.Query(ctx, `
		WITH due AS (
			SELECT id
			FROM cairnops_connectors
			WHERE kind = $1 AND status <> 'disabled'
			  AND next_sync_at <= now()
			  AND (lease_until IS NULL OR lease_until < now())
			ORDER BY next_sync_at, id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE cairnops_connectors connector
		SET lease_owner = $3, lease_until = $4
		FROM due
		WHERE connector.id = due.id
		RETURNING connector.id::text, connector.endpoint, connector.credential_sealed
	`, kind, limit, owner, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("claim due %s connectors: %w", kind, err)
	}
	connectors := make([]RuntimeConnector, 0, limit)
	for rows.Next() {
		var connector RuntimeConnector
		if err := rows.Scan(&connector.ID, &connector.Endpoint, &connector.CredentialSealed); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan due %s connector: %w", kind, err)
		}
		connectors = append(connectors, connector)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate due %s connectors: %w", kind, err)
	}
	rows.Close()

	for index := range connectors {
		bindingRows, err := store.pool.Query(ctx, `
			SELECT id::text, target_id::text, external_id, external_name, metadata
			FROM cairnops_connector_bindings
			WHERE connector_id = $1::uuid AND integration_enabled
			ORDER BY external_id, id
		`, connectors[index].ID)
		if err != nil {
			return nil, fmt.Errorf("list %s runtime bindings: %w", kind, err)
		}
		connectors[index].Bindings = make([]RuntimeBinding, 0)
		for bindingRows.Next() {
			var binding RuntimeBinding
			var metadata []byte
			if err := bindingRows.Scan(&binding.ID, &binding.TargetID, &binding.ExternalID, &binding.ExternalName, &metadata); err != nil {
				bindingRows.Close()
				return nil, fmt.Errorf("scan %s runtime binding: %w", kind, err)
			}
			if err := json.Unmarshal(metadata, &binding.Metadata); err != nil {
				bindingRows.Close()
				return nil, fmt.Errorf("decode %s runtime binding metadata: %w", kind, err)
			}
			connectors[index].Bindings = append(connectors[index].Bindings, binding)
		}
		if err := bindingRows.Err(); err != nil {
			bindingRows.Close()
			return nil, fmt.Errorf("iterate %s runtime bindings: %w", kind, err)
		}
		bindingRows.Close()
	}
	return connectors, nil
}

func (store *PostgresStore) CompleteConnectorSync(ctx context.Context, connectorID, owner string, completedAt time.Time) error {
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_connectors
		SET status = 'connected', last_checked_at = $3::timestamptz, last_synced_at = $3::timestamptz,
		    last_error = '', next_sync_at = $3::timestamptz + make_interval(secs => sync_interval_seconds),
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE id = $1::uuid AND lease_owner = $2
	`, connectorID, owner, completedAt)
	if err != nil {
		return fmt.Errorf("complete connector sync: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("connector %s is no longer leased by %s", connectorID, owner)
	}
	return nil
}

// RecordIntegrationObservations enregistre ce qu'un cycle de synchronisation a
// constaté sur les Sources d'une Intégration.
//
// L'Observation nourrit la mesure et rien d'autre : elle ne passe pas par la
// Politique de déclenchement, puisque l'Incident d'une Intégration est décidé
// par le rapprochement de ses propres signaux. La Source d'une liaison
// suspendue est ignorée, faute d'être encore attendue.
func (store *PostgresStore) RecordIntegrationObservations(ctx context.Context, observedAt time.Time, observations []IntegrationObservation) error {
	if len(observations) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, observation := range observations {
		latency := 0
		if observation.LatencyMilliseconds != nil {
			latency = max(0, *observation.LatencyMilliseconds)
		}
		details := observation.Details
		if details == nil {
			details = map[string]any{}
		}
		encodedDetails, err := json.Marshal(details)
		if err != nil {
			return fmt.Errorf("encode integration observation details: %w", err)
		}
		batch.Queue(`
			WITH observed AS (
				INSERT INTO cairnops_observations (
					source_id, target_id, observed_at, outcome, latency_milliseconds, reason, message, details
				)
				SELECT source.id, source.target_id, $2::timestamptz, $3, $4, $5, $6, $7::jsonb
				FROM cairnops_signal_sources source
				WHERE source.connector_binding_id = $1::uuid AND source.enabled
				RETURNING source_id
			)
			UPDATE cairnops_signal_sources
			SET last_observed_at = $2::timestamptz,
			    last_signal_at = CASE WHEN $3 IN ('healthy', 'unhealthy')
			                          THEN $2::timestamptz ELSE last_signal_at END,
			    last_signal_outcome = CASE WHEN $3 IN ('healthy', 'unhealthy')
			                               THEN $3 ELSE last_signal_outcome END,
			    updated_at = now()
			FROM observed
			WHERE cairnops_signal_sources.id = observed.source_id
		`, observation.BindingID, observedAt.UTC(), observation.Outcome, latency,
			observation.Reason, observation.Message, encodedDetails)
	}
	results := store.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range observations {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("record integration observation: %w", err)
		}
	}
	return nil
}

func (store *PostgresStore) UpdateArgusBindings(ctx context.Context, connectorID string, snapshots []ArgusBindingSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, snapshot := range snapshots {
		metadata, err := json.Marshal(snapshot.Metadata)
		if err != nil {
			return fmt.Errorf("encode Argus binding snapshot: %w", err)
		}
		batch.Queue(`
			WITH refreshed AS (
				UPDATE cairnops_connector_bindings
				SET external_name = $3, metadata = $4::jsonb, updated_at = now()
				WHERE id = $1::uuid AND connector_id = $2::uuid AND integration_enabled
				RETURNING id
			)
			UPDATE cairnops_signal_sources source
			SET name = $3, updated_at = now()
			FROM refreshed
			WHERE source.connector_binding_id = refreshed.id
		`, snapshot.BindingID, connectorID, snapshot.ExternalName, metadata)
	}
	results := store.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range snapshots {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("update Argus binding snapshot: %w", err)
		}
	}
	return nil
}

func (store *PostgresStore) FailConnectorSync(ctx context.Context, connectorID, owner string, failedAt time.Time, message string) error {
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_connectors
		SET status = 'degraded', last_checked_at = $3::timestamptz, last_error = $4,
		    next_sync_at = $3::timestamptz + make_interval(secs => sync_interval_seconds),
		    lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE id = $1::uuid AND lease_owner = $2
	`, connectorID, owner, failedAt, message)
	if err != nil {
		return fmt.Errorf("fail connector sync: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("connector %s is no longer leased by %s", connectorID, owner)
	}
	return nil
}

func (store *PostgresStore) RuntimeCredential(ctx context.Context, connectorID string) (RuntimeCredential, error) {
	var credential RuntimeCredential
	if err := store.pool.QueryRow(ctx, `
		SELECT kind, endpoint, credential_sealed
		FROM cairnops_connectors
		WHERE id = $1::uuid AND status <> 'disabled'
	`, connectorID).Scan(&credential.Kind, &credential.Endpoint, &credential.CredentialSealed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RuntimeCredential{}, fmt.Errorf("connector is unavailable")
		}
		return RuntimeCredential{}, fmt.Errorf("load connector credential: %w", err)
	}
	return credential, nil
}

type scanner interface {
	Scan(...any) error
}

func scanConnector(row scanner) (Connector, error) {
	var connector Connector
	if err := row.Scan(
		&connector.ID, &connector.Kind, &connector.Name, &connector.Endpoint,
		&connector.Status, &connector.RemoteVersion, &connector.Compatibility,
		&connector.EncryptedTransport, &connector.BindingCount, &connector.QuarantineCount,
		&connector.LastCheckedAt, &connector.LastError,
		&connector.CreatedAt, &connector.UpdatedAt,
	); err != nil {
		return Connector{}, fmt.Errorf("scan connector: %w", err)
	}
	return connector, nil
}

func countNewBindings(targets []ImportedTarget) int {
	count := 0
	for _, target := range targets {
		if !strings.EqualFold(target.Disposition, "already_imported") {
			count++
		}
	}
	return count
}
