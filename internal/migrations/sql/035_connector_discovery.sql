ALTER TABLE cairnops_connectors ADD COLUMN discovery_initialized boolean NOT NULL DEFAULT false;

CREATE TABLE cairnops_connector_inventory (
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    external_id text NOT NULL CHECK (length(external_id) BETWEEN 1 AND 512),
    excluded boolean NOT NULL DEFAULT false,
    pending boolean NOT NULL DEFAULT false,
    present boolean NOT NULL DEFAULT true,
    discovered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (connector_id, external_id)
);
