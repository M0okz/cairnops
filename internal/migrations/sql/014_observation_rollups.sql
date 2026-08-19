-- Agrégats horaires des Observations. Une fenêtre de 30 jours se lit alors en
-- 720 lignes par Source, indépendamment de la cadence des Contrôles natifs.
CREATE TABLE cairnops_observation_hours (
    source_id uuid NOT NULL REFERENCES cairnops_signal_sources(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    hour timestamptz NOT NULL,
    healthy integer NOT NULL DEFAULT 0 CHECK (healthy >= 0),
    unhealthy integer NOT NULL DEFAULT 0 CHECK (unhealthy >= 0),
    unknown integer NOT NULL DEFAULT 0 CHECK (unknown >= 0),
    -- Observations attendues sur l'heure, selon l'intervalle en vigueur et la
    -- date de création de la Source. Une heure sans Observation existe donc
    -- avec ses attentes déçues : c'est ainsi qu'une interruption se voit.
    expected integer NOT NULL DEFAULT 0 CHECK (expected >= 0),
    latency_sum_milliseconds bigint NOT NULL DEFAULT 0 CHECK (latency_sum_milliseconds >= 0),
    latency_count integer NOT NULL DEFAULT 0 CHECK (latency_count >= 0),
    latency_maximum_milliseconds integer NOT NULL DEFAULT 0 CHECK (latency_maximum_milliseconds >= 0),
    consolidated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, hour)
);

CREATE INDEX cairnops_observation_hours_target_idx
    ON cairnops_observation_hours (target_id, hour DESC);

-- Une seule ligne : l'heure jusqu'à laquelle la consolidation a conclu. Le
-- worker reprend là où il s'est arrêté, quel que soit son redémarrage.
CREATE TABLE cairnops_observation_rollup_state (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    consolidated_through timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
