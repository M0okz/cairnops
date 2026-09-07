import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const consumers = [
  '../routes/incidents/+page.svelte',
  '../routes/cibles/+page.svelte',
  '../routes/cibles/[id]/+page.svelte',
  '../routes/maintenance/+page.svelte',
  './components/TargetIndicators.svelte',
  './components/Rail.svelte'
].map((path) => readFileSync(new URL(path, import.meta.url), 'utf8'));

test('tous les groupes segmentés utilisent le même composant', () => {
  for (const consumer of consumers) {
    assert.doesNotMatch(consumer, /<div class="segments"/);
    assert.match(consumer, /SegmentedControl/);
  }
});
