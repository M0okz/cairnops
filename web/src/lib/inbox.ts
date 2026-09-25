import type { InboxEntry } from './api';

/* État actuel d'une entrée de boîte.
 *
 * L'entrée garde ce qui a été reçu ; l'Incident, lui, a pu avancer depuis.
 * Une ouverture dont l'Incident est déjà résolu ne doit plus se lire comme une
 * alerte en cours, et une ouverture acquittée ne demande plus la même attention.
 * Les anciens serveurs ne transmettent pas cet état : l'entrée garde alors
 * seulement ce qu'elle a reçu. */
export type InboxEntryState = 'open' | 'acknowledged' | 'resolved';

export function inboxEntryState(
  entry: Pick<InboxEntry, 'event_kind' | 'incident_status' | 'acknowledged_at'>
): InboxEntryState {
  if (entry.event_kind === 'resolved' || entry.incident_status === 'resolved') return 'resolved';
  if (entry.acknowledged_at) return 'acknowledged';
  return 'open';
}

/* Ouvrir le volet marque son contenu comme lu. Les entrées qui étaient
 * nouvelles à cet instant restent signalées tant que le volet est ouvert :
 * sinon, le geste même de regarder effacerait ce qu'on venait chercher. */
export function unreadEntryIds(entries: Pick<InboxEntry, 'id' | 'read_at'>[]): Set<number> {
  return new Set(entries.filter((entry) => !entry.read_at).map((entry) => entry.id));
}
