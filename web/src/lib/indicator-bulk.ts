import type { IndicatorCandidate } from './api';

export type BulkIndicatorBinding = {
  source: {
    external_id: string;
    candidates: IndicatorCandidate[];
  };
  enabled: boolean;
  targetId: string;
  selected: Set<string>;
};

const systemIndicatorKeys = new Set([
  'cpu.utilization',
  'memory.utilization',
  'filesystem.utilization'
]);

export const indicatorSelectionKey = (candidate: IndicatorCandidate) =>
  `${candidate.external_id}\u0000${candidate.semantic_key}\u0000${candidate.dimension ?? ''}`;

export function setBindingsEnabled(
  bindings: BulkIndicatorBinding[],
  externalIds: ReadonlySet<string>,
  enabled: boolean
) {
  let changed = 0;
  let skipped = 0;

  for (const binding of bindings) {
    if (!externalIds.has(binding.source.external_id)) continue;
    if (!binding.targetId.trim()) {
      if (enabled) skipped += 1;
      continue;
    }
    if (binding.enabled === enabled) continue;
    binding.enabled = enabled;
    changed += 1;
  }

  return { changed, skipped };
}

export function addSystemIndicators(bindings: BulkIndicatorBinding[]) {
  let added = 0;
  let affectedBindings = 0;

  for (const binding of bindings) {
    if (!binding.enabled) continue;
    let affected = false;
    for (const candidate of binding.source.candidates) {
      if (!candidate.available || !systemIndicatorKeys.has(candidate.semantic_key)) continue;
      const key = indicatorSelectionKey(candidate);
      if (binding.selected.has(key)) continue;
      binding.selected.add(key);
      added += 1;
      affected = true;
    }
    if (affected) affectedBindings += 1;
  }

  return { added, affectedBindings };
}
