import type { ContextIndicator, IndicatorPoint } from './api';

const hour = 3_600_000;
const day = 24 * hour;
const minimumHours = 30;

export type KumaLatencyEvaluation =
  | { status: 'unavailable' | 'training' }
  | { status: 'usual' | 'candidate'; observed: number; median: number; threshold: number; hours: number };

function quantile(sorted: number[], proportion: number): number {
  const position = (sorted.length - 1) * proportion;
  const lower = Math.floor(position);
  return sorted[lower] + (sorted[Math.ceil(position)] - sorted[lower]) * (position - lower);
}

/** The six preceding days train the baseline; the latest reading is held out. */
export function evaluateKumaLatency(
  indicator: ContextIndicator,
  weeklyPoints: IndicatorPoint[] | undefined,
  now: Date
): KumaLatencyEvaluation {
  const observedAt = Date.parse(indicator.last_observed_at ?? '');
  const observed = indicator.last_value;
  const current = now.getTime();
  if (indicator.last_error || !Number.isFinite(current) || !Number.isFinite(observedAt) ||
      current - observedAt < 0 || current - observedAt > 10 * 60_000 ||
      observed === undefined || !Number.isFinite(observed) || observed <= 0) {
    return { status: 'unavailable' };
  }
  if (!weeklyPoints) return { status: 'unavailable' };

  const byHour = new Map<number, number>();
  for (const point of weeklyPoints) {
    const at = Date.parse(point.at);
    if (!Number.isFinite(at) || at < current - 7 * day || at >= current - day ||
        !Number.isFinite(point.value) || point.value <= 0) continue;
    byHour.set(Math.floor(at / hour), point.value);
  }
  const values = [...byHour.values()].sort((left, right) => left - right);
  if (values.length < minimumHours) return { status: 'training' };
  const median = quantile(values, 0.5);
  const threshold = Math.max(quantile(values, 0.99), 2 * median, median + 50);
  return { status: observed > threshold ? 'candidate' : 'usual', observed, median, threshold, hours: values.length };
}
