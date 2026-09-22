import assert from 'node:assert/strict';
import test from 'node:test';
import type { Target, TargetMeasures, Incident, IncidentEvidence, Maintenance } from './api.ts';
// @ts-ignore -- Node executes tests directly with its TypeScript loader.
import { resourceState, resourceProblems, resourceDivergence, resourceUnderMaintenance, problemText } from './resources.ts';
const now = Date.parse('2026-09-22T12:00:00Z');
const date = new Date(now).toISOString();
const target: Target = { id: 'r', name: 'Home Assistant', description: '', created_at: date, external_source_count: 2, aliases: [], sources: [] };
const evidence = (active = true): IncidentEvidence => ({id: 'e', impact_id: 'p', target_id: 'r', origin: 'zabbix', last_seen_at: date, name: 'Version installée non relevée', active, severity: 'critical', opened_at: date, upstream_acknowledged: false, acknowledgement_sync_status: 'not_applicable', ...(!active ? {resolved_at: date} : {})});
const incident = (nature: string, proofs = [evidence()]): Incident => ({id: nature, nature_key: nature, nature_label: nature, nature_scope: 'canonical', nature_namespace: '', nature_fingerprint: nature, propagation_eligible: false, status: 'active', propagation_status: 'closed', severity: 'critical', opened_at: date, last_impact_at: date, propagation_window_seconds: 0, propagation_ends_at: date, acknowledgement_sync_status: 'not_applicable', extended: false, active_impact_count: 1, impact_count: 1, affected_target_count: 1, max_affected_targets: 1, revision: 1, impacts: [{id: nature, target_id: 'r', target_name: 'Home Assistant', status: 'active', source_severity: 'critical', effective_severity: 'critical', opened_at: date, maintenance_active: false, evidence: proofs, created_at: date, updated_at: date}], activity: [], created_at: date, updated_at: date});
const measures: TargetMeasures = {target_id: 'r', measures: [], trend: [], latency_trend: [], sources: [{source_id: 's', name: 'HTTP', kind: 'uptime_kuma', origin: 'integration', measures_availability: true, latest_outcome: 'healthy', latest_observed_at: date, measures: []}]};

test('a critical version alert does not make an accessible service unavailable', () => {
 assert.equal(resourceState(target, [incident('version-query')], measures, now), 'ok');
 assert.equal(resourceState(target, [incident('software-update-available')], measures, now), 'ok');
 assert.equal(resourceState(target, [incident('availability')], measures, now), 'down');
});

test('incident maintenance fallback expires and never masks an actionable incident indefinitely', () => {
 const item = incident('availability');
 item.impacts[0].maintenance_active = true;
 item.impacts[0].maintenance_ends_at = new Date(now + 60_000).toISOString();
 assert.equal(resourceUnderMaintenance(target.id, [item], now), true);
 assert.equal(resourceState(target, [item], measures, now), 'maintenance');
 assert.equal(resourceUnderMaintenance(target.id, [item], now + 120_000), false);
 assert.equal(resourceState(target, [item], measures, now + 120_000), 'down');
 // A successfully read empty list overrides an old, cancelled projection.
 assert.equal(resourceUnderMaintenance(target.id, [item], now, []), false);
});
test('missing, stale and non-availability measurements cannot prove availability', () => {
 assert.equal(resourceState(target, [], undefined, now), 'unknown');
 assert.equal(resourceState(target, [], measures, now + 3600_000), 'unknown');
 const posture = structuredClone(measures);posture.sources[0].measures_availability=false;
 assert.equal(resourceState(target, [], posture, now), 'unknown');
});
test('problems are scoped to active resource impacts and updates stay separate', () => {
 const update = incident('software-update-available');const resolved = incident('backup');resolved.impacts[0].status='resolved';
 const problems=resourceProblems('r',[update,resolved,incident('version-query')]);
 assert.equal(problems.length,1);assert.equal(problemText(problems[0],'fr'),'Version installée non relevée');
 assert.equal(resourceProblems('other',[incident('version-query')]).length,0);
});
test('contradictions require recent opposite proofs of the same condition', () => {
 assert.equal(resourceDivergence('r',[incident('version-query'),incident('availability',[evidence(false)])],now),false);
 assert.equal(resourceDivergence('r',[incident('availability',[evidence(),evidence(false)])],now),true);
 assert.equal(resourceDivergence('r',[incident('availability',[evidence(),evidence(false)])],now+3600_000),false);
 const ignored = evidence(false); ignored.invalidated_at=date;
 assert.equal(resourceDivergence('r',[incident('availability',[evidence(),ignored])],now),false);
});

test('a suspended availability check cannot establish an available resource', () => {
 const suspended=structuredClone(measures);suspended.sources[0].enabled=false;
 assert.equal(resourceState(target,[],suspended,now),'unknown');
});

test('a scheduled maintenance applies without an incident and expires by its dates', () => {
 const window: Maintenance = { id: 'm', name: 'Intervention', reason: 'Maintenance planifiée',
  state: 'upcoming', starts_at: date, ends_at: new Date(now + 1800_000).toISOString(),
  targets: [{ id: target.id, name: target.name }], created_at: date };
 assert.equal(resourceState(target, [], measures, now, [window]), 'maintenance');
 assert.equal(resourceState(target, [], measures, now - 1000, [window]), 'ok');
 assert.equal(resourceState(target, [], undefined, now + 1800_001, [window]), 'unknown');
 assert.equal(resourceState(target, [], measures, now, [{ ...window, cancelled_at: date }]), 'ok');
 assert.equal(resourceState(target, [], measures, now, [{ ...window, targets: [{ id: 'other', name: 'Other' }] }]), 'ok');
 const staleMaintenance = incident('availability');
 staleMaintenance.impacts[0].maintenance_active = true;
 assert.equal(resourceState(target, [staleMaintenance], undefined, now + 1800_001, [window]), 'unknown');
});


test('a partial or cached maintenance list keeps known windows and a bounded fallback for missing resources', () => {
 const window: Maintenance = { id: 'known', name: 'Intervention', reason: 'Maintenance planifiée',
  state: 'active', starts_at: date, ends_at: new Date(now + 1800_000).toISOString(),
  targets: [{ id: target.id, name: target.name }], created_at: date };
 const truncated = Array.from({ length: 200 }, (_, index) => ({ ...window, id: `window-${index}` }));
 // Reaching the transport limit must not discard a known active window,
 // including for a resource that has no incident to provide a fallback.
 assert.equal(resourceState(target, [], undefined, now, truncated, false), 'maintenance');
 assert.equal(resourceUnderMaintenance(target.id, [], now, truncated, false), true);
 assert.equal(resourceUnderMaintenance(target.id, [], now + 1800_001, truncated, false), false);
 const missing = incident('availability');
 missing.impacts[0].target_id = 'outside-page';
 missing.impacts[0].maintenance_active = true;
 missing.impacts[0].maintenance_ends_at = new Date(now + 60_000).toISOString();
 assert.equal(resourceUnderMaintenance('outside-page', [missing], now, truncated, false), true);
 assert.equal(resourceUnderMaintenance('outside-page', [missing], now + 60_001, truncated, false), false);
 // A successful complete empty list still establishes an early cancellation.
 assert.equal(resourceUnderMaintenance('outside-page', [missing], now, [], true), false);
});
