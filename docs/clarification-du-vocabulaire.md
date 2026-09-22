# Clarification du vocabulaire CairnOps

Décisions validées lors de l’entretien, appliquées à l’interface Web et au contrat de ressources. Les identifiants techniques des routes et champs historiques restent compatibles avec les clients existants.

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

## Règles de réalisation

La catégorie proposée provient des faits structurés : type de contrôle réseau, type d’objet Proxmox, interfaces Zabbix ou informations de machine PatchMon. Un heartbeat seul ne prouve pas une tâche planifiée : ce cas reste à classer. Un nom ressemblant à un serveur ne constitue pas une preuve de catégorie. Plusieurs catégories opérationnelles incompatibles produisent « À classer » ; le suivi logiciel enrichit la catégorie opérationnelle lorsqu’elle est connue.

La correction manuelle est persistante et prioritaire sur les découvertes suivantes. Les versions sont actualisées depuis le suivi logiciel existant. La dernière réussite d’une tâche vient des observations saines conservées de ses contrôles heartbeat ; elle reste absente si aucune preuve correspondante n’est disponible.

La disponibilité est établie par les contrôles dédiés, actifs et récents. La gravité seule ne rend plus une ressource indisponible. Une preuve ancienne ou un contrôle suspendu ne peut pas établir un état disponible. Le calcul historique des pourcentages reste fondé sur les observations ; l’aide du « Temps observé » précise cette estimation selon les cadences attendues.

Les messages connus utilisent la présentation structurée des connecteurs. Un message non reconnu conserve son texte original. Les incidents et notifications existants ne sont ni supprimés ni reclassifiés par cette évolution de présentation.

## Validation

Tests de catégories proposées et corrigées manuellement avec PostgreSQL isolé ; tests de séparation entre disponibilité, problèmes et mises à jour ; tests de preuves contradictoires et périmètre des ressources. Vérification visuelle locale sur le même scénario avant/après, clair et sombre, de 320 à 1 920 px, avec ouverture et fermeture de l’infobulle et filtrage par catégorie.
