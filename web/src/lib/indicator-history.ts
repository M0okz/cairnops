import type { IndicatorPoint } from './api';
import type { ChartCoordinate } from './chart-geometry';

export type IndicatorPeriod = '1h' | '24h' | '7d';
export const CHART_TWEEN_DURATION = 400;
const hour = 3_600_000;

/** A temporal marker does not snap to a sample or invent a measured value. */
export function indicatorMarkerX(at: string, bounds: [number, number], width: number, inset = 8): number | null {
  const time = Date.parse(at);
  const [start, end] = bounds;
  if (![time, start, end, width].every(Number.isFinite) || end <= start || width <= inset * 2 || time < start || time > end) return null;
  return inset + (time - start) / (end - start) * (width - inset * 2);
}

export function indicatorTimeBounds(generatedAt: string, period: IndicatorPeriod): [number, number] | null {
  const end = Date.parse(generatedAt);
  if (!Number.isFinite(end)) return null;
  return [end - (period === '1h' ? 1 : period === '24h' ? 24 : 168) * hour, end];
}

export function indicatorWindowPoints(points: IndicatorPoint[], bounds: [number, number] | null): IndicatorPoint[] {
  return points.filter((point) => {
    const time = Date.parse(point.at);
    return Number.isFinite(time) && Number.isFinite(point.value) && (!bounds || (time >= bounds[0] && time <= bounds[1]));
  }).sort((a, b) => Date.parse(a.at) - Date.parse(b.at));
}

/** Les bornes et les trous de collecte restent visibles, même avec un seul relevé. */
export function indicatorTimeTicks(bounds: [number, number], plotWidth: number): number[] {
  const [start, end] = bounds;
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return [];
  const count = Math.max(2, Math.floor(plotWidth / 72));
  const target = (end - start) / count;
  const steps = [1, 5, 10, 15, 30, 60, 120, 180, 240, 360, 720, 1440, 2880, 10080].map((minutes) => minutes * 60_000);
  // Multi-day axes show dates without hours: two ticks in a day would have
  // identical labels, even when the viewport has room for more ticks.
  const minimumStep = end - start > 36 * hour ? 24 * hour : 60_000;
  const step = steps.find((value) => value >= Math.max(minimumStep, target * 0.8)) ?? steps.at(-1)!;
  const ticks: number[] = [];
  for (let time = Math.ceil(start / step) * step; time <= end; time += step) ticks.push(time);
  return ticks;
}

/** Les maxima absents restent absents : une valeur instantanée n'est pas un maximum. */
export function maximumCoordinates(points: IndicatorPoint[], coordinates: ChartCoordinate[], bounds: [number, number], top: number, baseline: number): ChartCoordinate[] {
  return coordinates.flatMap((coordinate) => {
    const value = points[coordinate.index]?.maximum;
    if (value === undefined || !Number.isFinite(value)) return [];
    const ratio = (value - bounds[0]) / Math.max(1e-9, bounds[1] - bounds[0]);
    return [{ ...coordinate, value, y: baseline - Math.max(0, Math.min(1, ratio)) * (baseline - top) }];
  });
}

/** Une interpolation porte sur le dessin, jamais sur la valeur annoncée au survol.
 * Rééchantillonne l'ancien tracé en X pour accepter une nouvelle période, même
 * quand son nombre de points change. Un mouvement interrompu repart du dessin courant.
 */
export function interpolateChartCoordinates(from: ChartCoordinate[], to: ChartCoordinate[]): (progress: number) => ChartCoordinate[] {
  let left = 0;
  const starts = to.map((point) => {
    if (!from.length) return point.y;
    while (left < from.length - 2 && from[left + 1].x < point.x) left += 1;
    const a = from[left];
    const b = from[Math.min(left + 1, from.length - 1)];
    const ratio = b.x === a.x ? 0 : Math.max(0, Math.min(1, (point.x - a.x) / (b.x - a.x)));
    return a.y + (b.y - a.y) * ratio;
  });
  return (progress) => progress >= 1 ? to : to.map((point, index) => ({ ...point, y: starts[index] + (point.y - starts[index]) * progress }));
}

/** Un booléen ne passe pas par des états intermédiaires. */
export function stepChartPath(points: ChartCoordinate[]): string {
  return points.map((point, index) => index === 0 ? `M${point.x},${point.y}` : `H${point.x}V${point.y}`).join('');
}
