// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import { startReconciliationPolling } from './reconciliation-polling.ts';

test('polls once at a time and adapts the next delay to active operations', async () => {
  const scheduled: Array<{ callback: () => void; delay: number }> = [];
  let active = false;
  let calls = 0;

  const stop = startReconciliationPolling(
    async () => { calls += 1; },
    () => active,
    (callback, delay) => {
      scheduled.push({ callback, delay });
      return scheduled.length;
    },
    () => {}
  );

  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(calls, 1);
  assert.equal(scheduled.length, 1);
  assert.equal(scheduled[0].delay, 30_000);

  active = true;
  scheduled.shift()?.callback();
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(calls, 2);
  assert.equal(scheduled.length, 1);
  assert.equal(scheduled[0].delay, 2_000);

  stop();
});

test('stopping during a load does not schedule another poll', async () => {
  const scheduled: Array<{ callback: () => void; delay: number }> = [];
  let release!: () => void;
  const loading = new Promise<void>((resolve) => { release = resolve; });

  const stop = startReconciliationPolling(
    () => loading,
    () => false,
    (callback, delay) => {
      scheduled.push({ callback, delay });
      return scheduled.length;
    },
    () => {}
  );

  stop();
  release();
  await new Promise((resolve) => setImmediate(resolve));
  assert.deepEqual(scheduled, []);
});
