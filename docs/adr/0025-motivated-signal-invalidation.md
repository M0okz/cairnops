---
status: accepted
---

# Invalider une preuve sans réécrire l'histoire

Une preuve active peut être erronée alors que sa Source continue de la publier.
Un Administrateur ou un Opérateur peut donc l'écarter avec un motif obligatoire.
CairnOps conserve la preuve, son auteur, son horodatage et sa justification dans
le Journal, puis recalcule l'Incident à partir des autres preuves actives.

L'invalidation porte sur un cycle de panne, pas sur la Source entière. Un
événement Zabbix possède son propre identifiant et reste ignoré s'il réapparaît.
Uptime Kuma et les webhooks réutilisent une identité stable : après invalidation,
CairnOps attend une observation saine ou une résolution explicite avant de
réarmer cette identité. Un polling persistant ne peut donc pas annuler une
décision humaine, tandis qu'une panne future reste observable après un vrai
rétablissement.
