// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { ContextIndicator, Incident, IncidentIndicators } from './api.ts';
import { incidentActivity, incidentHref, incidentIndicatorRows } from './incident-detail.ts';

describe('incident detail', () => {
  it('builds the shared address of an Incident', () => {
    assert.equal(incidentHref('incident/with spaces'), '/incidents?incident=incident%2Fwith%20spaces');
  });

  it('shows the newest Activity Log entry first', () => {
    const incident = {
      activity: [
        { id: 1, occurred_at: '2026-08-29T10:00:00Z' },
        { id: 2, occurred_at: '2026-08-29T12:00:00Z' }
      ]
    } as Incident;

    assert.deepEqual(
      incidentActivity(incident).map((entry) => entry.id),
      [2, 1]
    );
  });

  it('keeps captured values first even when their Indicator no longer exists', () => {
    const current = {
      id: 'current',
      semantic_key: 'cpu.utilization',
      label: 'CPU',
      unit: 'percent'
    } as ContextIndicator;
    const additional = {
      id: 'additional',
      semantic_key: 'memory.utilization',
      label: 'RAM',
      unit: 'percent'
    } as ContextIndicator;
    const detail = {
      snapshots: [
        {
          indicator_id: 'current',
          semantic_key: 'cpu.utilization',
          label: 'CPU',
          unit: 'percent',
          value: 92,
          observed_at: '2026-08-29T10:00:00Z'
        },
        {
          semantic_key: 'filesystem.utilization',
          label: 'Volume',
          unit: 'percent',
          value: 88,
          observed_at: '2026-08-29T10:00:00Z'
        }
      ],
      indicators: [current, additional],
      series: {
        current: [{ at: '2026-08-29T10:00:00Z', value: 92 }],
        additional: [{ at: '2026-08-29T10:00:00Z', value: 61 }]
      }
    } as IncidentIndicators;

    const rows = incidentIndicatorRows(detail);

    assert.deepEqual(
      rows.captured.map((row) => [row.label, row.snapshot?.value, row.points.length]),
      [
        ['CPU', 92, 1],
        ['Volume', 88, 0]
      ]
    );
    assert.deepEqual(rows.additional.map((row) => row.label), ['RAM']);
  });
});
