import type {
  AlertFact,
  AlertFactKind,
  ContextIndicator,
  Incident,
  IncidentEvidence,
  IncidentIndicators,
  IndicatorPoint,
  IndicatorSemanticKey
} from './api';

export type IncidentIndicatorRow = {
  key: string;
  label: string;
  unit: IncidentIndicators['snapshots'][number]['unit'];
  snapshot?: IncidentIndicators['snapshots'][number];
  indicator?: ContextIndicator;
  points: IndicatorPoint[];
};

/** L'adresse est le contrat commun à la liste, la Palette et les notifications. */
export function incidentHref(incidentID: string): string {
  return `/incidents?incident=${encodeURIComponent(incidentID)}`;
}

/** Le Journal d'un détail opérationnel commence par ce qui vient de changer. */
export function incidentActivity(incident: Incident): Incident['activity'] {
  return visibleIncidentActivity(incident).sort(
    (left, right) =>
      new Date(right.occurred_at).getTime() - new Date(left.occurred_at).getTime()
  );
}

/** La fermeture du regroupement n'intéresse l'opérateur que si une autre Atteinte l'a rejoint. */
export function visibleIncidentActivity(incident: Incident): Incident['activity'] {
  return incident.activity.filter(
    (entry) => entry.kind !== 'propagation_closed' || incident.impact_count > 1
  );
}

/**
 * Les photographies prises à l'ouverture gagnent toujours sur les Indicateurs
 * encore configurés aujourd'hui. Une photographie survit donc à la suppression
 * de son Indicateur, tandis qu'une courbe non photographiée reste secondaire.
 */
export function incidentIndicatorRows(detail: IncidentIndicators): {
  captured: IncidentIndicatorRow[];
  additional: IncidentIndicatorRow[];
} {
  const capturedIndicatorIDs = new Set<string>();
  const capturedSemanticLabels = new Set<string>();

  const captured = detail.snapshots.map((snapshot) => {
    const indicator = detail.indicators.find(
      (candidate) =>
        candidate.target_id === snapshot.target_id && (
          (snapshot.indicator_id && candidate.id === snapshot.indicator_id) ||
          (candidate.semantic_key === snapshot.semantic_key && candidate.label === snapshot.label)
        )
    );
    if (indicator) capturedIndicatorIDs.add(indicator.id);
    capturedSemanticLabels.add(`${snapshot.target_id}:${snapshot.semantic_key}:${snapshot.label}`);
    return {
      key: snapshot.indicator_id ?? `snapshot:${snapshot.target_id}:${snapshot.semantic_key}:${snapshot.label}`,
      label: detail.target_ids.length > 1 ? `${snapshot.target_name} · ${snapshot.label}` : snapshot.label,
      unit: snapshot.unit,
      snapshot,
      indicator,
      points: indicator ? (detail.series[indicator.id] ?? []) : []
    };
  });

  const additional = detail.indicators
    .filter(
      (indicator) =>
        !capturedIndicatorIDs.has(indicator.id) &&
        !capturedSemanticLabels.has(`${indicator.target_id}:${indicator.semantic_key}:${indicator.label}`)
    )
    .map((indicator) => ({
      key: indicator.id,
      label: indicator.label,
      unit: indicator.unit,
      indicator,
      points: detail.series[indicator.id] ?? []
    }));

  return { captured, additional };
}

/**
 * La Preuve qui porte le Constat : une Preuve encore active et non écartée
 * d'abord, puis la plus ancienne non écartée, enfin la première connue.
 */
export function primaryEvidence(incident: Incident): IncidentEvidence | null {
  const evidence = incident.impacts.flatMap((impact) => impact.evidence);
  return (
    evidence.find((item) => item.active && !item.invalidated_at) ??
    evidence.find((item) => !item.invalidated_at) ??
    evidence[0] ??
    null
  );
}

/** Forme du Constat : ce que le fait reconnu permet d'afficher en premier. */
export type FindingShape =
  | { kind: 'versions'; fact: AlertFact; current: string; available: string }
  | { kind: 'count'; fact: AlertFact; count: number }
  | { kind: 'resource'; fact: AlertFact; resource: string; noun: 'disk' | 'volume' | 'certificate' }
  | { kind: 'plain'; fact: AlertFact }
  | { kind: 'original' };

const versionedKinds = new Set<AlertFactKind>([
  'software.update_available',
  'software.security_update_available',
  'software.major_update_available'
]);

export function findingShape(fact: AlertFact | undefined): FindingShape {
  if (!fact) return { kind: 'original' };
  if (versionedKinds.has(fact.kind) && fact.current_version && fact.available_version) {
    return { kind: 'versions', fact, current: fact.current_version, available: fact.available_version };
  }
  if (fact.kind === 'software.security_updates' && typeof fact.count === 'number') {
    return { kind: 'count', fact, count: fact.count };
  }
  if (fact.resource) {
    const noun = fact.kind === 'disk.latency.high'
      ? 'disk'
      : fact.kind.startsWith('certificate.')
        ? 'certificate'
        : 'volume';
    return { kind: 'resource', fact, resource: fact.resource, noun };
  }
  return { kind: 'plain', fact };
}

/**
 * Indicateurs qui mesurent la condition elle-même. Ils montrent le problème,
 * pas sa cause : les autres Indicateurs restent disponibles à part.
 */
const measuredBy: Partial<Record<AlertFactKind, IndicatorSemanticKey[]>> = {
  'availability.unavailable': ['response.time'],
  'disk.space.low': ['filesystem.utilization'],
  'cpu.usage.high': ['cpu.utilization'],
  'system.load.high': ['cpu.utilization'],
  'memory.usage.high': ['memory.utilization'],
  'memory.available.low': ['memory.utilization'],
  'certificate.expiring': ['certificate.days_remaining'],
  'certificate.invalid': ['certificate.valid'],
  'software.security_updates': ['security_updates.count'],
  'system.reboot_required': ['reboot.required']
};

const measuredConditions = new Set<AlertFactKind>([
  'availability.unavailable',
  'disk.latency.high',
  'disk.space.low',
  'disk.inodes.low',
  'cpu.usage.high',
  'system.load.high',
  'memory.usage.high',
  'memory.available.low',
  'swap.space.low'
]);

/** Conditions qu'une courbe devrait montrer, même lorsqu'aucune n'est collectée. */
export function measurable(fact: AlertFact | undefined): boolean {
  return Boolean(fact && measuredConditions.has(fact.kind));
}

function rowSemanticKey(row: IncidentIndicatorRow): string | undefined {
  return row.snapshot?.semantic_key ?? row.indicator?.semantic_key;
}

/**
 * Sépare les courbes du Constat des autres. Pour un volume nommé, seule sa
 * courbe est retenue lorsqu'elle existe, sinon toutes celles de même sens.
 */
export function splitIndicatorRows(
  rows: { captured: IncidentIndicatorRow[]; additional: IncidentIndicatorRow[] },
  fact: AlertFact | undefined
): { relevant: IncidentIndicatorRow[]; others: IncidentIndicatorRow[] } {
  const all = [...rows.captured, ...rows.additional];
  const keys = fact ? measuredBy[fact.kind] ?? [] : [];
  let relevant = all.filter((row) => keys.includes(rowSemanticKey(row) as IndicatorSemanticKey));
  if (fact?.resource) {
    const exact = relevant.filter((row) => row.indicator?.dimension === fact.resource);
    if (exact.length > 0) relevant = exact;
  }
  const chosen = new Set(relevant.map((row) => row.key));
  return { relevant, others: all.filter((row) => !chosen.has(row.key)) };
}
