CREATE TABLE cairnops_notification_channels (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('mattermost')),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    endpoint text NOT NULL CHECK (length(btrim(endpoint)) BETWEEN 1 AND 512),
    credential_sealed text NOT NULL CHECK (length(credential_sealed) > 0),
    severities text[] NOT NULL CHECK (
        cardinality(severities) BETWEEN 1 AND 4
        AND severities <@ ARRAY['information', 'warning', 'major', 'critical']::text[]
    ),
    enabled boolean NOT NULL DEFAULT true,
    status text NOT NULL DEFAULT 'connected' CHECK (status IN ('connected', 'degraded', 'disabled')),
    encrypted_transport boolean NOT NULL DEFAULT true,
    last_checked_at timestamptz NOT NULL DEFAULT now(),
    last_error text NOT NULL DEFAULT '',
    created_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cairnops_notification_outbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    channel_id uuid NOT NULL REFERENCES cairnops_notification_channels(id) ON DELETE CASCADE,
    event_kind text NOT NULL CHECK (event_kind IN ('firing', 'resolved')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'cancelled')),
    target_name text NOT NULL,
    nature_label text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    resolved_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    delivered_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (incident_id, channel_id, event_kind),
    CHECK ((event_kind = 'firing' AND resolved_at IS NULL) OR event_kind = 'resolved')
);

CREATE INDEX cairnops_notification_outbox_due_idx
    ON cairnops_notification_outbox (next_attempt_at, id)
    WHERE status IN ('pending', 'failed');

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed',
    'source.changed',
    'observation.created',
    'component.heartbeat',
    'connector.changed',
    'incident.changed',
    'maintenance.changed',
    'notification.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN ('target', 'source', 'component', 'connector', 'incident', 'maintenance', 'notification')
);

CREATE FUNCTION cairnops_notification_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'notification.changed',
        'notification',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_notification_channels_changed
AFTER INSERT OR UPDATE OF name, severities, enabled, status, last_error OR DELETE
ON cairnops_notification_channels
FOR EACH ROW EXECUTE FUNCTION cairnops_notification_changed_event();
