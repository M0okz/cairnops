# CairnOps — identité Convergence / Titane

## Décision retenue

Gregory a choisi le symbole Convergence et le coloris Titane, issu de la famille Graphite. Les anciennes pistes, le cuivre et le cairn en pierres empilées sont abandonnés pour la marque.

## Logo

Le signe comporte exactement trois tracés. Ne pas les arrondir, les réarranger ni modifier leurs proportions. L’arrondi concerne les icônes système et les masques des icônes d’application.
Employer les versions vectorisées de logos/svg : elles sont indépendantes des polices installées. Les sources avec texte éditable restent dans logos/editables.
Conserver un dégagement d’au moins 16 unités autour du signe dans son cadre 128. Largeur recommandée du signe : 24 px ; minimum contrôlé : 16 px. Pour le logo horizontal, viser 140 px de largeur ou plus ; en dessous, utiliser le signe seul.
Ne pas déformer, ajouter de contour, de dégradé ou d’ombre au signe. La version blanche sert aux fonds pleins suffisamment contrastés.

## Palette sélectionnée

| Rôle | Clair | Sombre |
| --- | --- | --- |
| Fond | #FAF8F4 | #1C1B19 |
| Texte principal | #1C1B19 | #FAF8F4 |
| Accent de marque | #58534C | #CCC6BC |
| Texte sur accent | #FFFFFF | #1C1B19 |
| Texte secondaire | #706A61 | #BBC3CC |

Les cinq couleurs principales retenues sont #1C1B19, #FAF8F4, #58534C, #CCC6BC et #706A61. Les fichiers tokens reprennent les valeurs de la proposition choisie.
Les nuances de surfaces, bordures et états interactifs doivent être déclinées par la refonte dans son système de jetons ; ce kit ne fige pas la composition des écrans.
La santé, la gravité, la maintenance et l’information gardent des couleurs sémantiques distinctes et des libellés explicites. Le coloris de marque ne remplace pas ces catégories.

## Typographie

Le mot-symbole vectorisé correspond à Helvetica Neue Medium, avec l’espacement de la proposition retenue. Il ne nécessite aucune police chargée à l’exécution.
Pour les interfaces, conserver la typographie système et les chiffres tabulaires existants. Le choix du logo n’impose pas une nouvelle police à l’application.

## Icônes système

Les 30 icônes sur mesure ont une grille 24 × 24, un trait 2, des extrémités et jointures arrondies. Elles sont livrées en SVG, sprite et PNG 24/48/72 px. Elles doivent rester monochromes et hériter de la couleur du contexte.
Le cadrage et le dessin ont été contrôlés aux tailles 16, 20, 24 et 32 px. La taille recommandée pour les icônes d’action est 20–24 px ; leur zone d’interaction appartient au composant.
Ne pas confondre la nouvelle icône Cibles avec un nouvel état : elle conserve sa fonction de navigation.

## Icône d’application

Référence principale : fond sombre #1C1B19 et symbole #CCC6BC. Les versions claire et sur fond couleur restent disponibles pour les surfaces qui les nécessitent.
Les images source sont carrées, opaques et de 1024 × 1024 px. Le masque arrondi est appliqué par la plateforme. Les exports web comprennent favicon, icône tactile et formats 192/512 px.
