---
status: accepted
---

# Faire du parcours incident partagé le critère de sortie de la V1

La V1 est réussie lorsqu'un utilisateur peut déployer CairnOps avec Docker Compose, créer des Contrôles natifs HTTP/HTTPS, TCP, ICMP, DNS et Heartbeat, connecter simplement Zabbix ou Uptime Kuma, importer ses Cibles et recevoir un signal par webhook générique. Un Incident issu de chacune de ces voies doit apparaître de façon cohérente sur le Web, iOS et Android, produire une notification Push puis pouvoir être acquitté depuis un appareil avec propagation immédiate sur toutes les interfaces. Toute fonction qui ne sécurise ni ne complète ce parcours est secondaire pour la première livraison, même si elle appartient au modèle cible documenté.
