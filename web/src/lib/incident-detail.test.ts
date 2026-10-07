// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { ContextIndicator, Incident, IncidentIndicators } from './api.ts';
import {
  findingShape,
  incidentActivity,
  incidentHref,
  incidentIndicatorRows,
  primaryEvidence,
  splitIndicatorRows,
  visibleIncidentActivity
} from './incident-detail.ts';

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

  it('hides the grouping window when no second Resource joined', () => {
    const activity = [
      { id: 1, kind: 'opened', message: 'Incident ouvert', occurred_at: '2026-09-24T10:00:00Z' },
      { id: 2, kind: 'propagation_closed', message: 'Regroupement terminé', occurred_at: '2026-09-24T10:05:00Z' }
    ];
    const single = { impact_count: 1, activity } as Incident;
    const grouped = { impact_count: 2, activity } as Incident;

    assert.deepEqual(visibleIncidentActivity(single).map((entry) => entry.kind), ['opened']);
    assert.deepEqual(incidentActivity(single).map((entry) => entry.kind), ['opened']);
    assert.deepEqual(visibleIncidentActivity(grouped).map((entry) => entry.kind), ['opened', 'propagation_closed']);
  });

  it('keeps captured values first even when their Indicator no longer exists', () => {
    const current = {
      id: 'current',
      target_id: 'target-api',
      semantic_key: 'cpu.utilization',
      label: 'CPU',
      unit: 'percent'
    } as ContextIndicator;
    const additional = {
      id: 'additional',
      target_id: 'target-api',
      semantic_key: 'memory.utilization',
      label: 'RAM',
      unit: 'percent'
    } as ContextIndicator;
    const detail = {
      target_ids: ['target-api'],
      snapshots: [
        {
          indicator_id: 'current',
          impact_id: 'impact-api',
          target_id: 'target-api',
          target_name: 'API',
          semantic_key: 'cpu.utilization',
          label: 'CPU',
          unit: 'percent',
          value: 92,
          observed_at: '2026-08-29T10:00:00Z'
        },
        {
          impact_id: 'impact-api',
          target_id: 'target-api',
          target_name: 'API',
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

  it('shapes the Finding from what the recognized fact establishes', () => {
    assert.deepEqual(findingShape(undefined), { kind: 'original' });
    const update = { kind: 'software.major_update_available', current_version: '34.0.4', available_version: '35.0.1' };
    assert.equal(findingShape(update).kind, 'versions');
    assert.equal(findingShape({ kind: 'software.update_available', available_version: '35.0.1' }).kind, 'plain');
    assert.deepEqual(findingShape({ kind: 'software.security_updates', count: 0 }).kind, 'count');
    assert.equal(findingShape({ kind: 'disk.latency.high', resource: 'sda' }).noun, 'disk');
    assert.equal(findingShape({ kind: 'disk.space.low', resource: '/' }).noun, 'volume');
    assert.equal(findingShape({ kind: 'certificate.expiring', resource: 'cloud.example' }).noun, 'certificate');
    assert.equal(findingShape({ kind: 'system.reboot_required' }).kind, 'plain');
  });

  it('carries the Finding on an active, non-invalidated Evidence first', () => {
    const invalidated = { id: 'a', active: true, invalidated_at: '2026-10-01T00:00:00Z' };
    const recovered = { id: 'b', active: false };
    const active = { id: 'c', active: true };
    const incident = { impacts: [{ evidence: [invalidated, recovered] }, { evidence: [active] }] } as Incident;
    assert.equal(primaryEvidence(incident)?.id, 'c');
    active.active = false;
    assert.equal(primaryEvidence(incident)?.id, 'b');
    assert.equal(primaryEvidence({ impacts: [] } as Incident), null);
  });

  it('puts the curve that measures the condition first, never an unrelated one', () => {
    const row = (key, semantic, dimension) => ({
      key,
      label: key,
      unit: 'percent',
      indicator: { id: key, semantic_key: semantic, dimension },
      points: []
    });
    const rows = {
      captured: [row('cpu', 'cpu.utilization'), row('ram', 'memory.utilization')],
      additional: [row('root', 'filesystem.utilization', '/'), row('data', 'filesystem.utilization', '/data')]
    };
    const keys = (split) => [split.relevant.map((item) => item.key), split.others.map((item) => item.key)];

    assert.deepEqual(keys(splitIndicatorRows(rows, { kind: 'disk.latency.high', resource: 'sda' })), [[], ['cpu', 'ram', 'root', 'data']]);
    assert.deepEqual(keys(splitIndicatorRows(rows, { kind: 'software.major_update_available' })), [[], ['cpu', 'ram', 'root', 'data']]);
    assert.deepEqual(keys(splitIndicatorRows(rows, { kind: 'cpu.usage.high' })), [['cpu'], ['ram', 'root', 'data']]);
    assert.deepEqual(keys(splitIndicatorRows(rows, { kind: 'disk.space.low', resource: '/data' })), [['data'], ['cpu', 'ram', 'root']]);
    assert.deepEqual(keys(splitIndicatorRows(rows, { kind: 'disk.space.low', resource: '/srv' })), [['root', 'data'], ['cpu', 'ram']]);
    assert.deepEqual(keys(splitIndicatorRows(rows, undefined)), [[], ['cpu', 'ram', 'root', 'data']]);
  });
});
