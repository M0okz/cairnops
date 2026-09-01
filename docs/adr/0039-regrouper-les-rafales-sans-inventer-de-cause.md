---
status: accepted
---

# Regrouper les Rafales sans inventer de cause

Une même condition amont peut ouvrir presque simultanément des Incidents sur de
nombreuses Cibles. CairnOps les notifie aujourd'hui séparément : une dégradation
de stockage vue par Zabbix comme une latence disque sur plusieurs machines
produit autant d'entrées, de Push et de messages que de machines affectées. Ce
bruit masque l'étendue du phénomène sans apporter de preuve supplémentaire.

Cette répétition ne permet pourtant pas de nommer une cause. CairnOps ne connaît
ni Ceph, ni un OSD, ni une Dépendance de stockage tant qu'aucune Cible ni Source
ne les lui décrit. La réduire ne doit donc ni fusionner les Incidents, ni créer
un Événement opérationnel, ni transformer une proximité temporelle en causalité.

CairnOps introduit une Rafale d'Incidents, objet opérationnel durable distinct
d'un Incident. Deux Incidents suffisent à la former lorsqu'ils portent la même
Nature fiable, affectent des Cibles distinctes et surviennent pendant la même
Propagation. CairnOps décide seul de l'appartenance ; aucune validation,
configuration, fusion ou séparation manuelle n'est demandée aux utilisateurs.

## Établir une Nature fiable

L'identité d'une Nature est indépendante de la Cible et du libellé affiché. Un
Connecteur la produit dans l'une de deux portées :

- une Nature canonique CairnOps peut être partagée entre des types de Sources
  différents lorsqu'ils expriment réellement la même conclusion, par exemple
  l'indisponibilité ;
- une Nature dérivée d'un produit externe reste limitée à l'instance du
  Connecteur qui l'a établie.

Deux instances Zabbix ne rapprochent donc jamais leurs identifiants locaux et
ne forment pas une Rafale inter-Cibles sur la seule égalité d'une classification
qu'elles portent séparément. Une Nature canonique peut franchir la frontière
entre des types de Connecteurs différents, jamais servir à comparer sans autre
preuve deux instances distinctes d'un même type. Sur une même Cible, des Sources
explicitement rattachées qui portent une Nature canonique continuent néanmoins
d'alimenter un Incident unique, y compris si elles viennent de Zabbix et
d'Uptime Kuma ou de deux instances Zabbix. Leur désaccord produit une Divergence
de Sources, pas un second Incident.

Pour Zabbix, le Connecteur remonte automatiquement à l'identité racine du
trigger de template ou du prototype de découverte. Il utilise son UUID stable
lorsqu'il existe, sinon son identité racine dans la portée du Connecteur. Un
trigger créé directement sur un hôte reçoit une empreinte canonique de sa règle
non développée : expression, fonctions, clés d'items et tags, sans identifiant
d'hôte. Le nom rendu du problème n'est jamais une preuve de Nature à lui seul.
Les autres Connecteurs doivent de même fournir une identité sémantique stable ;
un signal qui ne le peut pas reste isolé plutôt que regroupé par supposition.

## Former et fermer la Rafale

Le premier Incident est notifié immédiatement comme aujourd'hui. Un second
Incident admissible crée la Rafale, y rattache le premier et transforme
silencieusement la notification intégrée déjà visible. Chaque nouvelle
appartenance prolonge une fenêtre glissante égale à deux cycles de la Source,
avec un minimum de soixante secondes et un maximum de cinq minutes. Une Source
sans cadence déclarée utilise le minimum. Lorsque plusieurs cadences
participent, la plus longue admissible évite qu'un Connecteur plus lent soit
écarté par construction.

La proximité est évaluée à partir des instants normalisés par le serveur et le
Connecteur ; un lot historique dont les heures d'ouverture contredisent la
Propagation courante n'est pas absorbé au seul motif qu'il a été reçu en une
fois. Une rechute sur une Cible pendant la fenêtre crée son véritable nouvel
Incident, mais rejoint la même Rafale sans augmenter le nombre de Cibles
distinctes. Après deux cycles sans nouvelle appartenance, la Propagation se
ferme et son périmètre devient immuable. Un Incident ultérieur commence une
nouvelle séquence même si un ancien membre reste actif.

La Rafale ne prend fin que lorsque sa Propagation est fermée et que tous ses
Incidents sont résolus. Un passage provisoire à zéro pendant la Propagation ne
produit donc pas une fausse Résolution suivie d'une nouvelle ouverture. Chaque
Incident conserve malgré tout son heure d'ouverture, sa Résolution, ses preuves
et son Journal d'activité exacts.

## Notifier l'évolution sans recréer la tempête

La Gravité de la Rafale est le maximum des Gravités effectives de ses Incidents
actifs. Une baisse actualise silencieusement l'affichage. Une hausse au-dessus
de la plus forte Gravité déjà notifiée produit une nouvelle alerte ; une baisse
puis un retour au même niveau ne sonne pas une seconde fois.

