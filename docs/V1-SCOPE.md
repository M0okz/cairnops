# CairnOps V1

La V1 valide un produit de supervision serveur self-hosted dont l'état opérationnel est partagé en temps réel entre le Web, iOS et Android.

## Critères de sortie

- Navigation commune structurée autour de Vue d'ensemble, Cibles, Incidents, Santé et Réglages ; l'administration complète reste centrée sur le Web.
- Vue d'ensemble orientée exceptions : État global et fraîcheur, Incidents non acquittés puis acquittés, Défaillances de supervision, Cibles problématiques, puis résumé replié des Cibles Opérationnelles.
- Déploiement de l'application par Docker Compose avec serveur, worker et PostgreSQL séparés.
- Création de Contrôles natifs HTTP/HTTPS, TCP, ICMP, DNS et Heartbeat exécutés en permanence par le worker.
- HTTP/HTTPS : GET, HEAD et POST, en-têtes et corps configurables avec secrets masqués, statuts acceptés, recherche texte ou expression régulière, redirections configurables, validation et échéance TLS, mesures DNS/connexion/TLS/premier octet/total, et taille de réponse strictement limitée.
- TCP : connexion à un hôte et port avec délai configurable, TLS et SNI facultatifs, validation du certificat, envoi unique facultatif, attente d'une réponse contenant une valeur et volume lu strictement limité.
- DNS : requêtes A, AAAA, CNAME, MX, TXT, NS, SRV, CAA et PTR via le résolveur système ou des résolveurs configurés, assertions de présence, absence, non-vacuité ou égalité sans ordre, mesure de durée, code de réponse et repli UDP vers TCP.
- Heartbeat : URL secrète régénérable, fréquence attendue et grâce configurables, activation après le premier signal ou confirmation explicite, horodatage autoritaire à la réception, et contenu facultatif limité à un statut, une durée et un court message.
- Support IPv4 et IPv6 à parité pour les Contrôles natifs, avec famille automatique ou forçable par Source et diagnostic distinct des erreurs de résolution, routage, connexion et protocole.
- Connexion guidée à Zabbix et Uptime Kuma, avec aperçu puis import en masse des Cibles découvertes.
- Réception de signaux par webhook générique et mise en quarantaine des identités inconnues.
- Regroupement des preuves de plusieurs Sources rattachées à une même Cible dans un Incident unique par Nature.
- Ouverture, évolution, résolution et historique cohérents d'un Incident sur le Web, iOS et Android.
- Acquittement depuis n'importe quelle interface avec propagation immédiate et synchronisation limitée vers Zabbix.
- Invalidation motivée d'une Source défectueuse afin qu'elle ne maintienne pas indéfiniment un Incident actif.
- Journal d'activité immuable retraçant les décisions et synchronisations affectant chaque Incident.
- Disponibilité, Couverture, état et latence sur 24 heures dans les listes, puis 7 et 30 jours dans le détail ; stockage temporel en UTC et affichage dans le fuseau de l'utilisateur.
- Fenêtres de maintenance immédiates ou planifiées, avec début et fin, appliquées à une ou plusieurs Cibles.
- Notifications intégrées, Push iOS/Android et Mattermost.
- Contenu Push configurable par utilisateur en mode Complet, Discret ou Masqué, avec Complet par défaut.
- Acquittement depuis une notification sur appareil déverrouillé, après contrôle du rôle, revalidation serveur et confirmation explicite du résultat ; sinon ouverture de l'Incident dans l'application.
- Routage immédiat des notifications selon la Gravité, arrêté par l'Acquittement, avec notification de Résolution aux mêmes destinataires.
- Vue Santé de CairnOps pour le serveur, le worker, PostgreSQL, les Connecteurs et le Push.
- Installation initiale guidée, compte Administrateur local de secours et OpenID Connect facultatif.
- Association d'un appareil par QR code à usage unique et courte durée, suivie d'une authentification navigateur et d'une confirmation Web ; saisie manuelle de l'URL disponible et identité propre révocable pour chaque appareil.
- Rôles globaux Administrateur, Opérateur et Observateur.
- Consultation mobile du dernier état connu hors ligne, sans mutation tant que l'instance n'est pas joignable.
- Interfaces française et anglaise, sélectionnées selon le système et modifiables par utilisateur ; formats locaux et messages externes conservés dans leur langue d'origine.
- Verrouillage local biométrique ou par code système facultatif et configurable par appareil, avec effacement du cache et des jetons à la déconnexion ou révocation.

## Non bloquant pour la V1

- Connecteurs guidés Prometheus Alertmanager et Grafana Alerting.
- Dépendances entre Cibles et corrélation inter-Cibles en Événements opérationnels.
- Fenêtres de maintenance récurrentes.
- Bail de présence externe détectant la disparition de l'instance.
- Groupes de notification statiques.
- Politiques de notification multi-étapes, rappels et ciblage avancé.
- Assignation des Incidents et fils de commentaires.
- Notifications Web, courrier électronique SMTP et webhook sortant signé.
- Page de statut publique.
- Agents de supervision distants et emplacements de mesure multiples.
- Administration distante, scripts arbitraires et plugins tiers.
- Astreintes, calendriers et rotations.
- Parité des écrans d'administration complexes sur mobile.
- Certification formelle WCAG 2.2 AA.
- Widgets iOS et Android, Live Activities et Dynamic Island, tuile Android, puis surfaces de montre selon l'usage observé.
- SLO, budgets d'erreur et rapports périodiques.
- Scénarios HTTP multi-étapes, navigateur synthétique et assertions JSON structurées.
- Dialogues TCP multi-étapes.
- Validation DNSSEC et mesure de propagation multi-résolveurs.
