---
status: accepted
---

# Adapter la Propagation à la durée des conditions

Cette décision précise la fenêtre de l'ADR 0042 sans changer le cycle
Incident / Atteinte / Preuve ni la politique de notification de l'ADR 0044.

## Constat

La rafale du 6 septembre 2026 a produit cinq ouvertures de même Nature disque
sur douze minutes. Dix Cibles ont été concernées successivement, dont six
simultanément au maximum. La collecte Zabbix de trente secondes imposait une
Propagation de soixante secondes, alors que la règle évaluait les latences
sur quinze minutes. Les arrivées espacées et un rétablissement provisoire
séparaient ainsi un même épisode en plusieurs Incidents. Chaque ouverture
avait un seul envoi Push : le défaut précédait la livraison mobile.

Le nom de la fonction Zabbix manquait également dans les réponses, à cause
de [ZBX-23578](https://support.zabbix.com/browse/ZBX-23578). La condition
officielle restait donc locale au Connecteur et son titre générique.

## Décision

Les adapters peuvent fournir une `EvaluationWindow` dans le langage commun
des Preuves : la période vérifiée sur laquelle la Source évalue sa condition,
distincte de sa cadence de collecte. Zéro signifie inconnue. Le cycle choisit
le maximum entre cette période et deux cycles de collecte, puis le borne
entre soixante secondes et cinq minutes.

Chaque nouvelle Atteinte prolonge cette fenêtre à partir de son instant de
début normalisé. Une simple relecture de Preuve active ne la prolonge pas.
La plus longue fenêtre déjà établie reste applicable lorsqu'une Source plus
rapide rejoint l'Incident. Les instants historiques restent historiques ;
la réception tardive d'un lot ne réouvre pas une Propagation fermée.

L'adapter Zabbix demande les fonctions avec `selectFunctions: "extend"`,
compatible avec le défaut de projection. Il lit les périodes littérales des
agrégations `min`, `max`, `avg`, `sum` et `count` référencées dans l'expression
problème du trigger actif. Il ignore les fonctions de rétablissement seules,
les macros non résolues, les nombres d'échantillons et les périodes décalées
dans le passé. Aucun nombre issu du titre n'est utilisé. La reconnaissance
de la Nature canonique conserve ses vérifications d'identité et de condition.

La règle Linux de latence sur quinze minutes obtient ainsi une fenêtre de
cinq minutes, tout en restant collectée toutes les trente secondes. La
documentation de [trigger.get](https://www.zabbix.com/documentation/7.4/en/manual/api/reference/trigger/get)
décrit la projection des fonctions ; le
[template Linux actif officiel](https://github.com/zabbix/zabbix/blob/release/7.0/templates/os/linux_active/template_os_linux_active.yaml)
définit la condition vérifiée.

## Conséquences et vérification

L'ouverture non critique reste soumise au sas de stabilité de deux minutes,
et l'ouverture critique reste immédiate. La fenêtre de Propagation n'est
pas un nouveau délai de notification. Les arrivées ordinaires et les
rétablissements actualisent silencieusement la même entrée ; les faits
opérationnels déjà définis restent applicables.

Un passage provisoire à zéro Atteinte pendant la Propagation ne résout pas
l'Incident. Une nouvelle Atteinte après sa fermeture ouvre un autre Incident,
même si sa condition porte sur une période beaucoup plus longue. Le
regroupement n'affirme aucune cause commune. Les autres adapters qui ne
fournissent pas de période conservent le calcul fondé sur leur cadence.

Le test de non-régression rejoue la chronologie anonymisée avec une vraie
base PostgreSQL et une réponse API reproduisant le trigger découvert, ses
prototypes et ZBX-23578. Il exige une ouverture, une alerte et une entrée
intégrée, les dix Atteintes conservées et le titre canonique. Un autre test
vérifie qu'une relecture n'étend pas la fenêtre et qu'un épisode ultérieur
reste distinct. Aucun Incident historique n'est fusionné ou supprimé.
