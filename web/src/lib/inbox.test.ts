// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { inboxEntryState, unreadEntryIds } from './inbox.ts';

describe('inbox', () => {
  it('reads an opening through the current state of its Incident', () => {
    assert.equal(inboxEntryState({ event_kind: 'firing', incident_status: 'active', acknowledged_at: null }), 'open');
    assert.equal(
      inboxEntryState({ event_kind: 'firing', incident_status: 'active', acknowledged_at: '2026-09-25T10:00:00Z' }),
      'acknowledged'
    );
    assert.equal(
      inboxEntryState({ event_kind: 'firing', incident_status: 'resolved', acknowledged_at: '2026-09-25T10:00:00Z' }),
      'resolved'
    );
  });

  it('keeps what it received when the server does not send the Incident state', () => {
    assert.equal(inboxEntryState({ event_kind: 'firing' }), 'open');
    assert.equal(inboxEntryState({ event_kind: 'resolved' }), 'resolved');
  });

  it('remembers which entries were new when the panel opened', () => {
    const ids = unreadEntryIds([
      { id: 1, read_at: null },
      { id: 2, read_at: '2026-09-25T10:00:00Z' },
      { id: 3, read_at: null }
    ]);
    assert.deepEqual([...ids], [1, 3]);
  });
});
