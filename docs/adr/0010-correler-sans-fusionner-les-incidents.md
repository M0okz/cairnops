---
status: superseded
superseded_by: 0042-unifier-les-atteintes-dans-un-incident-propage.md
---

# Corréler sans fusionner les Incidents

CairnOps conserve un Incident distinct par Cible et Nature, mais peut les regrouper provisoirement et automatiquement dans un Événement opérationnel lorsque leur proximité temporelle s'explique par des Dépendances explicites entre Cibles. Sans Dépendance déclarée, il ne fait que suggérer le regroupement. L'Événement devient l'unité principale de notification afin de réduire le bruit, tandis que les preuves et cycles de vie restent attachés aux Incidents ; la cause proposée reste explicable et n'est jamais certaine sans confirmation d'un Opérateur, qui peut corriger le regroupement.
