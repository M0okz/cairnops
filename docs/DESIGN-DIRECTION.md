# Direction visuelle de CairnOps

CairnOps doit inspirer confiance avant de chercher à impressionner. Son interface est fonctionnelle, précise et soigneusement finie ; le mouvement agrémente la navigation et confirme les actions sans détourner l'attention de l'état opérationnel.

L’identité retenue est **Convergence · Titane**, décrite dans [le guide de marque](brand/BRAND-GUIDE.md). Elle remplace les pistes cuivre et Framboise et le cairn en pierres empilées. La grille des huit « Écrans » reste la base de l’application ; les compositions A/B/C de `web/design-prototype` sont encore à comparer. Le choix de marque ne choisit pas une composition. Les jetons de `web/src/styles/app.css` portent la traduction commune aux écrans et au prototype.

## Identité

- Le symbole Convergence représente trois signaux orientés vers un même état. Ses trois formes restent angulaires, avec leurs proportions et leur dégagement d’origine. Le composant `Brand.svelte` reprend le signe et le mot-symbole vectorisés du kit, sans dépendre d’une police installée. Le rail et la connexion associent le signe au nom de l’instance.
- Fond Titane sombre `#1C1B19`, fond clair `#FAF8F4`, texte principal inversé entre les deux. Les surfaces et bordures déclinent ces neutres dans les jetons communs, sans changer la grille ni la densité.
- Accent de marque `#CCC6BC` en sombre, `#58534C` en clair. Le texte sur accent utilise respectivement `#1C1B19` et `#FFFFFF`. Les textes secondaires suivent le kit : `#BBC3CC` en sombre et `#706A61` en clair. L’accent distingue l’action principale et la route courante.
- Vert, orange et rouge réservés à l'État de santé et à la Gravité. Le bleu signale l'information et la maintenance. Aucune couleur de Connecteur n'entre dans ce registre.
- Formes solides et discrètement arrondies : 4 px pour les micro-contrôles, 6 px pour les contrôles, 8 px pour les dalles, pastilles pleinement arrondies.
- Typographie `system-ui` pour toute l'interface : elle donne la densité et la neutralité attendues d'un poste de conduite, et se charge instantanément. Le monospace du système est réservé aux nombres — latences, disponibilités, durées, compteurs — toujours en chiffres tabulaires. Les capitales espacées et les libellés de type terminal ne servent jamais de décoration.
- Famille de 30 icônes originales : cadre 24, épaisseur 2, bouts et jointures arrondis, couleur héritée du contexte. Les 21 noms historiques de `Icon.svelte` sont conservés ; les neuf nouveaux noms permettent notamment de représenter les états et l’Acquittement. L’arrondi des icônes ne s’applique pas au signe Convergence. Les tailles compactes existantes restent disponibles, avec une cible de 20–24 px pour les actions de la refonte.
- Thèmes clair et sombre de qualité équivalente. Le sombre est la référence de conception ; le clair n'en est pas une dégradation.

Le film de présentation et de développement est destiné au futur site vitrine. Son entrée d’aperçu est indépendante ; il n’apparaît pas dans la navigation de CairnOps. Les ressources iOS du kit seront reprises dans le chantier natif, en conservant un seul AppIcon par cible.

### Signature : densité constante

La signature de CairnOps n'est pas un ornement, c'est la constance. Les huit écrans partagent la même grille, la même hauteur de barre supérieure, la même hauteur de contrôle et les mêmes colonnes lorsqu'ils montrent la même chose. Un opérateur qui passe de la liste des Cibles au journal des Incidents ne réapprend rien.

Cette constance se tient par un jeu de jetons unique : toute couleur, tout espacement et toute forme viennent de `web/src/styles/app.css`. Une valeur littérale dans un composant est un défaut, pas un raccourci.

L'exception vaut d'être nommée : les tendances sont rendues en SVG et non en barres CSS, parce que la politique de sécurité de l'instance ferme `style-src-attr`. Aucune hauteur ne peut transiter par un attribut `style`.

## Mouvement

- Priorité à la lisibilité, à la réactivité et à la confiance.
- Transitions courtes pour la navigation, l'insertion, le déplacement et la Résolution des éléments.
- Squash and Stretch réservé aux confirmations et retours d'action qui en bénéficient.
- Le mouvement se limite aux changements d'état réels : survol d'une ligne, apparition d'une notice, bascule d'un segment. Rien n'entre en scène par chorégraphie.
- Aucun mouvement continu sur les états normaux.
- Une animation ne retarde jamais un état critique ni une action.
- Les préférences système de réduction des animations sont respectées.

