---
status: accepted
---

# Notifier dans l'instance comme on notifie ailleurs

Une instance ne savait prévenir que Mattermost. Qui n'avait pas branché de
webhook n'était prévenu de rien : il fallait regarder l'écran pour apprendre
qu'un service était tombé, ce qui est exactement ce qu'une supervision est
censée éviter.

Les notifications intégrées sont donc un Canal, au même titre que Mattermost, et
elles passent par la même boîte d'envoi. Ce choix leur donne sans rien réécrire
tout ce que le cycle avait déjà appris : le routage selon la Gravité,
l'annulation de ce qui n'est pas encore parti dès l'Acquittement, la
neutralisation par une fenêtre de maintenance, la Résolution qui n'est envoyée
qu'aux Canaux ayant reçu l'ouverture, les baux, les tentatives et le report.
Écrire un second chemin en parallèle aurait dupliqué ces règles, et deux copies
d'une règle finissent toujours par diverger.

Ce qui change tient à la livraison. Le Canal intégré ne sort pas de l'instance :
il n'a ni adresse ni secret, et sa livraison est l'écriture elle-même. Là où
Mattermost émet un appel, l'intégré dépose une entrée par destinataire, puis
signale une fois la volée entière — une entrée par personne émettrait autant de
signalements pour une seule nouvelle.

L'ouverture s'adresse à tous les comptes actifs. La V1 n'a pas de Groupes de
notification, et un Observateur a le droit de savoir même s'il ne décide de
rien ; un compte désactivé, lui, ne reçoit plus rien, puisqu'il n'a plus d'accès
pour lire. La Résolution ne s'adresse qu'à ceux qui ont reçu l'ouverture, et
c'est en relisant la boîte qu'elle les retrouve : c'est ce que veut dire « aux
mêmes destinataires ». Quelqu'un désactivé entre-temps n'en reçoit pas la fin,
et quelqu'un arrivé depuis ne reçoit pas la fin d'une histoire dont il n'a pas
eu le début.

Il n'existe qu'un Canal intégré, posé par la migration : « intégré » désigne
l'instance elle-même, pas une destination que l'on choisirait plusieurs fois.
Une instance notifie donc dès son premier Incident, sans rien avoir à brancher.
Un Administrateur peut le suspendre comme n'importe quel autre Canal.

Lire n'efface pas. Une entrée lue reste dans la boîte avec sa date, parce que ce
qu'on a su et quand on l'a su fait partie de ce qu'on relit. Ouvrir la boîte
marque son contenu comme lu : c'est le geste que l'on vient de faire, et le
faire répéter par un bouton n'ajouterait rien. La boîte n'appartient qu'à son
propriétaire — l'identifiant vient de la session, jamais de la requête — et elle
se lit quel que soit le rôle : administrer des Canaux est une administration,
recevoir des nouvelles n'en est pas une.

La boîte est aussi ce que les compagnons mobiles liront. Le Relais Push de
l'[ADR 0009](0009-relais-push-officiel-a-connaissance-nulle.md) portera la
sonnerie jusqu'à l'appareil ; ce qui s'affiche à l'ouverture de l'application,
c'est cette même boîte, déjà remplie par le même Canal.
