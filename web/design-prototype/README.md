# CairnOps — atelier d’interface

Proposition interactive en Svelte 5, données de démonstration, aucun accès réseau à CairnOps. Le code est séparé du bundle de production. Il sert à choisir une direction avant son intégration dans les écrans réels.

## Lancer

Depuis ce dossier : `npm install`, puis `npm run dev`. Les dépendances Svelte/Vite proviennent de `web` (`npm ci --prefix web` à la racine si nécessaire). Adresse : http://127.0.0.1:5186. Cette commande compile puis sert la maquette ; après modification, `npm run build` actualise l’aperçu. Le prototype utilise trois compositions partageables par `?variant=A`, `B` ou `C`.

## Intention et question

Un opérateur vient comprendre ce qui nécessite une action, comparer les preuves et prendre en charge un incident. L’interface doit être calme, précise, immédiatement lisible. Question : quelle place donner à la synthèse, aux graphiques et aux incidents tout en gardant les urgences visibles ?

- Domaine : Cible, Source de signal, Incident, Preuve, Couverture, Disponibilité, Acquittement.
- Couleurs : Titane sombre et clair, accent minéral de la marque, vert des états établis, ambre des avertissements, rouge des indisponibilités, bleu des informations contextuelles.
- Signature : passer du verdict aux preuves dans le même axe de lecture ; chaque nombre rappelle son périmètre et sa fraîcheur.
- Alternatives aux automatismes : les métriques ont une fonction opérationnelle ; les graphiques restent contextuels ; les incidents conservent leur état après Acquittement.
- Composition A : synthèse, puis graphique et incidents côte à côte. Recommandée.
- Composition B : incidents en premier, puis les tendances. Pour une utilisation pendant une intervention.
- Composition C : analyse en grand, synthèse compacte et liste en dessous. Pour explorer les données.

## Finition proposée

Identité Convergence · Titane retenue par Grégory. Le symbole et le mot-symbole vectorisés proviennent du kit final ; les 30 icônes originales utilisent un trait de 2 et des angles arrondis. Les couleurs, surfaces et bordures suivent les jetons communs de l’application, sans surcharge de palette dans la maquette. Typographie système, chiffres tabulaires, titres 28 px, texte 13/14 px, contours fins et rayons de 6/8 px. Davantage d’air entre les groupes, alignements communs à l’intérieur. Les variantes de disposition et les espacements sont des propositions à reporter dans DESIGN-DIRECTION.md après choix.

Thèmes Clair, Sombre, Système et Solaire. Aucun emplacement déduit du seul fuseau horaire : choix explicite d’une ville. Calcul SunCalc local, heures affichées dans le fuseau de l’appareil, prise en compte des jours et nuits polaires, suivi des changements système et du retour au premier plan. Préférences gardées seulement en mémoire dans cette maquette.

## Vidéo pour le site de présentation

Le film est destiné au futur site de présentation de la solution. Son aperçu possède une entrée indépendante (`film.html`) ; aucun bouton ni lecteur vidéo ne figure dans l’interface CairnOps.

Premier film de direction : format 16:9, textes français, sans voix off. Marque → situation opérationnelle → courbe et preuves → Acquittement → clair/sombre → composants Svelte et vérifications → marque. L’utilisateur a choisi une présentation avec les coulisses du développement. Les données et le produit filmé sont ceux du prototype. L’export de présentation définitif doit être refait avec l’interface intégrée et validée.

## Suite d’intégration

1. Choisir la composition et ajuster le specimen ; la marque Convergence · Titane est déjà choisie.
2. Reporter les choix de composition et de densité dans les jetons et la direction visuelle.
3. Intégrer la coque, les thèmes, les graphiques et le détail avec les données réelles, en conservant les contrats et la sémantique métier.
4. Vérifier les états vides, erreurs, données périmées, clavier, mouvements réduits et petits écrans, en clair/sombre.
5. Exécuter les validations du dépôt, intégrer et observer le déploiement ; vérifier l’interface dans le navigateur externe connecté.
6. Produire le film définitif pour le site de présentation à partir du produit vérifié.

Références : https://shadcn-svelte.com/examples/dashboard ; https://github.com/mourner/suncalc ; docs/DESIGN-DIRECTION.md ; ADR 0012 et 0035 (indicateurs).

## État et vérification de cette proposition

Première phase de la refonte, sur `codex/dashboard-design`. La marque, les jetons Titane, les favicons et les icônes sont repris dans les composants Web partagés de cette branche ; les écrans métier conservent leur composition actuelle. La composition reste à choisir avant de reporter la proposition dans les écrans métier. Le prototype est une entrée indépendante de l’application, avec ses propres fixtures ; il ne constitue pas une intégration de la refonte en production.

Vérifications : trois compositions en clair/sombre ; largeurs 320, 390, 768, 1024 et 1440 px ; graphique au clavier ; filtre, recherche, vide, pagination ; détail et retour du focus ; Acquittement sans Résolution ; changement système en direct ; solaire jour/nuit et jours/nuits polaires. Compilation Svelte sans erreur ni avertissement. Les contrôles web et Go du dépôt ont aussi été exécutés.

Le film dure 42,5 secondes : MP4 H.264, 1920 × 1080, 30 images/s, sans piste audio. `render-film.py` expose `--preview` pour les sept plans fixes. Les scripts Python nécessitent Playwright ; FFmpeg doit être disponible pour l’export. `CAIRNOPS_DESIGN_OUTPUT` permet de choisir le dossier de sortie. L’aperçu animé séparé est accessible par `/film.html` et respecte la préférence de réduction des mouvements en attendant une lecture explicite.

Les sources de la maquette, les captures et le MP4 sont conservés comme éléments de travail. La prochaine phase porte sur les jetons communs, les écrans réels et les parcours complets. La branche reste active tant que cette intégration n’est pas terminée.
