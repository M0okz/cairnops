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

## Séries hebdomadaires

Une série possède un fuseau IANA et une date locale de fin obligatoire, comprise
entre une semaine et un an après son début. Ses occurrences (53 maximum) sont
matérialisées en une transaction dans les fenêtres existantes. Les projections
d'Incident, de disponibilité et de notification emploient donc les mêmes
intervalles, sans dépendre d'un navigateur ni d'un planificateur supplémentaire.

Le jour et l'heure locale du début sont conservés. Chaque fenêtre garde une
durée réelle constante, limitée à 24 heures à la création. Lors d'un changement
d'heure, une heure inexistante est sautée ; une heure ambiguë utilise uniquement
le premier passage. Ces règles sont annoncées lors de la planification.

L'arrêt d'une occurrence ne modifie pas les suivantes. L'annulation de la série
vise explicitement toutes ses fenêtres en cours et à venir et conserve celles
déjà terminées. Une prolongation ajoute 30 minutes à une occurrence active,
sans déplacer les autres. Elle compare atomiquement la fin vue par l'opérateur,
conserve l'auteur et les anciennes et nouvelles fins dans un journal dédié, et
ne peut empiéter sur la prochaine occurrence ni dépasser la borne de 31 jours.
