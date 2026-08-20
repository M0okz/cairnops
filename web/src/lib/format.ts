/* Mise en forme partagée par les huit écrans. Un seul endroit décide comment
 * une Gravité se nomme, comment une durée se dit et comment un nombre
 * s'écrit — les Écrans supposent cette cohérence d'un écran à l'autre.
 *
 * Tout ce qui se lit passe par le dictionnaire, et tout ce qui se chiffre par
 * Intl : une date se dit dans le fuseau de l'utilisateur et dans sa langue,
 * jamais dans celle du serveur. */

import type { Incident, IncidentSeverity, Measure, MeasureWindow, Target } from './api';
import { localeTag, t } from './i18n.svelte';

export type Tone = 'ok' | 'warn' | 'crit' | 'info' | 'idle';

const severityTones: Record<IncidentSeverity, Tone> = {
  information: 'info',
  warning: 'warn',
  major: 'crit',
  critical: 'crit'
};

const severityRank: Record<IncidentSeverity, number> = {
  critical: 4,
  major: 3,
  warning: 2,
  information: 1
};

export const severityLabel = (severity: IncidentSeverity) => t(`severity.${severity}`);
export const severityTone = (severity: IncidentSeverity): Tone => severityTones[severity];
export const severityWeight = (severity: IncidentSeverity) => severityRank[severity];

export type TargetState = 'down' | 'degraded' | 'maintenance' | 'unknown' | 'ok';

export const stateLabel = (state: TargetState) => t(`state.${state}`);

export const stateTones: Record<TargetState, Tone> = {
  down: 'crit',
  degraded: 'warn',
  maintenance: 'info',
  unknown: 'idle',
  ok: 'ok'
};

/* Les formateurs suivent la langue courante. Les construire à chaque appel
 * coûterait cher sur une liste de mille lignes ; ils sont donc retenus, et
 * jetés dès que la langue change. */
const formatters = new Map<string, Intl.NumberFormat | Intl.DateTimeFormat>();

function cached<T extends Intl.NumberFormat | Intl.DateTimeFormat>(name: string, build: () => T): T {
  const key = `${localeTag()}:${name}`;
  const existing = formatters.get(key);
  if (existing) return existing as T;
  if (formatters.size > 16) formatters.clear();
  const built = build();
  formatters.set(key, built);
  return built;
}

const nf = () =>
  cached(
    'number',
    () => new Intl.NumberFormat(localeTag(), { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  );
const dtf = () =>
  cached(
    'stamp',
    () =>
      new Intl.DateTimeFormat(localeTag(), {
        day: 'numeric',
        month: 'long',
        hour: '2-digit',
        minute: '2-digit'
      })
  );
const timef = () =>
  cached('clock', () => new Intl.DateTimeFormat(localeTag(), { hour: '2-digit', minute: '2-digit' }));
const dayf = () =>
  cached(
    'day',
    () =>
      new Intl.DateTimeFormat(localeTag(), { weekday: 'long', day: 'numeric', month: 'long' })
  );

export const percent = (ratio: number) => `${nf().format(ratio * 100)} %`;
export const clock = (value: string | Date) => timef().format(new Date(value));
export const stamp = (value: string | Date) => dtf().format(new Date(value));
export const today = (value: Date = new Date()) =>
  `${dayf().format(value)} · ${timef().format(value)}`;

/** Durées courtes et lisibles : « 18 min », « 2 h 41 », « 6 j ». */
export function duration(seconds: number): string {
  seconds = Math.max(0, Math.round(seconds));
  if (seconds < 60) return t('duration.seconds', { count: seconds });
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return t('duration.minutes', { count: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return minutes % 60 === 0
      ? t('duration.hours', { count: hours })
      : t('duration.hoursMinutes', { hours, minutes: String(minutes % 60).padStart(2, '0') });
  }
  return t('duration.days', { count: Math.floor(hours / 24) });
}

/** Le temps écoulé depuis un instant, dit comme une durée. */
export function since(value: string | Date, to: Date = new Date()): string {
  return duration((to.getTime() - new Date(value).getTime()) / 1000);
}

export function until(value: string | Date, from: Date = new Date()): string {
  return since(from, new Date(value));
}

/* ── Mesures ──────────────────────────────────────────────────────────────
 * La Disponibilité, la Couverture et la latence sont établies par le serveur
 * sur des agrégats horaires ; l'interface ne les recalcule pas. Elle décide
 * seulement comment se dit une mesure absente. */

export const emptyMeasure: Measure = {
  window: '24h',
  availability: null,
  coverage: null,
  average_latency_milliseconds: null,
  maximum_latency_milliseconds: null,
  conclusive_observations: 0,
  unknown_observations: 0,
  expected_observations: 0
};

export const windowLabel = (window: MeasureWindow) => t(`window.${window}`);

/** La mesure d'une fenêtre, ou une mesure vide tant que rien n'est chargé. */
export function inWindow(
  measured: { measures: Measure[] } | null | undefined,
  window: MeasureWindow
): Measure {
  return measured?.measures.find((measure) => measure.window === window) ?? { ...emptyMeasure, window };
}

/** Un ratio absent s'écrit « — » : zéro affirmerait ce que rien n'établit. */
export const ratio = (value: number | null) => (value === null ? t('common.none') : percent(value));

export const latency = (milliseconds: number | null) =>
  milliseconds === null ? t('common.none') : `${milliseconds} ms`;

/** La dernière Observation connue d'une Cible, toutes Sources confondues.
 *  Elle vient de la mesure : les Sources apportées par une Intégration ne
 *  figurent pas parmi les Contrôles natifs de la Cible, et compter sur ces
 *  derniers rendrait une installation entièrement importée éternellement muette. */
export function lastObserved(
  target: Target,
  measured?: { latest_observed_at?: string } | null
): string | null {
  const stamps = target.sources
    .map((source) => source.last_observed_at)
    .filter((value): value is string => Boolean(value));
  if (measured?.latest_observed_at) stamps.push(measured.latest_observed_at);
  return stamps.length > 0 ? stamps.reduce((left, right) => (left > right ? left : right)) : null;
}

/** L'Incident le plus grave d'une Cible — celui qui nomme sa Nature. */
export function leadIncident(incidents: Incident[]): Incident | null {
  if (incidents.length === 0) return null;
  return [...incidents].sort(
    (a, b) =>
      severityWeight(b.effective_severity) - severityWeight(a.effective_severity) ||
      new Date(a.opened_at).getTime() - new Date(b.opened_at).getTime()
  )[0];
}

/** Une preuve est en désaccord quand les Sources vivantes ne concluent pas
 *  la même chose. Cela ne crée pas un État de santé supplémentaire. */
export function diverges(incident: Incident): boolean {
  const live = incident.signals.filter((signal) => !signal.invalidated_at);
  return live.some((signal) => signal.active) && live.some((signal) => !signal.active);
}

export function activeSignalRatio(incident: Incident): string {
  const live = incident.signals.filter((signal) => !signal.invalidated_at);
  return `${live.filter((signal) => signal.active).length}/${live.length || 0}`;
}
