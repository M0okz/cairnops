---
status: accepted
---

# Analyser les notes de version indépendamment de la supervision

> Ajustement : l'ADR 0050 fait du classement `security` de l'analyse courante la condition d'ouverture de l'Incident « Mise à jour de sécurité disponible ». L'analyse ne produit toujours ni Gravité propre, ni notification directe, ni déploiement.

Le suivi des versions utilise une IA externe configurable pour synthétiser les notes officielles issues de la source publique fournie par Argus ou personnalisée par un Administrateur, car une compilation déterministe ne restitue pas les bénéfices cumulés entre deux versions. Cette analyse documentaire est distincte des Synthèses opérationnelles de l'ADR 0043 : elle ne produit ni Preuve d'Incident, ni Gravité, ni notification et n'exécute aucun déploiement. Argus reste l'autorité sur les versions observées et sur la version cible.

Les notes collectées et les analyses sont persistées avec leur comparaison et leur provenance. Un traitement asynchrone reprend les échecs et refuse de publier un résultat devenu obsolète pendant son calcul ; une indisponibilité du fournisseur IA ne bloque pas Argus. Chaque affirmation générée porte un extrait contrôlé dans les notes collectées, ce qui assure sa traçabilité sans constituer une garantie automatique de justesse de la reformulation.

Le fournisseur reçoit seulement les notes publiques et les versions comparées. Les identifiants internes, adresses et secrets de l'installation restent dans CairnOps. L'appel IA n'a aucun outil et son résultat est affiché comme texte, jamais exécuté. Les sources et le fournisseur sont joints uniquement en HTTPS public ; le contrôle des adresses au moment de la connexion évite qu'une URL de notes ouvre un accès au réseau interne.

## Ajustement du 22 septembre 2026

La source Argus est adoptée automatiquement sans confirmation supplémentaire. Elle suit les changements de configuration Argus ; une source personnalisée reste prioritaire. Sa disparition ou son invalidité suspend la collecte sans supprimer les analyses précédentes. Les changements de source invalident les traitements en cours.

Le modèle sélectionne des identifiants d’extraits fournis dans chaque requête. Le serveur résout la citation et sa version, refuse les références inconnues et borne à une seule tentative immédiate la réparation des réponses invalides. Cette résolution évite les rejets dus à une recopie imparfaite sans assouplir la vérification des sources.
