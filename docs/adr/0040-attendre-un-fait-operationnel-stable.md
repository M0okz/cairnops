---
status: accepted
---

# Attendre un fait opérationnel stable avant d'alerter

CairnOps conserve immédiatement chaque Observation, Preuve d'Incident et
Incident, mais une ouverture non critique attend deux minutes après la création
de l'Incident dans CairnOps avant de devenir livrable. Une Rafale reprend le
même sas depuis sa propre formation. Si l'Incident est résolu, acquitté ou placé
Sous maintenance avant l'échéance, l'ouverture est annulée et aucune
notification de Résolution ne la remplace. Un Incident Critique reste
immédiatement livrable.

Ce sas porte sur le premier constat de CairnOps, pas sur l'heure historique
fournie par l'Intégration. Un problème ancien découvert lors d'un import, d'une
reclassification ou d'un redémarrage doit donc survivre à un nouveau cycle de
synchronisation avant d'interrompre l'utilisateur. La collecte et l'État
partagé restent exhaustifs pendant cette attente : seul le Canal de
notification est temporisé.

Après une ouverture livrée, la boîte intégrée continue d'informer de toutes les
évolutions. Une Résolution ou une révision ordinaire de Rafale met à jour l'état
silencieusement ; une nouvelle alerte visible reste réservée à une hausse de
Gravité encore jamais notifiée ou à la première Propagation étendue. Mattermost
conserve son récapitulatif final, puisque ce Canal ne sait pas modifier un
message déjà livré.

Pour APNs, « silencieux » signifie une notification d'arrière-plan de priorité
5 contenant `content-available`, sans `alert`, `sound` ni `badge`. Baisser
seulement la priorité d'une charge de type `alert` ne la rend pas silencieuse et
recrée précisément le bruit que la Politique de notification doit absorber.

Cette décision remplace, pour les ouvertures non critiques, l'envoi immédiat
décrit dans les ADR 0027 et 0039. La durée fixe de deux minutes reste un socle
V1 explicable ; une Politique de notification administrable pourra ensuite la
remplacer sans modifier le cycle des Incidents.
