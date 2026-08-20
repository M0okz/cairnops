import type { TargetMatch, TargetReference } from '$lib/api';

export const REVIEW_TARGET = '__review_required__';

type ReconciliationItem = {
  external_id: string;
  candidate_targets?: TargetMatch[];
  suggested_target?: TargetReference;
  already_imported_to?: TargetReference;
};

export type ReconciliationCounts = {
  reused: number;
  created: number;
  review: number;
};

export function prepareTargetAssignments(items: ReconciliationItem[]): Record<string, string> {
  return Object.fromEntries(
    items
      .filter((item) => !item.already_imported_to)
      .map((item) => [
        item.external_id,
        item.suggested_target?.id ?? (item.candidate_targets?.length ? REVIEW_TARGET : '')
      ])
  );
}

export function reconciliationCounts(
  selected: string[],
  assignments: Record<string, string>
): ReconciliationCounts {
  const counts: ReconciliationCounts = { reused: 0, created: 0, review: 0 };
  for (const externalID of selected) {
    const targetID = assignments[externalID] ?? '';
    if (targetID === REVIEW_TARGET) counts.review += 1;
    else if (targetID) counts.reused += 1;
    else counts.created += 1;
  }
  return counts;
}

export function resolvedTargetAssignments(
  selected: string[],
  assignments: Record<string, string>
): Record<string, string> {
  return Object.fromEntries(
    selected
      .map((externalID) => [externalID, assignments[externalID] ?? ''] as const)
      .filter(([, targetID]) => targetID !== '' && targetID !== REVIEW_TARGET)
  );
}
