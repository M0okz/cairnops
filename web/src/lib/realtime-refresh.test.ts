// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import { projectionsFor, throttled } from './realtime-refresh.ts';

test('identity and device events leave the resource list alone', () => {
  assert.deepEqual(projectionsFor('reconciliation.changed'), []);
  assert.deepEqual(projectionsFor('device.changed'), []);
  assert.deepEqual(projectionsFor('observation.created'), []);
  assert.deepEqual(projectionsFor('target.changed'), ['targets']);
  assert.deepEqual(projectionsFor('source.changed'), ['targets']);
  assert.deepEqual(projectionsFor('maintenance.changed'), ['maintenances', 'incidents']);
});

test('an unknown event still refreshes the resource list', () => {
  assert.deepEqual(projectionsFor('future.changed'), ['targets']);
});

test('a burst of requests runs once, then at most once per interval', () => {
  let clock = 0;
  let runs = 0;
  const scheduled: Array<{ callback: () => void; at: number }> = [];
  const throttle = throttled(
    () => { runs += 1; },
    5000,
    90,
    (callback, delay) => { scheduled.push({ callback, at: clock + delay }); return scheduled.length; },
    () => {},
    () => clock
  );
  const advance = (to: number) => {
    clock = to;
    while (scheduled.length && scheduled[0].at <= clock) scheduled.shift().callback();
  };

  for (let index = 0; index < 20; index += 1) throttle.request();
  assert.equal(scheduled.length, 1);
  assert.equal(scheduled[0].at, 90);
  advance(90);
  assert.equal(runs, 1);

  // Une demande pendant l'intervalle attend sa fin, sans se perdre.
  advance(1000);
  throttle.request();
  throttle.request();
  assert.equal(scheduled.length, 1);
  assert.equal(scheduled[0].at, 5090);
  advance(5089);
  assert.equal(runs, 1);
  advance(5090);
  assert.equal(runs, 2);

  // Après un long silence, la demande suivante ne patiente que le regroupement.
  advance(20000);
  throttle.request();
  assert.equal(scheduled[0].at, 20090);
});

test('a cancelled request does not run', () => {
  let runs = 0;
  const pending: Array<() => void> = [];
  const throttle = throttled(() => { runs += 1; }, 5000, 90, (callback) => { pending.push(callback); return pending.length; }, () => { pending.length = 0; });
  throttle.request();
  throttle.cancel();
  assert.equal(pending.length, 0);
  throttle.request();
  assert.equal(pending.length, 1);
  pending[0]();
  assert.equal(runs, 1);
});
