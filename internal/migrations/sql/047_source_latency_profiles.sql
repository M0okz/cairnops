-- Le Profil de latence d'une Source décrit la latence habituelle de ses
-- Observations saines, heure par heure. C'est un artefact appris, persisté et
-- consultable : il porte sa fenêtre et son nombre d'Observations, et ne conclut
-- rien par lui-même. Aucune de ses valeurs n'est un score.
CREATE TABLE cairnops_source_latency_profiles (
    source_id uuid PRIMARY KEY REFERENCES cairnops_signal_sources(id) ON DELETE CASCADE,
    window_start timestamptz NOT NULL,
    window_end timestamptz NOT NULL,
    -- Un Profil sans Observation saine existe avec zéro échantillon : CairnOps
    -- a regardé et n'a rien établi, ce qui n'est pas la même chose que ne pas
    -- avoir regardé.
    samples integer NOT NULL CHECK (samples >= 0),
    computed_at timestamptz NOT NULL DEFAULT now(),
    CHECK (window_end > window_start)
);

-- Le worker reprend les Sources dont le Profil a vieilli. Celles qui n'en ont
-- pas encore se lisent par absence de ligne.
CREATE INDEX cairnops_source_latency_profiles_stale_idx
    ON cairnops_source_latency_profiles (computed_at);

-- Un seau par heure UTC de la fenêtre, comme tout horodatage stocké. L'heure
-- -1 réunit toutes les heures confondues et sert de repli aux Sources dont la
-- cadence ne remplit pas chacune d'elles.
--
-- Les seaux dépendent du Profil qui les a produits : ils ne peuvent pas
-- survivre à sa provenance.
CREATE TABLE cairnops_source_latency_hours (
    source_id uuid NOT NULL
        REFERENCES cairnops_source_latency_profiles(source_id) ON DELETE CASCADE,
    hour smallint NOT NULL CHECK (hour BETWEEN -1 AND 23),
    -- Une heure sans Observation saine n'a pas de seau : le Profil ne prétend
    -- rien sur ce qu'il n'a pas observé.
    samples integer NOT NULL CHECK (samples > 0),
    median_milliseconds integer NOT NULL CHECK (median_milliseconds >= 0),
    p95_milliseconds integer NOT NULL CHECK (p95_milliseconds >= 0),
    p99_milliseconds integer NOT NULL CHECK (p99_milliseconds >= 0),
    PRIMARY KEY (source_id, hour)
);
