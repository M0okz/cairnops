---
status: accepted
---

# Intégrer Uptime Kuma par son endpoint métriques

Le Connecteur Uptime Kuma de la V1 utilise l'endpoint Prometheus officiel
`/metrics`, authentifié par une clé API dédiée transmise comme mot de passe HTTP
Basic. La métrique `monitor_status` fournit l'identifiant stable, le nom, le type,
l'adresse et l'état de chaque moniteur ; CairnOps traite DOWN comme une preuve de
panne, UP comme une preuve de rétablissement et garde PENDING et MAINTENANCE
neutres. La clé est scellée par la clé maîtresse de l'instance et n'est jamais
renvoyée par l'API CairnOps.

L'API Socket.IO de Kuma n'est pas utilisée par la Supervision : elle est
documentée comme une API interne susceptible de changer sans préavis et
exigerait un compte interactif ou un jeton de session plus puissant. L'ADR 0041
autorise une exception versionnée et isolée pendant la connexion guidée et la
suppression, uniquement pour créer ou révoquer la clé dédiée à `/metrics` à
partir d'une autorisation temporaire. Une rupture de cette interface peut donc
affecter l'amorçage d'un nouveau Connecteur, jamais la Supervision d'un
Connecteur existant. L'intégration quotidienne reste ainsi en lecture seule et
stable, au prix de l'absence de propagation d'acquittement vers Kuma ; un
acquittement d'Incident provenant uniquement de Kuma reste local à CairnOps.

Références :

- [Documentation officielle de l'API interne Uptime Kuma](https://github.com/louislam/uptime-kuma/wiki/Internal-API)
- [Documentation officielle de l'intégration Prometheus](https://github.com/louislam/uptime-kuma/wiki/Prometheus-Integration)
- [Implémentation officielle des métriques et états](https://github.com/louislam/uptime-kuma/blob/master/server/prometheus.js)
