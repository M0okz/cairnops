# Direction visuelle de CairnOps

CairnOps doit inspirer confiance avant de chercher à impressionner. Son interface est fonctionnelle, précise et soigneusement finie ; le mouvement agrémente la navigation et confirme les actions sans détourner l'attention de l'état opérationnel.

L’identité retenue est **Convergence · Titane**, décrite dans [le guide de marque](brand/BRAND-GUIDE.md). Elle remplace les pistes cuivre et Framboise et le cairn en pierres empilées. La **composition A** de `web/design-prototype` et son échelle de lecture ont été retenues pour l’application : synthèse, analyse et incidents côte à côte, puis Cibles. Les autres compositions restent des explorations dans l’atelier. Les jetons de `web/src/styles/app.css` portent la traduction commune aux écrans et au prototype.

## Identité

- Le symbole Convergence représente trois signaux orientés vers un même état. Ses trois formes restent angulaires, avec leurs proportions et leur dégagement d’origine. Le composant `Brand.svelte` reprend le signe et le mot-symbole vectorisés du kit, sans dépendre d’une police installée. Le rail porte le mot-symbole CairnOps et distingue le nom de l’instance dans un cartouche ; la connexion associe le signe au nom de l’instance.
- Fond Titane sombre `#1C1B19`, fond clair `#FAF8F4`, texte principal inversé entre les deux. Les surfaces et bordures déclinent ces neutres dans les jetons communs, avec la même grille et la même échelle de lecture dans les deux thèmes.
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

La Vue d’ensemble garde les exceptions prioritaires : verdict et fraîcheur, quatre métriques issues de la projection réelle, puis grande courbe contextuelle et incidents à traiter. Les six premières Cibles sont triées par état, avec un filtre et un lien vers la liste complète. La Santé de l’instance reste accessible dans le volet et en bas de page. Une supervision vide ou partiellement inconnue ne donne jamais un verdict global opérationnel.

Les métriques globales portent sur les Cibles opérationnelles, la Couverture pondérée par les Observations attendues sur 24 heures, les Incidents actifs et les Sources configurées. La latence médiane du prototype est exclue : les agrégats réels ne permettent pas de la calculer. Les indicateurs choisis dans la vue d’ensemble se consultent sur 1 heure, 24 heures ou 7 jours, avec leur provenance, leur fraîcheur et les erreurs de collecte. La première heure filtre les relevés détaillés selon l’heure de la projection serveur. Leur sélection personnelle reste disponible.

### Graphiques contextuels

