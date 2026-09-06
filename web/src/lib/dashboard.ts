import type { Measure } from './api';

export type HealthState = 'ok' | 'down' | 'degraded' | 'unknown' | 'maintenance';

export function dashboardHealth(states: HealthState[]) {
  const counts: Record<HealthState, number> = { ok: 0, down: 0, degraded: 0, unknown: 0, maintenance: 0 };
  for (const state of states) counts[state]++;
  const state = states.length === 0 ? 'empty' : counts.down ? 'down' : counts.degraded ? 'degraded'
    : counts.unknown ? 'unknown' : counts.maintenance ? 'maintenance' : 'ok';
  return { counts, state, total: states.length, watched: counts.down + counts.degraded + counts.unknown };
}

/** Pondération identique à la synthèse précédente : aucune moyenne de pourcentages. */
export function dashboardCoverage(measures: Measure[]): number | null {
  let expected = 0;
  let covered = 0;
  for (const measure of measures) {
    if (measure.coverage === null || !Number.isFinite(measure.coverage) || measure.expected_observations <= 0) continue;
    expected += measure.expected_observations;
    covered += Math.max(0, Math.min(1, measure.coverage)) * measure.expected_observations;
  }
  return expected > 0 ? covered / expected : null;
}
