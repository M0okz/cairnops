---
status: accepted
---

# Faire du serveur l'unique source de vérité

Le serveur CairnOps détient et ordonne l'état de référence partagé par le Web, iOS et Android. Les compagnons mobiles conservent dans SQLite une projection opérationnelle des Cibles, Incidents, Événements récents et Journaux nécessaires hors ligne ; les jetons restent dans Keychain ou Keystore et aucune configuration sensible d'Intégration n'est mise en cache. La V1 reste strictement consultative hors ligne : toute mutation exige une connexion et une resynchronisation préalable. Les commandes en ligne portent néanmoins un identifiant idempotent pour tolérer les reprises réseau et doubles envois ; une mutation n'est définitive qu'après confirmation du serveur, qui peut la rejeter si elle est devenue incompatible.
