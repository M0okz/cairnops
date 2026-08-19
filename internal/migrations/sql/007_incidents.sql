ALTER TABLE cairnops_connectors
    ADD COLUMN sync_interval_seconds integer NOT NULL DEFAULT 30
        CHECK (sync_interval_seconds BETWEEN 20 AND 86400),
    ADD COLUMN next_sync_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN last_synced_at timestamptz,
    ADD COLUMN lease_owner text,
    ADD COLUMN lease_until timestamptz;

CREATE INDEX cairnops_connectors_due_idx
    ON cairnops_connectors (next_sync_at)
    WHERE status <> 'disabled';

DROP TRIGGER cairnops_connectors_changed ON cairnops_connectors;
CREATE TRIGGER cairnops_connectors_changed
AFTER INSERT OR DELETE OR UPDATE OF name, status, last_error, remote_version,
    compatibility, encrypted_transport, sync_interval_seconds
ON cairnops_connectors
FOR EACH ROW EXECUTE FUNCTION cairnops_connector_changed_event();

CREATE TABLE cairnops_incidents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    nature_key text NOT NULL CHECK (length(btrim(nature_key)) BETWEEN 1 AND 255),
    nature_label text NOT NULL CHECK (length(btrim(nature_label)) BETWEEN 1 AND 512),
    status text NOT NULL CHECK (status IN ('active', 'resolved')),
    source_severity text NOT NULL CHECK (source_severity IN ('information', 'warning', 'major', 'critical')),
    effective_severity text NOT NULL CHECK (effective_severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    resolved_at timestamptz,
    acknowledged_at timestamptz,
    acknowledged_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    acknowledgement_origin text CHECK (
        acknowledgement_origin IS NULL OR acknowledgement_origin IN ('user', 'connector')
    ),
    acknowledgement_sync_status text NOT NULL DEFAULT 'not_applicable' CHECK (
        acknowledgement_sync_status IN ('not_applicable', 'pending', 'synchronized', 'failed')
    ),
    acknowledgement_sync_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'active' AND resolved_at IS NULL) OR (status = 'resolved' AND resolved_at IS NOT NULL)),
    CHECK ((acknowledged_at IS NULL AND acknowledgement_origin IS NULL) OR acknowledged_at IS NOT NULL)
);

CREATE UNIQUE INDEX cairnops_incidents_active_nature_idx
    ON cairnops_incidents (target_id, nature_key)
    WHERE status = 'active';

CREATE INDEX cairnops_incidents_status_time_idx
    ON cairnops_incidents (status, opened_at DESC);

CREATE TABLE cairnops_incident_signals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    origin text NOT NULL CHECK (origin IN ('zabbix', 'uptime_kuma', 'webhook', 'native')),
    connector_id uuid REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    connector_binding_id uuid REFERENCES cairnops_connector_bindings(id) ON DELETE CASCADE,
    source_id uuid REFERENCES cairnops_signal_sources(id) ON DELETE CASCADE,
    external_event_id text NOT NULL DEFAULT '',
    external_object_id text NOT NULL DEFAULT '',
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 512),
    active boolean NOT NULL,
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    resolved_at timestamptz,
    upstream_acknowledged boolean NOT NULL DEFAULT false,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((active AND resolved_at IS NULL) OR (NOT active AND resolved_at IS NOT NULL)),
    CHECK (
        (origin IN ('zabbix', 'uptime_kuma') AND connector_id IS NOT NULL AND connector_binding_id IS NOT NULL
            AND length(btrim(external_event_id)) > 0)
        OR (origin = 'native' AND source_id IS NOT NULL)
        OR origin = 'webhook'
    )
);

CREATE UNIQUE INDEX cairnops_incident_signals_external_idx
    ON cairnops_incident_signals (origin, connector_id, connector_binding_id, external_event_id)
    WHERE connector_id IS NOT NULL;

CREATE INDEX cairnops_incident_signals_incident_active_idx
    ON cairnops_incident_signals (incident_id, active);

CREATE TABLE cairnops_incident_activity (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN (
        'opened', 'signal_added', 'signal_resolved', 'resolved',
        'acknowledged', 'ack_sync_succeeded', 'ack_sync_failed', 'upstream_acknowledged'
    )),
    origin text NOT NULL CHECK (origin IN ('cairnops', 'zabbix', 'uptime_kuma', 'webhook', 'user')),
    actor_id uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    message text NOT NULL DEFAULT '',
    data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_incident_activity_incident_time_idx
    ON cairnops_incident_activity (incident_id, occurred_at DESC, id DESC);

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed',
    'source.changed',
    'observation.created',
    'component.heartbeat',
    'connector.changed',
    'incident.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN ('target', 'source', 'component', 'connector', 'incident')
);

CREATE FUNCTION cairnops_incident_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'incident.changed',
        'incident',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_incidents_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_incidents
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_changed_event();

CREATE FUNCTION cairnops_incident_child_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'incident.changed',
        'incident',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.incident_id::text ELSE NEW.incident_id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_incident_signals_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_incident_signals
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_child_changed_event();

CREATE TRIGGER cairnops_incident_activity_changed
AFTER INSERT OR DELETE ON cairnops_incident_activity
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_child_changed_event();
