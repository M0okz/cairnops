import assert from 'node:assert/strict';
import test from 'node:test';
import type { ContextIndicator, IndicatorPoint } from './api.ts';
// @ts-ignore -- Node executes tests directly with its TypeScript loader.
import { evaluateKumaLatency } from './kuma-latency.ts';

const now = new Date('2026-10-01T12:00:00Z');
const indicator = { last_value: 350, last_observed_at: '2026-10-01T11:59:00Z' } as ContextIndicator;
const history: IndicatorPoint[] = Array.from({ length: 40 }, (_, index) => ({
  at: new Date(now.getTime() - (index + 25) * 3_600_000).toISOString(), value: 100
}));

test('compares a fresh Kuma reading against older hourly values', () => {
  assert.deepEqual(evaluateKumaLatency(indicator, history, now), {
    status: 'candidate', observed: 350, median: 100, threshold: 200, hours: 40
  });
});

test('holds out recent data and requires enough distinct training hours', () => {
  const recent = { at: '2026-10-01T11:00:00Z', value: 9000 };
  assert.equal(evaluateKumaLatency(indicator, [...history, recent], now).status, 'candidate');
  assert.equal(evaluateKumaLatency(indicator, history.slice(0, 29), now).status, 'training');
  assert.equal(evaluateKumaLatency(indicator, Array(40).fill(history[0]), now).status, 'training');
});

test('never treats stale, missing or non-positive readings as ordinary', () => {
  assert.equal(evaluateKumaLatency({ ...indicator, last_error: 'missing' }, history, now).status, 'unavailable');
  assert.equal(evaluateKumaLatency({ ...indicator, last_observed_at: '2026-10-01T11:40:00Z' }, history, now).status, 'unavailable');
  assert.equal(evaluateKumaLatency({ ...indicator, last_value: 0 }, history, now).status, 'unavailable');
  assert.equal(evaluateKumaLatency(indicator, undefined, now).status, 'unavailable');
});
