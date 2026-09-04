-- CairnOps est encore en développement : l'ADR 0042 autorise une bascule
-- franche sans conserver les données opérationnelles de l'ancien couple
-- Incident/Rafale. Les identités, Cibles, Sources et Connecteurs restent
-- intactes ; seuls les Incidents, notifications et instantanés associés sont
-- remis à zéro.

DROP TRIGGER IF EXISTS cairnops_indicator_incident_snapshot ON cairnops_context_indicators;
DROP TRIGGER IF EXISTS cairnops_incident_indicator_snapshot ON cairnops_incidents;
DROP FUNCTION IF EXISTS cairnops_capture_indicator_for_incidents();
DROP FUNCTION IF EXISTS cairnops_capture_incident_indicators();

DROP TABLE cairnops_push_outbox;
DROP TABLE cairnops_notification_inbox;
DROP TABLE cairnops_notification_outbox;
DROP TABLE cairnops_incident_indicator_snapshots;

DROP TABLE cairnops_incident_burst_activity;
DROP TABLE cairnops_incident_burst_members;
DROP TABLE cairnops_incident_bursts;
DROP TABLE cairnops_incident_activity;
DROP TABLE cairnops_incident_signals;
DROP TABLE cairnops_incidents;

DROP FUNCTION IF EXISTS cairnops_burst_child_changed_event();
DROP FUNCTION IF EXISTS cairnops_burst_changed_event();
DROP FUNCTION IF EXISTS cairnops_incident_child_changed_event();
DROP FUNCTION IF EXISTS cairnops_incident_changed_event();

CREATE TABLE cairnops_incidents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nature_key text NOT NULL CHECK (length(btrim(nature_key)) BETWEEN 1 AND 255),
    nature_label text NOT NULL CHECK (length(btrim(nature_label)) BETWEEN 1 AND 512),
    nature_scope text NOT NULL CHECK (nature_scope IN ('canonical', 'connector')),
    nature_namespace text NOT NULL CHECK (length(btrim(nature_namespace)) BETWEEN 1 AND 255),
    nature_fingerprint text NOT NULL DEFAULT '' CHECK (length(nature_fingerprint) <= 255),
    propagation_eligible boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'resolved')),
    propagation_status text NOT NULL DEFAULT 'open' CHECK (propagation_status IN ('open', 'closed')),
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    last_impact_at timestamptz NOT NULL,
    propagation_window_seconds integer NOT NULL CHECK (propagation_window_seconds BETWEEN 60 AND 300),
    propagation_ends_at timestamptz NOT NULL,
    propagation_closed_at timestamptz,
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
    extended boolean NOT NULL DEFAULT false,
    active_impact_count integer NOT NULL DEFAULT 0 CHECK (active_impact_count >= 0),
    impact_count integer NOT NULL DEFAULT 0 CHECK (impact_count > 0),
    affected_target_count integer NOT NULL DEFAULT 0 CHECK (affected_target_count >= 0),
    max_affected_targets integer NOT NULL DEFAULT 0 CHECK (max_affected_targets >= 0),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'active' AND resolved_at IS NULL)
        OR (status = 'resolved' AND resolved_at IS NOT NULL)),
    CHECK ((propagation_status = 'open' AND propagation_closed_at IS NULL)
        OR (propagation_status = 'closed' AND propagation_closed_at IS NOT NULL)),
    CHECK (status <> 'resolved' OR propagation_status = 'closed'),
    CHECK ((acknowledged_at IS NULL AND acknowledged_by IS NULL AND acknowledgement_origin IS NULL)
        OR acknowledged_at IS NOT NULL)
);

CREATE INDEX cairnops_incidents_open_nature_idx
    ON cairnops_incidents (
        nature_scope, nature_namespace, nature_fingerprint, opened_at, id
    )
    WHERE status = 'active' AND propagation_status = 'open' AND propagation_eligible;

CREATE INDEX cairnops_incidents_status_time_idx
    ON cairnops_incidents (status, opened_at DESC, id);

