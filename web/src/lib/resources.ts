import type { Incident, IncidentEvidence, IncidentImpact, Maintenance, ResourceCategory, Target, TargetMeasures } from './api.ts';

export const resourceCategories: ResourceCategory[] = ['service', 'infrastructure', 'scheduled_task', 'software', 'unclassified'];
export function resourceCategoryFromParam(value: string | null): ResourceCategory | 'all' {
  return resourceCategories.find((category) => category === value) ?? 'all';
}
const weights = { critical: 4, major: 3, warning: 2, information: 1 };
export const isVersionNotice = (incident: Incident) => incident.nature_key === 'software-update-available';
export function resourceProblems(targetId: string, incidents: Incident[]) {
  return incidents.filter((incident) => incident.status === 'active' && !isVersionNotice(incident)).flatMap((incident) =>
    incident.impacts.filter((impact) => impact.target_id === targetId && impact.status === 'active').map((impact) => ({ incident, impact }))
  ).sort((a, b) => weights[b.impact.effective_severity] - weights[a.impact.effective_severity] || a.incident.opened_at.localeCompare(b.incident.opened_at));
}
export function problemText(problem: { incident: Incident; impact: IncidentImpact }, locale: 'fr' | 'en') {
  const evidence = problem.impact.evidence.filter((item) => item.active && !item.invalidated_at);
  const proof = [...evidence].sort((a,b) => weights[b.severity]-weights[a.severity])[0];
  return proof?.presentation?.[locale]?.title || problem.incident.presentation?.[locale]?.title || proof?.name || problem.incident.nature_label;
}
export function problemOrigin(impact: IncidentImpact) {
  const names: Record<string, string> = {native: 'CairnOps', zabbix: 'Zabbix', uptime_kuma: 'Uptime Kuma', argus: 'Argus', patchmon: 'PatchMon', proxmox: 'Proxmox VE', webhook: 'Webhook'};
  return [...new Set(impact.evidence.filter((item) => item.active && !item.invalidated_at).map((item) => item.connector_name || names[item.origin] || item.origin))].join(', ');
}
export function fresh(date: string | undefined, now: number, intervalSeconds = 300) {
  const age = now - Date.parse(date ?? '');
  return Number.isFinite(age) && age >= -60_000 && age <= Math.max(900, intervalSeconds * 3) * 1000;
}
export function resourceUnderMaintenance(targetID: string, incidents: Incident[], now: number, maintenances?: Maintenance[], maintenancesComplete = maintenances !== undefined): boolean {
  // A planned window applies even when no incident exists. Read its dates,
  // since the last server snapshot may predate its start or end.
  if (maintenances?.some((window) => !window.cancelled_at &&
    Date.parse(window.starts_at) <= now && now <= Date.parse(window.ends_at) &&
    window.targets.some((item) => item.id === targetID))) return true;
  // Known windows remain useful in a partial or cached list. Only a complete
  // list can establish absence and override an older incident projection.
  if (maintenancesComplete) return false;
  // Otherwise the incident projection supplies a bounded fallback. A stale
  // boolean must never hide an incident forever.
  return incidents.some((incident) => incident.impacts.some((impact) =>
    impact.target_id === targetID && impact.maintenance_active &&
    Date.parse(impact.maintenance_ends_at ?? '') >= now));
}
export function resourceState(target: Target, incidents: Incident[], measured?: TargetMeasures, now = Date.now(), maintenances?: Maintenance[], maintenancesComplete = maintenances !== undefined): 'ok' | 'down' | 'degraded' | 'unknown' | 'maintenance' {
  if (resourceUnderMaintenance(target.id, incidents, now, maintenances, maintenancesComplete)) return 'maintenance';
  const own = resourceProblems(target.id, incidents);
  // Severity never establishes availability. Only the canonical availability
  // condition can do so; stale/non-availability observations cannot prove UP.
  if (own.some(({incident, impact}) => incident.nature_key === 'availability' && impact.evidence.some((e) => e.active && !e.invalidated_at && fresh(e.last_seen_at, now)))) return 'down';
  const available = measured?.sources.some((source) => source.enabled !== false && source.measures_availability && source.latest_outcome === 'healthy' && fresh(source.latest_observed_at, now, source.interval_seconds ?? target.sources.find((s) => s.id === source.source_id)?.interval_seconds));
  const nativeAvailable = target.sources.some((source) => source.enabled && source.latest_outcome === 'healthy' && fresh(source.last_observed_at, now, source.interval_seconds));
  return available || nativeAvailable ? 'ok' : 'unknown';
}
export function resourceDivergence(targetId: string, incidents: Incident[], now = Date.now()): boolean {
  // Proofs within one incident share the same semantic nature. Two unrelated
  // conditions, even on the same resource, do not contradict each other.
  return incidents.some((incident) => incident.status === 'active' && incident.impacts.some((impact) => {
    if (impact.target_id !== targetId || impact.status !== 'active') return false;
    const live = impact.evidence.filter((e: IncidentEvidence) => !e.invalidated_at && fresh(e.last_seen_at, now));
    return live.some((e) => e.active) && live.some((e) => !e.active && fresh(e.resolved_at, now));
  }));
}
