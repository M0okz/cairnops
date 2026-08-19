CREATE TABLE cairnops_targets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    description text NOT NULL DEFAULT '',
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cairnops_signal_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
    kind text NOT NULL CHECK (kind IN ('http', 'tcp', 'dns', 'icmp', 'heartbeat')),
    enabled boolean NOT NULL DEFAULT true,
    interval_seconds integer NOT NULL CHECK (interval_seconds BETWEEN 20 AND 86400),
    timeout_milliseconds integer NOT NULL CHECK (
        timeout_milliseconds BETWEEN 100 AND 60000
        AND timeout_milliseconds <= interval_seconds * 1000
    ),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    next_run_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    last_signal_at timestamptz,
    last_observed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_signal_sources_due_idx
    ON cairnops_signal_sources (next_run_at)
    WHERE enabled;

CREATE INDEX cairnops_signal_sources_target_idx
    ON cairnops_signal_sources (target_id);

CREATE TABLE cairnops_observations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id uuid NOT NULL REFERENCES cairnops_signal_sources(id) ON DELETE CASCADE,
    target_id uuid NOT NULL REFERENCES cairnops_targets(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('healthy', 'unhealthy', 'unknown')),
    latency_milliseconds integer NOT NULL CHECK (latency_milliseconds >= 0),
    reason text NOT NULL DEFAULT '',
    message text NOT NULL DEFAULT '',
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_observations_source_time_idx
    ON cairnops_observations (source_id, observed_at DESC);

CREATE INDEX cairnops_observations_target_time_idx
    ON cairnops_observations (target_id, observed_at DESC);
