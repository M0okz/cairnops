CREATE TABLE cairnops_component_heartbeats (
    component text NOT NULL,
    instance_id text NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (component, instance_id)
);

CREATE INDEX cairnops_component_heartbeats_last_seen_idx
    ON cairnops_component_heartbeats (last_seen_at DESC);
