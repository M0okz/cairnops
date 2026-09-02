-- Une ancienne suppression de Source pouvait laisser un Incident actif après
-- la suppression en cascade de sa dernière preuve. Ces Incidents sans preuve
-- active contredisent leur propre cycle de vie et apparaissent en 0/0 dans
-- l'interface. La migration les résout sans réinventer leur historique.
WITH resolved AS (
    UPDATE cairnops_incidents incident
    SET status = 'resolved', resolved_at = now(), updated_at = now()
    WHERE incident.status = 'active'
      AND NOT EXISTS (
          SELECT 1
          FROM cairnops_incident_signals signal
          WHERE signal.incident_id = incident.id AND signal.active
      )
    RETURNING incident.id
)
INSERT INTO cairnops_incident_activity (incident_id, kind, origin, message, data)
SELECT id, 'resolved', 'cairnops',
       'Incident résolu : aucune preuve active ne subsistait',
       '{"reason":"orphaned_incident_repair"}'::jsonb
FROM resolved;