CREATE TABLE cairnops_incident_impacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'resolved')),
    source_severity text NOT NULL CHECK (source_severity IN ('information', 'warning', 'major', 'critical')),
    effective_severity text NOT NULL CHECK (effective_severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (incident_id, target_id),
    CHECK ((status = 'active' AND resolved_at IS NULL)
        OR (status = 'resolved' AND resolved_at IS NOT NULL))
);

CREATE INDEX cairnops_incident_impacts_target_idx
    ON cairnops_incident_impacts (target_id, opened_at DESC, id);
CREATE INDEX cairnops_incident_impacts_active_idx
    ON cairnops_incident_impacts (incident_id, target_id)
    WHERE status = 'active';

CREATE TABLE cairnops_incident_evidence (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    impact_id uuid NOT NULL REFERENCES cairnops_incident_impacts(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    origin text NOT NULL CHECK (origin IN (
        'zabbix', 'uptime_kuma', 'webhook', 'native', 'patchmon', 'argus'
    )),
    connector_id uuid REFERENCES cairnops_connectors(id) ON DELETE SET NULL,
    connector_binding_id uuid REFERENCES cairnops_connector_bindings(id) ON DELETE SET NULL,
    source_id uuid REFERENCES cairnops_signal_sources(id) ON DELETE SET NULL,
    identity_scope text NOT NULL CHECK (length(btrim(identity_scope)) BETWEEN 1 AND 255),
    identity_key text NOT NULL CHECK (length(btrim(identity_key)) BETWEEN 1 AND 512),
    external_event_id text NOT NULL DEFAULT '',
    external_object_id text NOT NULL DEFAULT '',
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 512),
    active boolean NOT NULL,
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    resolved_at timestamptz,
    upstream_acknowledged boolean NOT NULL DEFAULT false,
    acknowledgement_sync_status text NOT NULL DEFAULT 'not_applicable' CHECK (
        acknowledgement_sync_status IN ('not_applicable', 'pending', 'synchronized', 'failed')
    ),
    acknowledgement_sync_error text NOT NULL DEFAULT '',
    acknowledgement_synced_at timestamptz,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    invalidated_at timestamptz,
    invalidated_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    invalidation_reason text NOT NULL DEFAULT '',
    rearmed_at timestamptz,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((active AND resolved_at IS NULL) OR (NOT active AND resolved_at IS NOT NULL)),
    CHECK ((invalidated_at IS NULL AND invalidation_reason = '')
        OR (invalidated_at IS NOT NULL AND length(btrim(invalidation_reason)) BETWEEN 8 AND 500)),
    CHECK (rearmed_at IS NULL OR invalidated_at IS NOT NULL),
    CHECK (
        (NOT active)
        OR (origin IN ('zabbix', 'uptime_kuma', 'patchmon', 'argus')
            AND connector_id IS NOT NULL AND connector_binding_id IS NOT NULL
            AND length(btrim(external_event_id)) > 0)
        OR (origin = 'native' AND source_id IS NOT NULL)
        OR origin = 'webhook'
    )
);

CREATE UNIQUE INDEX cairnops_incident_evidence_active_identity_idx
    ON cairnops_incident_evidence (origin, identity_scope, identity_key)
    WHERE active;
CREATE INDEX cairnops_incident_evidence_incident_idx
    ON cairnops_incident_evidence (incident_id, opened_at, id);
CREATE INDEX cairnops_incident_evidence_impact_active_idx
    ON cairnops_incident_evidence (impact_id, active);
CREATE INDEX cairnops_incident_evidence_ack_retry_idx
    ON cairnops_incident_evidence (
        incident_id, acknowledgement_sync_status, updated_at
    )
    WHERE active AND origin = 'zabbix';

