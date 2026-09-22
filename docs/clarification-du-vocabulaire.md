# Clarification du vocabulaire CairnOps

Session de conception en cours. Ces décisions sont consignées ; leur application dans le produit reste à réaliser après la synthèse de l’entretien.

## Décisions validées

- « Ressources » remplace « Cibles ».
- « Disponibilité sur 24 h » et « Temps observé » explicitent les deux valeurs auparavant présentées sous « Dispo. · Couv. 24 h ». « Temps observé » remplace « Couverture » et désigne la part de la période pour laquelle les preuves permettent de conclure, sans assimiler les périodes sans données à de la disponibilité.
- « Problèmes détectés » remplace « Nature · Gravité » comme intitulé de colonne ; la gravité accompagne chaque description précise.
- « État de CairnOps » remplace « Santé » pour le fonctionnement de CairnOps lui-même.
- « Doublons possibles » remplace « Rapprochements » pour les propositions concernant des ressources potentiellement identiques.
- « Contrôles » remplace « Sources » pour les vérifications d’une ressource. « Connecteurs » reste le nom des adaptateurs vers les outils externes.
- Une mise à jour disponible est présentée séparément et ne dégrade pas le fonctionnement d’un service disponible.
- Le problème précis est présenté en priorité lorsque les observations permettent de l’établir. « Problème signalé » remplace « En défaut » comme libellé de repli.
- « Résultats contradictoires » est réservé aux conclusions opposées sur un même aspect pour une même période. Un accès réussi et une alerte de version ne constituent pas à eux seuls une contradiction.
- En présence de plusieurs problèmes, la liste affiche le plus grave et « +N autre(s) problème(s) ». Le compteur ouvre une infobulle détaillant les problèmes supplémentaires et leur origine ; elle est accessible au survol, au clavier et au toucher. Les mises à jour restent séparées.

## Catégorisation validée

Les ressources sont organisées en quatre catégories :

| Catégorie | Exemples | Informations principales |
| --- | --- | --- |
| Services | Home Assistant, Nextcloud | Accès, temps de réponse, problèmes |
| Infrastructure | Serveurs, VM, équipements réseau, stockage | Fonctionnement, capacité, problèmes |
| Tâches planifiées | Sauvegardes, synchronisations | Dernière réussite, échecs, retards |
| Logiciels suivis | Projet suivi uniquement pour ses versions | Versions, mises à jour disponibles |

Une ressource conserve une fiche unique regroupant ses contrôles. Le suivi d’une version par Argus ne crée pas une seconde fiche pour un service déjà supervisé.

CairnOps propose une catégorie à partir des informations importées, avec correction manuelle possible. Le connecteur seul ne détermine pas la catégorie : un même outil peut superviser plusieurs types de ressources. Lorsque les informations ne permettent pas de conclure, la ressource reste « À classer ».

## Organisation de la liste validée

La page Ressources propose les onglets « Toutes », « Services », « Infrastructure », « Tâches planifiées », « Logiciels suivis » et « À classer », chacun accompagné de son compteur.

Les catégories présentent les informations pertinentes pour leur objet : accès et temps de réponse pour les services, fonctionnement et capacité pour l’infrastructure, dernière réussite et retards pour les tâches planifiées, versions pour les logiciels suivis. L’onglet « Toutes » conserve une vue d’ensemble et indique la catégorie sur chaque ligne.

## Points à finaliser

- Termes restants de navigation et de présentation.
- Synthèse des décisions avant toute modification du comportement de l’application.
- Harmonisation des références historiques à « Source de signal » dans le glossaire et les documents concernés.
