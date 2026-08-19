ALTER TABLE cairnops_incident_signals
    ADD COLUMN invalidated_at timestamptz,
    ADD COLUMN invalidated_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    ADD COLUMN invalidation_reason text NOT NULL DEFAULT '',
    ADD COLUMN rearmed_at timestamptz,
    ADD CONSTRAINT cairnops_incident_signals_invalidation_check CHECK (
        (invalidated_at IS NULL AND invalidation_reason = '' AND rearmed_at IS NULL)
        OR (
            invalidated_at IS NOT NULL
            AND length(btrim(invalidation_reason)) BETWEEN 8 AND 500
            AND NOT active
            AND resolved_at IS NOT NULL
            AND (rearmed_at IS NULL OR rearmed_at >= invalidated_at)
        )
    );

CREATE INDEX cairnops_incident_signals_invalidation_latch_idx
    ON cairnops_incident_signals (origin, connector_id, connector_binding_id, external_object_id)
    WHERE invalidated_at IS NOT NULL AND rearmed_at IS NULL;

ALTER TABLE cairnops_incident_activity DROP CONSTRAINT cairnops_incident_activity_kind_check;
ALTER TABLE cairnops_incident_activity ADD CONSTRAINT cairnops_incident_activity_kind_check CHECK (kind IN (
    'opened', 'signal_added', 'signal_resolved', 'resolved', 'invalidated',
    'acknowledged', 'ack_sync_succeeded', 'ack_sync_failed', 'upstream_acknowledged'
));
