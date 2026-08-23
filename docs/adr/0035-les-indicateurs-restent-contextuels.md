---
status: accepted
---

# Les Indicateurs restent contextuels et bornés

CairnOps doit permettre l'usage quotidien sans imposer l'ouverture de Zabbix,
Uptime Kuma ou PatchMon pour chaque diagnostic. Il ne devient pas pour autant
un second moteur de seuils ni un entrepôt de séries temporelles. Un Indicateur
contextuel est donc distinct d'une Source de signal et d'une Observation : il
explique une situation, mais ne décide jamais de l'État de santé, de la
Disponibilité ou de l'ouverture d'un Incident.

L'Administrateur choisit un catalogue sémantique court. Zabbix fournit CPU,
mémoire, occupation des volumes et débits réseau ; Uptime Kuma fournit temps
de réponse et certificat lorsqu'ils sont exposés ; PatchMon fournit mises à
jour, correctifs de sécurité, redémarrage et fraîcheur de remontée. La
correspondance conserve toujours l'identifiant externe exact. Une disparition
rend l'Indicateur périmé et demande une nouvelle confirmation : aucun
remplacement silencieux n'est permis.

La collecte est au plus minute. Les points détaillés expirent après vingt-quatre
heures ; des agrégats horaires permettent une vue compacte de sept jours, puis
expirent eux aussi. Désactiver un Indicateur arrête et masque sa collecte sans
effacer immédiatement les points existants. Un Incident peut conserver un seul
instantané des valeurs contemporaines de son ouverture, explicitement présenté
comme corrélation temporelle et jamais comme cause.

La configuration et son coût sont globaux à l'Espace opérationnel. Les
épingles sont personnelles et limitées à quatre. Les applications mobiles
consultent les mêmes projections que le Web, mais l'administration complexe des
Connecteurs reste réservée au Web.

Le périmètre d'Indicateurs et le périmètre opérationnel d'un Connecteur sont
stockés séparément. Activer un hôte uniquement pour son contexte ne l'autorise
pas à ouvrir des Incidents ; désactiver ses Indicateurs ne suspend ni ses
signaux, ni ses Observations, ni la synchronisation des acquittements.