Une Rafale produit également au plus une alerte de Propagation étendue. Elle
devient étendue lorsqu'au moins cinq Cibles distinctes encore affectées
représentent au moins vingt pour cent des Cibles actives, ou lorsque vingt
Cibles sont affectées quelle que soit la taille de l'Espace opérationnel. Les
Cibles Sous maintenance sont exclues. Cette alerte signale une modification
matérielle de l'impact, pas une hausse de Gravité ni une cause commune.

Un Acquittement de Rafale acquitte individuellement ses membres actuels et tout
Incident qui la rejoint avant la fermeture de sa Propagation. Chaque action et
chaque synchronisation externe restent attachées à l'Incident concerné. Une
hausse de Gravité jamais notifiée ou la première Propagation étendue constitue
un nouveau fait opérationnel et peut donc être signalée malgré cet
Acquittement ; ce n'est ni une reprise de l'escalade acquittée, ni un Rappel.

Les Résolutions intermédiaires mettent seulement à jour le nombre d'Incidents
actifs. Une unique notification de Résolution part, vers les destinataires de
l'ouverture, après fermeture de la Propagation et Résolution de tous les
membres. Une Fenêtre de maintenance garde sa priorité : un Incident Sous
maintenance reste enregistré mais ne participe ni à l'appartenance notifiée,
ni au compteur, ni à la Gravité. À la fin de la maintenance, s'il demeure
actif, il rejoint uniquement une Propagation encore ouverte ; sinon il reprend
le parcours normal d'un nouvel Incident visible.

La boîte intégrée et le Push conservent une seule entrée actualisable. Les mises
à jour de contenu sont silencieuses ; l'ouverture, une hausse de Gravité et
l'unique Propagation étendue sont les seules transitions susceptibles
d'alerter. Le Connecteur Mattermost actuel reste fondé sur un webhook entrant
sans droit de modification : il publie l'ouverture immédiatement, aucun
message intermédiaire, puis une Résolution récapitulative donnant le maximum de
Cibles affectées. Son lien ouvre toujours l'état courant dans CairnOps.

## Présenter et auditer sans masquer les Incidents

La Vue d'ensemble, la boîte de notifications, le Push et la liste globale des
Incidents présentent une seule ligne de Rafale repliable. Elle montre sa Nature,
sa Gravité, le nombre d'Incidents actifs, le nombre total et distinct de Cibles,
son Acquittement et son éventuelle Propagation étendue. Le détail d'une Cible,
la recherche, les liens directs et les Journaux continuent de présenter les
Incidents individuels.

Le détail explique à la demande la décision avec une formulation lisible, par
exemple « même Nature issue du même prototype Zabbix, sept Cibles en trente-
quatre secondes ». Il n'affiche aucun score de confiance artificiel. L'identité
technique, les adhésions, la fermeture, les maxima, les notifications et
l'Acquittement sont conservés dans le Journal de la Rafale ; chaque Incident
reçoit une entrée d'appartenance. L'état persiste côté serveur afin que Web,
iOS, Android et les workers reprennent exactement le même cycle après un
redémarrage.

Les statistiques ne sont jamais dédupliquées : sept Incidents restent sept
Incidents, avec sept cycles et leurs effets propres sur les mesures. CairnOps
peut compter séparément une Rafale, mais elle ne réduit que la présentation et
les notifications.

Une Rafale peut plus tard être référencée par un Événement opérationnel si de
véritables Cibles, Sources et Dépendances fournissent un indice causal. Ce lien
n'efface pas la Rafale, ne réécrit pas son historique et ne lui attribue jamais
rétroactivement la cause proposée par l'Événement.

## Conséquences

Les Rafales exigent une identité de Nature correctement portée par chaque
Connecteur, un cycle persistant et idempotent, une boîte d'envoi capable de
router par Rafale et des contrats API partagés par les trois interfaces. Le
Connecteur Zabbix doit enrichir sa lecture des triggers ; le Connecteur Uptime
Kuma doit exprimer l'indisponibilité comme Nature canonique plutôt que comme
identité propre à un monitor. Les capacités requises sont vérifiées pendant la
connexion guidée sans introduire de taxonomie à administrer.

Le déploiement de cette décision ne regroupe pas rétroactivement les Incidents
historiques et ne renvoie aucune ancienne notification. Les Incidents déjà
actifs conservent leur cycle existant ; les nouvelles ouvertures utilisent le
nouveau modèle. Cette limite évite de réécrire l'histoire ou de produire une
tempête au moment même où la fonction destinée à l'empêcher est activée.

Le mécanisme est déterministe. Il n'emploie ni modèle probabiliste, ni
ressemblance de texte, ni connaissance implicite de l'infrastructure. Son
intelligence tient à des identités sémantiques fiables, un état partagé et des
règles de notification explicables.
