---
status: accepted
---

# Remplacer l'ouverture affichée par sa Résolution sur l'appareil

L'[ADR 0044](0044-notifier-les-incidents-regroupes-avec-une-voix-commune.md)
faisait passer les Résolutions par la voie Push silencieuse. Sur iOS, cette
voie est une notification d'arrière-plan : le système peut la retarder, la
regrouper ou ne jamais la remettre si l'application a été fermée. L'ouverture
restait donc affichée comme une alerte en cours dans le centre de
notifications, et les rappels programmés sur l'appareil continuaient après le
rétablissement.

Une Résolution devient une livraison visible pour chaque appareil qui a
effectivement reçu l'ouverture en alerte. Elle porte le même identifiant de
regroupement que l'ouverture et la remplace. Un appareil qui n'a jamais affiché
l'ouverture, parce qu'elle était encore en attente ou en échec, reçoit la
Résolution par la voie silencieuse : il n'apprend pas une fin sans début.

L'instance ne décide pas de l'interruption. Le compagnon applique son réglage
de son de rétablissement : silencieux, la Résolution s'ajoute au centre de
notifications sans son ni bannière ; avec son, elle informe comme le prévoit la
Politique de notification. La décision des autres Canaux est inchangée.

Le message chiffré ajoute deux champs additifs : `unread_count`, le nombre
d'entrées non lues de la boîte de son destinataire, qui permet de tenir la
pastille de l'application sans l'ouvrir, et `acknowledged`, vrai lorsque
l'Incident actif est acquitté. Un client ancien les ignore.
