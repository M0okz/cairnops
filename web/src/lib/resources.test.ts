import assert from 'node:assert/strict';
import test from 'node:test';
import type { Target, Incident, IncidentEvidence, Maintenance } from './api.ts';
// @ts-ignore -- Node executes tests directly with its TypeScript loader.
import { groupResources, inResourceView, resourceViewFromParam, resourceFamily, resourceCategoryFromParam, resourceState, resourceProblems, resourceDivergence, resourceUnderMaintenance, problemText } from './resources.ts';

test('accepts only known category links', () => {
 assert.equal(resourceCategoryFromParam('infrastructure'), 'infrastructure');
 assert.equal(resourceCategoryFromParam('virtual_machine'), 'virtual_machine');
 assert.equal(resourceCategoryFromParam('proxmox'), 'all');
 assert.equal(resourceCategoryFromParam(null), 'all');
});
const now = Date.parse('2026-09-22T12:00:00Z');
const date = new Date(now).toISOString();
const target: Target = { id: 'r', name: 'Home Assistant', description: '', created_at: date, external_source_count: 2, aliases: [], sources: [], health_state: 'available' };
// La conclusion sur les preuves vient du serveur : ces fabriques posent l'État
// qu'il a servi. La règle elle-même est éprouvée dans internal/health.
const served = (state: Target['health_state']): Target => ({ ...target, health_state: state });
const evidence = (active = true): IncidentEvidence => ({id: 'e', impact_id: 'p', target_id: 'r', origin: 'zabbix', last_seen_at: date, name: 'Version installée non relevée', active, severity: 'critical', opened_at: date, upstream_acknowledged: false, acknowledgement_sync_status: 'not_applicable', ...(!active ? {resolved_at: date} : {})});
const incident = (nature: string, proofs = [evidence()]): Incident => ({id: nature, nature_key: nature, nature_label: nature, nature_scope: 'canonical', nature_namespace: '', nature_fingerprint: nature, propagation_eligible: false, status: 'active', propagation_status: 'closed', severity: 'critical', opened_at: date, last_impact_at: date, propagation_window_seconds: 0, propagation_ends_at: date, acknowledgement_sync_status: 'not_applicable', extended: false, active_impact_count: 1, impact_count: 1, affected_target_count: 1, max_affected_targets: 1, revision: 1, impacts: [{id: nature, target_id: 'r', target_name: 'Home Assistant', status: 'active', source_severity: 'critical', effective_severity: 'critical', opened_at: date, maintenance_active: false, evidence: proofs, created_at: date, updated_at: date}], activity: [], created_at: date, updated_at: date});

test('the state served by the server is displayed, never recomputed', () => {
 assert.equal(resourceState(served('available'), [], now), 'ok');
 assert.equal(resourceState(served('unavailable'), [], now), 'down');
 assert.equal(resourceState(served('degraded'), [], now), 'degraded');
 assert.equal(resourceState(served('unknown'), [], now), 'unknown');
});

test('a missing or unrecognized state reads as unknown, never as available', () => {
 assert.equal(resourceState({ id: 'r' }, [], now), 'unknown');
 // Un serveur plus récent pourrait servir un État que ce client ne connaît
 // pas. Il ne doit jamais passer pour « tout va bien ».
 assert.equal(resourceState({ id: 'r', health_state: 'inédit' as never }, [], now), 'unknown');
});

test('a critical version alert does not make an accessible service unavailable', () => {
 // Le serveur conclut « disponible » malgré l'alerte de version : le client
 // n'a rien à réinterpréter, et surtout rien à dégrader de son côté.
 assert.equal(resourceState(served('available'), [incident('software-update-available')], now), 'ok');
 assert.equal(resourceState(served('unavailable'), [incident('availability')], now), 'down');
});

