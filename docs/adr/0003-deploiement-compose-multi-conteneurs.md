---
status: accepted
---

# Distribuer CairnOps comme une application Compose multi-conteneurs

CairnOps est livré comme une seule application déployable en une commande avec Docker Compose, sans mode obligatoire regroupant tous les processus dans un conteneur. La topologie minimale sépare un serveur pour l'API, le temps réel et l'interface Web, un worker pour la supervision et les notifications, et PostgreSQL pour l'état durable ; le serveur et le worker peuvent partager la même image avec des rôles de démarrage différents. Cette séparation permet notamment de redémarrer l'interface sans interrompre les contrôles permanents. Le worker utilise les sockets ICMP non privilégiées lorsque l'hôte le permet, sinon reçoit uniquement `CAP_NET_RAW` ; il ne s'exécute ni en mode privilégié ni comme root, et l'absence de cette capacité désactive proprement ICMP sans affecter les autres contrôles.
