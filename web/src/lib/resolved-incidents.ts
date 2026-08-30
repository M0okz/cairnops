export type IncidentScope = 'active' | 'unacknowledged' | 'resolved';

type IncidentIdentity = { id: string };

export function incidentMembershipChanged(
  current: IncidentIdentity[],
  next: IncidentIdentity[]
): boolean {
  if (current.length !== next.length) return true;
  const currentIDs = new Set(current.map((incident) => incident.id));
  return next.some((incident) => !currentIDs.has(incident.id));
}

/* La liste résolue est volontairement paresseuse : elle ne coûte une requête
 * que lorsque son filtre est visible. Sa révision suit toutefois les
 * changements d'Incident reçus en temps réel afin qu'une Résolution ne reste
 * pas prisonnière d'une ancienne copie. */
export function shouldLoadResolvedIncidents(
  scope: IncidentScope,
  loadedRevision: number,
  incidentRevision: number
): boolean {
  return scope === 'resolved' && loadedRevision !== incidentRevision;
}
