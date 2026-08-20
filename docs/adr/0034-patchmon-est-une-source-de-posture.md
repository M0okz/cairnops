---
status: accepted
---

# PatchMon est une Source de posture, pas de disponibilité

PatchMon sait si un hôte rapporte encore, quels correctifs lui manquent et si
un redémarrage est requis. Ces faits sont utiles à CairnOps, mais aucun ne
démontre que le service porté par l'hôte est disponible. Assimiler « à jour » à
« sain » gonflerait la Disponibilité et la Couverture avec une preuve qui ne
mesure pas ce qu'elles annoncent.

Le Connecteur utilise donc l'Integration API officielle en lecture seule. Son
assistant vérifie l'adresse et les identifiants Key/Secret, montre tous les
hôtes visibles, propose leur rapprochement avec les Cibles, puis attend un
import explicite. Les identifiants sont scellés comme ceux des autres
Connecteurs. CairnOps ne demande, n'expose et n'implémente aucune action de
déploiement de correctif ou de redémarrage.

Chaque liaison crée bien une Source et chaque cycle conserve une Observation de
posture. Une propriété explicite `measures_availability` vaut toutefois faux
pour ces Sources : leurs Observations restent consultables, mais les agrégats
de Disponibilité, Couverture et latence les ignorent. Le silence de PatchMon ne
rend donc jamais un service indisponible par procuration.

Deux conditions alimentent des Incidents autonomes : des correctifs de sécurité
requis, de Gravité majeure, et un redémarrage requis, de Gravité avertissement.
Les mises à jour ordinaires restent de la posture consultable sans ouvrir
d'Incident. Un hôte absent d'une réponse ne résout pas sa condition précédente :
seul un hôte effectivement revu au cours d'une synchronisation réussie peut
confirmer son rétablissement.

Ce découpage garde une évolution possible vers des actions PatchMon, mais toute
écriture distante demandera une décision séparée, un contrat d'autorisation
explicite et une traçabilité propre.
