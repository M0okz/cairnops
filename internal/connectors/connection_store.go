package connectors

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ReplaceConnection updates only access settings. In particular it retains
// bindings, discovery decisions, suspension, and managed-account ownership.
// The old encrypted credential acts as a revision, including for name-only
// changes: every successful test seals a fresh replacement.
func (store *PostgresStore) ReplaceConnection(ctx context.Context, input connectionReplacement) (Connector, error) {
	var management, cleanupEndpoint string
	result, err := scanConnector(store.pool.QueryRow(ctx, `
  UPDATE cairnops_connectors SET name=$5, endpoint=$6, credential_sealed=$7,
   managed_cleanup_endpoint=CASE WHEN credential_management='managed' AND managed_cleanup_endpoint='' AND endpoint<>$6 THEN endpoint ELSE managed_cleanup_endpoint END,
   managed_cleanup_credential_sealed=CASE
    WHEN credential_management='managed' AND managed_cleanup_endpoint='' AND endpoint<>$6 THEN credential_sealed
    WHEN managed_cleanup_endpoint=$6 THEN $7 ELSE managed_cleanup_credential_sealed END,
   remote_version=$8, compatibility=$9, encrypted_transport=$10,
   status=CASE WHEN status='disabled' THEN status ELSE 'connected' END,
   last_checked_at=now(), last_error='', next_sync_at=now(),
   lease_owner=NULL, lease_until=NULL, updated_at=now()
  WHERE id=$1::uuid AND kind=$2 AND endpoint=$3 AND credential_sealed=$4 AND lease_owner IS NULL
  RETURNING id::text, kind, name, endpoint, status, remote_version, compatibility, encrypted_transport,
   (SELECT count(*)::integer FROM cairnops_connector_bindings b WHERE b.connector_id=cairnops_connectors.id AND b.integration_enabled),
   (SELECT count(*)::integer FROM cairnops_connector_inventory i WHERE i.connector_id=cairnops_connectors.id AND i.pending AND i.present)
   + (SELECT count(*)::integer FROM cairnops_proxmox_inventory i WHERE i.connector_id=cairnops_connectors.id AND i.pending AND i.present),
   last_checked_at, last_error, created_at, updated_at, credential_management, managed_cleanup_endpoint
 `, input.ConnectorID, input.Kind, input.PreviousEndpoint, input.PreviousCredential,
		input.Name, input.Endpoint, input.CredentialSealed, input.Version, input.Compatibility, input.EncryptedTransport), &management, &cleanupEndpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		var leased bool
		checkErr := store.pool.QueryRow(ctx, `SELECT lease_owner IS NOT NULL FROM cairnops_connectors
            WHERE id=$1::uuid AND kind=$2 AND endpoint=$3 AND credential_sealed=$4`, input.ConnectorID, input.Kind, input.PreviousEndpoint, input.PreviousCredential).Scan(&leased)
		if checkErr == nil && leased {
			return Connector{}, ErrConnectionBusy
		}
		if checkErr != nil && !errors.Is(checkErr, pgx.ErrNoRows) {
			return Connector{}, checkErr
		}
		return Connector{}, ErrConnectionChanged
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Connector{}, ErrEndpointConflict
	}
	if err != nil {
		return Connector{}, fmt.Errorf("replace connector connection: %w", err)
	}
	result.CredentialManagement = management
	result.ManagedCleanupEndpoint = cleanupEndpoint
	return result, nil
}
