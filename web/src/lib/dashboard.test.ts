// @ts-nocheck -- Node exécute directement les modules TypeScript.
import assert from 'node:assert/strict';
import test from 'node:test';
import { dashboardCategoryHealth, dashboardCoverage, dashboardHealth, dashboardIncidentLeaders, dashboardRecentActivity } from './dashboard.ts';

test('never presents an empty or partially unknown fleet as operational', () => {
  assert.equal(dashboardHealth([]).state, 'empty');
  assert.equal(dashboardHealth(['ok', 'unknown']).state, 'unknown');
  assert.equal(dashboardHealth(['ok', 'maintenance']).state, 'maintenance');
  assert.equal(dashboardHealth(['ok', 'ok']).state, 'ok');
});

test('preserves outage and degradation precedence over missing data and maintenance', () => {
  assert.deepEqual(dashboardHealth(['ok', 'down', 'unknown', 'maintenance', 'degraded']), {
    state: 'down', counts: { ok: 1, down: 1, degraded: 1, unknown: 1, maintenance: 1 }, total: 5, watched: 3
  });
  assert.equal(dashboardHealth(['maintenance', 'unknown', 'degraded']).state, 'degraded');
});

test('ranks categories by operational urgency and counts every resource once', () => {
  const categories = ['virtual_machine', 'container', 'service', 'infrastructure', 'scheduled_task', 'software', 'unclassified'];
  const groups = dashboardCategoryHealth([
    { category: 'service', state: 'ok' },
    { category: 'service', state: 'unknown' },
    { category: 'infrastructure', state: 'down' },
    { category: 'infrastructure', state: 'ok' },
    { category: 'virtual_machine', state: 'ok' },
    { category: 'scheduled_task', state: 'degraded', problem: true, problemSeverity: 'major' },
    { category: 'software', state: 'maintenance' },
    { state: 'ok' }
  ], categories);
  assert.deepEqual(groups.map(({ category, total }) => [category, total]), [
    ['infrastructure', 2], ['scheduled_task', 1], ['service', 2], ['virtual_machine', 1], ['software', 1], ['unclassified', 1]
  ]);
  assert.deepEqual(groups[0].counts, { ok: 1, down: 1, degraded: 0, unknown: 0, maintenance: 0 });
  assert.equal(groups[1].problems, 1);
  assert.deepEqual(groups[1].tracking, { problems: 1, updates: 0, other: 0 });
  assert.deepEqual(groups[1].attention, { crit: 1, warn: 0, info: 0 });
  assert.equal(groups.reduce((total, group) => total + group.total, 0), 8);
  assert.deepEqual(dashboardCategoryHealth([], categories), []);
});

test('software and scheduled tasks are ranked by observed problems, not availability', () => {
  const categories = ['service', 'infrastructure', 'scheduled_task', 'software', 'unclassified'];
  assert.deepEqual(dashboardCategoryHealth([
    { category: 'software', state: 'unknown', update: true, problem: true },
    { category: 'software', state: 'unknown', update: true },
    { category: 'scheduled_task', state: 'unknown' },
    { category: 'service', state: 'ok' }
  ], categories).map((group) => [group.category, group.problems, group.updates]), [
    ['software', 1, 2], ['service', 0, 0], ['scheduled_task', 0, 0]
  ]);
  assert.deepEqual(dashboardCategoryHealth([
    { category: 'software', state: 'unknown', update: true, problem: true },
    { category: 'software', state: 'unknown', update: true },
    { category: 'software', state: 'unknown' }
  ], categories)[0].tracking, { problems: 1, updates: 1, other: 1 });
});

test('weights coverage by expected observations and keeps missing measurements neutral', () => {
  assert.equal(dashboardCoverage([]), null);
  assert.equal(dashboardCoverage([{ coverage: 1, expected_observations: 0 }]), null);
  assert.equal(dashboardCoverage([{ coverage: null, expected_observations: 20 }]), null);
  assert.equal(dashboardCoverage([{ coverage: 1, expected_observations: 90 }, { coverage: 0, expected_observations: 10 }]), 0.9);
});

test('ranks only active problems by impact severity, then unacknowledged incidents', () => {
  const targets = [{ id: 'a', name: 'Alpha' }, { id: 'b', name: 'Beta' }, { id: 'c', name: 'Charlie' }];
  const incidents = [
    { id: '1', status: 'active', nature_key: 'availability', impacts: [{ target_id: 'a', status: 'active', effective_severity: 'warning' }, { target_id: 'a', status: 'active', effective_severity: 'major' }, { target_id: 'b', status: 'resolved', effective_severity: 'critical' }] },
    { id: '2', status: 'active', nature_key: 'availability', acknowledged_at: '2026-09-22T10:00:00Z', impacts: [{ target_id: 'a', status: 'active', effective_severity: 'major' }] },
    { id: '3', status: 'active', nature_key: 'availability', impacts: [{ target_id: 'c', status: 'active', effective_severity: 'major' }] },
    { id: '4', status: 'resolved', nature_key: 'availability', impacts: [{ target_id: 'b', status: 'active', effective_severity: 'critical' }] },
    { id: '5', status: 'active', nature_key: 'software-update-available', impacts: [{ target_id: 'b', status: 'active', effective_severity: 'critical' }] }
  ];
  assert.deepEqual(dashboardIncidentLeaders(incidents, targets).map(({ target, count, unacknowledged, severity }) => [target.id, count, unacknowledged, severity]), [
    ['a', 2, 1, 'major'], ['c', 1, 1, 'major']
  ]);
});

test('recent activity uses event time across incidents', () => {
  const incidents = [
    { id: 'first', activity: [{ id: 1, occurred_at: '2026-09-22T10:00:00Z' }, { id: 3, occurred_at: '2026-09-22T12:00:00Z' }] },
    { id: 'second', activity: [{ id: 2, occurred_at: '2026-09-22T11:00:00Z' }] }
  ];
  assert.deepEqual(dashboardRecentActivity(incidents).map(({ entry }) => entry.id), [3, 2]);
});
