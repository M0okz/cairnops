---
status: accepted
supersedes:
  - 0010-correler-sans-fusionner-les-incidents.md
  - 0039-regrouper-les-rafales-sans-inventer-de-cause.md
---

# Unifier les Atteintes dans un Incident propagé

CairnOps remplace l'Incident limité à une Cible et la Rafale qui le regroupait
par un seul objet opérationnel. Un Incident porte une Nature fiable, une
Synthèse opérationnelle et une ou plusieurs Atteintes de Cibles. Chaque
Atteinte conserve ses propres Preuves, sa Gravité et ses instants de début et
de fin. Ce regroupement décrit une situation commune, jamais une cause commune.

Une Preuve est la contribution traçable d'une Source à une Atteinte. Les
Connecteurs et Contrôles natifs traduisent leurs conclusions en Preuves
structurées ; un module unique possède ensuite leur application, leur
Invalidation, le rétablissement de l'Atteinte et le cycle de l'Incident. Les
callers ne modifient jamais directement ces transitions.

## Former et fermer la Propagation

La première Atteinte ouvre immédiatement l'Incident. Une nouvelle Atteinte le
rejoint automatiquement lorsqu'elle porte la même Nature fiable et survient
pendant sa Propagation. Aucune validation humaine ni Dépendance entre Cibles
n'est exigée. La proximité temporelle ne permet pas de nommer une cause.

Chaque nouvelle Atteinte prolonge une fenêtre glissante égale à deux cycles de
sa Source, avec un minimum de soixante secondes et un maximum de cinq minutes.
Une Source sans cadence déclarée utilise le minimum ; la plus longue cadence
admissible des Atteintes présentes gouverne la fermeture. Les instants
normalisés décrivent la réalité observée et empêchent un lot historique reçu
en une fois d'étendre artificiellement la Propagation.

La fermeture fige l'appartenance. Une Atteinte ultérieure ouvre un nouvel
Incident, même si un Incident antérieur de même Nature reste actif. Une Cible
peut donc apparaître dans deux Incidents actifs de même Nature lorsque son
ancienne Atteinte est déjà rétablie dans le premier et qu'une nouvelle Atteinte
est active dans le second.

Un passage provisoire à zéro Atteinte active pendant la Propagation ne résout
pas l'Incident. La Résolution survient seulement lorsque la Propagation est
fermée et que toutes les Atteintes sont rétablies. Elle n'est jamais prononcée
contre une Preuve encore active.

## Gravité, prise en charge et notification

La Gravité de l'Incident est la plus élevée de ses Atteintes actives. Une
hausse au-dessus de la plus forte Gravité déjà notifiée constitue un nouveau
Fait opérationnel ; une baisse actualise seulement la Synthèse. La Propagation
étendue conserve les seuils déterministes de l'ADR 0039 et ne produit qu'un
seul nouveau Fait opérationnel.

L'Acquittement porte sur l'Incident. Il couvre ses Atteintes présentes et
celles qui le rejoignent tant que la Propagation reste ouverte. Chaque Preuve
compatible conserve sa synchronisation externe et son résultat propres ; un
échec distant n'annule jamais la prise en charge dans CairnOps.

L'ouverture, une aggravation encore jamais notifiée, la première Propagation
étendue et la Résolution sont les seuls changements susceptibles d'interrompre
l'utilisateur. Les autres transitions révisent silencieusement la Synthèse
opérationnelle et l'entrée intégrée. Le sas de stabilité de l'ADR 0040 reste
applicable à l'ouverture non critique.

## Bascule de développement

CairnOps étant encore en développement, cette décision est appliquée par une
bascule franche. Le schéma, les projections Web et le routage des notifications
abandonnent la Rafale et ne conservent aucune implementation de compatibilité.
L'instance de développement peut être réinitialisée lors du déploiement ; la
migration ne tente ni de regrouper ni de préserver les données opérationnelles
créées avant ce modèle et ne produit aucune notification.

Cette décision remplace les ADR 0010 et 0039. Les Événements opérationnels
restent un futur regroupement explicable d'Incidents éventuellement de Natures
différentes, soutenu par de véritables Dépendances et sans fusionner leur cycle.
