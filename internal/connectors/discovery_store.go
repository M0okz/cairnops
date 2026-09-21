package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Save the whole confirmed inventory, including deliberately unselected objects.
// Ineligible objects may become eligible later and are not exclusions.
func rememberDiscoverySelection(ctx context.Context, tx pgx.Tx, connectorID string, objects []DiscoveryObject) error {
	if objects == nil {
		return nil
	}
	ids := make([]string, 0, len(objects))
	eligible := make([]bool, 0, len(objects))
	for _, object := range objects {
		ids = append(ids, object.ExternalID)
		eligible = append(eligible, object.Eligible)
	}
	_, err := tx.Exec(ctx, `INSERT INTO cairnops_connector_inventory (connector_id,external_id,excluded)
 SELECT $1::uuid,d.external_id,d.eligible AND NOT EXISTS(SELECT 1 FROM cairnops_connector_bindings b WHERE b.connector_id=$1::uuid AND b.external_id=d.external_id)
 FROM unnest($2::text[],$3::boolean[]) AS d(external_id,eligible)
 ON CONFLICT (connector_id,external_id) DO UPDATE SET excluded=EXCLUDED.excluded,pending=false,present=true`, connectorID, ids, eligible)
	if err != nil {
		return fmt.Errorf("remember connector selection: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE cairnops_connectors SET discovery_initialized=true WHERE id=$1::uuid`, connectorID)
	return err
}

// RefreshDiscovery adds genuinely new identities and returns the updated active
// bindings for observation during this same cycle. Everything is atomic and
// guarded by the worker's lease, including the first inventory after migration.
func (store *PostgresStore) RefreshDiscovery(ctx context.Context, connector RuntimeConnector, owner, kind string, objects []DiscoveryObject) ([]RuntimeBinding, error) {
	if kind != "zabbix" && kind != "uptime_kuma" && kind != "patchmon" && kind != "argus" {
		return nil, fmt.Errorf("unsupported discovery kind %q", kind)
	}
	seen := map[string]bool{}
	for _, o := range objects {
		if strings.TrimSpace(o.ExternalID) == "" || len(o.ExternalID) > 512 || strings.TrimSpace(o.Name) == "" || len(o.Name) > 160 || seen[o.ExternalID] {
			return nil, fmt.Errorf("invalid or duplicate discovery identity")
		}
		seen[o.ExternalID] = true
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var initialized bool
	err = tx.QueryRow(ctx, `SELECT discovery_initialized FROM cairnops_connectors WHERE id=$1::uuid AND kind=$3 AND lease_owner=$2 AND lease_until>now() AND status<>'disabled' FOR UPDATE`, connector.ID, owner, kind).Scan(&initialized)
	if err != nil {
		return nil, fmt.Errorf("discovery lease expired or connector suspended: %w", err)
	}
	if !initialized {
		if objects == nil {
			objects = []DiscoveryObject{}
		}
		// An old connector has no record of exclusions. Preserve its current selection.
		if err := rememberDiscoverySelection(ctx, tx, connector.ID, objects); err != nil {
			return nil, err
		}
	}
	// Serialize identity matching with concurrent target creation, including other
	// connector families. Use the same transaction/connection for its snapshot.
	if _, err = tx.Exec(ctx, `LOCK TABLE cairnops_targets IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, err
	}
	state, err := previewState(ctx, tx, kind, connector.Endpoint, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE cairnops_connector_inventory SET present=false WHERE connector_id=$1::uuid`, connector.ID); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(objects))
	for _, o := range objects {
		ids = append(ids, o.ExternalID)
	}
	inventoryRows, err := tx.Query(ctx, `INSERT INTO cairnops_connector_inventory (connector_id,external_id)
 SELECT $1::uuid,unnest($2::text[]) ON CONFLICT (connector_id,external_id) DO UPDATE SET present=true RETURNING external_id,excluded`, connector.ID, ids)
	if err != nil {
		return nil, err
	}
	excludedIDs := map[string]bool{}
	for inventoryRows.Next() {
		var id string
		var excluded bool
		if err = inventoryRows.Scan(&id, &excluded); err != nil {
			inventoryRows.Close()
			return nil, err
		}
		excludedIDs[id] = excluded
	}
	err = inventoryRows.Err()
	inventoryRows.Close()
	if err != nil {
		return nil, err
	}
	boundRows, err := tx.Query(ctx, `SELECT external_id FROM cairnops_connector_bindings WHERE connector_id=$1::uuid`, connector.ID)
	if err != nil {
		return nil, err
	}
	boundIDs := map[string]bool{}
	for boundRows.Next() {
		var id string
		if err = boundRows.Scan(&id); err != nil {
			boundRows.Close()
			return nil, err
		}
		boundIDs[id] = true
	}
	err = boundRows.Err()
	boundRows.Close()
	if err != nil {
		return nil, err
	}
	pendingIDs := []string{}
	for _, o := range objects {
		pending := false
		if !excludedIDs[o.ExternalID] && !boundIDs[o.ExternalID] && o.Eligible {
			pending = len(matchTargets(o.Identity, state.Targets)) > 0
			if !pending {
				var targetID, bindingID string
				if err = tx.QueryRow(ctx, `INSERT INTO cairnops_targets (name,description) VALUES ($1,'Découvert automatiquement par un Connecteur.') RETURNING id::text`, o.Name).Scan(&targetID); err != nil {
					return nil, err
				}
				metadata, encodeErr := json.Marshal(o.Metadata)
				if encodeErr != nil {
					return nil, encodeErr
				}
				if err = tx.QueryRow(ctx, `INSERT INTO cairnops_connector_bindings (connector_id,target_id,external_id,external_name,metadata) VALUES ($1::uuid,$2::uuid,$3,$4,$5::jsonb) RETURNING id::text`, connector.ID, targetID, o.ExternalID, o.Name, metadata).Scan(&bindingID); err != nil {
					return nil, err
				}
				if err = ensureIntegrationSource(ctx, tx, bindingID, kind == "zabbix" || kind == "uptime_kuma"); err != nil {
					return nil, err
				}
				state.Targets = append(state.Targets, TargetIdentity{TargetReference: TargetReference{ID: targetID, Name: o.Name}, Names: o.Identity.Names, Addresses: o.Identity.Addresses, Identifiers: o.Identity.Identifiers})
			}
		}
		if pending {
			pendingIDs = append(pendingIDs, o.ExternalID)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE cairnops_connector_inventory SET pending=(external_id=ANY($2::text[])) WHERE connector_id=$1::uuid`, connector.ID, pendingIDs); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT b.id::text,b.target_id::text,b.external_id,b.external_name,b.metadata FROM cairnops_connector_bindings b JOIN cairnops_targets t ON t.id=b.target_id WHERE b.connector_id=$1::uuid AND b.integration_enabled AND t.archived_at IS NULL ORDER BY b.external_id`, connector.ID)
	if err != nil {
		return nil, err
	}
	bindings := []RuntimeBinding{}
	for rows.Next() {
		var b RuntimeBinding
		var raw []byte
		if err = rows.Scan(&b.ID, &b.TargetID, &b.ExternalID, &b.ExternalName, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(raw, &b.Metadata); err != nil {
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
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return bindings, nil
}
