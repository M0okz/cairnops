-- An address change must not redirect cleanup of an account CairnOps created.
-- The original credential remains encrypted under its original endpoint and
-- carries the certificate pin needed by the existing explicit cleanup flow.
ALTER TABLE cairnops_connectors
    ADD COLUMN managed_cleanup_endpoint text NOT NULL DEFAULT '',
    ADD COLUMN managed_cleanup_credential_sealed text NOT NULL DEFAULT '',
    ADD CONSTRAINT cairnops_connectors_cleanup_origin_check CHECK (
        (managed_cleanup_endpoint = '' AND managed_cleanup_credential_sealed = '')
        OR (credential_management = 'managed'
            AND length(managed_cleanup_endpoint) > 0
            AND length(managed_cleanup_credential_sealed) > 0)
    );
