import type { LatencyProfile, LatencyProfileHour } from '$lib/api';

/** Le seau qui réunit toutes les heures de la fenêtre. */
export const allHours = -1;

/**
 * Résumé affichable du Profil de latence d'une Source à un instant : sa
 * latence habituelle, le seuil au-delà duquel elle cesse de l'être, et
 * l'étendue sur laquelle cela s'appuie.
 */
export type LatencyHabit = {
  median: number;
  threshold: number;
  samples: number;
  /** L'heure concernée, ou toutes les heures confondues en repli. */
  scope: 'hour' | 'all';
};

/**
 * Choisit le seau qui décrit un instant : celui de son heure UTC lorsqu'il
 * établit un seuil, sinon celui de toutes les heures confondues.
 *
 * Un seuil nul signale un seau trop pauvre. C'est le serveur qui en décide,
 * et le client s'en remet à lui plutôt que de reproduire sa règle : une
 * seconde définition du « assez d'Observations » finirait par en contredire
 * la première.
 */
export function bucketAt(
  profile: LatencyProfile | undefined,
  at: Date
): LatencyProfileHour | null {
  if (!profile) return null;
  const established = (hour: number) =>
    profile.hours.find((bucket) => bucket.hour === hour && bucket.threshold_milliseconds !== null);
  return established(at.getUTCHours()) ?? established(allHours) ?? null;
}

/**
 * Rend l'habitude de latence d'une Source, ou null lorsque le Profil ne
 * l'établit pas. Rien n'est inventé en attendant assez d'Observations.
 */
export function latencyHabit(
  profile: LatencyProfile | undefined,
  at: Date = new Date()
): LatencyHabit | null {
  const bucket = bucketAt(profile, at);
  if (!bucket || bucket.threshold_milliseconds === null) return null;
  return {
    median: bucket.median_milliseconds,
    threshold: bucket.threshold_milliseconds,
    samples: bucket.samples,
    scope: bucket.hour === allHours ? 'all' : 'hour'
  };
}
