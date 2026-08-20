// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import type { Incident } from './api.ts';
import { incidentTimelineForTarget } from './incident-timeline.ts';

function incident(input: Pick<Incident, 'id' | 'target_id' | 'status' | 'activity'>): Incident {
  return input as Incident;
}

test('keeps the activity of an incident resolved by invalidating its last signal', () => {
  const resolved = incident({
    id: 'incident-resolved-by-invalidation',
    target_id: 'target-1',
    status: 'resolved',
    activity: [
      {
        id: 3,
        kind: 'invalidated',
        origin: 'user',
        actor_name: 'Grégory',
        message: 'Preuve invalidée par Grégory',
        data: { reason: 'Cette source publie un faux positif' },
        occurred_at: '2026-08-20T06:58:00Z'
      },
      {
        id: 2,
        kind: 'resolved',
        origin: 'system',
        message: 'Incident résolu',
        data: {},
        occurred_at: '2026-08-20T06:58:00Z'
      },
      {
        id: 1,
        kind: 'opened',
        origin: 'zabbix',
        message: 'Incident ouvert depuis un problème Zabbix',
        data: {},
        occurred_at: '2026-08-20T06:50:00Z'
      }
    ]
  });

  const journal = incidentTimelineForTarget([], [resolved], 'target-1');

  assert.deepEqual(
    journal.map(({ entry }) => entry.kind),
    ['invalidated', 'resolved', 'opened']
  );
});

test('does not leak another target activity into the journal', () => {
  const other = incident({
    id: 'other-incident',
    target_id: 'target-2',
    status: 'resolved',
    activity: [
      {
        id: 1,
        kind: 'opened',
        origin: 'zabbix',
        message: 'Un autre incident',
        data: {},
        occurred_at: '2026-08-20T07:00:00Z'
      }
    ]
  });

  assert.deepEqual(incidentTimelineForTarget([], [other], 'target-1'), []);
});

test('does not duplicate an active incident already present in history', () => {
  const active = incident({
    id: 'active-incident',
    target_id: 'target-1',
    status: 'active',
    activity: [
      {
        id: 1,
        kind: 'opened',
        origin: 'zabbix',
        message: 'Incident ouvert',
        data: {},
        occurred_at: '2026-08-20T07:00:00Z'
      }
    ]
  });

  assert.equal(incidentTimelineForTarget([active], [active], 'target-1').length, 1);
});
