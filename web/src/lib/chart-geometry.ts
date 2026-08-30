import type { IndicatorPoint } from './api';

export type ChartCoordinate = {
  at: string;
  value: number;
  index: number;
  x: number;
  y: number;
};

type ChartGeometry = {
  width: number;
  height: number;
  insetX: number;
  insetTop: number;
  insetBottom: number;
  bounds: [number, number];
};

const rounded = (value: number) => Math.round(value * 100) / 100;
const printed = (value: number) => String(rounded(value));

/**
 * Positionne les preuves selon leur temps réel. Une collecte irrégulière garde
 * donc ses silences au lieu de répartir artificiellement tous les points.
 */
export function chartCoordinates(
  points: Array<Pick<IndicatorPoint, 'at' | 'value'>>,
  geometry: ChartGeometry
): ChartCoordinate[] {
  const { width, height, insetX, insetTop, insetBottom, bounds } = geometry;
  if (points.length === 0) return [];

  const times = points.map((point) => new Date(point.at).getTime());
  const timed = times.every(Number.isFinite) && times.at(-1)! > times[0];
  const firstTime = timed ? times[0] : 0;
  const timeSpan = timed ? times.at(-1)! - firstTime : Math.max(1, points.length - 1);
  const plotWidth = Math.max(0, width - insetX * 2);
  const plotHeight = Math.max(0, height - insetTop - insetBottom);
  const low = Math.min(bounds[0], bounds[1]);
  const high = Math.max(bounds[0], bounds[1]);
  const valueSpan = Math.max(1e-9, high - low);

  return points.map((point, index) => {
    const progress = timed
      ? (times[index] - firstTime) / timeSpan
      : points.length === 1
        ? 0.5
        : index / (points.length - 1);
    const value = Math.min(high, Math.max(low, point.value));
    return {
      at: point.at,
      value: point.value,
      index,
      x: rounded(insetX + progress * plotWidth),
      y: rounded(insetTop + (1 - (value - low) / valueSpan) * plotHeight)
    };
  });
}

/** Courbe de Hermite monotone : elle adoucit les angles sans dépasser les preuves. */
export function monotoneChartPath(points: ChartCoordinate[]): string {
  if (points.length === 0) return '';
  if (points.length === 1) return `M${printed(points[0].x)},${printed(points[0].y)}`;

  const slopes = points.slice(0, -1).map((point, index) => {
    const next = points[index + 1];
    const distance = next.x - point.x;
    return distance === 0 ? 0 : (next.y - point.y) / distance;
  });
  const tangents = points.map((_, index) => {
    if (index === 0) return slopes[0];
    if (index === points.length - 1) return slopes.at(-1)!;
    return (slopes[index - 1] + slopes[index]) / 2;
  });

  for (let index = 0; index < slopes.length; index += 1) {
    const slope = slopes[index];
    if (slope === 0) {
      tangents[index] = 0;
      tangents[index + 1] = 0;
      continue;
    }
    const before = tangents[index] / slope;
    const after = tangents[index + 1] / slope;
    const magnitude = before * before + after * after;
    if (magnitude <= 9) continue;
    const scale = 3 / Math.sqrt(magnitude);
    tangents[index] = scale * before * slope;
    tangents[index + 1] = scale * after * slope;
  }

  let path = `M${printed(points[0].x)},${printed(points[0].y)}`;
  for (let index = 0; index < points.length - 1; index += 1) {
    const point = points[index];
    const next = points[index + 1];
    const distance = next.x - point.x;
    path += [
      'C',
      printed(point.x + distance / 3),
      ',',
      printed(point.y + (tangents[index] * distance) / 3),
      ' ',
      printed(next.x - distance / 3),
      ',',
      printed(next.y - (tangents[index + 1] * distance) / 3),
      ' ',
      printed(next.x),
      ',',
      printed(next.y)
    ].join('');
  }
  return path;
}

export function chartAreaPath(points: ChartCoordinate[], baseline: number): string {
  const line = monotoneChartPath(points);
  if (!line || points.length < 2) return '';
  return `${line}L${printed(points.at(-1)!.x)},${printed(baseline)}L${printed(points[0].x)},${printed(baseline)}Z`;
}

export function nearestChartPoint(
  points: ChartCoordinate[],
  x: number
): ChartCoordinate | null {
  if (points.length === 0) return null;
  return points.reduce((closest, point) =>
    Math.abs(point.x - x) < Math.abs(closest.x - x) ? point : closest
  );
}

export function chartSegments(
  points: ChartCoordinate[],
  gapThresholdMilliseconds: number | null
): ChartCoordinate[][] {
  if (points.length === 0) return [];
  if (!gapThresholdMilliseconds || gapThresholdMilliseconds <= 0) return [points];

  const segments: ChartCoordinate[][] = [];
  for (const point of points) {
    const segment = segments.at(-1);
    const previous = segment?.at(-1);
    const gap = previous
      ? new Date(point.at).getTime() - new Date(previous.at).getTime()
      : 0;
    if (!segment || !Number.isFinite(gap) || gap > gapThresholdMilliseconds) {
      segments.push([point]);
    } else {
      segment.push(point);
    }
  }
  return segments;
}
