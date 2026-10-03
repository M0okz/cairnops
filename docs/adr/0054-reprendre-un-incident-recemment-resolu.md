---
status: accepted
---

# Reprendre un Incident récemment résolu

Un problème qui oscille — un processus Zabbix saturé à chaque passage du
housekeeper, une charge qui dépasse son seuil quelques minutes par heure —
ouvrait jusqu'ici un nouvel Incident à chaque retour. Chaque Incident franchissait
le sas de l'[ADR 0040](0040-attendre-un-fait-operationnel-stable.md), puis
notifiait une ouverture identique à la précédente, suivie de sa Résolution.
L'Opérateur recevait plusieurs fois par jour le même message, sans que rien ne
le distingue d'un problème nouveau.

Lorsqu'une Atteinte de même Nature revient sur la même Ressource moins de six
heures après la Résolution de l'Incident qui la portait, CairnOps reprend cet
Incident au lieu d'en ouvrir un nouveau. Il redevient actif avec son ouverture,
son historique, sa Synthèse et son Acquittement, et compte ses Reprises. La
fenêtre de six heures part de la dernière Résolution : un problème qui continue
d'osciller reste un seul Incident. Seul un Incident d'une unique Atteinte est
repris : rouvrir un Incident propagé pour l'une de ses Ressources lui ferait
annoncer une étendue qu'il n'a plus. Un retour sur plusieurs Ressources ouvre
donc un nouvel Incident, avec sa propre Propagation. Une Reprise ne rouvre pas
la Propagation de l'Incident repris.

Seule la première Reprise constitue un Fait opérationnel. L'Opérateur a appris
la Résolution ; il doit apprendre une fois que le problème est revenu. Elle
attend le même sas de deux minutes qu'une ouverture non critique, compté depuis
la Reprise, et n'est pas notifiée si l'Incident est de nouveau résolu, acquitté
ou entièrement Sous maintenance avant l'échéance. Les Reprises suivantes
actualisent l'état et la Synthèse sans interrompre, sauf hausse de Gravité
encore jamais notifiée. Un Incident acquitté reste acquitté : sa Reprise ne
relance aucune alerte.

Une Résolution n'est plus livrée en alerte que si elle clôt une alerte
effectivement livrée sur ce Canal ou cet appareil depuis la Résolution
précédente. La règle de l'[ADR 0051](0051-remplacer-l-ouverture-par-sa-resolution-sur-l-appareil.md)
devient ainsi « pas de fin sans début » à chaque cycle, et non plus une fois
pour toute la vie de l'Incident. Mattermost suit la même règle pour son
récapitulatif final. La durée annoncée d'une Résolution est celle du dernier
retour, comptée depuis la Reprise.

La fenêtre fixe de six heures reste un socle V1 explicable, comme le sas de
deux minutes. Une Politique de notification administrable pourra la remplacer
sans modifier le cycle des Incidents.
