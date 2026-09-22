import type { Incident, IncidentFilterOption, IncidentSeverity } from './api';

export type IncidentFilters = {
  query: string;
  targetID: string;
  natureKey: string;
  severity: IncidentSeverity | '';
};

export type HistoryPeriod = '7' | '30' | '90' | 'all' | 'custom';
export type HistoryBounds = { from?: string; before?: string };

const severityOrder: Record<IncidentSeverity, number> = {
  critical: 0, major: 1, warning: 2, information: 3
};

/** The tie-breaker is stable even when incidents share their opening instant. */
export function compareActiveIncidents(left: Incident, right: Incident): number {
  return Number(Boolean(left.acknowledged_at)) - Number(Boolean(right.acknowledged_at))
    || severityOrder[left.severity] - severityOrder[right.severity]
    || Date.parse(left.opened_at) - Date.parse(right.opened_at)
    || left.id.localeCompare(right.id);
}

export function matchesIncidentFilters(incident: Incident, filters: IncidentFilters): boolean {
  if (filters.targetID && !incident.impacts.some((impact) => impact.target_id === filters.targetID)) return false;
  if (filters.natureKey && incident.nature_key !== filters.natureKey) return false;
  if (filters.severity && incident.severity !== filters.severity) return false;
  const query = filters.query.trim().toLowerCase();
  if (!query) return true;
  return [incident.nature_label, incident.nature_key, incident.presentation?.fr.title ?? '', incident.presentation?.en.title ?? '', ...incident.impacts.flatMap((impact) => [
    impact.target_name, ...impact.evidence.map((evidence) => evidence.name)
  ])].some((value) => value.toLowerCase().includes(query));
}

export function incidentFilterOptions(incidents: Incident[]): { targets: IncidentFilterOption[]; natures: IncidentFilterOption[] } {
  const targets = new Map<string, string>();
  const natures = new Map<string, string>();
  for (const incident of incidents) {
    natures.set(incident.nature_key, incident.nature_label);
    for (const impact of incident.impacts) targets.set(impact.target_id, impact.target_name);
  }
  const options = (values: Map<string, string>) => [...values].map(([value, label]) => ({ value, label }))
    .sort((left, right) => left.label.localeCompare(right.label));
  return { targets: options(targets), natures: options(natures) };
}

export function localDateValue(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function parseLocalDate(value: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const date = new Date(`${value}T00:00:00`);
  return Number.isFinite(date.getTime()) && localDateValue(date) === value ? date : null;
}

/** Calendar boundaries use the reader's timezone, including DST transitions. */
export function historyBounds(period: HistoryPeriod, from: string, through: string, now = new Date()): HistoryBounds | null {
  if (period === 'all') return {};
  let start: Date;
  let end: Date;
  if (period === 'custom') {
    const parsedFrom = parseLocalDate(from);
    const parsedThrough = parseLocalDate(through);
    if (!parsedFrom || !parsedThrough || parsedFrom > parsedThrough) return null;
    start = parsedFrom;
    end = parsedThrough;
  } else {
    start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    end = new Date(start);
    start.setDate(start.getDate() - Number(period) + 1);
  }
  end.setDate(end.getDate() + 1);
  return { from: start.toISOString(), before: end.toISOString() };
}

export function resolvedIncidentQuery(filters: IncidentFilters, bounds: HistoryBounds, cursor = ''): string {
  const query = new URLSearchParams({ status: 'resolved', page: 'true', limit: '50' });
  if (filters.query.trim()) query.set('q', filters.query.trim());
  if (filters.targetID) query.set('target_id', filters.targetID);
  if (filters.natureKey) query.set('nature_key', filters.natureKey);
  if (filters.severity) query.set('severity', filters.severity);
  if (bounds.from) query.set('resolved_from', bounds.from);
  if (bounds.before) query.set('resolved_before', bounds.before);
  if (cursor) query.set('cursor', cursor);
  return `/api/v1/incidents?${query}`;
}
