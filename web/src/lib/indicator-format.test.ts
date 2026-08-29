// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  formatIndicator,
  incidentMarkerIndex,
  incidentMarkerIsVisible,
  indicatorBounds
} from './indicator-format.ts';

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

test('pins an incident opening to the closest plotted indicator sample', () => {
  const points = [
    { at: '2026-08-30T09:00:00Z', value: 18 },
    { at: '2026-08-30T09:30:00Z', value: 35 },
    { at: '2026-08-30T10:00:00Z', value: 52 }
  ];

  assert.equal(incidentMarkerIndex(points, '2026-08-30T09:43:00Z'), 1);
  assert.equal(incidentMarkerIndex(points, 'date-invalide'), null);
  assert.equal(incidentMarkerIndex([], '2026-08-30T09:43:00Z'), null);
});

test('shows the incident marker only inside the selected indicator window', () => {
  const generatedAt = '2026-08-30T12:00:00Z';

  assert.equal(incidentMarkerIsVisible('2026-08-30T09:43:00Z', generatedAt, '24h'), true);
  assert.equal(incidentMarkerIsVisible('2026-08-28T09:43:00Z', generatedAt, '24h'), false);
  assert.equal(incidentMarkerIsVisible('2026-08-28T09:43:00Z', generatedAt, '7d'), true);
  assert.equal(incidentMarkerIsVisible('2026-08-30T12:01:00Z', generatedAt, '7d'), false);
});