La carte reprend le [graphique interactif du dashboard shadcn-svelte](https://github.com/huntabyte/shadcn-svelte/blob/main/docs/src/lib/registry/blocks/dashboard-01/components/chart-area-interactive.svelte) : arrondi de 16 px, commandes segmentées, grille horizontale continue, axes temporels adaptatifs, traits neutres de 1 px et aires en dégradé. Une infobulle compacte réunit la date et les valeurs, avec leurs repères sur les courbes. Les couleurs et la typographie suivent les jetons Titane dans les deux thèmes.

La Vue d’ensemble, les Indicateurs des Cibles et les courbes du détail d’Incident utilisent le même composant. Chaque Indicateur de Cible dispose de sa carte et partage la sélection de période 1 h / 24 h / 7 jours. Les métadonnées restent lisibles, les épingles personnelles et la provenance restent accessibles. Dans un Incident, la courbe compacte garde une fenêtre fixe de deux heures avant et après l’ouverture ; les valeurs capturées à l’ouverture restent distinctes des relevés du graphique. Le repère d’ouverture se place à son instant exact, même dans une interruption de collecte, sans inventer une mesure sur la courbe.

Les courbes restent des projections des données réelles. Sur sept jours, `value` est la **dernière valeur de l’heure**, jamais sa moyenne ; `maximum`, lorsqu’il existe, fournit la seconde série. Ces deux valeurs se superposent sans être additionnées. Les absences de collecte restent des interruptions, un maximum absent reste absent, et les booléens se dessinent par paliers. Le domaine temporel conserve les bornes de la période même lorsque la collecte est partielle.

Le tracé s’interpole en 400 ms à l’arrivée des données et au changement de période. Le mouvement est interruptible ; le survol ou le clavier donnent immédiatement les valeurs exactes. Redimensionnement, booléens et réduction des animations désactivent l’interpolation. Les chiffres annoncés ne sont jamais interpolés. Cette animation demandée pour le graphique est une exception ciblée à l’absence d’animation d’entrée générale.

Les infobulles utilisent les composants officiels `Chart.Container` et `Chart.Tooltip` de shadcn-svelte, copiés avec leur licence dans `components/ui/chart`, et le contexte manuel de LayerChart. Le même rendu couvre les Indicateurs de la Vue d’ensemble, des Cibles et des Incidents. Le composant officiel conserve sa disposition, ses repères et ses ombres ; les unités localisées et les jetons Titane sont fournis par CairnOps. Les infobulles restent dans le graphique, y compris dans le volet d’Incident, avec déplacement court et réduction des animations respectée. Les utilitaires Tailwind sont limités aux composants de graphique, sans reset global. Les couleurs dynamiques passent par les directives Svelte `style:` et le CSSOM ; la politique CSP reste inchangée.

### Chronologie du Journal

Le Journal des Incidents et des Cibles partage une chronologie : date une fois par journée dans le fuseau du lecteur, heure alignée, icône dans un repère circulaire et ligne verticale entre les repères. Les messages et les auteurs sont conservés, y compris les faits simultanés. Les événements se lisent du plus récent au plus ancien ; les événements administratifs des Cibles participent au même ordre chronologique. Sur petit écran, l’heure passe au-dessus du message. Le vert signale uniquement un rétablissement ; acquittement, invalidation et fermeture de propagation gardent des repères neutres distincts. Une preuve restaurée est explicitement identifiée sous son message d’origine.

## Densité adaptative

- Bureau au-delà de 68 rem : échelle de lecture à 125 % de la police par défaut du navigateur, soit une base de 20 px avec le réglage usuel. Rail de 330 px, barre supérieure de 80 px, gouttières de 50 px et contrôles de 50–55 px. Commandes et noms de Cibles à 17,5 px, métadonnées à 15–16,25 px, titres à 40/22,5 px et métriques à 50 px. Le contenu occupe la largeur disponible jusqu’à 2400 px. Cette échelle répond à la lecture trop petite à 100 % sur le MacBook ; elle ne dépend ni du modèle d’écran ni de son ratio de pixels. Le zoom et la police par défaut du navigateur restent respectés.
- Fenêtres de 68 rem ou moins : base à 100 % de la police du navigateur, soit 16 px par défaut, avec les contrôles de 40–44 px et le repli mobile existants.
- Sous 85 rem : volet compact de 5 rem avec icônes et noms accessibles ; la recherche conserve son déclencheur. Sous 100 rem, les Incidents masquent la colonne du Journal pour réserver de la place aux Cibles, et les cartes de synthèse se replient. Sous 48 rem, identité et compte passent en haut, et toutes les routes restent accessibles dans une navigation horizontale défilante. Les contrôles se replient avant de déborder.
- Les listes des Cibles et Incidents se replient selon la largeur de leur table : sous 44 rem, l’en-tête disparaît, les lignes passent sur deux colonnes et les informations secondaires restent dans le détail. Les Cibles masquent déjà Nature et Tendance sous 65 rem pour préserver leur nom. Le repli mobile général demeure sous 48 rem.
- La densité est unique et assumée. L'ancien réglage Confortable/Compact est retiré : deux densités concurrentes empêchaient de régler la seule qui compte.

## Apparence et mises à jour

Clair, Sombre, Système et Auto partagent les mêmes jetons. Système suit les changements de l’appareil. Auto calcule le lever et le coucher du soleil pour une ville explicitement choisie, affiche les heures dans le fuseau de l’appareil et gère les jours et nuits polaires. Un fuseau ne suffit pas à localiser l’utilisateur ; sans ville valide, Auto suit le système. Les préférences restent locales, survivent au rechargement et se synchronisent entre les onglets. Elles sont disponibles dans la barre supérieure, le compte et les Réglages.

La mise à jour disponible est signalée dans le volet, sous le logo. Le bouton recharge l’application pour lire la version déjà servie ; il ne déclenche aucune installation ni aucun redéploiement.

## Ligne de Cible

La liste principale affiche le nom, l'État de santé, la Nature et la Gravité du principal Incident actif, la dernière Observation et sa fraîcheur, la latence lorsque pertinente, la disponibilité sur 24 heures, le nombre de Sources, une éventuelle contradiction et une mini-tendance. Les adresses, preuves détaillées par Source et historiques complets restent dans le détail.

## Ligne d'Incident

Une ligne d'Incident affiche sa Nature, sa plus forte Gravité effective active, l'état d'Acquittement, l'heure de début et la durée, le nombre d'Atteintes actives sur le total, le nombre de Cibles distinctes et une éventuelle Propagation étendue. Son développement révèle les Atteintes dans une structure comparable : Cible, Gravité, instants, nombre de Preuves actives, contradictions ou données manquantes et dernière transition significative.

Le détail explique à la demande pourquoi les Atteintes partagent l'Incident, dans une phrase lisible fondée sur l'identité de Nature et l'intervalle observé. Il n'affiche ni score de confiance, ni réglage, ni action de fusion ou séparation, et aucun code couleur ni vocabulaire ne suggère une cause commune. Le détail d'une Cible montre les Incidents qui l'affectent en donnant la priorité à son Atteinte ; la boîte intégrée et le Push conservent une entrée unique par Incident, actualisée silencieusement hors nouveau Fait opérationnel.

### Volet de détail d’Incident

Depuis la liste ou la Vue d’ensemble, le détail s’ouvre à droite dans un volet de 52 rem au maximum, sur toute la hauteur disponible. La liste reste visible derrière un voile discret. Sur mobile, le volet remplit l’écran. Son en-tête et ses actions restent visibles ; seule la zone de contenu défile. Dates, preuves et chronologie se replient selon la largeur du volet.

L’ouverture et la fermeture utilisent des transitions courtes et interruptibles ; la réduction des animations les supprime. Le dialogue natif conserve l’arrière-plan inerte, le focus dans le volet et le retour au déclencheur. Échap ferme d’abord une infobulle ou un formulaire d’Invalidation actif, puis le volet. Le lien direct d’un Incident et les actions métier restent identiques.

## Chronologie d'Incident

La chronologie fusionne les origines mais affiche d'abord les transitions significatives : ouverture, arrivée ou rétablissement d'une Atteinte, contradiction, Acquittement, requalification, Invalidation, maintenance, fermeture de la Propagation et Résolution. Les Observations brutes restent accessibles à la demande, regroupées par Source et condensées sur les périodes stables, avec une origine CairnOps, Connecteur ou humaine toujours explicite.

Visuellement, la chronologie s'appuie sur un filet vertical discret le long duquel les entrées s'alignent. Les preuves actives sont présentées en lignes comparables et conservent chacune leur verdict, leur fraîcheur et leur origine ; la divergence reste visible sans produire un nouvel état.

## Connexion guidée d'un Connecteur

Le parcours suit trois strates stables — Adresse, Autorisation, Aperçu — et montre les vérifications effectuées par CairnOps avant l'import. L'utilisateur voit le niveau d'accès, la compatibilité et les Cibles découvertes sans devoir comprendre le format des événements ou configurer un mapping. Toute écriture éventuelle vers l'outil externe est annoncée avant confirmation.

## Gabarit des notifications

Les notifications complètes partagent deux lignes : le problème en titre, puis
« Cible · gravité ». Les quatre libellés courts sont « information »,
« avertissement », « majeur » et « critique ». Une notification concernant
plusieurs Cibles affiche leur nombre à la place d'un nom individuel.

Le Push, la boîte intégrée et Mattermost utilisent le même rendu de
`internal/synthesis`. Les titres de notification omettent le préambule
« Signalement : ». Un sens vérifié à partir des faits du Connecteur peut être traduit et abrégé, comme
« Charge système moyenne élevée » ; les autres sont conservés sur une ligne, bornée à
80 caractères. Le détail conserve le texte original attribué à sa Source.

Le [lexique de supervision](notification-lexicon.md) fixe les termes connus :
« disque » pour les accès et l’espace disque, « CPU » pour son utilisation,
« charge système moyenne » pour le load average. Les noms de produits, les
sigles usuels et les messages inconnus ne sont pas traduits à l’aveugle.

La Résolution reste explicite dans le titre et conserve le contexte des Cibles
concernées. Les modes discret et masqué gardent leurs messages confidentiels.

## Association d'un appareil

Le QR code occupe une dalle dédiée et reste accompagné de son expiration et de sa portée. Les trois confirmations — scan, authentification navigateur, confirmation Web — sont décrites séparément afin que la simplicité du parcours ne masque jamais son modèle de sécurité. La liste des appareils rappelle que chaque identité est individuelle et révocable.

## Divergence de Sources

Une Divergence de Sources ne crée pas un cinquième État de santé. Une pastille secondaire et un libellé explicite signalent le désaccord sur la Cible et l'Incident, tandis que le détail nomme les conclusions de chaque Source ; l'indication disparaît automatiquement lorsque les preuves convergent.
