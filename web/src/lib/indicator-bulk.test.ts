// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';
import { addSystemIndicators, indicatorSelectionKey, setBindingsEnabled } from './indicator-bulk.ts';

const candidate = (semanticKey, externalId, dimension = '', available = true) => ({
  semantic_key: semanticKey,
  external_id: externalId,
  dimension,
  available,
  recommended: false,
  label: externalId,
  unit: 'percent',
  metadata: {}
});

const binding = (externalId, targetId, candidates = []) => ({
  source: { external_id: externalId, candidates },
  enabled: false,
  targetId,
  selected: new Set()
});

test('enables only displayed bindings already attached to a target', () => {
  const attached = binding('host-1', 'target-1');
  const unattached = binding('host-2', '');
  const hidden = binding('host-3', 'target-3');

  const result = setBindingsEnabled(
    [attached, unattached, hidden],
    new Set(['host-1', 'host-2']),
    true
  );

  assert.deepEqual(result, { changed: 1, skipped: 1 });
  assert.equal(attached.enabled, true);
  assert.equal(unattached.enabled, false);
  assert.equal(hidden.enabled, false);
});

test('deselects displayed attached bindings without touching hidden bindings', () => {
  const visible = binding('host-1', 'target-1');
  const hidden = binding('host-2', 'target-2');
  visible.enabled = true;
  hidden.enabled = true;

  const result = setBindingsEnabled([visible, hidden], new Set(['host-1']), false);

  assert.deepEqual(result, { changed: 1, skipped: 0 });
  assert.equal(visible.enabled, false);
  assert.equal(hidden.enabled, true);
});

test('adds every available CPU, RAM and filesystem indicator to active hosts', () => {
  const cpu = candidate('cpu.utilization', 'cpu');
  const memory = candidate('memory.utilization', 'memory');
  const root = candidate('filesystem.utilization', 'disk-root', '/');
  const data = candidate('filesystem.utilization', 'disk-data', '/data');
  const network = candidate('network.receive', 'network');
  const unavailableDisk = candidate('filesystem.utilization', 'disk-missing', '/missing', false);
  const active = binding('host-1', 'target-1', [cpu, memory, root, data, network, unavailableDisk]);
  active.enabled = true;
  active.selected.add(indicatorSelectionKey(network));
  const inactive = binding('host-2', 'target-2', [cpu]);

  const result = addSystemIndicators([active, inactive]);

  assert.deepEqual(result, { added: 4, affectedBindings: 1 });
  assert.equal(active.selected.has(indicatorSelectionKey(network)), true);
  assert.equal(active.selected.has(indicatorSelectionKey(root)), true);
  assert.equal(active.selected.has(indicatorSelectionKey(data)), true);
  assert.equal(active.selected.has(indicatorSelectionKey(unavailableDisk)), false);
  assert.equal(inactive.selected.size, 0);
});