test('incident maintenance fallback expires and never masks an actionable incident indefinitely', () => {
 const item = incident('availability');
 item.impacts[0].maintenance_active = true;
 item.impacts[0].maintenance_ends_at = new Date(now + 60_000).toISOString();
 assert.equal(resourceUnderMaintenance(target.id, [item], now), true);
 assert.equal(resourceState(served('unavailable'), [item], now), 'maintenance');
 assert.equal(resourceUnderMaintenance(target.id, [item], now + 120_000), false);
 // La fenêtre expirée, l'Incident redevient immédiatement visible : le client
 // n'attend pas un nouvel instantané du serveur pour cesser de le neutraliser.
 assert.equal(resourceState(served('unavailable'), [item], now + 120_000), 'down');
 // A successfully read empty list overrides an old, cancelled projection.
 assert.equal(resourceUnderMaintenance(target.id, [item], now, []), false);
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



test('a scheduled maintenance applies without an incident and expires by its dates', () => {
 const window: Maintenance = { id: 'm', name: 'Intervention', reason: 'Maintenance planifiée',
  state: 'upcoming', starts_at: date, ends_at: new Date(now + 1800_000).toISOString(),
  targets: [{ id: target.id, name: target.name }], created_at: date };
 assert.equal(resourceState(target, [], now, [window]), 'maintenance');
 assert.equal(resourceState(target, [], now - 1000, [window]), 'ok');
 assert.equal(resourceState(served('unknown'), [], now + 1800_001, [window]), 'unknown');
 assert.equal(resourceState(target, [], now, [{ ...window, cancelled_at: date }]), 'ok');
 assert.equal(resourceState(target, [], now, [{ ...window, targets: [{ id: 'other', name: 'Other' }] }]), 'ok');
 const staleMaintenance = incident('availability');
 staleMaintenance.impacts[0].maintenance_active = true;
 assert.equal(resourceState(served('unknown'), [staleMaintenance], now + 1800_001, [window]), 'unknown');
});


test('a partial or cached maintenance list keeps known windows and a bounded fallback for missing resources', () => {
 const window: Maintenance = { id: 'known', name: 'Intervention', reason: 'Maintenance planifiée',
  state: 'active', starts_at: date, ends_at: new Date(now + 1800_000).toISOString(),
  targets: [{ id: target.id, name: target.name }], created_at: date };
 const truncated = Array.from({ length: 200 }, (_, index) => ({ ...window, id: `window-${index}` }));
 // Reaching the transport limit must not discard a known active window,
 // including for a resource that has no incident to provide a fallback.
 assert.equal(resourceState(target, [], now, truncated, false), 'maintenance');
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

test('ranges categories into families and keeps versions apart', () => {
 assert.equal(resourceFamily('virtual_machine'), 'infrastructure');
 assert.equal(resourceFamily('application'), 'services');
 assert.equal(resourceFamily('software'), undefined);
 assert.equal(inResourceView('unclassified', 'all'), true);
 assert.equal(inResourceView('software', 'all'), false);
 assert.equal(inResourceView('software', 'software'), true);
 assert.equal(inResourceView('storage', 'services'), false);
 assert.equal(resourceViewFromParam('services'), 'services');
 assert.equal(resourceViewFromParam('toString'), 'all');
 assert.equal(resourceViewFromParam(null), 'all');
});

const resource = (id: string, rank: number, extra: Partial<Target> = {}) => ({ target: { ...target, id, name: id, ...extra }, rank });
const pve = { id: 'pve-01', name: 'pve-01' };

test('groups resources under the Ressource hôte that carries them', () => {
 const rows = [resource('nextcloud', 4, { host: pve, category: 'virtual_machine' }), resource('argus', 2, { category: 'service' }), resource('pve-01', 0, { category: 'virtualization_host' }), resource('jellyfin', 0, { host: { id: 'pve-02', name: 'pve-02' } })];
 const groups = groupResources(rows, 'host', (row) => row.rank);
 assert.deepEqual(groups.map((group) => group.key), ['pve-01', 'pve-02', '']);
 assert.equal(groups[0].head?.target.id, 'pve-01');
 assert.deepEqual(groups[0].rows.map((row) => row.target.id), ['nextcloud']);
 // L'hôte filtré hors de la liste garde son nom d'en-tête.
 assert.equal(groups[1].head, undefined);
 assert.equal(groups[1].host?.name, 'pve-02');
 assert.deepEqual(groups[2].rows.map((row) => row.target.id), ['argus']);
});

test('groups resources by category in navigation order', () => {
 const rows = [resource('argus', 2, { category: 'service' }), resource('nextcloud', 4, { category: 'virtual_machine' }), resource('mystere', 0)];
 assert.deepEqual(groupResources(rows, 'category', (row) => row.rank).map((group) => group.key), ['virtual_machine', 'service', 'unclassified']);
});
