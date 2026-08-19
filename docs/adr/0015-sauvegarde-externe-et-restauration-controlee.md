---
status: accepted
---

# Confier la sauvegarde à l'exploitation et contrôler la reprise

CairnOps ne fournit pas de moteur de sauvegarde interne en V1 : une sauvegarde complète réunit PostgreSQL, la clé maîtresse et les paramètres de déploiement conservés séparément, avec des commandes documentées de sauvegarde, restauration et vérification. Un export portable exclut les secrets et l'historique complet ; après restauration, les workers ne reprennent les contrôles qu'une fois la cohérence de la base et l'accès aux secrets validés, afin d'éviter une rafale de faux Incidents.
