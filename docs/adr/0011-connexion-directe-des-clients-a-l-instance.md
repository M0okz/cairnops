---
status: accepted
---

# Connecter directement les clients à l'instance

Les interfaces Web et mobiles échangent l'état et les commandes directement avec l'instance CairnOps en HTTPS, que celle-ci soit exposée publiquement ou accessible par un VPN administré par l'utilisateur. CairnOps ne fournit aucun tunnel ni relais de données hébergé : le Relais Push ne transporte que des notifications chiffrées, une projection hors ligne reste non autoritative, et une autorité TLS privée doit faire l'objet d'une confiance explicite plutôt que d'une désactivation de la validation.
