-- Materialized occurrences retain the existing neutralization queries across
-- incidents, availability and notifications. No scheduler is needed for correctness.
CREATE TABLE cairnops_maintenance_series (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    timezone text NOT NULL,
    until_date date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE cairnops_maintenances
    ADD COLUMN series_id uuid REFERENCES cairnops_maintenance_series(id),
    ADD COLUMN extension_count integer NOT NULL DEFAULT 0 CHECK (extension_count >= 0);
CREATE INDEX cairnops_maintenances_series_idx ON cairnops_maintenances(series_id, starts_at) WHERE series_id IS NOT NULL;
CREATE TABLE cairnops_maintenance_extensions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    maintenance_id uuid NOT NULL REFERENCES cairnops_maintenances(id) ON DELETE CASCADE,
    actor_id uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    previous_ends_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at = previous_ends_at + interval '30 minutes')
);
