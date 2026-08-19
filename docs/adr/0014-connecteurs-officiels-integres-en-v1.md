---
status: accepted
---

# Limiter la V1 aux Connecteurs officiels intégrés

Les Connecteurs de la V1 sont livrés, versionnés et testés dans l'image CairnOps ; son périmètre bloquant couvre Uptime Kuma, Zabbix et le webhook générique. Les Connecteurs guidés Prometheus Alertmanager et Grafana Alerting sont prévus immédiatement après la V1, le webhook générique permettant entre-temps de recevoir leurs événements. Aucun plugin tiers ne s'exécute dans le serveur ou le worker avec accès aux secrets et au réseau interne ; les autres besoins personnalisés passent par les interfaces génériques entrantes ou par un adaptateur externe, en attendant qu'une API stable permette éventuellement des Connecteurs externes isolés.
