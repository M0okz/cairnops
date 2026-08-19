---
status: accepted
---

# Neutraliser une projection sans suspendre la collecte

Une maintenance est une fenêtre temporelle motivée appliquée à une ou plusieurs
Cibles. Elle ne désactive ni les Sources, ni les Connecteurs : les Observations,
preuves et Incidents continuent d'être enregistrés normalement.

Pendant la fenêtre, CairnOps marque les Incidents concernés comme neutralisés et
les retire des compteurs d'urgence, du calcul de disponibilité et du routage des
notifications. L'interface conserve un rail violet distinct où les Cibles et le
nombre de preuves restent visibles. La fin prévue ou l'arrêt anticipé rend
immédiatement leur poids opérationnel aux preuves encore actives.

Les maintenances planifiées sont projetées à partir de l'heure du serveur. Les
clients réévaluent cette projection périodiquement en plus du flux temps réel,
afin qu'un début ou une fin de fenêtre ne dépende pas d'un navigateur ouvert.
