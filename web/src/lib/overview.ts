import type { InstanceHour } from './api';

const hourMilliseconds = 60 * 60 * 1000;

/**
 * Aligne la couverture reçue du serveur sur une vraie fenêtre horaire.
 * Une heure absente ou sans Observation attendue reste inconnue : elle ne
 * doit surtout pas devenir une heure verte par défaut.
 */
export function coverageWindow(
  hours: InstanceHour[],
  checkedAt: string | undefined,
  slots = 24
): Array<number | null> {
  const checkedAtMilliseconds = checkedAt ? new Date(checkedAt).getTime() : Number.NaN;
  if (!Number.isFinite(checkedAtMilliseconds) || slots < 1) return [];

  const currentHour = Math.floor(checkedAtMilliseconds / hourMilliseconds) * hourMilliseconds;
  const firstHour = currentHour - (slots - 1) * hourMilliseconds;
  const values = Array.from<number | null>({ length: slots }).fill(null);

  for (const hour of hours) {
    const timestamp = new Date(hour.hour).getTime();
    if (!Number.isFinite(timestamp)) continue;
    const index = Math.round((timestamp - firstHour) / hourMilliseconds);
    if (index < 0 || index >= slots || hour.expected_observations <= 0) continue;
    values[index] = Math.max(
      0,
      Math.min(1, hour.conclusive_observations / hour.expected_observations)
    );
  }

  return values;
}
