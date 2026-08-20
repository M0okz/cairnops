import type { Incident } from './api';

export type IncidentTimelineEntry = {
  incident: Incident;
  entry: Incident['activity'][number];
};

/**
 * Construit le Journal d'une Cible à partir de la projection actuellement
 * disponible. Le second jeu d'Incidents représente l'histoire résolue que le
 * détail de la Cible pourra charger séparément de l'état opérationnel actif.
 */
export function incidentTimelineForTarget(
  activeIncidents: Incident[],
  historicalIncidents: Incident[],
  targetId: string
): IncidentTimelineEntry[] {
  /* L'histoire chargée contient aussi les Incidents encore actifs. La
   * projection active, rafraîchie plus souvent, gagne donc en cas de doublon. */
  const incidents = new Map(
    [...historicalIncidents, ...activeIncidents].map((incident) => [incident.id, incident])
  );

  return [...incidents.values()]
    .filter((incident) => incident.target_id === targetId)
    .flatMap((incident) => incident.activity.map((entry) => ({ incident, entry })))
    .sort(
      (a, b) =>
        new Date(b.entry.occurred_at).getTime() - new Date(a.entry.occurred_at).getTime()
    );
}
