-- Une Nature utilisable par les Rafales ne repose ni sur une Cible, ni sur un
-- libellé rendu. Les Incidents existants restent volontairement inéligibles :
-- la migration ne réécrit pas l'histoire et ne produit aucune notification.
ALTER TABLE cairnops_incidents
    ADD COLUMN nature_scope text NOT NULL DEFAULT 'connector'
        CHECK (nature_scope IN ('canonical', 'connector')),
    ADD COLUMN nature_namespace text NOT NULL DEFAULT 'legacy',
    ADD COLUMN nature_fingerprint text NOT NULL DEFAULT '',
    ADD COLUMN burst_eligible boolean NOT NULL DEFAULT false;

CREATE INDEX cairnops_incidents_burst_candidates_idx
    ON cairnops_incidents (
        nature_scope, nature_namespace, nature_fingerprint, opened_at, id
    )
    WHERE burst_eligible;

CREATE FUNCTION cairnops_severity_rank(value text)
RETURNS integer
LANGUAGE sql
IMMUTABLE
STRICT
AS $$
    SELECT CASE value
        WHEN 'information' THEN 1 WHEN 'warning' THEN 2
        WHEN 'major' THEN 3 WHEN 'critical' THEN 4 ELSE 0
    END
$$;

-- Une Rafale est une projection durable et explicable d'Incidents distincts.
-- Elle ne porte aucune cause supposée et ne remplace jamais ses membres.
CREATE TABLE cairnops_incident_bursts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    anchor_incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE RESTRICT,
    nature_scope text NOT NULL CHECK (nature_scope IN ('canonical', 'connector')),
    nature_namespace text NOT NULL,
    nature_fingerprint text NOT NULL CHECK (length(btrim(nature_fingerprint)) BETWEEN 1 AND 255),
    nature_label text NOT NULL CHECK (length(btrim(nature_label)) BETWEEN 1 AND 512),
    status text NOT NULL CHECK (status IN ('propagating', 'sealed', 'resolved')),
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    opened_at timestamptz NOT NULL,
    last_joined_at timestamptz NOT NULL,
    propagation_window_seconds integer NOT NULL CHECK (propagation_window_seconds BETWEEN 60 AND 300),
    propagation_ends_at timestamptz NOT NULL,
    sealed_at timestamptz,
    resolved_at timestamptz,
    acknowledged_at timestamptz,
    acknowledged_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    extended boolean NOT NULL DEFAULT false,
    active_incident_count integer NOT NULL DEFAULT 0 CHECK (active_incident_count >= 0),
    incident_count integer NOT NULL DEFAULT 0 CHECK (incident_count >= 0),
    affected_target_count integer NOT NULL DEFAULT 0 CHECK (affected_target_count >= 0),
    target_count integer NOT NULL DEFAULT 0 CHECK (target_count >= 0),
    max_affected_targets integer NOT NULL DEFAULT 0 CHECK (max_affected_targets >= 0),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'propagating' AND sealed_at IS NULL AND resolved_at IS NULL)
        OR (status = 'sealed' AND sealed_at IS NOT NULL AND resolved_at IS NULL)
        OR (status = 'resolved' AND sealed_at IS NOT NULL AND resolved_at IS NOT NULL)),
    CHECK ((acknowledged_at IS NULL AND acknowledged_by IS NULL) OR acknowledged_at IS NOT NULL)
);

CREATE INDEX cairnops_incident_bursts_propagating_nature_idx
    ON cairnops_incident_bursts (nature_scope, nature_namespace, nature_fingerprint)
    WHERE status = 'propagating';

CREATE INDEX cairnops_incident_bursts_status_time_idx
    ON cairnops_incident_bursts (status, opened_at DESC);

CREATE TABLE cairnops_incident_burst_members (
    burst_id uuid NOT NULL REFERENCES cairnops_incident_bursts(id) ON DELETE CASCADE,
    incident_id uuid NOT NULL UNIQUE REFERENCES cairnops_incidents(id) ON DELETE RESTRICT,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE RESTRICT,
    joined_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (burst_id, incident_id)
);

CREATE INDEX cairnops_incident_burst_members_target_idx
    ON cairnops_incident_burst_members (burst_id, target_id);

