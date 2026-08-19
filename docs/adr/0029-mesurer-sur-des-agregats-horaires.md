---
status: accepted
---

# Mesurer sur des agrégats horaires, et dire ce qui manque

Disponibilité, Couverture et latence répondent à trois questions distinctes.
La Disponibilité est la part des Observations concluantes qui ont conclu à la
disponibilité. La Couverture est la part des Observations attendues qui ont
effectivement conclu : une Observation Inconnue, une sonde suspendue ou un
worker absent la font baisser. La latence est la moyenne et le maximum des
Observations saines. Une Disponibilité de 100 % adossée à une Couverture de 4 %
n'affirme rien, et l'interface doit pouvoir le montrer plutôt que rassurer.

Les trois mesures se lisent sur des agrégats horaires consolidés par le worker,
jamais sur les Observations brutes. Une fenêtre de 30 jours coûte alors 720
lignes par Source au lieu de 130 000, et l'affichage ne dépend plus de la
rétention des Observations décidée par l'[ADR
0008](0008-retention-et-agregation-des-observations.md). L'heure en cours, elle,
se lit sur les Observations brutes et rejoint les heures consolidées : une
mesure reste donc vraie à la minute sans attendre la consolidation.

Chaque heure consolidée porte le nombre d'Observations attendues, calculé depuis
l'intervalle de la Source en vigueur à ce moment et depuis sa date de création.
Une heure sans aucune Observation existe donc quand même, avec ses Observations
attendues et rien de collecté : c'est ainsi qu'une interruption du worker se
voit. Une Source suspendue n'est pas consolidée et ne pénalise pas la Couverture
des heures pendant lesquelles personne ne lui demandait rien.

Le rollup ne conserve que des sommes, un compte et un maximum. La latence
moyenne d'une fenêtre est donc exacte, et aucun percentile n'est annoncé :
p50 et p95 ne s'agrègent pas sans histogramme, et une valeur approchée présentée
comme exacte vaut moins qu'une valeur absente.

Une Cible mesure la somme de ses Sources : ses Observations se comptent
ensemble, ce qui pondère chaque Source par sa cadence réelle. Les Sources
apportées par une Intégration y entrent au même titre que les Contrôles natifs,
depuis l'[ADR 0030](0030-les-integrations-produisent-des-observations.md).

La maintenance ne réécrit pas la mesure. Elle neutralise la projection
opérationnelle — Incidents, escalade, notifications — conformément à l'[ADR
0026](0026-maintenance-projection.md), mais une indisponibilité observée pendant
une fenêtre reste une indisponibilité observée. Effacer la mesure reviendrait à
laisser une décision d'exploitation modifier l'histoire.
