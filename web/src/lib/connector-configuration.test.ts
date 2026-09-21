// @ts-nocheck -- Node regression suite.
import test from 'node:test';
import assert from 'node:assert/strict';
import { createDrafts, importRequest, applyImported, indicatorPayload, draftFingerprint, equipmentFrom, readonlyConfiguration } from './connector-configuration.ts';
const candidate = { external_id: 'cpu-1', semantic_key: 'cpu.utilization', dimension: '', label: 'CPU', unit: 'percent', available: true, recommended: true };
const preview = { kind: 'zabbix', receipt: 'receipt', hosts: [{ external_id: 'host-1', name: 'Serveur', candidate_targets: [] }], available_targets: [] };
const config = { bindings: [{ external_id: 'host-1', external_name: 'Serveur', enabled: false, imported: false, indicators: [], candidates: [candidate] }] };
test('opening does not silently import or select recommended indicators', () => {
 const [draft] = createDrafts(preview, config);
 assert.equal(draft.supervise, false); assert.equal(draft.indicators.selected.size, 0);
 assert.deepEqual(indicatorPayload([draft]), []);
});
test('sources and contextual indicators keep independent activation', () => {
 const drafts = createDrafts(preview, config); drafts[0].indicators.enabled = true; drafts[0].indicators.targetId = 'target-1';
 assert.deepEqual(importRequest(preview, drafts).host_ids, []);
 assert.equal(indicatorPayload(drafts)[0].enabled, true);
});
test('ambiguous reconciliation blocks import before writing', () => {
 const drafts = createDrafts({ ...preview, hosts: [{ ...preview.hosts[0], candidate_targets: [{ target: { id: 'possible' } }] }] }, config);
 drafts[0].supervise = true;
 assert.throws(() => importRequest(preview, drafts), /Choisissez une Cible/);
});
test('successful source import assigns new target IDs and cannot repeat on indicator retry', () => {
 const drafts = createDrafts(preview, config); drafts[0].supervise = true; drafts[0].indicators.enabled = true; drafts[0].indicators.selected.add('cpu-1\0cpu.utilization\0');
 assert.throws(() => indicatorPayload(drafts), /Choisissez une Cible/);
 applyImported(drafts, { targets: [{ external_id: 'host-1', target_id: 'created-target', target_name: 'Serveur' }] });
 assert.deepEqual(importRequest(preview, drafts).host_ids, []);
 assert.equal(indicatorPayload(drafts)[0].target_id, 'created-target');
 assert.equal(indicatorPayload(drafts)[0].indicators.length, 1);
});
test('unavailable selected indicators can be removed without silently replacing identities', () => {
 const drafts = createDrafts(preview, config); const b = drafts[0].indicators;
 b.enabled = true; b.targetId = 'target'; b.selected.add('vanished\0cpu.utilization\0');
 assert.throws(() => indicatorPayload(drafts), /indisponibles/);
 b.selected.clear(); assert.equal(indicatorPayload(drafts)[0].indicators.length, 0);
});
test('saved bindings remain in payload when disabled, undiscovered empty drafts do not', () => {
 const drafts = createDrafts(preview, {bindings:[{...config.bindings[0],imported:true,target_id:'existing'}]});
 assert.equal(indicatorPayload(drafts)[0].enabled, false);
});
test('dirty tracking notices multi-equipment changes and returns clean after undo', () => {
 const drafts = createDrafts(preview, config); const initial = draftFingerprint(drafts, []);
 drafts[0].indicators.enabled = true; assert.notEqual(draftFingerprint(drafts, []), initial);
 drafts[0].indicators.enabled = false; assert.equal(draftFingerprint(drafts, []), initial);
 assert.notEqual(draftFingerprint(drafts, [{name:'Profile'}]), initial);
});
test('Argus eligibility and Proxmox expected-running settings are preserved', () => {
 const argus = {kind:'argus',services:[{external_id:'app',name:'App',importable:false,ineligibility:'inactive'}]};
 assert.equal(equipmentFrom(argus)[0].importable,false);
 const p = {kind:'proxmox',receipt:'r',resources:[{external_id:'qemu/100',name:'VM',importable:true,expected_running:true}],available_targets:[]};
 const drafts = createDrafts(p,null); drafts[0].supervise=true;
 assert.deepEqual(importRequest(p,drafts).expected_running_ids,['qemu/100']);
});

test('failed discovery displays saved indicators read-only without erasing their selection', () => {
 const stored = { bindings: [{...config.bindings[0], imported:true, enabled:true, target_id:'t1', candidates:[], indicators:[{...candidate,enabled:true}]}] };
 const readonly = readonlyConfiguration(stored);
 const drafts = createDrafts(preview,readonly);
 assert.equal(readonly.bindings[0].candidates[0].label,'CPU');
 assert.equal(readonly.bindings[0].candidates[0].available,false);
 assert.equal(drafts[0].indicators.selected.has('cpu-1\0cpu.utilization\0'),true);
 assert.deepEqual(stored.bindings[0].candidates,[]);
});
