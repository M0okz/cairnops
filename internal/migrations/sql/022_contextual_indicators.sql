-- Les deux périmètres restent indépendants. Une liaison découverte uniquement
-- pour du contexte ne devient pas une Source d'Incident, et masquer ses
-- Indicateurs ne suspend jamais la synchronisation opérationnelle existante.
ALTER TABLE cairnops_connector_bindings
    ADD COLUMN integration_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN indicators_enabled boolean NOT NULL DEFAULT false;

-- Une capacité décrit une partie du contrat distant. La synchronisation des
-- Incidents peut rester disponible lorsque les Indicateurs ne le sont plus.
CREATE TABLE cairnops_connector_capabilities (
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    capability text NOT NULL CHECK (length(btrim(capability)) BETWEEN 1 AND 100),
    status text NOT NULL CHECK (status IN ('available', 'degraded', 'unavailable')),
    message text NOT NULL DEFAULT '' CHECK (length(message) <= 500),
    checked_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (connector_id, capability)
);

CREATE TABLE cairnops_indicator_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    specification jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(specification) = 'array'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (connector_id, name)
);

CREATE TABLE cairnops_context_indicators (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    connector_binding_id uuid NOT NULL REFERENCES cairnops_connector_bindings(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    profile_id uuid REFERENCES cairnops_indicator_profiles(id) ON DELETE SET NULL,
    semantic_key text NOT NULL CHECK (semantic_key IN (
        'cpu.utilization', 'memory.utilization', 'filesystem.utilization',
        'network.in', 'network.out', 'response.time',
        'certificate.days_remaining', 'certificate.valid',
        'updates.count', 'security_updates.count', 'reboot.required',
        'reporting.age'
    )),
    label text NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 160),
    external_id text NOT NULL CHECK (length(btrim(external_id)) BETWEEN 1 AND 255),
    dimension text NOT NULL DEFAULT '' CHECK (length(dimension) <= 255),
    unit text NOT NULL CHECK (unit IN (
        'percent', 'bytes_per_second', 'milliseconds', 'days',
        'count', 'boolean', 'seconds'
    )),
    enabled boolean NOT NULL DEFAULT true,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    last_value double precision,
    last_observed_at timestamptz,
    last_error text NOT NULL DEFAULT '' CHECK (length(last_error) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (connector_binding_id, semantic_key, dimension),
    CHECK (last_value IS NULL OR last_value::text NOT IN ('NaN', 'Infinity', '-Infinity'))
);

CREATE INDEX cairnops_context_indicators_connector_idx
    ON cairnops_context_indicators (connector_id, enabled, target_id);
CREATE INDEX cairnops_context_indicators_target_idx
    ON cairnops_context_indicators (target_id, enabled, semantic_key);

-- Un point par minute et par Indicateur. Une réécriture de la même minute
-- remplace la valeur plutôt que de gonfler la série.
CREATE TABLE cairnops_indicator_samples (
    indicator_id uuid NOT NULL REFERENCES cairnops_context_indicators(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL,
    value double precision NOT NULL CHECK (value::text NOT IN ('NaN', 'Infinity', '-Infinity')),
    PRIMARY KEY (indicator_id, observed_at)
);

CREATE INDEX cairnops_indicator_samples_time_idx
    ON cairnops_indicator_samples (observed_at);

CREATE TABLE cairnops_indicator_hours (
    indicator_id uuid NOT NULL REFERENCES cairnops_context_indicators(id) ON DELETE CASCADE,
    hour timestamptz NOT NULL,
    minimum double precision NOT NULL CHECK (minimum::text NOT IN ('NaN', 'Infinity', '-Infinity')),
    maximum double precision NOT NULL CHECK (maximum::text NOT IN ('NaN', 'Infinity', '-Infinity')),
    average double precision NOT NULL CHECK (average::text NOT IN ('NaN', 'Infinity', '-Infinity')),
    latest double precision NOT NULL CHECK (latest::text NOT IN ('NaN', 'Infinity', '-Infinity')),
    samples integer NOT NULL CHECK (samples > 0),
    consolidated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (indicator_id, hour)
);

CREATE INDEX cairnops_indicator_hours_time_idx ON cairnops_indicator_hours (hour);

CREATE TABLE cairnops_user_indicator_pins (
    user_id uuid NOT NULL REFERENCES cairnops_users(id) ON DELETE CASCADE,
    indicator_id uuid NOT NULL REFERENCES cairnops_context_indicators(id) ON DELETE CASCADE,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 3),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, indicator_id),
    UNIQUE (user_id, position)
);

