---
status: accepted
---

# Dimensionner la V1 pour une installation petite à moyenne

Une installation V1 doit prendre en charge 1 000 Cibles actives, jusqu'à 5 Sources par Cible, des contrôles espacés d'au moins 20 secondes, 25 utilisateurs, 50 appareils et 10 000 Incidents conservés sans dégradation notable de navigation. Un changement enregistré par le serveur doit apparaître sur les clients connectés en moins de deux secondes ; les workers peuvent être multipliés pour absorber les contrôles, sans imposer plusieurs serveurs applicatifs ni un PostgreSQL distribué.
