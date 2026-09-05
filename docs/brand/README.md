# Convergence · Titane

Identité finale choisie par Grégory le 5 septembre 2026, transmise depuis le kit `cairnops-convergence-titane-final`. Le [guide](BRAND-GUIDE.md) et les [rôles de couleur](titane.json) conservent la référence. Cette décision porte sur la marque et les icônes, pas sur les compositions A/B/C du tableau de bord.

Les SVG originaux utiles au Web sont copiés dans `web/static/brand`, et les favicons et icônes tactiles dans `web/static`. `Brand.svelte` conserve exactement les tracés vectorisés du signe et du mot-symbole ; `web/src/lib/brand/cairnops-icon-paths.ts` conserve les 30 pictogrammes du kit. Aucun asset d’exécution ne dépend du dossier de livraison externe.

Les couleurs du guide sont reportées dans `web/src/styles/app.css`, où les surfaces et bordures sont déclinées. Les fichiers de liaison CSS du kit ne sont pas importés : les composants et le prototype lisent les mêmes jetons. Les couleurs de santé, gravité et provenance conservent leurs rôles.

Le film est un livrable du futur site de présentation. Les ressources iOS sont conservées dans le kit source pour le chantier natif ; cette reprise Web ne modifie pas son catalogue d’assets.
