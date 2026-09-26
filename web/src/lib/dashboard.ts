import type { Incident, IncidentSeverity, Measure, ResourceCategory, Target } from './api';

export type HealthState = 'ok' | 'down' | 'degraded' | 'unknown' | 'maintenance';

export function dashboardHealth(states: HealthState[]) {
  const counts: Record<HealthState, number> = { ok: 0, down: 0, degraded: 0, unknown: 0, maintenance: 0 };
  for (const state of states) counts[state]++;
  const state = states.length === 0 ? 'empty' : counts.down ? 'down' : counts.degraded ? 'degraded'
    : counts.unknown ? 'unknown' : counts.maintenance ? 'maintenance' : 'ok';
  return { counts, state, total: states.length, watched: counts.down + counts.degraded + counts.unknown };
}

export type CategoryHealth = {
  category: ResourceCategory;
  counts: Record<HealthState, number>;
  total: number;
  problems: number;
  updates: number;
  tracking: { problems: number; updates: number; other: number };
  attention: { crit: number; warn: number; info: number };
};

/** Une ressource contribue à une seule catégorie et à un seul état courant. */
export function dashboardCategoryHealth(resources: Array<{ category?: ResourceCategory; state: HealthState; problem?: boolean; problemSeverity?: IncidentSeverity; update?: boolean }>, categories: readonly ResourceCategory[]): CategoryHealth[] {
  const groups = new Map<ResourceCategory, CategoryHealth>();
  for (const resource of resources) {
    const category = categories.includes(resource.category ?? 'unclassified')
      ? resource.category ?? 'unclassified' : 'unclassified';
    let group = groups.get(category);
    if (!group) {
      group = { category, counts: { ok: 0, down: 0, degraded: 0, unknown: 0, maintenance: 0 }, total: 0, problems: 0, updates: 0,
        tracking: { problems: 0, updates: 0, other: 0 }, attention: { crit: 0, warn: 0, info: 0 } };
      groups.set(category, group);
    }
    group.counts[resource.state]++;
    group.total++;
    if (resource.problem) group.problems++;
    if (resource.update) group.updates++;
    if (resource.problem) group.attention[resource.problemSeverity === 'critical' || resource.problemSeverity === 'major' ? 'crit'
      : resource.problemSeverity === 'information' ? 'info' : 'warn']++;
    // Une seule part par ressource : un problème actif prime sur une mise à jour.
    if (resource.problem) group.tracking.problems++;
    else if (resource.update) group.tracking.updates++;
    else group.tracking.other++;
  }
  const trackedOnly = (category: ResourceCategory) => category === 'scheduled_task' || category === 'software';
  const rank = (group: CategoryHealth) => {
    if (!trackedOnly(group.category) && group.counts.down) return 5;
    if (group.problems) return 4;
    if (!trackedOnly(group.category) && group.counts.degraded) return 3;
    if (!trackedOnly(group.category) && group.counts.unknown) return 2;
    if (!trackedOnly(group.category) && group.counts.maintenance) return 1;
    return 0;
  };
  const impact = (group: CategoryHealth) => {
    switch (rank(group)) {
      case 5: return group.counts.down;
      case 4: return group.problems;
      case 3: return group.counts.degraded;
      case 2: return group.counts.unknown;
      case 1: return group.counts.maintenance;
      default: return 0;
    }
  };
  return [...groups.values()].sort((left, right) =>
    rank(right) - rank(left)
      || impact(right) - impact(left)
      || categories.indexOf(left.category) - categories.indexOf(right.category)
  );
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

/** Ressources affectées maintenant, classées par gravité puis par prise en charge. */
export function dashboardIncidentLeaders(incidents: Incident[], targets: Target[]) {
  const ranks: Record<IncidentSeverity, number> = { information: 1, warning: 2, major: 3, critical: 4 };
  const counts = new Map<string, { count: number; unacknowledged: number; severity: IncidentSeverity }>();
  for (const incident of incidents) {
    // Les anciennes notifications Argus de mise à jour ne sont pas des problèmes actifs.
    if (incident.status !== 'active' || incident.nature_key === 'software-update-available') continue;
    const impacts = new Map<string, IncidentSeverity>();
    for (const impact of incident.impacts) {
      if (impact.status !== 'active') continue;
      const previous = impacts.get(impact.target_id);
      if (!previous || ranks[impact.effective_severity] > ranks[previous]) impacts.set(impact.target_id, impact.effective_severity);
    }
    for (const [targetID, severity] of impacts) {
      const current = counts.get(targetID);
      counts.set(targetID, {
        count: (current?.count ?? 0) + 1,
        unacknowledged: (current?.unacknowledged ?? 0) + (incident.acknowledged_at ? 0 : 1),
        severity: current && ranks[current.severity] > ranks[severity] ? current.severity : severity
      });
    }
  }
  return targets
    .filter((target) => counts.has(target.id))
    .map((target) => ({ target, ...counts.get(target.id)! }))
    .sort((left, right) => ranks[right.severity] - ranks[left.severity]
      || right.unacknowledged - left.unacknowledged
      || right.count - left.count
      || left.target.name.localeCompare(right.target.name))
    .slice(0, 7);
}

export function dashboardRecentActivity(incidents: Incident[]) {
  const ordered = incidents
    .flatMap((incident) => incident.activity.map((entry) => ({ incident, entry })))
    .filter(({ entry }) => Number.isFinite(Date.parse(entry.occurred_at)))
    .sort((left, right) => Date.parse(right.entry.occurred_at) - Date.parse(left.entry.occurred_at));
  const seen = new Set<string>();
  return ordered.filter(({ incident }) => {
    if (seen.has(incident.id)) return false;
    seen.add(incident.id);
    return true;
  }).slice(0, 5);
}