CREATE TABLE cairnops_connector_configuration_activity (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    actor_id uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    summary text NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 500),
    data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_connector_configuration_activity_idx
    ON cairnops_connector_configuration_activity (connector_id, occurred_at DESC, id DESC);

-- L'Incident conserve une photographie ponctuelle ; la courbe environnante
-- reste soumise à la rétention courte.
CREATE TABLE cairnops_incident_indicator_snapshots (
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    indicator_id uuid REFERENCES cairnops_context_indicators(id) ON DELETE SET NULL,
    semantic_key text NOT NULL,
    label text NOT NULL,
    unit text NOT NULL,
    value double precision NOT NULL,
    observed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, semantic_key, label)
);

CREATE FUNCTION cairnops_capture_incident_indicators()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO cairnops_incident_indicator_snapshots (
        incident_id, indicator_id, semantic_key, label, unit, value, observed_at
    )
    SELECT NEW.id, indicator.id, indicator.semantic_key, indicator.label,
           indicator.unit, indicator.last_value, indicator.last_observed_at
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

CREATE TRIGGER cairnops_incident_indicator_snapshot
AFTER INSERT ON cairnops_incidents
FOR EACH ROW EXECUTE FUNCTION cairnops_capture_incident_indicators();

-- Un Incident peut s'ouvrir quelques secondes avant le relevé de la minute.
-- Le premier relevé contemporain complète alors l'instantané sans réécrire
-- ceux déjà pris à l'ouverture.
CREATE FUNCTION cairnops_capture_indicator_for_incidents()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO cairnops_incident_indicator_snapshots (
        incident_id, indicator_id, semantic_key, label, unit, value, observed_at
    )
    SELECT incident.id, NEW.id, NEW.semantic_key, NEW.label, NEW.unit,
           NEW.last_value, NEW.last_observed_at
    FROM cairnops_incidents incident
    WHERE incident.target_id = NEW.target_id
      AND NEW.enabled
      AND NEW.last_value IS NOT NULL
      AND NEW.last_observed_at >= incident.opened_at - interval '5 minutes'
      AND NEW.last_observed_at <= incident.opened_at + interval '1 minute'
      AND incident.opened_at >= now() - interval '10 minutes'
    ON CONFLICT DO NOTHING;
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_indicator_incident_snapshot
AFTER UPDATE OF last_value, last_observed_at ON cairnops_context_indicators
FOR EACH ROW
WHEN (NEW.last_value IS NOT NULL AND NEW.last_observed_at IS NOT NULL)
EXECUTE FUNCTION cairnops_capture_indicator_for_incidents();

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed',
    'source.changed',
    'observation.created',
    'component.heartbeat',
    'connector.changed',
    'incident.changed',
    'maintenance.changed',
    'notification.changed',
    'device.changed',
    'indicator.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN (
        'target', 'source', 'component', 'connector', 'incident',
        'maintenance', 'notification', 'device', 'indicator'
    )
);

CREATE FUNCTION cairnops_indicator_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'indicator.changed',
        'indicator',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_context_indicators_changed
AFTER INSERT OR DELETE OR UPDATE OF enabled, label, last_value, last_observed_at, last_error
ON cairnops_context_indicators
FOR EACH ROW EXECUTE FUNCTION cairnops_indicator_changed_event();
