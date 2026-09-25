---
status: accepted
---

# Ordonner les versions avant de signaler une mise à jour

> Ajustement : depuis l'ADR 0050, une mise à jour (`update`) n'ouvre une Preuve que si ses notes officielles citent un correctif de sécurité.

L'ADR 0036 ouvrait un Incident dès que la version disponible remontée par Argus différait de la version déployée. En production, cette égalité de chaînes signalait comme mises à jour une préversion proposée à une installation stable (`11.6.0` → `12.0.0-rc1`), une cible en retard sur l'installation (`0.1.147` → `0.1.146`) et un identifiant de build (`2024.10.22` → `2024.10.22-7ca5933`). À l'inverse, un échec de lecture de la version installée masquait les dernières versions connues, y compris une vraie mise à jour.

Argus reste l'autorité sur les valeurs observées et sur la version cible. CairnOps établit désormais leur ordre selon la précédence SemVer, étendue à une à quatre composantes numériques ; un suffixe formé d'un identifiant de commit désigne un build et non une préversion. Chaque comparaison reçoit une situation unique, partagée par le Connecteur, le suivi des notes et les interfaces :

- `update` : cible plus récente et au moins aussi stable que l'installation. Elle seule ouvre la Preuve « Mise à jour logicielle disponible », qualifiée de majeure, mineure ou corrective selon la première composante modifiée ;
- `prerelease` : préversion proposée à une installation stable. Elle reste consultable sans Incident ;
- `target_older` et `unordered` : cible antérieure ou versions impossibles à ordonner. L'Observation est inconnue et la vue demande de vérifier le suivi dans Argus ;
- `current` : même publication, y compris avec un préfixe `v` ou une métadonnée de build.

Un service lisible sans mise à jour à appliquer reste observé : une Preuve ouverte par l'ancienne règle est donc résolue au cycle suivant. Les règles de l'ADR 0036 sur les versions ignorées, approuvées ou inconnues sont inchangées ; un service dont Argus ne relit plus les versions n'ouvre ni ne résout aucune Preuve, mais la vue conserve ses dernières versions valides avec la raison de leur non-confirmation.

La vue des mises à jour classe chaque service dans un seul groupe calculé par le serveur : à appliquer, à vérifier ou à jour. Une version ignorée dans Argus ne demande aucune action. L'historique distingue les changements de version installée, les retours arrière et les nouvelles cibles proposées.

Le suivi des notes n'interroge plus les sources lorsqu'aucune cible plus récente n'existe. Une absence de notes publiées devient un état stable vérifié chaque jour. Une limite de débit signalée par `Retry-After` ou par les en-têtes `X-RateLimit` suspend toute requête vers l'hôte concerné jusqu'à sa réinitialisation, afin de ne pas épuiser le quota anonyme partagé par tous les services. Une adresse d'API GitHub fournie par Argus désigne son dépôt.