CREATE TABLE cairnops_incident_burst_activity (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    burst_id uuid NOT NULL REFERENCES cairnops_incident_bursts(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN (
        'formed', 'incident_joined', 'sealed', 'severity_changed',
        'extended', 'acknowledged', 'resolved'
    )),
    actor_id uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    message text NOT NULL DEFAULT '' CHECK (length(message) <= 1000),
    data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_incident_burst_activity_time_idx
    ON cairnops_incident_burst_activity (burst_id, occurred_at DESC, id DESC);

ALTER TABLE cairnops_incident_activity DROP CONSTRAINT cairnops_incident_activity_kind_check;
ALTER TABLE cairnops_incident_activity ADD CONSTRAINT cairnops_incident_activity_kind_check CHECK (kind IN (
    'opened', 'signal_added', 'signal_updated', 'signal_resolved', 'resolved', 'invalidated',
    'acknowledged', 'ack_sync_succeeded', 'ack_sync_failed', 'upstream_acknowledged',
    'target_reconciled', 'source_reassigned', 'burst_joined'
));

-- La boîte de sortie garde l'Incident d'ancrage pour la compatibilité des
-- liens, et ajoute l'identité et la révision de Rafale pour l'idempotence.
ALTER TABLE cairnops_notification_outbox
    ADD COLUMN burst_id uuid REFERENCES cairnops_incident_bursts(id) ON DELETE CASCADE,
    ADD COLUMN burst_revision integer NOT NULL DEFAULT 0 CHECK (burst_revision >= 0),
    ADD COLUMN event_key text NOT NULL DEFAULT '',
    ADD COLUMN presentation text NOT NULL DEFAULT 'alert'
        CHECK (presentation IN ('alert', 'silent')),
    ADD COLUMN incident_count integer NOT NULL DEFAULT 1 CHECK (incident_count > 0),
    ADD COLUMN affected_target_count integer NOT NULL DEFAULT 1 CHECK (affected_target_count >= 0),
    ADD COLUMN max_affected_targets integer NOT NULL DEFAULT 1 CHECK (max_affected_targets >= 0),
    ADD COLUMN burst_status text CHECK (
        burst_status IS NULL OR burst_status IN ('propagating', 'sealed', 'resolved')
    ),
    ADD COLUMN burst_extended boolean NOT NULL DEFAULT false;

UPDATE cairnops_notification_outbox SET event_key = event_kind WHERE event_key = '';

DO $$
DECLARE constraint_name text;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'cairnops_notification_outbox'::regclass
      AND contype = 'u'
      AND pg_get_constraintdef(oid) LIKE 'UNIQUE (incident_id, channel_id, event_kind)%';
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE cairnops_notification_outbox DROP CONSTRAINT %I', constraint_name);
    END IF;
END;
$$;

CREATE UNIQUE INDEX cairnops_notification_outbox_incident_event_idx
    ON cairnops_notification_outbox (incident_id, channel_id, event_key)
    WHERE burst_id IS NULL;

CREATE UNIQUE INDEX cairnops_notification_outbox_burst_event_idx
    ON cairnops_notification_outbox (burst_id, channel_id, event_key)
    WHERE burst_id IS NOT NULL;

ALTER TABLE cairnops_notification_outbox DROP CONSTRAINT cairnops_notification_outbox_event_kind_check;
ALTER TABLE cairnops_notification_outbox ADD CONSTRAINT cairnops_notification_outbox_event_kind_check
    CHECK (event_kind IN ('firing', 'resolved', 'burst_update'));

ALTER TABLE cairnops_notification_outbox DROP CONSTRAINT cairnops_notification_outbox_check;
ALTER TABLE cairnops_notification_outbox ADD CONSTRAINT cairnops_notification_outbox_resolution_check
    CHECK ((event_kind IN ('firing', 'burst_update') AND resolved_at IS NULL)
        OR event_kind = 'resolved');

ALTER TABLE cairnops_notification_inbox
    ADD COLUMN burst_id uuid REFERENCES cairnops_incident_bursts(id) ON DELETE SET NULL,
    ADD COLUMN revision integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
    ADD COLUMN incident_count integer NOT NULL DEFAULT 1 CHECK (incident_count > 0),
    ADD COLUMN affected_target_count integer NOT NULL DEFAULT 1 CHECK (affected_target_count >= 0),
    ADD COLUMN max_affected_targets integer NOT NULL DEFAULT 1 CHECK (max_affected_targets >= 0),
    ADD COLUMN burst_status text CHECK (
        burst_status IS NULL OR burst_status IN ('propagating', 'sealed', 'resolved')
    ),
    ADD COLUMN burst_extended boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX cairnops_notification_inbox_user_burst_idx
    ON cairnops_notification_inbox (user_id, burst_id)
    WHERE burst_id IS NOT NULL;

-- Une entrée intégrée est mutable ; chaque révision peut déclencher au plus un
-- Push par appareil. Les révisions silencieuses remplacent seulement le contenu.
ALTER TABLE cairnops_push_outbox
    ADD COLUMN revision integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
    ADD COLUMN presentation text NOT NULL DEFAULT 'alert'
        CHECK (presentation IN ('alert', 'silent'));

DO $$
DECLARE constraint_name text;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'cairnops_push_outbox'::regclass
      AND contype = 'u'
      AND pg_get_constraintdef(oid) LIKE 'UNIQUE (device_id, inbox_id)%';
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE cairnops_push_outbox DROP CONSTRAINT %I', constraint_name);
    END IF;
END;
$$;

CREATE UNIQUE INDEX cairnops_push_outbox_device_inbox_revision_idx
    ON cairnops_push_outbox (device_id, inbox_id, revision);

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed', 'source.changed', 'observation.created', 'component.heartbeat',
    'connector.changed', 'incident.changed', 'maintenance.changed',
    'notification.changed', 'device.changed', 'indicator.changed',
    'reconciliation.changed', 'burst.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN (
        'target', 'source', 'component', 'connector', 'incident',
        'maintenance', 'notification', 'device', 'indicator',
        'reconciliation', 'burst'
    )
);

CREATE FUNCTION cairnops_burst_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'burst.changed',
        'burst',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_incident_bursts_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_incident_bursts
FOR EACH ROW EXECUTE FUNCTION cairnops_burst_changed_event();

CREATE FUNCTION cairnops_burst_child_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'burst.changed',
        'burst',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.burst_id::text ELSE NEW.burst_id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_incident_burst_members_changed
AFTER INSERT OR DELETE ON cairnops_incident_burst_members
FOR EACH ROW EXECUTE FUNCTION cairnops_burst_child_changed_event();

CREATE TRIGGER cairnops_incident_burst_activity_changed
AFTER INSERT OR DELETE ON cairnops_incident_burst_activity
FOR EACH ROW EXECUTE FUNCTION cairnops_burst_child_changed_event();
