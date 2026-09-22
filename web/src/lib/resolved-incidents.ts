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

/* Le temps réel propose un nouveau snapshot, sans effacer les pages parcourues.
 * La révision reste celle du début de la première requête : un changement reçu
 * pendant cette requête ne doit pas être avalé par sa réponse. */
export function resolvedHistoryChanged(
  loadedRevision: number,
  incidentRevision: number
): boolean {
  return loadedRevision >= 0 && loadedRevision !== incidentRevision;
}
