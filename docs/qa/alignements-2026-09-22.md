# Passe d’alignement du 22 septembre 2026

Périmètre : interface web CairnOps, à partir de `2239548` (v0.1.113).
Les arrondis de la Vue d’ensemble et les cartes d’incidents sont conservés.
Aucun contrat HTTP ni comportement de supervision n’est modifié.

## Défauts reproduits et corrections

| Zone | Reproduction | Correction |
| --- | --- | --- |
| Bouton de mise à jour | À 1920 px, bord gauche à 22,5 px contre 12,5 px pour le rail ; largeur propre au texte | Mêmes bords que les cartes et la navigation ; icône centrée en rail compact |
| Rail desktop | Compte à 1003,875 px dans une fenêtre de 900 px de haut | Navigation défilante, compte et identité préservés |
| Bandeau mobile | Hauteur étirée par le contenu et les lignes implicites de la grille | Première ligne de grille à la hauteur de son contenu |
| Réglages | Apparence dans la colonne centrale (bord droit 1321 px), autres actions à 1849 px ; pastilles coupées à 320 px | Colonne d’action commune ; pastilles avec retour à la ligne |
| Santé | Compteurs et statuts se chevauchent sur téléphone | Grille explicite ; informations secondaires masquées, statut sous le nom |
| Incidents et maintenance | Boutons étirés selon la largeur du badge précédent | Action sur une ligne complète, alignée au bord final ; repli selon la largeur disponible |
| Connecteurs | Métadonnées séparant les boutons sur différentes lignes | Groupe d’actions solidaire, avec retour à la ligne interne |
| Filtres | Bordures segmentées cassées après retour à la ligne | Segments indépendants avec espacement constant |
| Création de ressource, maintenance et langue des réglages | Anciens boutons natifs sans style, superposés sur téléphone | Réutilisation du sélecteur commun |
| Menus des ressources | Panneau collé au bord du viewport | Marge de collision de 16 px |
| Pieds de fenêtre | Boutons en escalier sur petit écran | Bords et largeur communs sous 480 px |
| Personnalisation | Recherche native non stylée ; Enregistrer coupé à 390 × 600 | Champ commun ; en-tête et pied non rétractables, catalogue défilant |

## Vérification reproductible

Voir [les scripts et commandes de QA](../../scripts/ui-qa/README.md).
Les scénarios utilisent des données synthétiques, sur le serveur local uniquement.
Ils sauvegardent des captures avant/après et des mesures de géométrie.

La campagne couvre 16 routes en clair/sombre à 1920, 1280, 768, 390 et 320 px,
et 19 états de menus, panneaux et formulaires à 1440, 768, 390 et 320 px.
Les compléments couvrent une hauteur de 600 px, les données vides, les erreurs,
les noms longs et les principaux écrans en anglais. Les pieds des fenêtres sont
contrôlés séparément pour détecter une action coupée même sans débordement de page.

Les captures locales sont classées dans
`/tmp/cairnops-alignment-evidence/` : `before`, `states-pass2`, `short-states`
pour les reproductions ; `release`, `states-verified`, `short`,
`short-states-after`, `empty`, `error`, `long`, `english` pour la validation.
Les captures de travail intermédiaires ne constituent pas une validation finale.

Les tests Web, le contrôle Svelte/TypeScript, le build, les tests Go,
`go vet` et `git diff --check` accompagnent cette revue.
Les tests métier des écritures et les connecteurs réels restent hors du périmètre
visuel de ces fixtures. Les contrôles post-publication doivent être faits dans
le navigateur externe de l’utilisateur, avec preuve du SHA servi et de la santé
des conteneurs ; une capture locale ne prouve pas le déploiement.

Le contrôle post-publication dans Opera a confirmé les tableaux mobiles, la
Santé en sombre et les actions de la personnalisation avec le catalogue réel.
Il a également révélé le dernier groupe natif `act segments` dans la langue
des Réglages : il utilise désormais le sélecteur commun. Le contrôle existant
des consommateurs couvre aussi les classes composées et ces trois formulaires.
