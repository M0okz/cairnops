import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const component = readFileSync(
  new URL('./components/IndicatorConfigurator.svelte', import.meta.url),
  'utf8'
);

function declarationsFor(selector: string) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const rule = component.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`));

  assert.ok(rule, `Règle CSS introuvable pour ${selector}`);
  return rule[1];
}

test('la navigation des Indicateurs garde toute sa hauteur dans la modale', () => {
  const navigation = declarationsFor('.config-nav');
  const body = declarationsFor('.modal-body');

  assert.match(navigation, /(?:^|;)\s*flex\s*:\s*none\s*(?:;|$)/);
  assert.match(body, /(?:^|;)\s*flex\s*:\s*1(?:\s+1\s+auto)?\s*(?:;|$)/);
});
