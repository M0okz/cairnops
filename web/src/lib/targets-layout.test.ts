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

/* La colonne « État » porte le libellé d'un État de santé. Le plus long,
 * « Fonctionnement dégradé », contient un mot insécable de 14 caractères qui
 * débordait sur « Problèmes détectés » — comme « Indisponible » le faisait
 * déjà discrètement avant que « Fonctionnement dégradé » ne devienne
 * atteignable. La colonne demande donc min-content : elle ne descend jamais
 * sous son mot le plus long, quelle que soit la langue ou l'échelle de
 * lecture, là où une largeur en rem aurait été un nombre à réviser. */
test("la colonne État ne descend jamais sous son libellé le plus long", () => {
  const declarations = [
    ...[...page.matchAll(/--cols:\s*([^;]+);/g)].map((match) => match[1]),
    ...[...page.matchAll(/grid-template-columns:\s*([^;]+);/g)].map((match) => match[1])
  ];
  assert.ok(declarations.length >= 3, `trois paliers attendus, ${declarations.length} trouvés`);

  for (const declaration of declarations) {
    const columns = declaration.split(/\)\s+/).map((part) => (part.endsWith(')') ? part : `${part})`));
    assert.match(
      columns[1] ?? '',
      /minmax\(\s*min-content\s*,/,
      `la colonne État doit demander min-content pour ne pas rogner son libellé : ${declaration}`
    );
  }

  /* break-word et non anywhere : anywhere ramènerait la largeur min-content de
   * la pastille à un seul caractère, ce qui annulerait la protection que la
   * colonne vient de demander. Les deux correctifs se neutraliseraient. */
  const pill = page.match(/\.pill\s*\{([^}]*)\}/)?.[1] ?? '';
  assert.match(pill, /overflow-wrap:\s*break-word/,
    'la pastille doit rompre un mot pathologique en dernier recours');
  assert.doesNotMatch(pill, /overflow-wrap:\s*anywhere/,
    'anywhere annulerait la largeur min-content de la colonne');
});
