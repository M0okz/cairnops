// @ts-nocheck -- executed by Node without the browser type environment.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { compareActiveIncidents, historyBounds, incidentFilterOptions, matchesIncidentFilters, resolvedIncidentQuery } from './incident-list.ts';

const filters = { query: '', targetID: '', natureKey: '', severity: '' };
const incident = (id, overrides = {}) => ({
  id, severity: 'major', opened_at: '2026-09-22T10:00:00Z',
  nature_key: 'availability', nature_label: 'Indisponibilité',
  impacts: [
    { target_id: 'api', target_name: 'API', evidence: [] },
    { target_id: 'storage', target_name: 'Storage 100%', evidence: [{ name: 'Latency above 200 ms' }] }
  ], ...overrides
});

describe('incident triage', () => {
  it('keeps unacknowledged incidents ahead of an older acknowledged critical one, then sorts severity and age', () => {
    const rows = [
      incident('handled', { severity: 'critical', acknowledged_at: '2026-09-21T12:00:00Z', opened_at: '2026-09-21T10:00:00Z' }),
      incident('warning', { severity: 'warning', opened_at: '2026-09-21T10:00:00Z' }),
      incident('recent'), incident('old', { opened_at: '2026-09-22T09:00:00Z' }),
      incident('critical', { severity: 'critical' })
    ];
    assert.deepEqual(rows.sort(compareActiveIncidents).map((row) => row.id), ['critical', 'old', 'recent', 'warning', 'handled']);
  });

  it('keeps a deterministic order for equal instants', () => {
    assert.deepEqual([incident('b'), incident('a')].sort(compareActiveIncidents).map((row) => row.id), ['a', 'b']);
  });

  it('searches the French and English titles shown instead of the raw provider nature', () => {
    const row = incident('localized', {
      nature_key: 'connector:opaque', nature_label: 'Provider condition 42',
      presentation: { fr: { title: 'Charge système moyenne élevée' }, en: { title: 'High average system load' } }
    });
    assert.equal(matchesIncidentFilters(row, { ...filters, query: 'CHARGE SYSTÈME' }), true);
    assert.equal(matchesIncidentFilters(row, { ...filters, query: 'average system load' }), true);
    assert.equal(matchesIncidentFilters(row, { ...filters, query: 'Provider condition' }), true);
    assert.equal(matchesIncidentFilters(row, { ...filters, query: 'average system load', severity: 'critical' }), false);
  });

  it('matches every resource and evidence in a multi-resource incident, combining filters', () => {
    const row = incident('multi');
    assert.equal(matchesIncidentFilters(row, { ...filters, targetID: 'storage', query: '  LATENCY ', severity: 'major' }), true);
    assert.equal(matchesIncidentFilters(row, { ...filters, query: '100%' }), true);
    assert.equal(matchesIncidentFilters(row, { ...filters, query: '100_' }), false);
    assert.equal(matchesIncidentFilters(row, { ...filters, targetID: 'absent' }), false);
    assert.equal(matchesIncidentFilters(row, { ...filters, natureKey: 'latency' }), false);
    assert.equal(matchesIncidentFilters(row, { ...filters, severity: 'critical' }), false);
    assert.equal(incidentFilterOptions([row, incident('another')]).targets.length, 2);
  });
});

describe('resolved incident history', () => {
  it('sends all filters and dates to the server before requesting more results', () => {
    const bounds = { from: '2026-09-01T00:00:00Z', before: '2026-09-23T00:00:00Z' };
    const url = new URL(resolvedIncidentQuery({ query: ' Storage 100% ', targetID: 'second', natureKey: 'availability', severity: 'major' }, bounds, 'opaque+cursor'), 'https://example.test');
    assert.deepEqual(Object.fromEntries(url.searchParams), {
      status: 'resolved', page: 'true', limit: '50', q: 'Storage 100%', target_id: 'second',
      nature_key: 'availability', severity: 'major', resolved_from: bounds.from,
      resolved_before: bounds.before, cursor: 'opaque+cursor'
    });
  });

  it('uses inclusive calendar days in the reader timezone, including the short DST day', () => {
    const previous = process.env.TZ;
    process.env.TZ = 'Europe/Paris';
    try {
      assert.deepEqual(historyBounds('custom', '2026-03-29', '2026-03-29'), {
        from: '2026-03-28T23:00:00.000Z', before: '2026-03-29T22:00:00.000Z'
      });
      assert.deepEqual(historyBounds('7', '', '', new Date('2026-09-22T12:00:00Z')), {
        from: '2026-09-15T22:00:00.000Z', before: '2026-09-22T22:00:00.000Z'
      });
    } finally {
      if (previous === undefined) delete process.env.TZ;
      else process.env.TZ = previous;
    }
  });

  it('accepts unbounded history but rejects incomplete, nonexistent and reversed dates', () => {
    assert.deepEqual(historyBounds('all', '', ''), {});
    for (const [from, through] of [['', '2026-09-22'], ['2026-02-30', '2026-03-02'], ['2026-09-23', '2026-09-22']]) {
      assert.equal(historyBounds('custom', from, through), null);
    }
    assert.equal(new URL(resolvedIncidentQuery(filters, {}), 'https://example.test').searchParams.has('resolved_from'), false);
  });
});
