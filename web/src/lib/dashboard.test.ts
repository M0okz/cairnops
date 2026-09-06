// @ts-nocheck -- Node exécute directement les modules TypeScript.
import assert from 'node:assert/strict';
import test from 'node:test';
import { dashboardCoverage, dashboardHealth } from './dashboard.ts';

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

test('weights coverage by expected observations and keeps missing measurements neutral', () => {
  assert.equal(dashboardCoverage([]), null);
  assert.equal(dashboardCoverage([{ coverage: 1, expected_observations: 0 }]), null);
  assert.equal(dashboardCoverage([{ coverage: null, expected_observations: 20 }]), null);
  assert.equal(dashboardCoverage([{ coverage: 1, expected_observations: 90 }, { coverage: 0, expected_observations: 10 }]), 0.9);
});
