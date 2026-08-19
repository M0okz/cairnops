CREATE TABLE cairnops_connectors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('zabbix', 'uptime_kuma', 'generic_webhook')),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    endpoint text NOT NULL CHECK (length(btrim(endpoint)) BETWEEN 1 AND 2048),
    credential_sealed text NOT NULL CHECK (length(credential_sealed) BETWEEN 32 AND 16384),
    status text NOT NULL CHECK (status IN ('connected', 'degraded', 'disabled')),
    remote_version text NOT NULL DEFAULT '',
    compatibility text NOT NULL DEFAULT 'warning' CHECK (compatibility IN ('supported', 'warning')),
    encrypted_transport boolean NOT NULL,
    last_checked_at timestamptz NOT NULL DEFAULT now(),
    last_error text NOT NULL DEFAULT '',
    created_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX cairnops_connectors_endpoint_idx
    ON cairnops_connectors (kind, lower(endpoint));

CREATE TABLE cairnops_connector_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    external_id text NOT NULL CHECK (length(btrim(external_id)) BETWEEN 1 AND 255),
    external_name text NOT NULL CHECK (length(btrim(external_name)) BETWEEN 1 AND 160),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (connector_id, external_id)
);

CREATE INDEX cairnops_connector_bindings_target_idx
    ON cairnops_connector_bindings (target_id);

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed',
    'source.changed',
    'observation.created',
    'component.heartbeat',
    'connector.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN ('target', 'source', 'component', 'connector')
);

CREATE FUNCTION cairnops_connector_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'connector.changed',
        'connector',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_connectors_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_connectors
FOR EACH ROW EXECUTE FUNCTION cairnops_connector_changed_event();

CREATE FUNCTION cairnops_connector_binding_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'connector.changed',
        'connector',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.connector_id::text ELSE NEW.connector_id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_connector_bindings_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_connector_bindings
FOR EACH ROW EXECUTE FUNCTION cairnops_connector_binding_changed_event();
