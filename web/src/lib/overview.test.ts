// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import type { InstanceHour } from './api.ts';
import { coverageWindow, healthyEvidenceWindow, pinnedIndicatorIDs } from './overview.ts';

const hour = (
  at: string,
  expected: number,
  conclusive: number
): InstanceHour => ({
  hour: at,
  expected_observations: expected,
  conclusive_observations: conclusive,
  healthy_observations: conclusive,
  average_latency_milliseconds: null
});

test('keeps missing and unmeasured hours neutral in the coverage window', () => {
  const values = coverageWindow(
    [
      hour('2026-08-20T12:00:00Z', 10, 5),
      hour('2026-08-20T13:00:00Z', 0, 0),
      hour('2026-08-20T14:00:00Z', 10, 12)
    ],
    '2026-08-20T14:37:00Z',
    4
  );

  assert.deepEqual(values, [null, 0.5, null, 1]);
});

test('does not fabricate a window without a server timestamp', () => {
  assert.deepEqual(coverageWindow([], undefined), []);
});

test('aligns healthy evidence and leaves silent hours neutral', () => {
  const values = healthyEvidenceWindow(
    [
      { ...hour('2026-08-20T12:00:00Z', 10, 5), healthy_observations: 4 },
      { ...hour('2026-08-20T13:00:00Z', 10, 0), healthy_observations: 0 },
      { ...hour('2026-08-20T14:00:00Z', 10, 12), healthy_observations: 15 }
    ],
    '2026-08-20T14:37:00Z',
    4
  );

  assert.deepEqual(values, [null, 0.8, null, 1]);
});

test('does not fabricate healthy evidence without a server timestamp', () => {
  assert.deepEqual(healthyEvidenceWindow([], undefined), []);
});

test('keeps automatic suggestions out of personal indicator pins', () => {
  assert.deepEqual(
    pinnedIndicatorIDs([
      { id: 'suggested-disk', pinned: false },
      { id: 'second-pin', pinned: true, pin_position: 1 },
      { id: 'first-pin', pinned: true, pin_position: 0 }
    ]),
    ['first-pin', 'second-pin']
  );
});
