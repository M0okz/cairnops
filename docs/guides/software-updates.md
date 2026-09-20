# Suivi des mises à jour logicielles

La vue **Mises à jour** et l'onglet homonyme d'une Cible montrent les services importés depuis Argus. Les versions réellement observées déterminent la comparaison, y compris après un retour arrière. Aucun bouton ne déclenche un déploiement.

## Première configuration

1. Dans **Réglages → Analyse des notes de version**, renseigner l'adresse de base HTTPS d'une API compatible Chat Completions, son modèle et sa clé. La clé est scellée avec la clé maîtresse CairnOps et ne revient jamais dans la réponse API. Une nouvelle adresse de fournisseur exige de ressaisir la clé.
2. Dans le détail d'un service, vérifier la proposition de source publique remontée depuis Argus, puis confirmer le nom public du logiciel, le format et l'adresse officielle. Une source peut être corrigée par un Administrateur ; cela invalide la comparaison courante et programme une nouvelle analyse.
3. Laisser le worker collecter les notes et préparer la synthèse. Les notes restent consultables même sans fournisseur IA configuré.

Les formats disponibles sont les dépôts publics GitHub, Forgejo/Gitea et GitLab, ainsi que les changelogs Markdown/HTML organisés par titres de version ou liens vers des notes de version sur le même site. Les pages qui exigent JavaScript ou une authentification ne sont pas collectées. La proposition utilise l'adresse de publication d'Argus ; elle ne transmet pas les noms internes à un moteur de recherche.

## Comparaison et actualisation

Les versions numériques à deux, trois ou quatre composantes, éventuellement préfixées par `v` et accompagnées d'une préversion, sont ordonnées explicitement. Les tags arbitraires comme `latest` ne sont pas ordonnés à l'aveugle. Les préversions intermédiaires sont exclues lorsque la cible est stable. Le catalogue est limité à vingt pages de cent entrées par ressource ; un dépassement est indiqué comme incomplet. Les tags du dépôt permettent de signaler les versions sans notes, sans inventer les numéros intermédiaires.

La comparaison en cours est contrôlée chaque jour. Un changement de contenu déclenche une nouvelle analyse ; une consultation ne relance pas l'IA. Une panne reporte le traitement et conserve les résultats précédents avec leurs versions couvertes. Les reprises sont espacées d'une heure et les traitements sont bornés à huit minutes avec un bail de dix minutes. Une analyse trop volumineuse reste différée ; les notes déjà collectées restent accessibles.

Les synthèses sont produites en français ; les commandes de l'interface existent en français et en anglais. Chaque point donne accès à son extrait officiel. Une absence de consigne de migration ne produit aucune rubrique de parcours ni aucun avertissement à ce sujet.

## Références des protocoles

- [API GitHub Releases](https://docs.github.com/en/rest/releases/releases)
- [API Chat Completions](https://platform.openai.com/docs/api-reference/chat/create)
