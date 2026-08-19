ALTER TABLE cairnops_connectors
    ADD COLUMN webhook_public_id text
        CHECK (webhook_public_id IS NULL OR length(webhook_public_id) = 32);

CREATE UNIQUE INDEX cairnops_connectors_webhook_public_id_idx
    ON cairnops_connectors (webhook_public_id)
    WHERE webhook_public_id IS NOT NULL;

CREATE TABLE cairnops_webhook_quarantine (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    external_identity text NOT NULL CHECK (length(btrim(external_identity)) BETWEEN 1 AND 255),
    target_name text NOT NULL CHECK (length(btrim(target_name)) BETWEEN 1 AND 160),
    external_event_key text NOT NULL CHECK (length(btrim(external_event_key)) BETWEEN 1 AND 255),
    nature_key text NOT NULL CHECK (length(btrim(nature_key)) BETWEEN 1 AND 255),
    nature_label text NOT NULL CHECK (length(btrim(nature_label)) BETWEEN 1 AND 512),
    status text NOT NULL CHECK (status IN ('firing', 'resolved')),
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    summary text NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 512),
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    occurrences integer NOT NULL DEFAULT 1 CHECK (occurrences > 0),
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    approved_at timestamptz,
    approved_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    UNIQUE (connector_id, external_identity, external_event_key)
);

CREATE INDEX cairnops_webhook_quarantine_pending_idx
    ON cairnops_webhook_quarantine (connector_id, last_seen_at DESC)
    WHERE approved_at IS NULL;

CREATE TRIGGER cairnops_webhook_quarantine_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_webhook_quarantine
FOR EACH ROW EXECUTE FUNCTION cairnops_connector_binding_changed_event();

CREATE UNIQUE INDEX cairnops_incident_signals_webhook_active_idx
    ON cairnops_incident_signals (connector_id, connector_binding_id, external_object_id)
    WHERE origin = 'webhook' AND active;
