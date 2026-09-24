import test from 'node:test';
import assert from 'node:assert/strict';
// @ts-expect-error Node's native test runner requires the TypeScript extension.
import { activityDays, activityMarker, activityMessage, activityOrigin } from './activity-timeline.ts';

test('calendar days use the reader timezone and preserve simultaneous facts', () => {
  const entries = [
    { id: 'opening', occurred_at: '2026-09-06T21:59:00Z' },
    { id: 'propagation', occurred_at: '2026-09-06T22:01:00Z' },
    { id: 'resolved', occurred_at: '2026-09-06T22:01:00Z' }
  ];
  const days = activityDays(entries, 'Europe/Paris');
  assert.equal(days.length, 2);
  assert.deepEqual(days.map((day) => day.entries.map((entry) => entry.id)), [['propagation', 'resolved'], ['opening']]);
  assert.equal(activityDays(entries, 'America/New_York').length, 1);
  assert.equal(entries[0].id, 'opening');
});

test('acknowledgement, invalidation and propagation closure never imply recovery', () => {
  for (const kind of ['acknowledged', 'upstream_acknowledged', 'invalidated', 'propagation_closed', 'new_future_event']) {
    assert.equal(activityMarker(kind).restored, false);
  }
  assert.equal(activityMarker('resolved').restored, true);
  assert.equal(activityMarker('evidence_resolved').restored, true);
  assert.equal(activityMarker('new_future_event').icon, 'activity');
  assert.equal(activityOrigin('uptime_kuma'), 'Uptime Kuma');
  assert.equal(activityOrigin('new_connector'), 'new_connector');
});

test('a stored propagation event uses the current clear label without changing other messages', () => {
  const translate = (key: string) => ({
    'incidents.propagation.closed': 'Regroupement terminé',
    'timeline.recordedEvent': 'Événement enregistré'
  })[key] ?? key;

  assert.equal(activityMessage({ kind: 'propagation_closed', message: 'Propagation fermée' }, translate), 'Regroupement terminé');
  assert.equal(activityMessage({ kind: 'opened', message: 'Incident ouvert' }, translate), 'Incident ouvert');
  assert.equal(activityMessage({ kind: 'opened', message: ' ' }, translate), 'Événement enregistré');
});
