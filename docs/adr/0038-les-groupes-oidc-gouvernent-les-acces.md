---
status: accepted
---

# Autoriser les Utilisateurs OIDC par groupes externes

Une authentification OpenID Connect ne suffit pas à ouvrir CairnOps. Un Administrateur met explicitement des groupes transmis par le Fournisseur d'identité en correspondance avec les rôles globaux ; une appartenance reconnue autorise la création de l'Utilisateur au premier accès et gouverne ensuite son rôle, tandis qu'une identité sans groupe reconnu reste sans accès. Authentik ou Keycloak demeure ainsi l'autorité sur les appartenances de ses Utilisateurs, CairnOps conserve l'autorité sur leur signification opérationnelle, et l'Administrateur local de secours reste indépendant des deux.

Chaque Utilisateur relève d'un unique Régime d'autorisation, local ou externe. La V1 ne convertit pas un Utilisateur d'un régime vers l'autre : l'Administrateur de secours né de l'amorçage reste local, et les Utilisateurs créés au premier accès OIDC restent externes. Cette limite évite qu'un mot de passe local puisse contourner le retrait d'un Groupe d'accès externe.

L'administration des comptes locaux et de la configuration OIDC exige elle-même un Administrateur local. Un rôle Administrateur reçu par OIDC conserve toutes les capacités opérationnelles, mais ne peut ni créer un autre accès local, ni modifier la source dont dépend sa propre autorisation.

Les Groupes d'accès externes accordent des capacités : lorsqu'un Utilisateur appartient à plusieurs groupes reconnus, CairnOps retient le rôle le plus puissant selon l'ordre Administrateur, Opérateur, Observateur. Les configurations comportant des groupes généraux, imbriqués ou cumulés restent ainsi déterministes sans neutraliser une appartenance plus privilégiée.

La V1 lit les appartenances dans un claim de premier niveau dont le nom est configurable et vaut `groups` par défaut. Sa valeur doit être un tableau de chaînes ; un claim absent ou d'une autre forme refuse l'accès au lieu d'inventer une appartenance, et Authentik comme Keycloak peuvent projeter leurs groupes vers ce contrat sans langage de chemin propre à CairnOps.

Chaque rôle accepte plusieurs identifiants de groupe, comparés exactement et en respectant la casse. La V1 n'interprète ni joker ni expression régulière : une faute de saisie refuse une appartenance au lieu d'élargir accidentellement les privilèges.

CairnOps demande `offline_access` lors du premier accès et conserve un jeton de rafraîchissement scellé par Utilisateur externe. Le serveur renouvelle périodiquement un jeton d'accès, relit `userinfo`, vérifie que son `sub` désigne toujours la même Identité externe, puis synchronise le rôle ou suspend les accès si plus aucun groupe n'est reconnu. Un jeton tournant remplace atomiquement le précédent ; les Sessions Web et les appareils restent ainsi soumis aux groupes sans réauthentification interactive périodique.

Une réponse valide sans groupe reconnu ou le refus explicite d'un jeton suspend immédiatement l'accès externe. Une erreur réseau, TLS, DNS ou distante en `5xx` conserve temporairement la dernière autorisation connue, tout en créant une Défaillance de supervision ; au terme d'une grâce bornée, CairnOps ferme également ces accès. Cette distinction évite qu'une panne du Fournisseur expulse immédiatement l'équipe sans laisser survivre indéfiniment une autorisation devenue invérifiable.

Lorsque le Fournisseur fonctionne, CairnOps synchronise chaque Utilisateur externe environ toutes les cinq minutes. Les échéances sont légèrement décalées entre Utilisateurs afin de borner le délai de retrait d'un privilège sans provoquer une rafale de requêtes simultanées.

La grâce accordée à une panne technique dure douze heures depuis la dernière synchronisation réussie. À son expiration, CairnOps suspend tous les accès externes concernés ; l'Administrateur local de secours reste disponible pour diagnostiquer le Fournisseur ou corriger sa configuration.

La Suspension d'accès externe est un état automatique distinct de la Désactivation d'un Utilisateur. Elle révoque les Sessions et refuse les appareils sans effacer leurs identités ; le retour d'un groupe reconnu pour le même couple `issuer` et `sub` la lève automatiquement, sauf si une Désactivation administrative reste en vigueur.
