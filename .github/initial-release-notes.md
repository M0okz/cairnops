Première publication versionnée de CairnOps. Cette version pose les fondations de la supervision continue, du traitement partagé des incidents et du déploiement autonome.

## Supervision et incidents

- Supervision native HTTP/HTTPS, TCP, ICMP, DNS et Heartbeat avec politiques de déclenchement configurables.
- Incidents partagés en temps réel, avec acquittement, assignation, requalification, invalidation motivée et journal d’activité.
- Disponibilité, Couverture et latence suivies sur 24 heures, 7 jours et 30 jours.
- Fenêtres de maintenance et notifications intégrées, Push et Mattermost.

## Connecteurs et contexte opérationnel

- Connecteurs guidés pour Zabbix, Uptime Kuma, PatchMon et Argus, ainsi qu’un webhook générique signé.
- Import contrôlé des Cibles découvertes et rapprochement explicable sans perte d’historique.
- Indicateurs contextuels pour comprendre la posture d’une Cible sans altérer son État de santé.

## Interfaces et accès

- Interface Web adaptative pour administrer l’Espace opérationnel et suivre les Incidents.
- Comptes Administrateur, Opérateur et Observateur, sessions révocables et rétablissement d’accès sécurisé.
- Association d’appareils mobiles et transport Push chiffré de bout en bout via le Relais CairnOps.

## Exploitation

- Images Docker séparées pour le serveur, le worker et le Relais Push.
- Migrations appliquées par le serveur avant l’ouverture du service HTTP, contrôles de santé et identification précise du commit déployé.
- Secrets de Connecteurs chiffrés, jetons entrants non conservés en clair et composants exécutés sans privilèges inutiles.
