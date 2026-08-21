// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  pairingCanCancel,
  pairingQRCode,
  pairingSecondsRemaining,
  pairingShouldPoll,
  pairingStep
} from './device-pairing.ts';

test('pairing countdown rounds up and never becomes negative', () => {
  const now = new Date('2026-08-21T10:00:00.000Z');
  assert.equal(pairingSecondsRemaining('2026-08-21T10:00:01.001Z', now), 2);
  assert.equal(pairingSecondsRemaining('2026-08-21T10:00:00.000Z', now), 0);
  assert.equal(pairingSecondsRemaining('2026-08-21T09:59:00.000Z', now), 0);
  assert.equal(pairingSecondsRemaining('not-a-date', now), 0);
});

test('only live pairing states keep polling', () => {
  assert.equal(pairingShouldPoll('awaiting_scan'), true);
  assert.equal(pairingShouldPoll('awaiting_confirmation'), true);
  assert.equal(pairingShouldPoll('confirmed'), true);
  assert.equal(pairingShouldPoll('credential_consumed'), false);
  assert.equal(pairingShouldPoll('expired'), false);
  assert.equal(pairingShouldPoll('cancelled'), false);
});

test('an invitation can only be cancelled before confirmation', () => {
  assert.equal(pairingCanCancel('awaiting_scan'), true);
  assert.equal(pairingCanCancel('awaiting_confirmation'), true);
  assert.equal(pairingCanCancel('confirmed'), false);
  assert.equal(pairingCanCancel('credential_consumed'), false);
});

test('pairing states map to the three confirmations', () => {
  assert.equal(pairingStep('awaiting_scan'), 1);
  assert.equal(pairingStep('awaiting_confirmation'), 2);
  assert.equal(pairingStep('confirmed'), 3);
  assert.equal(pairingStep('credential_consumed'), 3);
});

test('the QR helper renders the deep link as a PNG data URL', async () => {
  const dataURL = await pairingQRCode(
    'cairnops://pair?instance=https%3A%2F%2Fcairnops.example&token=temporary-secret'
  );
  assert.match(dataURL, /^data:image\/png;base64,/);
});
