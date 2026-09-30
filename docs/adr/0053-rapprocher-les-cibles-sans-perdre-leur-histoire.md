---
status: accepted
---

# Rapprocher les Cibles sans perdre leur histoire

Une Intégration découvre une perspective technique, pas nécessairement la
bonne identité opérationnelle. Un service observé par Uptime Kuma et sa machine
observée par Zabbix peuvent ainsi entrer comme deux Cibles alors que
l'Administrateur veut une seule identité CairnOps. Le premier import sait
proposer une Cible existante, mais une liaison déjà confirmée était jusque-là
figée : corriger l'identité exigeait de supprimer un Connecteur et son histoire.

CairnOps distingue désormais deux décisions administratives :

- **Rapprocher deux Cibles** confirme qu'elles représentent la même chose. Une
  Cible explicitement choisie survit ; l'autre devient une identité redirigée.
  Sources, Observations, agrégats, Incidents, maintenances, Indicateurs et
  relations sont consolidés sans supprimer la trace de l'identité absorbée.
- **Rattacher une Source** corrige rétroactivement une liaison erronée lorsque
  les deux Cibles restent distinctes. Seule la preuve attribuable à cette
  Source change de Cible ; les Incidents partagés sont scindés sans réécrire les
  décisions qui concernaient leurs autres Sources.

Le rapprochement est irréversible. Il s'exécute comme une opération durable et
transactionnelle : la préparation peut survivre à un redémarrage, tandis que la
mutation finale réussit entièrement ou ne laisse aucun état partiel. La
supervision continue pendant le travail. Un Incident actif de même Nature sur
les deux Cibles devient un Incident unique, ouvert à la date la plus ancienne,
avec la Gravité la plus forte ; il ne reste acquitté que si les deux Incidents
l'étaient. Les Incidents résolus restent des épisodes distincts de l'histoire.

La Cible absorbée est archivée avec un lien vers la survivante, jamais effacée.
Son nom devient un alias recherchable et ses anciennes URL résolvent vers la
survivante. Une justification humaine, l'auteur, l'aperçu et le résultat sont
conservés dans un Journal administratif immuable.

## Suggestions explicables

Un moteur déterministe compare en continu les facettes d'identité apportées par
les Sources : identifiant machine, FQDN, adresse, URL et noms. Il ne donne aucun
poids absolu à une marque de Connecteur et ne type pas globalement la Cible :
une Cible peut légitimement réunir un service et la machine qui le porte.

Les preuves sont pondérées, datées et accompagnées de contradictions. Une
contradiction forte impose l'abstention ; une ressemblance faible ne remplit pas
la file d'examen. Le moteur suggère soit un rapprochement de Cibles, soit la
correction d'une seule Source, mais ne modifie jamais l'état sans confirmation
d'un Administrateur. Les rejets, reports et confirmations restent auditables ;
un rejet ne revient que si des preuves matérielles nouvelles apparaissent.

La précision prime sur le rappel. Une suggestion manquée peut être déclenchée
manuellement ; des faux positifs répétés détruiraient la confiance dans l'outil.
