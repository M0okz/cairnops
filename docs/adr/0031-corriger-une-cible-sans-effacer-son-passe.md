---
status: accepted
---

# Corriger une Cible sans effacer son passé

Une Cible se renomme, se décrit autrement, cesse d'être supervisée. Jusqu'ici
CairnOps ne savait que créer : une faute de frappe restait, un service démantelé
encombrait le tableau pour toujours. Le schéma prévoyait pourtant `archived_at`
depuis la fondation, sans que rien ne l'écrive.

Une Cible ne se supprime pas, elle s'archive. Ses Incidents, son Journal
d'activité et ses mesures ont servi à décider ; les effacer réécrirait un passé
sur lequel des gens se sont appuyés. Une Cible archivée quitte les listes, les
mesures et le routage des notifications, mais son histoire reste lisible et
l'archivage se défait.

L'archivage clôt ce qui n'a plus de sens : les Incidents actifs de la Cible sont
résolus avec une entrée au Journal qui en donne la raison, ses Contrôles natifs
cessent d'être dus, et aucun signal — natif, entrant ou rapproché depuis une
Intégration — ne rouvre d'Incident sur elle. Une Intégration qui continue de
publier son état ne ressuscite donc pas une Cible retirée du service ; ses
liaisons subsistent et reprendront si la Cible est restaurée.

Un Contrôle natif se modifie entièrement, y compris sa configuration : c'est une
sonde que CairnOps exécute, et corriger une URL ne doit pas obliger à recréer la
Source ni à perdre ses Observations. Changer sa cadence change du même coup ce
que la Couverture attend de lui, sans réécrire les heures déjà consolidées. Le
suspendre l'arrête sans rien perdre ; le retirer emporte ses Observations, car
une preuve sans Source qui la porte ne s'interprète plus.

Une Source apportée par une Intégration ne se modifie pas depuis CairnOps. Elle
appartient à son Intégration conformément à l'[ADR
0030](0030-les-integrations-produisent-des-observations.md) : son nom et sa
cadence viennent du produit distant, et c'est la suspension du Connecteur, non
celle de la Source, qui arrête la lecture. Prétendre le contraire ferait
diverger CairnOps de l'outil qu'il intègre au premier cycle suivant.

Le renommage n'exige pas l'unicité. Deux Cibles peuvent porter le même nom —
c'est déjà vrai à la création — mais un Connecteur qui rapproche une Cible
découverte par son nom ne considère que les Cibles non archivées : archiver
libère donc un nom pour un import ultérieur, sans jamais fusionner deux
histoires.
