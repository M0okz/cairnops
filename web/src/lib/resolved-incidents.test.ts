// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import { incidentMembershipChanged, resolvedHistoryChanged } from './resolved-incidents.ts';

test('invalidates resolved incidents only when active incident membership changes', () => {
  assert.equal(incidentMembershipChanged([{ id: 'a' }, { id: 'b' }], [{ id: 'b' }, { id: 'a' }]), false);
  assert.equal(incidentMembershipChanged([{ id: 'a' }, { id: 'b' }], [{ id: 'b' }]), true);
  assert.equal(incidentMembershipChanged([{ id: 'a' }], [{ id: 'b' }]), true);
});

test('offers refresh only for an existing snapshot and keeps changes received during a request visible', () => {
  assert.equal(resolvedHistoryChanged(-1, 0), false);
  assert.equal(resolvedHistoryChanged(-1, 1), false);
  assert.equal(resolvedHistoryChanged(0, 0), false);
  assert.equal(resolvedHistoryChanged(0, 1), true);
  assert.equal(resolvedHistoryChanged(0, 2), true);
  assert.equal(resolvedHistoryChanged(2, 2), false);
});
