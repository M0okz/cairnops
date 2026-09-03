ALTER TABLE cairnops_connectors
    ADD COLUMN credential_management text NOT NULL DEFAULT 'provided'
        CHECK (credential_management IN ('provided', 'managed')),
    ADD COLUMN managed_credential_id text NOT NULL DEFAULT ''
        CHECK (length(managed_credential_id) <= 512),
    ADD CONSTRAINT cairnops_connectors_managed_credential_check CHECK (
        (credential_management = 'provided' AND managed_credential_id = '')
        OR
        (credential_management = 'managed' AND length(btrim(managed_credential_id)) > 0)
    );

DROP TRIGGER cairnops_connectors_changed ON cairnops_connectors;
CREATE TRIGGER cairnops_connectors_changed
AFTER INSERT OR DELETE OR UPDATE OF name, status, last_error, remote_version,
    compatibility, encrypted_transport, sync_interval_seconds,
    credential_management, managed_credential_id
ON cairnops_connectors
FOR EACH ROW EXECUTE FUNCTION cairnops_connector_changed_event();
