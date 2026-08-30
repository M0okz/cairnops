import type { IndicatorPoint, IndicatorUnit } from './api';

export function formatIndicator(value: number | undefined, unit: IndicatorUnit): string {
  if (value === undefined || !Number.isFinite(value)) return '—';
  switch (unit) {
    case 'percent': return `${new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 }).format(value)} %`;
    case 'bytes_per_second': return `${formatBytes(value)}/s`;
    case 'milliseconds': return `${new Intl.NumberFormat('fr-FR', { maximumFractionDigits: value < 100 ? 1 : 0 }).format(value)} ms`;
    case 'days': return `${new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 }).format(value)} j`;
    case 'count': return new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 }).format(value);
    case 'boolean': return value >= 0.5 ? 'Oui' : 'Non';
    case 'seconds': return formatDuration(value);
  }
}

function formatBytes(value: number): string {
  const units = ['o', 'Kio', 'Mio', 'Gio', 'Tio'];
  let scaled = Math.max(0, value);
  let index = 0;
  while (scaled >= 1024 && index < units.length - 1) { scaled /= 1024; index += 1; }
  return `${new Intl.NumberFormat('fr-FR', { maximumFractionDigits: scaled < 10 ? 1 : 0 }).format(scaled)} ${units[index]}`;
}

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)} s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)} min`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)} h`;
  return `${Math.round(seconds / 86400)} j`;
}

export function indicatorBounds(points: IndicatorPoint[], unit: IndicatorUnit): [number, number] {
  if (unit === 'percent') return [0, 100];
  if (unit === 'boolean') return [0, 1];
  const values = points.flatMap((point) => [point.value, point.minimum ?? point.value, point.maximum ?? point.value]).filter(Number.isFinite);
  if (values.length === 0) return [0, 1];
  const low = unit === 'days' ? 0 : Math.min(...values);
  const high = Math.max(...values);
  if (low === high) return unit === 'days' ? [0, Math.max(1, high * 1.1)] : [Math.max(0, low - Math.max(1, Math.abs(low) * .1)), high + Math.max(1, Math.abs(high) * .1)];
  const padding = (high - low) * .08;
  return [unit === 'days' ? 0 : Math.max(0, low - padding), high + padding];
}

export function incidentMarkerIndex(points: IndicatorPoint[], openedAt: string): number | null {
  const opened = new Date(openedAt).getTime();
  if (!Number.isFinite(opened) || points.length === 0) return null;

  let closest: number | null = null;
  let distance = Number.POSITIVE_INFINITY;
  points.forEach((point, index) => {
    const observed = new Date(point.at).getTime();
    if (!Number.isFinite(observed)) return;
    const candidate = Math.abs(observed - opened);
    if (candidate < distance) {
      closest = index;
      distance = candidate;
    }
  });
  return closest;
}

export function incidentMarkerLabelY(
  markerY: number,
  chartHeight: number,
  labelHeight: number,
  inset = 3
): number {
  const markerRadius = 3.25;
  const top = inset;
  const bottom = Math.max(top, chartHeight - inset - labelHeight);
  const clearanceAbove = markerY - markerRadius - (top + labelHeight);
  const clearanceBelow = bottom - (markerY + markerRadius);
  return clearanceBelow > clearanceAbove ? bottom : top;
}

export function incidentMarkerIsVisible(
  openedAt: string,
  generatedAt: string,
  window: '24h' | '7d'
): boolean {
  const opened = new Date(openedAt).getTime();
  const generated = new Date(generatedAt).getTime();
  if (!Number.isFinite(opened) || !Number.isFinite(generated)) return false;
  const duration = window === '24h' ? 24 * 60 * 60 * 1_000 : 7 * 24 * 60 * 60 * 1_000;
  return opened >= generated - duration && opened <= generated;
}
