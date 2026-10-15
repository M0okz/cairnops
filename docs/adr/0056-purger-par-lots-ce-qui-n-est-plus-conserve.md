---
status: accepted
---

# Purger par lots ce qui n'est plus conservé

L'[ADR 0008](0008-retention-et-agregation-des-observations.md) promettait 30
jours d'Observations brutes et 13 mois d'agrégats horaires, mais rien ne les
effaçait. Les synchronisations de connecteurs écrivent une Observation par
liaison toutes les 30 secondes, et chacune laisse aussi un événement temps réel :
une installation modeste accumulait des centaines de milliers de lignes par
jour sans limite.

Le worker applique désormais la rétention à intervalle régulier. Il supprime par
lots de quelques milliers de lignes, chacun dans sa propre transaction, et
interrompt une passe trop longue pour la reprendre à la suivante : une purge ne
retient jamais une table assez longtemps pour retarder l'ordonnanceur qui
observe. Un seul worker purge à la fois.

- **Observations brutes**, 30 jours par défaut. Une Observation ne s'efface que
  si son heure est déjà consolidée : une consolidation en retard retarde la
  purge, elle ne perd jamais de mesure. La dernière Observation de chaque Source
  survit : une Source suspendue ou muette garde son dernier verdict au lieu de
  paraître n'en avoir jamais eu. La durée ne descend pas sous 29 jours, la
  fenêtre sur laquelle s'apprend le Profil de latence.
- **Agrégats horaires**, 13 mois calendaires par défaut, jamais moins de 2,
  pour couvrir la fenêtre de mesure de 30 jours.
- **Événements temps réel**, 24 heures par défaut. C'est la fenêtre de reprise
  incrémentale de l'[ADR 0022](0022-api-rest-et-signalement-websocket.md). Le
  plus récent reste toujours : il porte la version courante. Un client qui
  revient avec un curseur antérieur aux événements conservés, ou postérieur à
  la version courante parce que la base a été restaurée, reçoit un message
  `ready` portant `resync` et la version courante ; il recharge une projection
  complète puis suit le flux. Rejouer la suite tronquée tairait des changements.
- **Livraisons de notifications**, boîte d'envoi des Canaux et envois Push,
  30 jours par défaut. Elles forment la mémoire de notification d'un Incident :
  l'ouverture livrée conditionne les révisions et la Résolution, la plus forte
  Gravité livrée conditionne l'escalade, la plus haute révision évite un
  renvoi. Seules s'effacent donc les livraisons closes, livrées ou annulées,
  d'un Incident résolu depuis plus longtemps que la rétention, bien au-delà de
  la fenêtre de Reprise de l'[ADR 0054](0054-reprendre-un-incident-recemment-resolu.md).
  Une livraison encore en échec reste, puisqu'elle sera retentée.

Ces durées se règlent par variables d'environnement du worker. Une valeur sous
le minimum empêche le worker de démarrer plutôt que de laisser CairnOps relire
ce qu'il vient d'effacer.

Les Incidents et leur journal humain restent conservés sans limite, comme le
veut l'ADR 0008. Les agrégats journaliers qu'elle conserve sans limite n'existent
pas encore ; rien ne les purgera.
