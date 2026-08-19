---
status: accepted
---

# Rendre les mises à jour administrées et prévisibles

CairnOps publie des images Docker immuables et versionnées, sans recommander l'étiquette `latest`, et signale les nouvelles versions sans les appliquer automatiquement. Une tâche dédiée exécute les migrations avant la reprise du serveur et des workers ; les notes de version indiquent les sauvegardes et incompatibilités requises, aucun retour arrière automatique ne suit une migration destructive, et le serveur reste compatible avec la version mobile précédente pendant une fenêtre de transition.
