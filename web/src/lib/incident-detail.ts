import type { ContextIndicator, Incident, IncidentIndicators, IndicatorPoint } from './api';

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
  return [...incident.activity].sort(
    (left, right) =>
      new Date(right.occurred_at).getTime() - new Date(left.occurred_at).getTime()
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
