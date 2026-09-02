-- Le passage des problèmes Zabbix à une Nature issue du prototype pouvait
-- déplacer une preuve existante vers un nouvel Incident sans recalculer
-- l'ancien. Réparer les Incidents ainsi vidés avant que le prochain cycle de
-- synchronisation ne s'exécute avec la logique corrigée.
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