CREATE TABLE cairnops_incident_activity (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    impact_id uuid REFERENCES cairnops_incident_impacts(id) ON DELETE CASCADE,
    evidence_id uuid REFERENCES cairnops_incident_evidence(id) ON DELETE SET NULL,
    kind text NOT NULL CHECK (kind IN (
        'opened', 'impact_joined', 'impact_reopened', 'impact_resolved',
        'evidence_added', 'evidence_updated', 'evidence_resolved', 'invalidated',
        'propagation_closed', 'extended', 'severity_changed', 'resolved',
        'acknowledged', 'ack_sync_succeeded', 'ack_sync_failed',
        'upstream_acknowledged', 'target_reconciled', 'source_reassigned'
    )),
    origin text NOT NULL CHECK (origin IN (
        'cairnops', 'zabbix', 'uptime_kuma', 'webhook', 'user', 'native',
        'patchmon', 'argus'
    )),
    actor_id uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    message text NOT NULL DEFAULT '' CHECK (length(message) <= 1000),
    data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_incident_activity_time_idx
    ON cairnops_incident_activity (incident_id, occurred_at DESC, id DESC);

CREATE FUNCTION cairnops_incident_changed_event()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM cairnops_append_event(
        'incident.changed', 'incident',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE FUNCTION cairnops_incident_child_changed_event()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM cairnops_append_event(
        'incident.changed', 'incident',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.incident_id::text ELSE NEW.incident_id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_incidents_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_incidents
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_changed_event();
CREATE TRIGGER cairnops_incident_impacts_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_incident_impacts
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_child_changed_event();
CREATE TRIGGER cairnops_incident_evidence_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_incident_evidence
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_child_changed_event();
CREATE TRIGGER cairnops_incident_activity_changed
AFTER INSERT OR DELETE ON cairnops_incident_activity
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_child_changed_event();

CREATE TABLE cairnops_incident_indicator_snapshots (
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    impact_id uuid NOT NULL REFERENCES cairnops_incident_impacts(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    indicator_id uuid REFERENCES cairnops_context_indicators(id) ON DELETE SET NULL,
    semantic_key text NOT NULL,
    label text NOT NULL,
    unit text NOT NULL,
    value double precision NOT NULL,
    observed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (impact_id, semantic_key, label)
);

CREATE FUNCTION cairnops_capture_impact_indicators()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO cairnops_incident_indicator_snapshots (
        incident_id, impact_id, target_id, indicator_id,
        semantic_key, label, unit, value, observed_at
    )
    SELECT NEW.incident_id, NEW.id, NEW.target_id, indicator.id,
           indicator.semantic_key, indicator.label, indicator.unit,
           indicator.last_value, indicator.last_observed_at
    FROM cairnops_context_indicators indicator
    WHERE indicator.target_id = NEW.target_id
      AND indicator.enabled
      AND indicator.last_value IS NOT NULL
      AND indicator.last_observed_at >= NEW.opened_at - interval '5 minutes'
      AND indicator.last_observed_at <= NEW.opened_at + interval '1 minute'
    ON CONFLICT DO NOTHING;
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_impact_indicator_snapshot
AFTER INSERT ON cairnops_incident_impacts
FOR EACH ROW EXECUTE FUNCTION cairnops_capture_impact_indicators();

CREATE FUNCTION cairnops_capture_indicator_for_impacts()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO cairnops_incident_indicator_snapshots (
        incident_id, impact_id, target_id, indicator_id,
        semantic_key, label, unit, value, observed_at
    )
    SELECT impact.incident_id, impact.id, impact.target_id, NEW.id,
           NEW.semantic_key, NEW.label, NEW.unit, NEW.last_value,
           NEW.last_observed_at
    FROM cairnops_incident_impacts impact
    WHERE impact.target_id = NEW.target_id
      AND NEW.enabled
      AND NEW.last_value IS NOT NULL
      AND NEW.last_observed_at >= impact.opened_at - interval '5 minutes'
      AND NEW.last_observed_at <= impact.opened_at + interval '1 minute'
      AND impact.opened_at >= now() - interval '10 minutes'
    ON CONFLICT DO NOTHING;
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_indicator_impact_snapshot
AFTER UPDATE OF last_value, last_observed_at ON cairnops_context_indicators
FOR EACH ROW
WHEN (NEW.last_value IS NOT NULL AND NEW.last_observed_at IS NOT NULL)
EXECUTE FUNCTION cairnops_capture_indicator_for_impacts();

CREATE TABLE cairnops_notification_outbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    channel_id uuid NOT NULL REFERENCES cairnops_notification_channels(id) ON DELETE CASCADE,
    incident_revision integer NOT NULL DEFAULT 0 CHECK (incident_revision >= 0),
    event_kind text NOT NULL CHECK (event_kind IN ('firing', 'resolved', 'incident_update')),
    event_key text NOT NULL,
    presentation text NOT NULL DEFAULT 'alert' CHECK (presentation IN ('alert', 'silent')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'cancelled')),
    target_name text NOT NULL,
    nature_label text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    resolved_at timestamptz,
    impact_count integer NOT NULL DEFAULT 1 CHECK (impact_count > 0),
    affected_target_count integer NOT NULL DEFAULT 1 CHECK (affected_target_count >= 0),
    max_affected_targets integer NOT NULL DEFAULT 1 CHECK (max_affected_targets >= 0),
    propagation_status text NOT NULL CHECK (propagation_status IN ('open', 'closed')),
    extended boolean NOT NULL DEFAULT false,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    delivered_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (incident_id, channel_id, event_key),
    CHECK ((event_kind = 'firing' AND resolved_at IS NULL)
        OR event_kind IN ('incident_update', 'resolved'))
);

CREATE INDEX cairnops_notification_outbox_due_idx
    ON cairnops_notification_outbox (next_attempt_at, id)
    WHERE status IN ('pending', 'failed');

CREATE TABLE cairnops_notification_inbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES cairnops_users(id) ON DELETE CASCADE,
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    target_id uuid REFERENCES cairnops_targets(id) ON DELETE SET NULL,
    revision integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
    event_kind text NOT NULL CHECK (event_kind IN ('firing', 'resolved', 'incident_update')),
    target_name text NOT NULL,
    nature_label text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    occurred_at timestamptz NOT NULL,
    impact_count integer NOT NULL DEFAULT 1 CHECK (impact_count > 0),
    affected_target_count integer NOT NULL DEFAULT 1 CHECK (affected_target_count >= 0),
    max_affected_targets integer NOT NULL DEFAULT 1 CHECK (max_affected_targets >= 0),
    propagation_status text NOT NULL CHECK (propagation_status IN ('open', 'closed')),
    extended boolean NOT NULL DEFAULT false,
    read_at timestamptz,
    dismissed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, incident_id)
);

