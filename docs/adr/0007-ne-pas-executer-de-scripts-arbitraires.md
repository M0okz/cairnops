---
status: accepted
---

# Ne pas exécuter de scripts arbitraires

CairnOps fournit initialement des contrôles natifs HTTP/HTTPS, TCP, ICMP, DNS et TLS, ainsi que des Heartbeats et webhooks génériques entrants, mais son worker n'exécute aucun script arbitraire fourni par l'utilisateur. Les contrôles personnalisés s'exécutent hors de CairnOps et lui transmettent leurs résultats, ce qui évite de transformer la configuration de supervision en capacité d'exécution de code sur le serveur.
