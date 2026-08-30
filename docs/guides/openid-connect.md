# OpenID Connect avec Authentik ou Keycloak

CairnOps accepte un Fournisseur d'identité OIDC actif par instance. Le
Fournisseur authentifie les personnes et administre leurs groupes ; CairnOps
associe explicitement ces groupes à ses trois rôles. Une identité authentifiée
mais sans groupe reconnu n'obtient aucun accès.

La configuration et l'administration des comptes locaux restent réservées à
un Administrateur local. L'Administrateur créé lors de l'amorçage doit donc
rester actif avec son mot de passe : il constitue la porte de secours si le
Fournisseur devient indisponible.

## Contrat du client OIDC

Créer un client confidentiel avec les propriétés suivantes :

- flux Authorization Code ;
- PKCE `S256` ;
- URL de redirection exacte
  `https://cairnops.example.net/api/v1/oidc/callback`, en remplaçant l'origine
  par `CAIRNOPS_PUBLIC_URL` ;
- scopes `openid`, `profile` et `offline_access` ;
- émission d'un refresh token ;
- endpoint `userinfo` qui renvoie `sub`, les attributs de profil et un claim de
  groupes de premier niveau, nommé `groups` par défaut ;
- claim de groupes strictement constitué d'un tableau de chaînes.

Le nom du claim est configurable dans CairnOps. En revanche, sa forme ne l'est
pas : ni chaîne unique, ni objet imbriqué, ni chemin JSON ne sont acceptés. Le
claim doit être disponible dans `userinfo`, et pas seulement dans l'ID Token,
car le serveur le relit périodiquement avec le refresh token.

## Authentik

Créer un fournisseur **OAuth2/OpenID Provider**, lui associer une application
et enregistrer l'URL de redirection exacte. Ajouter ensuite une Property
Mapping de scope qui expose les identifiants de groupes attendus dans le claim
`groups`. La mapping doit participer à la réponse `userinfo` et retourner une
liste de chaînes.

Authentik documente la création du fournisseur et les Property Mappings dans
ses guides [OAuth2/OpenID](https://docs.goauthentik.io/add-secure-apps/providers/oauth2/)
et [Property mappings](https://docs.goauthentik.io/add-secure-apps/providers/property-mappings/).

## Keycloak

Créer un client OpenID Connect confidentiel, activer le **Standard flow** et
enregistrer l'URL de redirection exacte. Ajouter un client scope ou un mapper
de type Group Membership pour produire le claim `groups`, puis activer son
inclusion dans `userinfo`. Autoriser également `offline_access` pour les
Utilisateurs ou le client concernés.

Selon la version de Keycloak, le libellé des écrans varie ; les invariants à
vérifier restent le tableau de chaînes dans `userinfo` et la remise d'un
refresh token. Les références sont le guide
[Server Administration](https://www.keycloak.org/docs/latest/server_admin/)
et la documentation des [protocol mappers](https://www.keycloak.org/admin-api/protocol-mappers).

## Activer dans CairnOps

Dans **Réglages → OpenID Connect** :

1. saisir l'issuer exact publié par le Fournisseur, le client ID et le secret ;
2. associer un ou plusieurs identifiants de groupes à Administrateur,
   Opérateur et Observateur ;
3. enregistrer le brouillon ;
4. lancer le test interactif avec un Utilisateur membre d'au moins un groupe
   reconnu ;
5. activer le brouillon seulement après la confirmation du test.

Le test vérifie la découverte, Authorization Code, PKCE, `state`, `nonce`, l'ID
Token, `userinfo`, le claim de groupes et le refresh token sans créer
d'Utilisateur. Un test ou une activation ratés ne modifient pas la
configuration active.

Les identifiants de groupes sont comparés exactement et en respectant la casse.
Il n'existe ni joker ni expression régulière. Si plusieurs groupes sont
reconnus, le rôle le plus élevé l'emporte : Administrateur, puis Opérateur,
puis Observateur.

## Cycle de vie des accès

Au premier accès autorisé, CairnOps crée un Utilisateur externe identifié
uniquement par le couple `(issuer, sub)`. Son adresse électronique, son nom et
ses groupes ne servent jamais de clé d'identité.

Le serveur renouvelle ensuite l'autorisation environ toutes les cinq minutes :
il utilise le refresh token scellé, relit `userinfo`, exige le même `sub` et
recalcule le rôle. Un groupe retiré, un claim invalide ou un refresh token
explicitement refusé suspend immédiatement les Sessions et les appareils de
l'Utilisateur. Une panne réseau, DNS, TLS, une limitation temporaire ou une
erreur `5xx` conserve la dernière autorisation pendant douze heures au plus.

La suspension externe est réversible lorsque le Fournisseur redevient valide.
Une Désactivation manuelle reste prioritaire et n'est jamais levée par la
synchronisation.

Dès qu'un premier Utilisateur externe existe, l'issuer ne peut plus changer en
V1. Le client ID, le secret, le claim et les mappings restent remplaçables par
un nouveau brouillon testé.
