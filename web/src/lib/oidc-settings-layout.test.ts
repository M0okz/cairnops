import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const component = readFileSync(
  new URL('./components/OIDCSettings.svelte', import.meta.url),
  'utf8'
);
const french = readFileSync(new URL('./locales/fr.ts', import.meta.url), 'utf8');
const english = readFileSync(new URL('./locales/en.ts', import.meta.url), 'utf8');

test('le panneau OIDC porte sa propre structure sans dépendre de la page Réglages', () => {
  assert.match(component, /<section class="oidc-panel"/);
  assert.match(component, /class="provider-grid"/);
  assert.match(component, /class="role-grid"/);
  assert.match(component, /<details class="advanced-options">/);
  assert.doesNotMatch(component, /class="(?:band-row|row|grid)"/);
});

test('les groupes restent compacts et se replient proprement sur petit écran', () => {
  assert.equal(
    (component.match(/rows="2"/g) ?? []).length,
    3,
    'chaque rôle doit utiliser une zone compacte de deux lignes'
  );
  assert.match(
    component,
    /\.role-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\)/s
  );
  assert.match(
    component,
    /@media\s*\(max-width:\s*48rem\)[\s\S]*\.role-grid[\s\S]*grid-template-columns:\s*minmax\(0,\s*1fr\)/
  );
});

test('le parcours utilisateur ne parle plus de brouillon', () => {
  const oidcFrench = french.slice(french.indexOf("'oidc.title'"), french.indexOf("'devices.title'"));
  const oidcEnglish = english.slice(english.indexOf("'oidc.title'"), english.indexOf("'devices.title'"));

  assert.doesNotMatch(oidcFrench, /brouillon/i);
  assert.doesNotMatch(oidcEnglish, /draft/i);
});
