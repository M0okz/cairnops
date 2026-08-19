---
status: accepted
---

# Partager une API REST et signaler les changements par WebSocket

Le Web, iOS et Android utilisent une API HTTP REST décrite par OpenAPI pour toutes les lectures, commandes et opérations administratives. OpenAPI génère uniquement les modèles de transport et clients HTTP de base pour TypeScript, Swift et Kotlin ; chaque plateforme les enveloppe dans une couche métier idiomatique et ne les expose pas directement à ses vues, tandis que la CI vérifie la cohérence du contrat et du serveur Go. Un WebSocket authentifié annonce rapidement les événements, identifiants et versions concernés sans devenir une seconde API de commande ; après interruption, chaque client récupère les changements depuis sa dernière version connue ou recharge une projection complète si la reprise incrémentale n'est plus possible.
