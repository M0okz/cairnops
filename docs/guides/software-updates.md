# Suivi des mises à jour logicielles

La vue **Mises à jour** et l'onglet homonyme d'une Cible montrent les services importés depuis Argus. Les versions réellement observées déterminent la comparaison, y compris après un retour arrière. Aucun bouton ne déclenche un déploiement.

## Classement des services

CairnOps ordonne les versions remontées par Argus (ADR 0049) et range chaque service dans un seul groupe :

- **À appliquer** : la cible est plus récente et au moins aussi stable que l'installation. Le badge indique une version majeure, mineure ou corrective ; « Sécurité » signale une synthèse qui cite un point de sécurité. Une mise à jour n'est pas un Incident : seule une mise à jour marquée « Sécurité » ouvre l'Incident « Mise à jour de sécurité disponible » (ADR 0050). Les mentions de sécurité et les versions majeures passent en tête.
- **À vérifier** : Argus ne relit plus une version (les dernières versions valides restent affichées avec la raison), propose une préversion à une installation stable, remonte une cible antérieure à l'installation, ou fournit des versions impossibles à ordonner. Ces cas indiquent généralement un suivi Argus à corriger.
- **À jour** : même publication, y compris avec un préfixe `v` ou un identifiant de build, ou version ignorée dans Argus.

L'historique distingue les mises à jour constatées, les retours à une version antérieure et les nouvelles cibles proposées par Argus.

## Première configuration

1. Dans **Réglages → Analyse des notes de version**, choisir le fournisseur (Gemini, OpenAI, Mistral ou DeepSeek), puis un modèle proposé et saisir sa clé. L'adresse HTTPS est préremplie. Les choix sont une sélection de modèles compatibles, pas un inventaire du compte fournisseur. **Personnalisé** permet une autre API compatible Chat Completions ; **Autre modèle** permet de saisir son identifiant. Une configuration existante hors liste reste conservée. La clé est scellée avec la clé maîtresse CairnOps et ne revient jamais dans la réponse API. Une nouvelle adresse de fournisseur exige de ressaisir la clé.
2. La source publique renseignée dans Argus est reprise automatiquement, sans confirmation. Un Administrateur peut enregistrer une source personnalisée, prioritaire sur Argus. Si Argus ne fournit aucune source publique exploitable, la renseigner dans le détail du service. Un changement de source invalide la comparaison courante et programme une nouvelle analyse, sans supprimer l’historique.
3. Laisser le worker collecter les notes et préparer la synthèse. Les notes restent consultables même sans fournisseur IA configuré.

Les formats disponibles sont les dépôts publics GitHub, Forgejo/Gitea et GitLab, ainsi que les changelogs Markdown/HTML organisés par titres de version ou liens vers des notes de version sur le même site. Les pages qui exigent JavaScript ou une authentification ne sont pas collectées. La proposition utilise l'adresse de publication d'Argus ; elle ne transmet pas les noms internes à un moteur de recherche.

## Comparaison et actualisation

Les versions numériques à deux, trois ou quatre composantes, éventuellement préfixées par `v` et accompagnées d'une préversion, sont ordonnées explicitement. Les tags arbitraires comme `latest` ne sont pas ordonnés à l'aveugle. Les préversions intermédiaires sont exclues lorsque la cible est stable. Le catalogue est limité à vingt requêtes par ressource, avec cent entrées par page au départ ; la pagination des publications s'arrête dès qu'une page triée ne contient que des versions antérieures à l'installation. Si une réponse dépasse 2 Mio, la collecte réduit la taille des pages sans sauter d’entrée. Un dépassement du budget est indiqué comme incomplet. Les tags du dépôt permettent de signaler les versions sans notes, sans inventer les numéros intermédiaires. Une adresse d'API GitHub fournie par Argus (`api.github.com/repos/…`) est ramenée à son dépôt.

Aucune collecte n'a lieu lorsque la cible n'est pas plus récente que l'installation. Lorsqu'aucune note n'est publiée, le service affiche « Notes officielles introuvables » et la source est revérifiée chaque jour. Les requêtes anonymes vers GitHub sont limitées à 60 par heure : une limite signalée par `Retry-After` ou `X-RateLimit` suspend toute requête vers cet hôte jusqu'à l'heure de reprise affichée.

La comparaison en cours est contrôlée chaque jour. Un changement de contenu déclenche une nouvelle analyse ; une consultation ne relance pas l'IA. Une panne reporte le traitement et conserve les résultats précédents avec leurs versions couvertes. Les reprises sont espacées d'une heure et les traitements sont bornés à huit minutes avec un bail de dix minutes. Une analyse trop volumineuse reste différée ; les notes déjà collectées restent accessibles.

Les synthèses sont produites en français ; les commandes de l'interface existent en français et en anglais. Chaque point donne accès à son extrait officiel. L’IA sélectionne des identifiants d’extraits ; CairnOps reprend leur texte et leur version directement dans les notes, sans demander au modèle de les recopier. Une réponse au format ou aux références invalides bénéficie d’une seule tentative immédiate de réparation, puis de la reprise différée habituelle. Une absence de consigne de migration ne produit aucune rubrique de parcours ni aucun avertissement à ce sujet.

## Références des protocoles

- [API GitHub Releases](https://docs.github.com/en/rest/releases/releases)
- [API Chat Completions](https://platform.openai.com/docs/api-reference/chat/create)

- [Modèles OpenAI](https://developers.openai.com/api/docs/models/gpt-4.1-mini)
- [API Mistral et mode JSON](https://docs.mistral.ai/api)
- [Mode JSON DeepSeek](https://api-docs.deepseek.com/guides/json_mode)

- [Compatibilité Chat Completions de Gemini](https://ai.google.dev/gemini-api/docs/openai)
- [Catalogue des modèles Gemini](https://ai.google.dev/gemini-api/docs/models)
