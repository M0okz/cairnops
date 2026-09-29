import assert from 'node:assert/strict';
import test from 'node:test';
import type { LatencyProfile, LatencyProfileHour } from './api.ts';
// @ts-ignore -- Node executes tests directly with its TypeScript loader.
import { allHours, bucketAt, latencyHabit } from './latency-profile.ts';

const hour = (
  hour: number,
  median: number,
  threshold: number | null,
  samples = 400
): LatencyProfileHour => ({
  hour,
  samples,
  median_milliseconds: median,
  p95_milliseconds: median * 2,
  p99_milliseconds: threshold ?? median * 3,
  threshold_milliseconds: threshold
});

const profile = (hours: LatencyProfileHour[]): LatencyProfile => ({
  window_start: '2026-09-01T00:00:00Z',
  window_end: '2026-09-29T00:00:00Z',
  samples: hours.reduce((total, bucket) => total + bucket.samples, 0),
  computed_at: '2026-09-29T00:05:00Z',
  hours
});

const at = (isoHour: string) => new Date(`2026-09-29T${isoHour}:00:00Z`);

test("l'heure concernée prime sur le repli", () => {
  const selected = bucketAt(profile([hour(allHours, 100, 300), hour(14, 800, 2000)]), at('14'));
  assert.equal(selected?.hour, 14);
});

test('une heure sans seuil se replie sur toutes les heures confondues', () => {
  const selected = bucketAt(profile([hour(allHours, 100, 300), hour(14, 800, null, 4)]), at('14'));
  assert.equal(selected?.hour, allHours);
});

test("une heure absente se replie aussi sur toutes les heures confondues", () => {
  const selected = bucketAt(profile([hour(allHours, 100, 300)]), at('03'));
  assert.equal(selected?.hour, allHours);
});

test("sans aucun seau établi, rien n'est conclu", () => {
  assert.equal(bucketAt(profile([hour(allHours, 100, null, 4), hour(14, 800, null, 2)]), at('14')), null);
  assert.equal(latencyHabit(profile([hour(allHours, 100, null, 4)]), at('14')), null);
});

test("un Profil absent ne rend aucune habitude", () => {
  assert.equal(bucketAt(undefined, at('14')), null);
  assert.equal(latencyHabit(undefined, at('14')), null);
});

test("l'habitude nomme sa portée et son étendue", () => {
  const hourly = latencyHabit(profile([hour(allHours, 100, 300), hour(14, 800, 2000, 120)]), at('14'));
  assert.deepEqual(hourly, { median: 800, threshold: 2000, samples: 120, scope: 'hour' });

  const overall = latencyHabit(profile([hour(allHours, 100, 300, 900)]), at('14'));
  assert.deepEqual(overall, { median: 100, threshold: 300, samples: 900, scope: 'all' });
});

test('les seaux sont ceux d\'UTC quel que soit le fuseau du lecteur', () => {
  const selected = bucketAt(profile([hour(14, 800, 2000)]), new Date('2026-09-29T16:05:00+02:00'));
  assert.equal(selected?.hour, 14);
});
