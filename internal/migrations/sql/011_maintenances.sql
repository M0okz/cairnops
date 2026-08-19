CREATE TABLE cairnops_maintenances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 3 AND 160),
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 8 AND 500),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    cancelled_at timestamptz,
    cancelled_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    created_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (ends_at <= starts_at + interval '31 days')
);

CREATE TABLE cairnops_maintenance_targets (
    maintenance_id uuid NOT NULL REFERENCES cairnops_maintenances(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    PRIMARY KEY (maintenance_id, target_id)
);

CREATE INDEX cairnops_maintenances_window_idx
    ON cairnops_maintenances (starts_at, ends_at)
    WHERE cancelled_at IS NULL;

CREATE INDEX cairnops_maintenance_targets_target_idx
    ON cairnops_maintenance_targets (target_id, maintenance_id);

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed',
    'source.changed',
    'observation.created',
    'component.heartbeat',
    'connector.changed',
    'incident.changed',
    'maintenance.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN ('target', 'source', 'component', 'connector', 'incident', 'maintenance')
);

CREATE FUNCTION cairnops_maintenance_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'maintenance.changed',
        'maintenance',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_maintenances_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_maintenances
FOR EACH ROW EXECUTE FUNCTION cairnops_maintenance_changed_event();

CREATE FUNCTION cairnops_maintenance_target_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'maintenance.changed',
        'maintenance',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.maintenance_id::text ELSE NEW.maintenance_id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_maintenance_targets_changed
AFTER INSERT OR DELETE ON cairnops_maintenance_targets
FOR EACH ROW EXECUTE FUNCTION cairnops_maintenance_target_changed_event();
