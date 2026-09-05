package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/M0okz/cairnops/internal/connectors/proxmox"
	"github.com/jackc/pgx/v5"
)

func (store *PostgresStore) ProxmoxSettings(ctx context.Context, endpoint string) (map[string]bool, error) {
	rows, err := store.pool.Query(ctx, `SELECT binding.external_id, coalesce((binding.metadata->>'expected_running')::boolean, false)
		FROM cairnops_connector_bindings binding JOIN cairnops_connectors connector ON connector.id = binding.connector_id
		WHERE connector.kind = 'proxmox' AND lower(connector.endpoint) = lower($1)`, endpoint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var id string
		var expected bool
		if err := rows.Scan(&id, &expected); err != nil {
			return nil, err
		}
		result[id] = expected
	}
	return result, rows.Err()
}

func (store *PostgresStore) RemovalCredential(ctx context.Context, id string) (RuntimeCredential, error) {
	var result RuntimeCredential
	err := store.pool.QueryRow(ctx, `SELECT kind, endpoint, credential_sealed, credential_management, managed_credential_id FROM cairnops_connectors WHERE id = $1::uuid`, id).Scan(&result.Kind, &result.Endpoint, &result.CredentialSealed, &result.CredentialManagement, &result.ManagedCredentialID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	return result, err
}

func (store *PostgresStore) ReplaceProxmoxCredential(ctx context.Context, id, previous, replacement string) error {
	result, err := store.pool.Exec(ctx, `UPDATE cairnops_connectors SET credential_sealed = $3, next_sync_at = now(), lease_owner = NULL, lease_until = NULL, updated_at = now()
		WHERE id = $1::uuid AND kind = 'proxmox' AND credential_sealed = $2`, id, previous, replacement)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: connector changed during certificate approval", ErrInvalidInput)
	}
	return nil
}