CREATE INDEX cairnops_notification_inbox_unread_idx
    ON cairnops_notification_inbox (user_id, occurred_at DESC)
    WHERE read_at IS NULL AND dismissed_at IS NULL;
CREATE INDEX cairnops_notification_inbox_recent_idx
    ON cairnops_notification_inbox (user_id, occurred_at DESC)
    WHERE dismissed_at IS NULL;

CREATE TABLE cairnops_push_outbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_id uuid NOT NULL REFERENCES cairnops_devices(id) ON DELETE CASCADE,
    inbox_id bigint NOT NULL REFERENCES cairnops_notification_inbox(id) ON DELETE CASCADE,
    revision integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
    presentation text NOT NULL DEFAULT 'alert' CHECK (presentation IN ('alert', 'silent')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'cancelled')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    delivered_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (device_id, inbox_id, revision)
);

CREATE INDEX cairnops_push_outbox_due_idx
    ON cairnops_push_outbox (next_attempt_at, id)
    WHERE status IN ('pending', 'failed');

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed', 'source.changed', 'observation.created', 'component.heartbeat',
    'connector.changed', 'incident.changed', 'maintenance.changed',
    'notification.changed', 'device.changed', 'indicator.changed',
    'reconciliation.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN (
        'target', 'source', 'component', 'connector', 'incident',
        'maintenance', 'notification', 'device', 'indicator', 'reconciliation'
    )
);
