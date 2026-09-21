import type { ArgusPreview, ConnectorImportResult, IndicatorBinding, IndicatorConfiguration, PatchMonPreview, ProxmoxPreview, TargetMatch, TargetReference, UptimeKumaPreview, ZabbixPreview } from './api';
// @ts-expect-error Node executes these shared helpers directly in the regression suite.
import { indicatorSelectionKey } from './indicator-bulk.ts';

export type SourcePreview = ZabbixPreview | UptimeKumaPreview | PatchMonPreview | ArgusPreview | ProxmoxPreview;
export type Equipment = {
  external_id: string;
  name: string;
  candidate_targets?: TargetMatch[];
  suggested_target?: TargetReference;
  already_imported_to?: TargetReference;
  importable: boolean;
  reason?: string;
  expectedRunning: boolean;
};
export type EquipmentDraft = {
  equipment: Equipment;
  supervise: boolean;
  targetId: string;
  indicators: { source: IndicatorBinding; enabled: boolean; targetId: string; selected: Set<string> } | null;
};

export function equipmentFrom(preview: SourcePreview): Equipment[] {
  if (preview.kind === 'argus') return preview.services.map(item => ({ ...item, expectedRunning: false, reason: item.importable ? undefined : item.ineligibility === 'inactive' ? 'Service désactivé dans Argus.' : 'Version installée non configurée dans Argus.' }));
  if (preview.kind === 'proxmox') return preview.resources.map(item => ({ ...item, expectedRunning: item.expected_running, reason: item.importable ? undefined : 'Ressource non importable.' }));
  return (preview.kind === 'uptime_kuma' ? preview.monitors : preview.hosts).map(item => ({ ...item, importable: true, expectedRunning: false }));
}

export function createDrafts(preview: SourcePreview | null, configuration: IndicatorConfiguration | null): EquipmentDraft[] {
  const equipment = preview ? equipmentFrom(preview) : [];
  for (const binding of configuration?.bindings ?? []) {
    if (!equipment.some(item => item.external_id === binding.external_id)) {
      equipment.push({ external_id: binding.external_id, name: binding.external_name, importable: false, expectedRunning: false, reason: 'Absent de la découverte des sources.' });
    }
  }
  return equipment.map(item => {
    const source = configuration?.bindings.find(binding => binding.external_id === item.external_id);
    const targetId = item.already_imported_to?.id ?? source?.target_id ?? item.suggested_target?.id ?? (item.candidate_targets?.length ? '__review_required__' : '');
    return {
      equipment: item, supervise: false, targetId,
      indicators: source ? {
        source, targetId: source.target_id ?? item.already_imported_to?.id ?? '', enabled: source.enabled,
        // Existing empty selections are intentional. Recommendations are opt-in.
        selected: new Set(source.indicators.filter(indicator => indicator.enabled).map(indicatorSelectionKey))
      } : null
    };
  });
}

export function importRequest(preview: SourcePreview, drafts: EquipmentDraft[]) {
  const selected = drafts.filter(item => item.supervise && !item.equipment.already_imported_to);
  if (selected.some(item => !item.equipment.importable || item.targetId === '__review_required__')) throw new Error('Choisissez une Cible pour chaque équipement à superviser.');
  const field = preview.kind === 'uptime_kuma' ? 'monitor_ids' : preview.kind === 'argus' ? 'service_ids' : preview.kind === 'proxmox' ? 'resource_ids' : 'host_ids';
  return { receipt: preview.receipt, [field]: selected.map(item => item.equipment.external_id), target_assignments: Object.fromEntries(selected.filter(item => item.targetId).map(item => [item.equipment.external_id, item.targetId])), ...(preview.kind === 'proxmox' ? { expected_running_ids: selected.filter(item => item.equipment.expectedRunning).map(item => item.equipment.external_id) } : {}) };
}

export function applyImported(drafts: EquipmentDraft[], result: ConnectorImportResult) {
  for (const target of result.targets) {
    const draft = drafts.find(item => item.equipment.external_id === target.external_id);
    if (!draft) continue;
    draft.equipment.already_imported_to = { id: target.target_id, name: target.target_name };
    draft.targetId = target.target_id;
    draft.supervise = false;
    if (draft.indicators) draft.indicators.targetId = target.target_id;
  }
}

export function indicatorPayload(drafts: EquipmentDraft[]) {
  return drafts.flatMap(draft => {
    const binding = draft.indicators;
    if (!binding || (!binding.source.imported && !binding.enabled)) return [];
    const targetId = binding.source.target_id || draft.equipment.already_imported_to?.id || binding.targetId;
    if (binding.enabled && (!targetId || targetId === '__review_required__')) throw new Error(`Choisissez une Cible pour les indicateurs de ${draft.equipment.name}.`);
    if (binding.enabled && [...binding.selected].some(key => !binding.source.candidates.some(candidate => candidate.available && indicatorSelectionKey(candidate) === key))) throw new Error(`Retirez les indicateurs indisponibles de ${draft.equipment.name}.`);
    return [{ id: binding.source.id, external_id: binding.source.external_id, external_name: binding.source.external_name, target_id: targetId, enabled: binding.enabled, indicators: binding.source.candidates.filter(candidate => binding.selected.has(indicatorSelectionKey(candidate))).map(({ semantic_key, label, external_id, dimension, unit, metadata }) => ({ semantic_key, label, external_id, dimension: dimension ?? '', unit, metadata })) }];
  });
}

export function draftFingerprint(drafts: EquipmentDraft[], profiles: unknown) {
  return JSON.stringify({ drafts: drafts.map(draft => ({ id: draft.equipment.external_id, supervise: draft.supervise, target: draft.targetId, expected: draft.equipment.expectedRunning, indicators: draft.indicators && { enabled: draft.indicators.enabled, target: draft.indicators.targetId, selected: [...draft.indicators.selected].sort() } })), profiles });
}
