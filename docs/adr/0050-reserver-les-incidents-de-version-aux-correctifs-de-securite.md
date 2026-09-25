---
status: accepted
---

# Réserver les Incidents de version aux correctifs de sécurité

## Constat

Depuis l'ADR 0036, toute version plus récente remontée par Argus ouvrait l'Incident « Mise à jour logicielle disponible ». En production, ces preuves formaient la plus grande partie des Incidents actifs : trois Incidents couvrant une quarantaine de Ressources, sans altération de fonctionnement ni action urgente. Une mise à jour disponible est une information de suivi des versions ; elle ne devient un problème opérationnel que lorsqu'elle corrige une faille de sécurité.

## Décision

Une mise à jour disponible n'ouvre plus d'Incident. Elle reste visible dans la vue des mises à jour, dans la liste des Ressources et dans la synthèse par catégorie, qui la lisent désormais dans le suivi des versions.

Une mise à jour n'ouvre une Preuve que si l'analyse des notes officielles de la comparaison observée cite au moins un point de catégorie `security` :

- `nature_key` : `software-security-update-available` ;
- libellé : « Mise à jour de sécurité disponible » / « Security update available » ;
- Gravité `major`, comme les correctifs de sécurité requis signalés par PatchMon ;
- Observation `unhealthy` de raison `argus_security_update_available`. Une mise à jour ordinaire produit une Observation `healthy` de raison `argus_update_available`.

Le verdict n'est retenu que s'il porte exactement sur les versions installée et cible observées par Argus. Tant que la comparaison courante attend sa collecte ou son analyse (`pending`, `retry`), le service n'ouvre ni ne résout de Preuve : une nouvelle version ne résout donc pas puis ne rouvre pas un Incident de sécurité encore justifié. Lorsqu'aucune analyse ne peut être produite (source absente, notes introuvables, IA désactivée), aucun correctif de sécurité n'est établi et aucune Preuve n'est ouverte.

Les preuves « Mise à jour logicielle disponible » encore actives sont résolues au premier cycle Argus qui établit le verdict de leur service. Une preuve de sécurité ouverte change de Nature et rejoint un nouvel Incident, conformément à la reclassification des preuves.

## Conséquences

Cette décision remplace, pour la projection opérationnelle, la règle de l'ADR 0036 et l'exception de l'ADR 0048 selon laquelle l'analyse des notes ne produit jamais de Preuve d'Incident. L'analyse reste documentaire et n'exécute aucun déploiement, mais son classement `security` qualifie désormais l'ouverture d'un Incident. Chaque point de sécurité reste rattaché à un extrait des notes officielles, ce qui permet à un Opérateur de vérifier la conclusion et, si nécessaire, d'invalider la Preuve.

Un Incident de sécurité s'ouvre au plus tard au cycle Argus qui suit la fin de l'analyse. Sa fiabilité dépend de la classification du modèle : une mention de politique de sécurité sans faille corrigée peut encore être classée `security`. Sans fournisseur IA configuré, Argus n'ouvre aucun Incident.

PatchMon n'est pas modifié : il n'ouvre déjà d'Incident que pour des correctifs de sécurité en attente ou un redémarrage requis.
