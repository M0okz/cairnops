// @ts-nocheck -- Node executes these tests without the browser's Svelte types.
import assert from 'node:assert/strict';
import test from 'node:test';
import { chartCoordinates, chartSegments } from './chart-geometry.ts';
import { indicatorMarkerX, indicatorTimeBounds, indicatorTimeTicks, indicatorWindowPoints, interpolateChartCoordinates, maximumCoordinates, stepChartPath } from './indicator-history.ts';

const generated = '2026-09-06T12:30:00Z';
const end = Date.parse(generated);
const sample = (minutes, value = 42) => ({ at: new Date(end + minutes * 60_000).toISOString(), value });
const geometry = { width: 200, height: 100, insetX: 0, insetTop: 0, insetBottom: 0, bounds: [0, 100], timeBounds: [end - 3_600_000, end] };

test('one hour uses the projection time, never the last available sample as a fake present', () => {
  const bounds = indicatorTimeBounds(generated, '1h');
  assert.deepEqual(bounds, [end - 3_600_000, end]);
  assert.deepEqual(indicatorWindowPoints([sample(-70), sample(-120)], bounds), []);
  assert.deepEqual(indicatorWindowPoints([sample(-20), sample(-60), sample(1), { at: 'bad', value: 1 }, sample(-10, NaN)], bounds), [sample(-60), sample(-20)]);
  assert.equal(indicatorTimeBounds('invalid', '7d'), null);
});

test('partial collection and isolated samples keep their actual position within the whole window', () => {
  const coordinates = chartCoordinates([sample(-45), sample(-30)], geometry);
  assert.deepEqual(coordinates.map((point) => point.x), [50, 100]);
  assert.equal(chartCoordinates([sample(-30)], geometry)[0].x, 100);
});

test('axis ticks adapt to available width without inventing observation times', () => {
  const bounds = indicatorTimeBounds(generated, '24h');
  const wide = indicatorTimeTicks(bounds, 1100);
  const narrow = indicatorTimeTicks(bounds, 240);
  assert.ok(wide.length >= 8);
  assert.ok(narrow.length <= 4);
  assert.ok(indicatorTimeTicks(indicatorTimeBounds(generated, '7d'), 240).length >= 3);
  assert.ok(wide.every((time) => time >= bounds[0] && time <= bounds[1] && time % 3_600_000 === 0));
  assert.deepEqual(indicatorTimeTicks([NaN, NaN], 200), []);
});

test('hourly latest values and maxima stay distinct, missing maxima do not become zero or latest', () => {
  const points = [{ ...sample(-60, 30), maximum: 90 }, sample(-30, 20), { ...sample(0, 10), maximum: 50 }];
  const coordinates = chartCoordinates(points, geometry);
  const maxima = maximumCoordinates(points, coordinates, [0, 100], 0, 100);
  assert.deepEqual(maxima.map(({ at, value, y, index }) => ({ at, value, y, index })), [
    { at: points[0].at, value: 90, y: 10, index: 0 },
    { at: points[2].at, value: 50, y: 50, index: 2 }
  ]);
  assert.equal(points[0].value, 30);
  assert.deepEqual(maximumCoordinates([sample(0)], chartCoordinates([sample(0)], geometry), [0, 100], 0, 100), []);
});

test('a wide weekly axis never repeats date labels with invisible half-day ticks', () => {
  const ticks = indicatorTimeTicks(indicatorTimeBounds(generated, '7d'), 1200);
  const labels = ticks.map((time) => new Intl.DateTimeFormat('fr-FR', { day: 'numeric', month: 'short', timeZone: 'Europe/Paris' }).format(time));
  assert.equal(new Set(labels).size, labels.length);
  assert.ok(ticks.length >= 6);
});

test('animation accepts a different sample count and can be interrupted without mutating evidence', () => {
  const before = chartCoordinates([sample(-60, 10), sample(0, 50)], geometry);
  const after = chartCoordinates([sample(-60, 40), sample(-30, 20), sample(0, 80)], geometry);
  const transition = interpolateChartCoordinates(before, after);
  assert.deepEqual(transition(0).map((point) => point.y), [90, 70, 50]);
  assert.deepEqual(transition(1), after);
  const middle = transition(0.5);
  assert.deepEqual(middle.map((point) => point.value), after.map((point) => point.value));
  const interrupted = interpolateChartCoordinates(middle, before);
  assert.equal(interrupted(0)[0].y, middle[0].y);
  assert.deepEqual(interrupted(1), before);
  assert.deepEqual(interpolateChartCoordinates([], after)(1), after);
  assert.deepEqual(interpolateChartCoordinates(before, [])(0.5), []);
});

test('missing collection remains a gap throughout animation and booleans remain steps', () => {
  const points = chartCoordinates([sample(-60, 0), sample(-59, 1), sample(0, 0)], geometry);
  const segments = chartSegments(interpolateChartCoordinates(points, points)(0.5), 5 * 60_000);
  assert.deepEqual(segments.map((segment) => segment.length), [2, 1]);
  assert.match(stepChartPath(points), /^M0,100H/);
  assert.ok(!stepChartPath(points).includes('C'));
});


test('incident markers keep their exact time in collection gaps and disappear outside the window', () => {
  const bounds = [end - 4 * 3_600_000, end];
  assert.equal(indicatorMarkerX(sample(-120).at, bounds, 400), 200);
  assert.equal(indicatorMarkerX(sample(-240).at, bounds, 400), 8);
  assert.equal(indicatorMarkerX(sample(0).at, bounds, 400), 392);
  assert.equal(indicatorMarkerX(sample(-241).at, bounds, 400), null);
  assert.equal(indicatorMarkerX(sample(1).at, bounds, 400), null);
  assert.equal(indicatorMarkerX('invalid', bounds, 400), null);
  assert.equal(indicatorMarkerX(sample(-120).at, [end, end], 400), null);
  assert.equal(indicatorMarkerX(sample(-120).at, bounds, 0), null);
});
