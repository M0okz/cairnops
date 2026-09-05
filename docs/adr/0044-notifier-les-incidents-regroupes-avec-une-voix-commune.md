---
status: accepted
---

# Notifier les Incidents regroupés avec une voix commune

Le cycle commun d'Atteintes et de Preuves de l'ADR 0042 reste l'autorité du
regroupement. Les Connecteurs remettent des faits normalisés ; ils ne rédigent
pas les notifications. Une seule Source peut ouvrir un Incident. Un manque
d'enrichissement ne bloque jamais l'ouverture ni le signalement.

Le module `internal/synthesis` rend les mêmes faits pour la boîte intégrée,
le détail Web, le Push et Mattermost : Nature constatée, Cibles distinctes
concernées et Gravité. Une aggravation change donc aussi le texte visible.
Le détail conserve le libellé original et les Preuves par Atteinte.
Les messages ne contiennent ni rapport périodique, ni conseil, ni cause supposée.
La Résolution indique le nombre maximal de Cibles concernées ; un simple
passage à zéro pendant la Propagation ne dit pas que l'Incident est résolu.

Le premier catalogue canonique distingue l'indisponibilité, la latence de
stockage, l'espace insuffisant, l'échec de sauvegarde, l'ancienneté excessive
d'une sauvegarde et l'expiration proche d'un certificat. Il définit des sens,
pas de nouveaux seuils de supervision. Zabbix peut les déclarer par le tag
réservé `cairnops.nature` d'un trigger ou de son ancêtre. Une clé inconnue ou
ambiguë conserve une Nature locale. Aucun rapprochement ne dépend de mots
comme « disque », d'un libellé ressemblant ou d'une Gravité commune.

La latence des templates Linux officiels Zabbix 7.0 (agent actif et passif)
est reconnue automatiquement par l'UUID du prototype et sa condition exacte
sur les temps d'attente en lecture/écriture. Un template modifié, un ancêtre
inaccessible ou une condition non reconnue reste local. La règle est issue des
[templates Linux officiels](https://github.com/zabbix/zabbix/tree/release/7.0/templates/os)
et ne permet pas de conclure à la saturation d'un datastore ou à une cause commune.

Les libellés de Natures locales sont présentés comme des signalements, sans
être promus au rang de conclusions canoniques. Une intégration ultérieure
pourra exploiter le catalogue si ses données établissent le même sens.
Proxmox VE, Proxmox Backup Server et Checkmk ne sont pas livrés par ce chantier.

La décision de notification est commune à tous les Canaux : ouverture après
le sas de stabilité existant (immédiate si Critique), Gravité supérieure à
toutes celles déjà notifiées, première Propagation étendue. Les autres
révisions actualisent silencieusement la boîte intégrée et le Push ; elles
ne créent pas de message Mattermost. Mattermost conserve un bref message de
Résolution vers le Canal qui a reçu l'ouverture. Aucun rappel automatique
n'est ajouté.

Les reprises de livraison gardent leur temporisation exponentielle, y compris
lorsque plusieurs révisions remplacent successivement un message en échec.
Une révision silencieuse conserve l'intention interruptive d'un Push encore
non livré, sauf si l'Incident s'est résolu. Un Push en cours de transmission
termine avant son remplacement. Une aggravation survenue entièrement Sous
maintenance reste à notifier à sa sortie si elle demeure active ; le détail
Web continue d'en montrer les Preuves pendant la maintenance.
Une ouverture annulée lors
d'un passage transitoire sans Atteinte ou d'une maintenance peut redevenir
éligible si le même Incident redevient actif et non acquitté.

La plus longue cadence admissible gouverne chaque prolongation de la
Propagation, même quand une Source plus rapide arrive ensuite. La durée de
tolérance reste bornée de 60 à 300 secondes ; la durée totale d'une Propagation
glissante peut dépasser cinq minutes tant que de nouvelles Atteintes arrivent.

Les notions d'Assurance et de divergence de l'ADR 0043 ne sont pas inventées à
partir du seul nombre de Preuves. Leur qualification exige encore de décrire
l'indépendance des Sources et leurs observations saines et fraîches. Ce
chantier livre la formulation minimale et la conservation de `last_seen_at`,
sans afficher une corroboration ni un diagnostic que les données ne prouvent pas.

Les scénarios de regroupement et de livraison s'exécutent sur PostgreSQL en CI.
Ils doivent couvrir le rejeu, les cadences différentes, les lectures partielles,
le maintien d'un Incident soutenu par une autre Source et l'absence de rappels.
