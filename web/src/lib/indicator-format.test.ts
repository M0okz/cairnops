// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';
import { formatIndicator, indicatorBounds } from './indicator-format.ts';

test('keeps percent charts fixed from zero to one hundred', () => {
  assert.deepEqual(indicatorBounds([{ at: '2026-01-01', value: 52 }], 'percent'), [0, 100]);
});

test('anchors certificate days at zero and adapts other metrics', () => {
  assert.equal(indicatorBounds([{ at: '2026-01-01', value: 12 }], 'days')[0], 0);
  assert.deepEqual(indicatorBounds([{ at: '2026-01-01', value: 40 }, { at: '2026-01-02', value: 50 }], 'milliseconds'), [39.2, 50.8]);
});

test('formats network and boolean values semantically', () => {
  assert.equal(formatIndicator(2048, 'bytes_per_second'), '2 Kio/s');
  assert.equal(formatIndicator(1, 'boolean'), 'Oui');
});