## Hiérarchie

La Vue d'ensemble est orientée exceptions : État global et fraîcheur, Incidents non acquittés puis acquittés, Défaillances de supervision, Cibles problématiques, puis résumé replié des Cibles Opérationnelles.

## Densité adaptative

- Desktop : rail fixe de 224 px, barre supérieure de 48 px portant le fil d'Ariane, la recherche et l'identité, puis un contenu en tables denses. Contrôles à 30 px, actions de tête à 32 px.
- Sous 68 rem : le rail bascule en barre horizontale défilante, l'espace de travail et le compteur de fraîcheur s'effacent, la recherche se réduit.
- Sous 48 rem : l'en-tête de table disparaît, chaque ligne se replie sur deux colonnes et les colonnes secondaires — Nature, Latence, Dispo. 24 h, Sources, Tendance — sont masquées plutôt que comprimées.
- La densité est unique et assumée. L'ancien réglage Confortable/Compact est retiré : deux densités concurrentes empêchaient de régler la seule qui compte.

## Ligne de Cible

La liste principale affiche le nom, l'État de santé, la Nature et la Gravité du principal Incident actif, la dernière Observation et sa fraîcheur, la latence lorsque pertinente, la disponibilité sur 24 heures, le nombre de Sources, une éventuelle contradiction et une mini-tendance. Les adresses, preuves détaillées par Source et historiques complets restent dans le détail.

## Ligne d'Incident

Une ligne d'Incident affiche sa Nature, sa plus forte Gravité effective active, l'état d'Acquittement, l'heure de début et la durée, le nombre d'Atteintes actives sur le total, le nombre de Cibles distinctes et une éventuelle Propagation étendue. Son développement révèle les Atteintes dans une structure comparable : Cible, Gravité, instants, nombre de Preuves actives, contradictions ou données manquantes et dernière transition significative.

Le détail explique à la demande pourquoi les Atteintes partagent l'Incident, dans une phrase lisible fondée sur l'identité de Nature et l'intervalle observé. Il n'affiche ni score de confiance, ni réglage, ni action de fusion ou séparation, et aucun code couleur ni vocabulaire ne suggère une cause commune. Le détail d'une Cible montre les Incidents qui l'affectent en donnant la priorité à son Atteinte ; la boîte intégrée et le Push conservent une entrée unique par Incident, actualisée silencieusement hors nouveau Fait opérationnel.

## Chronologie d'Incident

La chronologie fusionne les origines mais affiche d'abord les transitions significatives : ouverture, arrivée ou rétablissement d'une Atteinte, contradiction, Acquittement, requalification, Invalidation, maintenance, fermeture de la Propagation et Résolution. Les Observations brutes restent accessibles à la demande, regroupées par Source et condensées sur les périodes stables, avec une origine CairnOps, Connecteur ou humaine toujours explicite.

Visuellement, la chronologie s'appuie sur un filet vertical discret le long duquel les entrées s'alignent. Les preuves actives sont présentées en lignes comparables et conservent chacune leur verdict, leur fraîcheur et leur origine ; la divergence reste visible sans produire un nouvel état.

## Connexion guidée d'un Connecteur

Le parcours suit trois strates stables — Adresse, Autorisation, Aperçu — et montre les vérifications effectuées par CairnOps avant l'import. L'utilisateur voit le niveau d'accès, la compatibilité et les Cibles découvertes sans devoir comprendre le format des événements ou configurer un mapping. Toute écriture éventuelle vers l'outil externe est annoncée avant confirmation.

## Association d'un appareil

Le QR code occupe une dalle dédiée et reste accompagné de son expiration et de sa portée. Les trois confirmations — scan, authentification navigateur, confirmation Web — sont décrites séparément afin que la simplicité du parcours ne masque jamais son modèle de sécurité. La liste des appareils rappelle que chaque identité est individuelle et révocable.

## Divergence de Sources

Une Divergence de Sources ne crée pas un cinquième État de santé. Une pastille secondaire et un libellé explicite signalent le désaccord sur la Cible et l'Incident, tandis que le détail nomme les conclusions de chaque Source ; l'indication disparaît automatiquement lorsque les preuves convergent.
