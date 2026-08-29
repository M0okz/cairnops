---
status: accepted
---

# Argus est une Source de posture logicielle

## Contexte

[Release-Argus](https://github.com/release-argus/Argus) suit la version déployée
et la dernière version disponible de services logiciels. Une mise à jour en
attente constitue une preuve opérationnelle utile, mais pas une mesure de
Disponibilité. Argus peut être protégé par Basic Auth, sans proposer de jeton
limité à la lecture.

Depuis Argus 0.29.0, `GET /api/v1/counts` expose les versions déployée et
disponible ainsi que les décisions `approved` et `skipped`. La configuration
censurée, les métriques Prometheus et le rendu de gabarit complètent cette vue.

## Décision

CairnOps intègre Argus comme Connecteur HTTP pull strictement en lecture seule :

- version minimale prise en charge : Argus 0.29.0 ;
- synchronisation toutes les 300 secondes ;
- lecture de `GET /api/v1/version`, `/api/v1/config`, `/api/v1/counts`,
  `/metrics` et `/api/v1/template` ;
- refus de toute redirection avant transmission d'un éventuel Basic Auth ;
- validation TLS normale, sans option pour ignorer un certificat invalide ;
- HTTP interne autorisé avec un avertissement visible et
  `encrypted_transport: false` persistant ;
- identifiants scellés côté serveur et jamais renvoyés au navigateur.

Un service Argus n'est importable que s'il est actif et possède une
configuration `deployed_version`. L'aperçu montre aussi les services
inéligibles, avec leur raison. Les services éligibles non encore liés sont
présélectionnés, mais chaque liaison à une Cible reste explicite et modifiable.
Une nouvelle découverte ultérieure n'est jamais importée automatiquement.

L'identité externe est l'identifiant du service Argus. Son nom est un libellé
mutable. Si l'identifiant change, CairnOps voit une nouvelle découverte et
conserve l'ancienne Source dans son histoire.

## Projection opérationnelle

La Source Argus porte `measures_availability: false`. Ses Observations gardent
les versions déployée et disponible, `approved`, `skipped`, `last_checked`, les
résultats des deux requêtes de version et les liens HTTP rendus.

Une version disponible et non ignorée ouvre un Incident de Gravité `warning` :

- `nature_key`: `software-update-available` ;
- libellé français : « Mise à jour logicielle disponible » ;
- libellé anglais : « Software update available ».

La nature ne contient ni le Connecteur ni le service, afin de regrouper sur une
même Cible plusieurs preuves de mise à jour logicielle. Une version plus récente
met à jour le même Incident actif et ajoute une Activité uniquement lorsque les
valeurs changent. Après résolution, une nouvelle version ouvre un nouvel
Incident.

`approved` maintient l'Incident actif jusqu'au déploiement. `skipped` le résout
et laisse une Activité. Si la même version ignorée redevient ensuite disponible,
elle ouvre un nouvel Incident. L'Acquittement CairnOps reste local et ne modifie
jamais Argus.

Un échec de `latest_version_query_result_last` ou de
`deployed_version_query_result_last` produit une Observation Inconnue et ne
résout jamais une preuve avec des valeurs en cache. Un service importé supprimé,
désactivé ou privé de `deployed_version` suit la même règle. Les autres services
valides continuent d'être traités et le Connecteur devient dégradé avec le
nombre de Sources Argus inconnues.

## Conséquences

Le premier cycle suivant l'import peut ouvrir immédiatement plusieurs Incidents
et appliquer les politiques de notification habituelles. L'aperçu annonce ce
nombre avant confirmation.

Comme tout Incident actif, une mise à jour logicielle peut rendre la Cible
Dégradée et faire apparaître un état global « Services dégradés ». Elle ne
modifie cependant ni la Disponibilité ni le SLA.

La suppression ou la disparition distante ne résout et n'efface rien
implicitement. La Source et son histoire restent administrables dans CairnOps et
reprennent si le même identifiant externe réapparaît.
