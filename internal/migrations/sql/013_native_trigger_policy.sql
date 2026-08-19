ALTER TABLE cairnops_signal_sources
    ADD COLUMN failure_threshold integer NOT NULL DEFAULT 3
        CHECK (failure_threshold BETWEEN 1 AND 10),
    ADD COLUMN recovery_threshold integer NOT NULL DEFAULT 2
        CHECK (recovery_threshold BETWEEN 1 AND 10),
    ADD COLUMN severity text NOT NULL DEFAULT 'major'
        CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    ADD COLUMN consecutive_unhealthy integer NOT NULL DEFAULT 0
        CHECK (consecutive_unhealthy >= 0),
    ADD COLUMN consecutive_healthy integer NOT NULL DEFAULT 0
        CHECK (consecutive_healthy >= 0);

DROP TRIGGER cairnops_sources_configured ON cairnops_signal_sources;
CREATE TRIGGER cairnops_sources_configured
AFTER UPDATE OF name, enabled, interval_seconds, timeout_milliseconds, config,
    failure_threshold, recovery_threshold, severity
ON cairnops_signal_sources
FOR EACH ROW EXECUTE FUNCTION cairnops_source_changed_event();

-- Une Source de signal native n'alimente qu'une seule preuve active à la fois.
CREATE UNIQUE INDEX cairnops_incident_signals_native_active_idx
    ON cairnops_incident_signals (source_id)
    WHERE origin = 'native' AND active;

-- Verrou d'Invalidation : une Source écartée ne réalimente pas l'Incident
-- tant qu'un cycle sain ne l'a pas réarmée.
CREATE INDEX cairnops_incident_signals_native_latch_idx
    ON cairnops_incident_signals (source_id)
    WHERE origin = 'native' AND invalidated_at IS NOT NULL AND rearmed_at IS NULL;

ALTER TABLE cairnops_incident_activity DROP CONSTRAINT cairnops_incident_activity_origin_check;
ALTER TABLE cairnops_incident_activity ADD CONSTRAINT cairnops_incident_activity_origin_check CHECK (
    origin IN ('cairnops', 'zabbix', 'uptime_kuma', 'webhook', 'user', 'native')
);