func (store *PostgresStore) ImportProxmox(ctx context.Context, input PersistProxmoxInput) (ProxmoxImport, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ProxmoxImport{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	management := "provided"
	if input.ManagedUserID != "" {
		management = "managed"
	}
	var id string
	if input.ExistingID == "" {
		err = tx.QueryRow(ctx, `INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, status, remote_version, compatibility, encrypted_transport, last_checked_at, created_by, sync_interval_seconds, credential_management, managed_credential_id)
			VALUES ('proxmox', $1, $2, $3, 'connected', $4, 'supported', true, now(), $5::uuid, 60, $6, $7)
			ON CONFLICT (kind, (lower(endpoint))) DO NOTHING RETURNING id::text`, input.Name, input.Endpoint, input.CredentialSealed, input.Version, input.ActorID, management, input.ManagedUserID).Scan(&id)
	} else {
		// Reopening the inventory preserves the existing permanent credential.
		// A concurrent deletion/recreation cannot silently adopt an old receipt.
		err = tx.QueryRow(ctx, `UPDATE cairnops_connectors SET name = $2, remote_version = $4, updated_at = now(), next_sync_at = now(), lease_owner = NULL, lease_until = NULL
			WHERE id = $1::uuid AND kind = 'proxmox' AND endpoint = $3 AND managed_credential_id = $5 AND status <> 'disabled' RETURNING id::text`, input.ExistingID, input.Name, input.Endpoint, input.Version, input.ManagedUserID).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ProxmoxImport{}, fmt.Errorf("%w: connector changed; reopen its inventory", ErrInvalidInput)
	}
	if err != nil {
		return ProxmoxImport{}, fmt.Errorf("save Proxmox VE connector: %w", err)
	}
	result := ProxmoxImport{Targets: make([]ImportedTarget, 0, len(input.Resources))}
	for _, resource := range input.Resources {
		target, err := bindProxmoxResource(ctx, tx, id, resource, input.ExpectedRunning[resource.ID], input.TargetAssignments[resource.ID])
		if err != nil {
			return ProxmoxImport{}, err
		}
		result.Targets = append(result.Targets, target)
	}
	// Remember unselected objects too: auto-discovery must not immediately
	// import the resources the administrator deliberately left unselected.
	for _, resource := range input.Inventory {
		if _, err := tx.Exec(ctx, `INSERT INTO cairnops_proxmox_inventory (connector_id, external_id, pending)
			VALUES ($1::uuid, $2, false) ON CONFLICT (connector_id, external_id) DO NOTHING`, id, resource.ID); err != nil {
			return ProxmoxImport{}, err
		}
	}
	for _, resource := range input.Resources {
		if _, err := tx.Exec(ctx, `UPDATE cairnops_proxmox_inventory SET pending = false WHERE connector_id = $1::uuid AND external_id = $2`, id, resource.ID); err != nil {
			return ProxmoxImport{}, err
		}
	}
	result.Connector, err = scanConnector(tx.QueryRow(ctx, `SELECT id::text, kind, name, endpoint, status, remote_version, compatibility, encrypted_transport,
		(SELECT count(*)::integer FROM cairnops_connector_bindings WHERE connector_id = $1::uuid AND integration_enabled),
		(SELECT count(*)::integer FROM cairnops_proxmox_inventory WHERE connector_id = $1::uuid AND pending), last_checked_at, last_error, created_at, updated_at
		FROM cairnops_connectors WHERE id = $1::uuid`, id))
	if err != nil {
		return ProxmoxImport{}, err
	}
	result.Connector.CredentialManagement = management
	if err := tx.Commit(ctx); err != nil {
		return ProxmoxImport{}, err
	}
	return result, nil
}

func bindProxmoxResource(ctx context.Context, tx pgx.Tx, connectorID string, resource proxmox.Resource, expected bool, assigned string) (ImportedTarget, error) {
	result := ImportedTarget{ExternalID: resource.ID, Disposition: "already_imported"}
	var bindingID string
	err := tx.QueryRow(ctx, `SELECT b.id::text, t.id::text, t.name FROM cairnops_connector_bindings b JOIN cairnops_targets t ON t.id = b.target_id
		WHERE b.connector_id = $1::uuid AND b.external_id = $2 FOR UPDATE OF b`, connectorID, resource.ID).Scan(&bindingID, &result.TargetID, &result.TargetName)
	if err == nil && assigned != "" && assigned != result.TargetID {
		return result, fmt.Errorf("%w: resource already belongs to another target", ErrInvalidInput)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if assigned != "" {
			err = selectAssignedTarget(ctx, tx, assigned, &result.TargetID, &result.TargetName)
			result.Disposition = "reused"
			if errors.Is(err, pgx.ErrNoRows) {
				return result, fmt.Errorf("%w: assigned target is missing or archived", ErrInvalidInput)
			}
		} else {
			// No automatic name attachment: a blank assignment means create.
			err = tx.QueryRow(ctx, `INSERT INTO cairnops_targets (name, description) VALUES ($1, 'Découvert par Proxmox VE.') RETURNING id::text, name`, resource.Name).Scan(&result.TargetID, &result.TargetName)
			result.Disposition = "created"
		}
	}
	if err != nil {
		return result, err
	}
	metadata := resource.Metadata()
	metadata["expected_running"] = expected
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return result, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name, metadata)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
		ON CONFLICT (connector_id, external_id) DO UPDATE SET external_name = EXCLUDED.external_name, metadata = EXCLUDED.metadata, integration_enabled = true, updated_at = now()
		RETURNING id::text`, connectorID, result.TargetID, resource.ID, resource.Name, encoded).Scan(&bindingID)
	if err != nil {
		return result, err
	}
	if err := ensureIntegrationSource(ctx, tx, bindingID, resource.Type == "node" || resource.Guest() && expected); err != nil {
		return result, err
	}
	return result, nil
}

// RefreshProxmox records topology changes by stable resource identity. New
// objects with plausible matches stay pending until explicitly reconciled.
func (store *PostgresStore) RefreshProxmox(ctx context.Context, connector RuntimeConnector, owner string, resources []proxmox.Resource) ([]RuntimeBinding, error) {
	state, err := store.PreviewState(ctx, "proxmox", connector.Endpoint, nil)
	if err != nil {
		return nil, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var lockedID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM cairnops_connectors WHERE id = $1::uuid AND lease_owner = $2 AND lease_until > now() AND status <> 'disabled' FOR UPDATE`, connector.ID, owner).Scan(&lockedID)
	if err != nil {
		return nil, fmt.Errorf("Proxmox VE lease expired or connector suspended: %w", err)
	}
	for _, resource := range resources {
		var known bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cairnops_proxmox_inventory WHERE connector_id = $1::uuid AND external_id = $2)`, connector.ID, resource.ID).Scan(&known); err != nil {
			return nil, err
		}
		pending := false
		if !known && resource.Importable() {
			pending = len(matchTargets(DiscoveredIdentity{Names: []string{resource.Name}}, state.Targets)) > 0
			if !pending {
				target, err := bindProxmoxResource(ctx, tx, connector.ID, resource, false, "")
				if err != nil {
					return nil, err
				}
				state.Targets = append(state.Targets, TargetIdentity{TargetReference: TargetReference{ID: target.TargetID, Name: target.TargetName}, Names: []string{target.TargetName}})
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO cairnops_proxmox_inventory (connector_id, external_id, pending) VALUES ($1::uuid, $2, $3) ON CONFLICT (connector_id, external_id) DO NOTHING`, connector.ID, resource.ID, pending); err != nil {
			return nil, err
		}
		metadata, err := json.Marshal(resource.Metadata())
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE cairnops_connector_bindings SET external_name = $3, metadata = metadata || $4::jsonb || '{"missing":false}'::jsonb, updated_at = now()
			WHERE connector_id = $1::uuid AND external_id = $2`, connector.ID, resource.ID, resource.Name, metadata); err != nil {
			return nil, err
		}
	}
	ids := make([]string, 0, len(resources))
	for _, resource := range resources {
		ids = append(ids, resource.ID)
	}
	if _, err := tx.Exec(ctx, `UPDATE cairnops_connector_bindings SET metadata = metadata || '{"missing":true}'::jsonb
		WHERE connector_id = $1::uuid AND NOT (external_id = ANY($2::text[]))`, connector.ID, ids); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text, target_id::text, external_id, external_name, metadata FROM cairnops_connector_bindings WHERE connector_id = $1::uuid AND integration_enabled ORDER BY external_id`, connector.ID)
	if err != nil {
		return nil, err
	}
	bindings := []RuntimeBinding{}
	for rows.Next() {
		var b RuntimeBinding
		var raw []byte
		if err := rows.Scan(&b.ID, &b.TargetID, &b.ExternalID, &b.ExternalName, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(raw, &b.Metadata); err != nil {
			rows.Close()
			return nil, err
		}
		bindings = append(bindings, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return bindings, nil
}
