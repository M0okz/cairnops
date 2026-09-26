---
status: accepted
---

# Présenter les notifications sur trois lignes

L'[ADR 0044](0044-notifier-les-incidents-regroupes-avec-une-voix-commune.md)
plaçait le problème en titre, puis « Cible · gravité ». Sur l'écran verrouillé
d'un iPhone, le titre tient sur une seule ligne à côté de l'heure : un libellé
source long, comme « Linux: Load average is too high (per CPU load over 1.5 for
5m) », était tronqué au premier tiers, et la notification n'avait aucune place
pour dire d'où venait l'alerte.

Le gabarit compact passe à trois lignes. Le titre, toujours court, nomme la
Ressource (ou leur nombre) et la gravité. Le corps porte le problème, qui peut
s'étendre sur deux lignes, puis une ligne de contexte. Le Push et Mattermost
ajoutent une pastille de gravité qui double le libellé écrit. Le corps utilise
un simple saut de ligne : le compagnon iOS n'a pas besoin d'évoluer.

Le contexte est calculé lors de la mise en file et figé dans l'entrée de la
boîte intégrée, que le Push relit. Il ne contient que des faits établis : les
Intégrations des Preuves actives non invalidées (toutes celles du cycle pour
une Résolution), un fait structuré seulement s'il est identique sur chacune de
ces Preuves, les trois premières Ressources d'un groupe, la gravité précédente
seulement lorsqu'une hausse est notifiée en alerte, et la durée d'un Incident
résolu. Les entrées antérieures restent sans contexte.

Cette présentation ne modifie ni les Natures, ni le regroupement, ni les
décisions d'envoi. Les modes discret et masqué conservent leurs textes
confidentiels, sans pastille.
