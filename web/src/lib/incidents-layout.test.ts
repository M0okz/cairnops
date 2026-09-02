import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const page = readFileSync(
  new URL('../routes/incidents/+page.svelte', import.meta.url),
  'utf8'
);
const styles = readFileSync(new URL('../styles/app.css', import.meta.url), 'utf8');
const french = readFileSync(new URL('./locales/fr.ts', import.meta.url), 'utf8');
const english = readFileSync(new URL('./locales/en.ts', import.meta.url), 'utf8');

test('la grille des Incidents réserve la même colonne aux actions dans chaque ligne', () => {
  const declaration = page.match(/\.cols\s*\{[^}]*--cols:\s*([^;]+);/s);

  assert.ok(declaration, 'Déclaration --cols introuvable pour la liste des Incidents');
  assert.match(
    declaration[1],
    /var\(--table-action-width\)\s*$/,
    "la colonne d’action doit garder une largeur stable entre l’en-tête vide et les lignes"
  );
  assert.match(
    styles,
    /--table-action-width:\s*[\d.]+rem;/,
    "la largeur de la colonne d’action doit venir des jetons de l’interface"
  );
  assert.match(
    page,
    /\.cols\s+:global\(\.trow\s*>\s*\.btn:last-child\)\s*\{[^}]*justify-self:\s*end;/s,
    "l’action doit rester alignée sur le bord final de la table"
  );
  assert.match(
    page,
    /@media\s*\(max-width:\s*48rem\)[\s\S]*?\.cols\s+:global\(\.trow\s*>\s*\.btn:last-child\)\s*\{[^}]*justify-self:\s*auto;/s,
    "le repli mobile doit conserver le placement naturel de l’action"
  );
});

test('le ratio des Sources explique son numérateur et son dénominateur', () => {
  assert.match(
    page,
    /t\('incidents\.column\.activeSources'\)/,
    "l’en-tête ne doit pas présenter un ratio ambigu sous le seul mot Sources"
  );
  assert.match(
    page,
    /aria-label=\{t\('incidents\.sourcesRatio', \{ ratio: activeSignalRatio\(incident\) \}\)\}/,
    'chaque ratio doit porter une explication accessible'
  );
  assert.match(french, /'incidents\.column\.activeSources': 'Actives \/ total'/);
  assert.match(english, /'incidents\.column\.activeSources': 'Active \/ total'/);
});

test('la largeur intermédiaire réserve de la place à la Cible', () => {
  assert.match(
    page,
    /@media\s*\(max-width:\s*68rem\)[\s\S]*?\.cols\s*\{[^}]*--cols:\s*minmax\(0,\s*1fr\)[^;}]*;/,
    'la grille intermédiaire doit donner toute la largeur restante à la Cible'
  );
  assert.match(
    page,
    /@media\s*\(max-width:\s*68rem\)[\s\S]*?\.cols\s+:global\(\.log\)\s*\{[^}]*display:\s*none;/,
    'le Journal doit se masquer avant de comprimer le nom de la Cible'
  );
});
