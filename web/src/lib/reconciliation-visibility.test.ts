// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import { showReconciliationReviewInTopbar } from './reconciliation-visibility.ts';

test('keeps a single reconciliation review entry point in the Targets workflow', () => {
  assert.equal(showReconciliationReviewInTopbar('/cibles'), false);
  assert.equal(showReconciliationReviewInTopbar('/cibles/rapprochements'), false);
});

test('keeps the reconciliation review visible from unrelated screens', () => {
  assert.equal(showReconciliationReviewInTopbar('/'), true);
  assert.equal(showReconciliationReviewInTopbar('/incidents'), true);
});
