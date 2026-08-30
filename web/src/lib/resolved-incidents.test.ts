// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import { incidentMembershipChanged, shouldLoadResolvedIncidents } from './resolved-incidents.ts';

test('invalidates resolved incidents only when active incident membership changes', () => {
  assert.equal(incidentMembershipChanged([{ id: 'a' }, { id: 'b' }], [{ id: 'b' }, { id: 'a' }]), false);
  assert.equal(incidentMembershipChanged([{ id: 'a' }, { id: 'b' }], [{ id: 'b' }]), true);
  assert.equal(incidentMembershipChanged([{ id: 'a' }], [{ id: 'b' }]), true);
});

test('loads resolved incidents lazily and invalidates them after an incident change', () => {
  assert.equal(shouldLoadResolvedIncidents('active', -1, 0), false);
  assert.equal(shouldLoadResolvedIncidents('resolved', -1, 0), true);
  assert.equal(shouldLoadResolvedIncidents('resolved', 0, 0), false);
  assert.equal(shouldLoadResolvedIncidents('resolved', 0, 1), true);
});
