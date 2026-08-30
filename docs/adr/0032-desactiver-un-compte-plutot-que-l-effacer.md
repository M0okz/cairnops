---
status: accepted
---

# Désactiver un Utilisateur plutôt que l'effacer

Jusqu'ici une instance ne savait vivre qu'avec l'Administrateur né de sa mise en
service. Les trois rôles existaient, la base les contraignait, l'API les
exigeait — mais rien ne permettait d'ouvrir un second compte. Une équipe entière
partageait donc un identifiant, et le Journal d'activité attribuait ses
décisions à une personne qui ne les avait pas prises.

Un Administrateur ouvre désormais un compte, corrige son nom d'affichage,
change son rôle, et lui retire l'accès. L'identifiant, lui, ne change jamais :
c'est par lui qu'une personne se reconnaît dans les sessions et dans le Journal,
et le renommer brouillerait ce que l'on relit.

Un Utilisateur ne se supprime pas, il se désactive. Chaque trace d'une décision —
acquittement, invalidation, entrée au Journal, Connecteur raccordé, fenêtre de
maintenance — pointe vers son auteur avec `ON DELETE SET NULL` : effacer le
compte rendrait anonyme un passé sur lequel des gens se sont appuyés. C'est la
même raison qui archive une Cible plutôt que de la détruire, à l'[ADR
0031](0031-corriger-une-cible-sans-effacer-son-passe.md). La désactivation
révoque les sessions ouvertes, ferme la connexion et ferme aussi la porte de
secours ; elle ne touche ni l'empreinte du mot de passe, ni l'identifiant, qui
reste pris. La réactivation rend l'accès tel quel.

Cette Désactivation d'Utilisateur reste distincte de la Suspension d'accès
externe définie pour OIDC à l'[ADR 0038](0038-les-groupes-oidc-gouvernent-les-acces.md).
La première exprime une décision administrative qui domine tous les régimes
d'autorisation ; la seconde suit automatiquement les Groupes d'accès externes
et peut disparaître à leur retour sans jamais réactiver un Utilisateur désactivé.

CairnOps n'envoie pas de courrier en V1. Pour un Utilisateur local,
l'Administrateur choisit donc le premier mot de passe et le transmet lui-même,
hors de CairnOps, exactement comme une réinitialisation. Un Utilisateur externe,
lui, naît lors de son premier accès autorisé par ses Groupes d'accès externes.

Deux barrières gardent l'instance de se refermer sur elle-même. La première est
lisible et suffit presque toujours : un Administrateur n'agit ni sur son propre
rôle, ni sur son propre accès. La seconde tient en transaction et vise ce que la
première ne voit pas — deux Administrateurs qui se retireraient l'un l'autre au
même instant liraient tous deux un état encore rassurant : après chaque geste,
la transaction recompte les Administrateurs actifs et refuse de valider s'il n'en
reste aucun. L'instance garde donc toujours quelqu'un pour la rouvrir, et la
porte de secours du Jeton d'amorçage garde toujours un compte à qui répondre.

Avec OIDC, cette seconde barrière compte plus précisément les Administrateurs
locaux de secours actifs plutôt que tous les Administrateurs : un Administrateur
externe ne garantit pas l'accès lorsque le Fournisseur d'identité est indisponible.

Changer manuellement le rôle d'un Utilisateur local révoque ses sessions. Un rôle décide de ce qui
est permis à chaque requête ; laisser vivre une session ouverte sous l'ancien
laisserait ouvertes des portes que le nouveau ne franchit plus.
