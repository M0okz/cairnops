CREATE TABLE cairnops_events (
    version bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN (
        'target.changed',
        'source.changed',
        'observation.created',
        'component.heartbeat'
    )),
    entity_type text NOT NULL CHECK (entity_type IN ('target', 'source', 'component')),
    entity_id text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_events_occurred_at_idx
    ON cairnops_events (occurred_at DESC);

CREATE FUNCTION cairnops_append_event(event_kind text, event_entity_type text, event_entity_id text)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    emitted_version bigint;
BEGIN
    INSERT INTO cairnops_events (kind, entity_type, entity_id)
    VALUES (event_kind, event_entity_type, event_entity_id)
    RETURNING version INTO emitted_version;

    PERFORM pg_notify('cairnops_events', emitted_version::text);
END;
$$;

CREATE FUNCTION cairnops_target_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'target.changed',
        'target',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_targets_changed
AFTER INSERT OR UPDATE OR DELETE ON cairnops_targets
FOR EACH ROW EXECUTE FUNCTION cairnops_target_changed_event();

CREATE FUNCTION cairnops_source_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'source.changed',
        'source',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_sources_created_or_deleted
AFTER INSERT OR DELETE ON cairnops_signal_sources
FOR EACH ROW EXECUTE FUNCTION cairnops_source_changed_event();

CREATE TRIGGER cairnops_sources_configured
AFTER UPDATE OF name, enabled, interval_seconds, timeout_milliseconds, config
ON cairnops_signal_sources
FOR EACH ROW EXECUTE FUNCTION cairnops_source_changed_event();

CREATE FUNCTION cairnops_observation_created_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event('observation.created', 'target', NEW.target_id::text);
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_observations_created
AFTER INSERT ON cairnops_observations
FOR EACH ROW EXECUTE FUNCTION cairnops_observation_created_event();

CREATE FUNCTION cairnops_component_heartbeat_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR NEW.last_seen_at - OLD.last_seen_at > interval '45 seconds' THEN
        PERFORM cairnops_append_event(
            'component.heartbeat',
            'component',
            NEW.component || ':' || NEW.instance_id
        );
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_component_heartbeat_recorded
AFTER INSERT OR UPDATE ON cairnops_component_heartbeats
FOR EACH ROW EXECUTE FUNCTION cairnops_component_heartbeat_event();
