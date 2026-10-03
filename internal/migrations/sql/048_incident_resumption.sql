-- Reprise d'un Incident récemment résolu (ADR 0054). Les Incidents existants
-- n'ont jamais été repris : leur compteur part de zéro.
ALTER TABLE cairnops_incidents
    ADD COLUMN resumption_count integer NOT NULL DEFAULT 0 CHECK (resumption_count >= 0),
    ADD COLUMN resumed_at timestamptz;

-- Une Résolution n'est livrée en alerte que si elle clôt une alerte livrée
-- depuis la Résolution précédente : chaque envoi Push retient donc ce qu'il
-- annonçait. Les envois antérieurs restent sans valeur et ne sont pas
-- reconstitués.
ALTER TABLE cairnops_push_outbox
    ADD COLUMN event_kind text
    CHECK (event_kind IN ('firing', 'resolved', 'incident_update'));

ALTER TABLE cairnops_incident_activity DROP CONSTRAINT cairnops_incident_activity_kind_check;
ALTER TABLE cairnops_incident_activity ADD CONSTRAINT cairnops_incident_activity_kind_check CHECK (kind IN (
    'opened', 'impact_joined', 'impact_reopened', 'impact_resolved',
    'evidence_added', 'evidence_updated', 'evidence_resolved', 'invalidated',
    'propagation_closed', 'extended', 'severity_changed', 'resolved',
    'acknowledged', 'ack_sync_succeeded', 'ack_sync_failed',
    'upstream_acknowledged', 'target_reconciled', 'source_reassigned',
    'resumed'
));
