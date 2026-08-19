---
status: accepted
---

# Conclure sur une suite d'Observations, jamais sur une seule

Un Contrôle natif produit des Observations, pas des Incidents. Une Observation
isolée reflète autant l'état de la Cible qu'un aléa du réseau ou de la charge du
worker. Chaque Source de signal porte donc sa propre Politique de déclenchement :
un nombre d'Observations défavorables consécutives avant d'alimenter un Incident,
un nombre d'Observations saines consécutives avant de confirmer le rétablissement,
et la Gravité source qu'elle signale.

Les compteurs sont persistés sur la Source, pas recalculés à la lecture : la
décision reste identique après un redémarrage du worker et ne dépend ni de la
rétention ni de l'agrégation des Observations.

Une Observation Inconnue ne conclut rien. Elle laisse les compteurs intacts au
lieu de les remettre à zéro, car l'absence de preuve ne constitue jamais un
rétablissement et ne doit pas non plus effacer les preuves défavorables déjà
accumulées. Une sonde qui alterne entre échec et indécision finit donc par
déclencher, tandis qu'une instance aveugle ne prononce aucun rétablissement.

L'évaluation s'exécute dans la transaction qui enregistre l'Observation. La
preuve et l'Incident qu'elle alimente ne peuvent pas diverger, y compris lorsque
plusieurs workers observent des Sources d'une même Cible en parallèle.

Les preuves natives rejoignent les Incidents par la Nature `availability`, aux
côtés de celles des webhooks. Plusieurs Sources rattachées à une même Cible
alimentent ainsi un Incident unique et l'Invalidation motivée, la Résolution
automatique et les notifications restent celles déjà en place pour les
Intégrations : le cycle Incident ne se dédouble pas selon l'origine de la preuve.
