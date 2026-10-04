---
status: accepted
---

# Graduer la Gravité des mises à jour selon leur niveau et leurs notes

## Constat

L'ADR 0050 réserve les Incidents de version aux correctifs de sécurité, tous en Gravité `major`. Une mise à jour majeure sans faille corrigée restait donc invisible dans les Incidents alors qu'elle annonce une branche installée bientôt sans correctifs et une migration à préparer. Le suivi des versions établit déjà deux qualifications exploitables : le niveau du changement (`major`, `minor`, `patch`, d'après la première composante modifiée) et les catégories de l'analyse des notes officielles, dont `security` et `impact`.

## Décision

La Gravité d'une mise à jour disponible suit cette matrice :

| Niveau | Sans faille corrigée | Avec faille corrigée |
|---|---|---|
| Correctif (`patch`) | aucun Incident | `major` |
| Mineure (`minor`) | aucun Incident | `major` |
| Majeure (`major`) | `information`, `warning` si les notes citent un Impact conditionnel | `major` |

- Une faille corrigée l'emporte sur le niveau : la Nature reste `software-security-update-available`, l'Observation `unhealthy` de raison `argus_security_update_available`.
- Une mise à jour majeure sans faille ouvre la Nature `software-major-update-available` (« Mise à jour majeure disponible » / « Major update available », condition reconnue `software.major_update_available`). Son Observation reste `healthy`, de raison `argus_major_update_available` : elle n'altère pas le fonctionnement et n'entre pas dans l'État de santé dégradé.
- Un point de catégorie `impact` sur la comparaison observée élève une mise à jour majeure sans faille à `warning`. Il n'élève ni une mise à jour mineure ou corrective, ni une mise à jour de sécurité déjà `major`.
- `critical` n'est jamais attribué automatiquement ; il reste une requalification par un Opérateur.

Les règles d'attente de l'ADR 0050 s'appliquent aux deux Natures : tant que la comparaison courante attend sa collecte ou son analyse, le service n'ouvre ni ne résout de Preuve. Sans analyse possible, aucune faille ni aucun impact n'est établi ; une mise à jour majeure ouvre alors une Preuve `information`.

## Conséquences

Cette décision amende l'ADR 0050 : une mise à jour disponible peut désormais ouvrir un Incident sans faille corrigée, mais seulement lorsqu'elle est majeure. Le passage d'une Nature à l'autre, par exemple lorsqu'une nouvelle cible corrige une faille, résout la Preuve existante et en ouvre une nouvelle, conformément à la reclassification des preuves. Un changement d'impact sur une même Nature met à jour la Gravité de la Preuve active.

La catégorie `impact` décrit une condition que l'utilisateur doit vérifier, pas un changement cassant confirmé : un Avertissement signale qu'une vérification préalable est documentée, sans affirmer qu'elle concerne le service suivi.
