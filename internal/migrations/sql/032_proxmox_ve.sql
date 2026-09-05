ALTER TABLE cairnops_connectors DROP CONSTRAINT cairnops_connectors_kind_check;
ALTER TABLE cairnops_connectors ADD CONSTRAINT cairnops_connectors_kind_check
    CHECK (kind IN ('zabbix', 'uptime_kuma', 'generic_webhook', 'patchmon', 'argus', 'proxmox'));
ALTER TABLE cairnops_signal_sources DROP CONSTRAINT cairnops_signal_sources_kind_check;
ALTER TABLE cairnops_signal_sources ADD CONSTRAINT cairnops_signal_sources_kind_check
    CHECK (kind IN ('http', 'tcp', 'dns', 'icmp', 'heartbeat', 'zabbix', 'uptime_kuma', 'generic_webhook', 'patchmon', 'argus', 'proxmox'));

-- Migration 031 used unnamed multi-column CHECKs. Select precisely the two
-- origin checks by their definition instead of relying on generated numbering.
DO $$
DECLARE constraint_name text;
BEGIN
    FOR constraint_name IN SELECT conname FROM pg_constraint
        WHERE conrelid = 'cairnops_incident_evidence'::regclass AND contype = 'c'
          AND pg_get_constraintdef(oid) ~ '\morigin\M'
    LOOP EXECUTE format('ALTER TABLE cairnops_incident_evidence DROP CONSTRAINT %I', constraint_name); END LOOP;
END $$;
ALTER TABLE cairnops_incident_evidence ADD CONSTRAINT cairnops_incident_evidence_origin_check
    CHECK (origin IN ('zabbix', 'uptime_kuma', 'webhook', 'native', 'patchmon', 'argus', 'proxmox'));
ALTER TABLE cairnops_incident_evidence ADD CONSTRAINT cairnops_incident_evidence_origin_fields_check CHECK (
    (NOT active)
    OR (origin IN ('zabbix', 'uptime_kuma', 'patchmon', 'argus', 'proxmox')
        AND connector_id IS NOT NULL AND connector_binding_id IS NOT NULL
        AND length(btrim(external_event_id)) > 0)
    OR (origin = 'native' AND source_id IS NOT NULL)
    OR origin = 'webhook'
);
ALTER TABLE cairnops_incident_activity DROP CONSTRAINT cairnops_incident_activity_origin_check;
ALTER TABLE cairnops_incident_activity ADD CONSTRAINT cairnops_incident_activity_origin_check
    CHECK (origin IN ('cairnops', 'zabbix', 'uptime_kuma', 'webhook', 'user', 'native', 'patchmon', 'argus', 'proxmox'));

CREATE TABLE cairnops_proxmox_inventory (
    connector_id uuid NOT NULL REFERENCES cairnops_connectors(id) ON DELETE CASCADE,
    external_id text NOT NULL CHECK (length(external_id) BETWEEN 1 AND 512),
    pending boolean NOT NULL DEFAULT false,
    discovered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (connector_id, external_id)
);
