// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import {
  chartCoordinates,
  chartSegments,
  monotoneChartPath,
  nearestChartPoint
} from './chart-geometry.ts';

test('places samples according to their real timestamps', () => {
  const coordinates = chartCoordinates(
    [
      { at: '2026-08-30T10:00:00Z', value: 20 },
      { at: '2026-08-30T10:10:00Z', value: 40 },
      { at: '2026-08-30T10:40:00Z', value: 60 }
    ],
    { width: 100, height: 50, insetX: 0, insetTop: 0, insetBottom: 0, bounds: [0, 100] }
  );

  assert.deepEqual(coordinates.map(({ x }) => x), [0, 25, 100]);
  assert.deepEqual(coordinates.map(({ y }) => y), [40, 30, 20]);
});

test('draws a smooth monotone curve through every proof point', () => {
  const path = monotoneChartPath([
    { at: 'a', value: 10, index: 0, x: 0, y: 40 },
    { at: 'b', value: 40, index: 1, x: 50, y: 10 },
    { at: 'c', value: 30, index: 2, x: 100, y: 20 }
  ]);

  assert.match(path, /^M0,40C/);
  assert.match(path, /100,20$/);
  assert.equal((path.match(/C/g) ?? []).length, 2);
});

test('selects the proof closest to the pointer', () => {
  const coordinates = [
    { at: 'a', value: 10, index: 0, x: 10, y: 20 },
    { at: 'b', value: 20, index: 1, x: 60, y: 15 },
    { at: 'c', value: 30, index: 2, x: 110, y: 10 }
  ];

  assert.equal(nearestChartPoint(coordinates, 43)?.index, 1);
  assert.equal(nearestChartPoint([], 43), null);
});

test('keeps missing hourly evidence as a visible gap', () => {
  const coordinates = [
    { at: '2026-08-30T10:00:00Z', value: 90, index: 0, x: 0, y: 10 },
    { at: '2026-08-30T11:00:00Z', value: 91, index: 1, x: 30, y: 9 },
    { at: '2026-08-30T14:00:00Z', value: 89, index: 2, x: 100, y: 11 }
  ];

  assert.deepEqual(chartSegments(coordinates, 90 * 60 * 1_000).map((segment) => segment.length), [2, 1]);
});
