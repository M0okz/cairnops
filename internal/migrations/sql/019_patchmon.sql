-- PatchMon apporte une posture de maintenance et de sécurité, pas une
-- conclusion de Disponibilité. La Source reste observable et peut alimenter
-- des Incidents, mais ses relevés n'entrent dans aucun agrégat de disponibilité.
ALTER TABLE cairnops_connectors DROP CONSTRAINT cairnops_connectors_kind_check;
ALTER TABLE cairnops_connectors ADD CONSTRAINT cairnops_connectors_kind_check CHECK (
    kind IN ('zabbix', 'uptime_kuma', 'generic_webhook', 'patchmon')
);

ALTER TABLE cairnops_signal_sources DROP CONSTRAINT cairnops_signal_sources_kind_check;
ALTER TABLE cairnops_signal_sources ADD CONSTRAINT cairnops_signal_sources_kind_check CHECK (
    kind IN ('http', 'tcp', 'dns', 'icmp', 'heartbeat', 'zabbix', 'uptime_kuma', 'generic_webhook', 'patchmon')
);

ALTER TABLE cairnops_signal_sources
    ADD COLUMN measures_availability boolean NOT NULL DEFAULT true;

ALTER TABLE cairnops_incident_signals DROP CONSTRAINT cairnops_incident_signals_origin_check;
ALTER TABLE cairnops_incident_signals ADD CONSTRAINT cairnops_incident_signals_origin_check CHECK (
    origin IN ('zabbix', 'uptime_kuma', 'webhook', 'native', 'patchmon')
);

ALTER TABLE cairnops_incident_signals DROP CONSTRAINT cairnops_incident_signals_check1;
ALTER TABLE cairnops_incident_signals ADD CONSTRAINT cairnops_incident_signals_origin_fields_check CHECK (
    (origin IN ('zabbix', 'uptime_kuma', 'patchmon')
        AND connector_id IS NOT NULL AND connector_binding_id IS NOT NULL
        AND length(btrim(external_event_id)) > 0)
    OR (origin = 'native' AND source_id IS NOT NULL)
    OR origin = 'webhook'
);

ALTER TABLE cairnops_incident_activity DROP CONSTRAINT cairnops_incident_activity_origin_check;
ALTER TABLE cairnops_incident_activity ADD CONSTRAINT cairnops_incident_activity_origin_check CHECK (
    origin IN ('cairnops', 'zabbix', 'uptime_kuma', 'webhook', 'user', 'native', 'patchmon')
);

CREATE UNIQUE INDEX cairnops_incident_signals_patchmon_active_idx
    ON cairnops_incident_signals (connector_id, connector_binding_id, external_object_id)
    WHERE origin = 'patchmon' AND active;
