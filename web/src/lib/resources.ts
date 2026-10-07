import type { Incident, IncidentEvidence, IncidentImpact, Maintenance, ResourceCategory, ResourceHealthState, Target, TargetMeasures } from './api.ts';
import type { TargetState } from './format.ts';

export const resourceCategories: ResourceCategory[] = ['virtual_machine', 'container', 'virtualization_host', 'storage', 'network', 'host', 'service', 'application', 'scheduled_task', 'software', 'infrastructure', 'unclassified'];
export function resourceCategoryFromParam(value: string | null): ResourceCategory | 'all' {
  return resourceCategories.find((category) => category === value) ?? 'all';
}
/* La navigation range les catégories en familles. Les Logiciels suivis
 * relèvent du suivi des versions et les Ressources à classer d'un classement à
 * compléter : chacun garde un accès dédié, hors des familles. */
export type ResourceFamily = 'infrastructure' | 'services' | 'tasks';
export const resourceFamilies: Record<ResourceFamily, ResourceCategory[]> = {
  infrastructure: ['virtualization_host', 'host', 'virtual_machine', 'container', 'storage', 'network', 'infrastructure'],
  services: ['service', 'application'],
  tasks: ['scheduled_task']
};
export type ResourceView = 'all' | ResourceFamily | 'software' | 'unclassified';
export function resourceFamily(category: ResourceCategory): ResourceFamily | undefined {
  return (Object.keys(resourceFamilies) as ResourceFamily[]).find((family) => resourceFamilies[family].includes(category));
}
export function resourceViewFromParam(value: string | null): ResourceView {
  return value === 'software' || value === 'unclassified' || (value !== null && Object.hasOwn(resourceFamilies, value)) ? value as ResourceView : 'all';
}
/** Une vue « Toutes » couvre ce qui se supervise ; le suivi des versions a sa vue. */
export function inResourceView(category: ResourceCategory, view: ResourceView): boolean {
  if (view === 'all') return category !== 'software';
  if (view === 'software' || view === 'unclassified') return category === view;
  return resourceFamily(category) === view;
}

export type ResourceGrouping = 'host' | 'category';
export type ResourceGroup<T> = {
  key: string;
  /** Nom de la Ressource hôte, ou catégorie du groupe. */
  host?: { id: string; name: string };
  category?: ResourceCategory;
  /** La Ressource hôte elle-même, lorsqu'elle figure parmi les lignes. */
  head?: T;
  rows: T[];
};
/* Regroupe des lignes déjà triées sans changer leur ordre interne. Une
 * Ressource qui en porte d'autres devient l'en-tête de son groupe ; celles que
 * rien ne situe se rassemblent en dernier, sous une clé vide. */
export function groupResources<T extends { target: Pick<Target, 'id' | 'name' | 'category' | 'host'> }>(rows: T[], grouping: ResourceGrouping, rank: (row: T) => number): ResourceGroup<T>[] {
  const groups = new Map<string, ResourceGroup<T>>();
  const group = (key: string, init: () => Omit<ResourceGroup<T>, 'key' | 'rows'>) => {
    let found = groups.get(key);
    if (!found) groups.set(key, found = { key, rows: [], ...init() });
    return found;
  };
  if (grouping === 'category') {
    for (const row of rows) {
      const category = row.target.category ?? 'unclassified';
      group(category, () => ({ category })).rows.push(row);
    }
    const order = resourceCategories;
    return [...groups.values()].sort((a, b) => order.indexOf(a.category!) - order.indexOf(b.category!));
  }
  const carriers = new Set(rows.flatMap((row) => row.target.host ? [row.target.host.id] : []));
  for (const row of rows) {
    if (carriers.has(row.target.id)) group(row.target.id, () => ({ host: { id: row.target.id, name: row.target.name } })).head = row;
    else if (row.target.host) group(row.target.host.id, () => ({ host: row.target.host })).rows.push(row);
    else group('', () => ({})).rows.push(row);
  }
  const worst = (item: ResourceGroup<T>) => Math.max(0, ...[item.head, ...item.rows].filter((row): row is T => row !== undefined).map(rank));
  return [...groups.values()].sort((a, b) =>
    Number(a.key === '') - Number(b.key === '') || worst(b) - worst(a) || (a.host?.name ?? '').localeCompare(b.host?.name ?? ''));
}

const weights = { critical: 4, major: 3, warning: 2, information: 1 };
// Nature des anciennes preuves Argus ouvertes pour toute mise à jour. Elles ne
// sont plus produites, mais une preuve restée active ne doit pas passer pour un
// problème. Une mise à jour de sécurité, elle, reste un problème à traiter.
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
/* La conclusion vient du serveur ; il ne reste ici qu'une traduction vers le
 * vocabulaire d'affichage des écrans. La règle elle-même — fraîcheur des
 * preuves, cadence des Contrôles, ce qui altère le fonctionnement — vit dans
 * internal/health et nulle part ailleurs.
 *
 * Un État absent se lit « inconnu ». Jamais « disponible » : une projection
 * muette ne prouve pas qu'une Ressource va bien. */
const displayStates: Record<ResourceHealthState, TargetState> = {
  unavailable: 'down',
  degraded: 'degraded',
  maintenance: 'maintenance',
  available: 'ok',
  unknown: 'unknown'
};

export function resourceState(target: Pick<Target, 'id' | 'health_state'>, incidents: Incident[], now = Date.now(), maintenances?: Maintenance[], maintenancesComplete = maintenances !== undefined): TargetState {
  // La maintenance se compose ici, pas au serveur. Une fenêtre porte ses
  // dates : le client sait donc que l'horloge vient d'en franchir le terme,
  // sans attendre un nouvel instantané. C'est ce qui permet à un Incident
  // encore actif de « redevenir immédiatement visible », comme l'exige
  // CONTEXT.md. La conclusion sur les preuves, elle, vient du serveur.
  if (resourceUnderMaintenance(target.id, incidents, now, maintenances, maintenancesComplete)) return 'maintenance';
  return displayStates[target.health_state ?? 'unknown'] ?? 'unknown';
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
