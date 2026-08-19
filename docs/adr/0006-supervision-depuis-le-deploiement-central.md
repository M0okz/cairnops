---
status: accepted
---

# Superviser uniquement depuis le déploiement central

Les contrôles faisant autorité sont exécutés exclusivement par les workers du déploiement CairnOps, éventuellement répliqués avec coordination pour éviter les doublons. L'interface Web et les compagnons iOS et Android n'exécutent aucun contrôle faisant autorité ; les agents distants et les emplacements de mesure multiples sont hors du périmètre initial, de sorte que l'état observé représente toujours le point de vue réseau du serveur.
