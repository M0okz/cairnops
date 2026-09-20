# Suivi des versions logicielles

Cadrage fonctionnel validé par Gregory le 21 septembre 2026 à l'issue de l'entretien. Ce document décrit le comportement attendu ; il ne signifie pas que la fonctionnalité est implémentée ou déployée.

## Décisions confirmées

- Le suivi est consultable dans une vue globale des mises à jour et dans la fiche de chaque Cible concernée. La vue globale permet d'identifier les services à examiner ; la fiche présente la synthèse, le détail version par version et l'historique.
- La disponibilité ou l'actualisation d'une synthèse IA enrichit les vues sans déclencher de notification supplémentaire. Les alertes Argus conservent leur fonctionnement existant.
- L'historique enregistre les Changements de version constatés par Argus, y compris les retours à une version antérieure. Il distingue la date du constat de l'heure réelle du déploiement, inconnue, et n'invente ni auteur ni étapes intermédiaires.
- Une IA synthétise et classe les changements à partir des seules notes officielles collectées. Chaque point renvoie à sa source et les notes officielles restent accessibles version par version ; l'IA ne doit inventer ni impact ni parcours de migration.
- L'analyse est préparée automatiquement et son résultat est conservé pour les consultations suivantes, sans nouvel appel IA à chaque ouverture. Un changement de version installée ou de version cible déclenche une nouvelle analyse.
- Les notes officielles de la comparaison en cours sont vérifiées quotidiennement. Leur modification ou la disponibilité de notes auparavant manquantes déclenche une actualisation de la synthèse ; cette vérification quotidienne ne provoque aucun nouvel appel IA si le contenu est inchangé.
- Lorsque les notes officielles de certaines versions intermédiaires sont introuvables, CairnOps présente une synthèse partielle des notes disponibles et indique clairement les versions dont les notes manquent. Cette synthèse ne doit pas laisser croire que tous les changements ont été analysés.
- CairnOps propose automatiquement les sources officielles d'un service. Un Administrateur confirme leur association à ce service avant la première analyse ; les analyses suivantes utilisent cette association sans nouvelle confirmation.
- L'analyse utilise un fournisseur IA externe configurable. Les données transmises sont limitées aux notes publiques, au nom du logiciel et aux versions à comparer ; les noms internes des services, leurs adresses et leurs configurations restent dans CairnOps.
- Une indisponibilité du fournisseur IA n'interrompt pas la détection Argus. Les notes officielles déjà collectées restent consultables et l'analyse reprend automatiquement ultérieurement. Toute synthèse existante est conservée avec les versions qu'elle couvre, sans être présentée comme valable pour une nouvelle version installée ou cible.
- CairnOps ne déclenche aucune mise à jour ; elle est réalisée avec des outils extérieurs.
- Le périmètre initial couvre uniquement les services suivis par Argus, sans saisie manuelle de la version installée.
- La comparaison part de la version installée et se termine à la version cible remontée par Argus, pas nécessairement à la dernière version publiée par l'éditeur.
- Les impacts propres à une configuration sont présentés comme des conditions à vérifier par l'utilisateur, avec leurs sources officielles. CairnOps ne prétend pas avoir vérifié la configuration de l'installation.
- Le parcours de mise à jour apparaît seulement lorsque les sources officielles donnent une consigne explicite, par exemple une étape obligatoire ou la possibilité d'un passage direct. Sans indication, la rubrique est absente : aucun message « Parcours non confirmé » n'est affiché et aucun passage direct n'est affirmé par déduction.

## Objectif validé

Présenter un résumé global des changements jusqu'à la version cible, puis un détail version par version : nouveautés, correctifs et sécurité, impacts conditionnels et liens officiels. Conserver l'historique des versions détectées et repartir de la nouvelle version installée après un changement constaté.
