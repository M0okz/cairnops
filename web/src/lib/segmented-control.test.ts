import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const component = readFileSync(
  new URL('./components/ui/SegmentedControl.svelte', import.meta.url),
  'utf8'
);

const consumers = [
  '../routes/incidents/+page.svelte',
  '../routes/cibles/+page.svelte',
  '../routes/cibles/[id]/+page.svelte',
  '../routes/maintenance/+page.svelte',
  './components/TargetIndicators.svelte',
  './components/Rail.svelte'
].map((path) => readFileSync(new URL(path, import.meta.url), 'utf8'));

test('le contrôle segmenté repose sur la primitive radio partagée', () => {
  assert.match(component, /import \{ RadioGroup as RadioGroupPrimitive \} from 'bits-ui'/);
  assert.match(component, /<RadioGroupPrimitive\.Root[^>]*\{value\}[^>]*orientation="horizontal"/s);
  assert.match(component, /<RadioGroupPrimitive\.Item[^>]*value=\{item\.value\}/s);
  assert.doesNotMatch(component, /Odometer/);
});

test('les compteurs des segments restent stables et correctement alignés', () => {
  assert.match(
    component,
    /\.count\s*\{[^}]*min-inline-size:\s*var\(--counter-slot-w\)[^}]*font-variant-numeric:\s*tabular-nums/s
  );
  assert.match(
    component,
    /button\s*\{[^}]*display:\s*inline-flex[^}]*align-items:\s*center[^}]*justify-content:\s*center/s
  );
  assert.match(
    component,
    /button:focus-visible\s*\{[^}]*outline-offset:\s*-[\d.]+(?:px|rem)/s,
    'le contour clavier doit rester entièrement visible dans le groupe rogné'
  );
});

test('tous les groupes segmentés utilisent le même composant', () => {
  for (const consumer of consumers) {
    assert.doesNotMatch(consumer, /<div class="segments"/);
    assert.match(consumer, /SegmentedControl/);
  }
});
