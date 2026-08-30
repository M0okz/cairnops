import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const page = readFileSync(
  new URL('../routes/cibles/+page.svelte', import.meta.url),
  'utf8'
);

test('la grille des Cibles répartit la largeur sans étirer seule le nom', () => {
  const declaration = page.match(/\.cols\s*\{[^}]*--cols:\s*([^;]+);/s);

  assert.ok(declaration, 'Déclaration --cols introuvable pour la liste des Cibles');
  assert.match(
    declaration[1],
    /^minmax\(0,\s*24rem\)/,
    'la colonne Cible doit être plafonnée sans empêcher la grille de se contracter'
  );
  assert.ok(
    (declaration[1].match(/fr\b/g) ?? []).length >= 2,
    'la largeur restante doit être répartie entre plusieurs colonnes utiles'
  );
});
