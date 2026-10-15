import type { RealtimeMessage } from './api';

/* Ce qu'un événement temps réel oblige à recharger.
 *
 * Les Doublons possibles et les appareils ont leurs propres écrans, qui se
 * chargent eux-mêmes : la liste des Ressources n'en dépend pas. Les
 * Observations arrivent par rafales à chaque cycle de Contrôle ; elles passent
 * par un rechargement limité plutôt que par cette table. */
export type Projection = 'health' | 'connectors' | 'incidents' | 'indicators' | 'maintenances' | 'notifications' | 'targets';

export function projectionsFor(kind: RealtimeMessage['kind']): Projection[] {
  switch (kind) {
    case 'component.heartbeat': return ['health'];
    case 'connector.changed': return ['connectors'];
    case 'incident.changed': return ['incidents'];
    case 'indicator.changed': return ['indicators'];
    case 'maintenance.changed': return ['maintenances', 'incidents'];
    case 'notification.changed': return ['notifications'];
    case 'observation.created':
    case 'reconciliation.changed':
    case 'device.changed':
      return [];
    // Un genre inconnu recharge les Ressources : mieux vaut une lecture de trop
    // qu'une projection figée.
    default: return ['targets'];
  }
}

export const observationRefreshInterval = 5000;

/* Exécute `run` au plus une fois par intervalle. Une demande reçue pendant
 * l'attente n'est pas perdue : elle part à la fin de l'intervalle. `minimum`
 * laisse une rafale d'événements se regrouper avant la première exécution. */
export function throttled(
  run: () => void,
  interval: number,
  minimum: number,
  schedule: (callback: () => void, delay: number) => unknown = setTimeout,
  cancel: (timer: unknown) => void = (timer) => clearTimeout(timer as ReturnType<typeof setTimeout>),
  now: () => number = Date.now
) {
  let lastRun = -Infinity;
  let timer: unknown;
  return {
    request() {
      if (timer !== undefined) return;
      const delay = Math.max(minimum, lastRun + interval - now());
      timer = schedule(() => {
        timer = undefined;
        lastRun = now();
        run();
      }, delay);
    },
    cancel() {
      if (timer !== undefined) cancel(timer);
      timer = undefined;
    }
  };
}
