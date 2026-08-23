# Indicateurs contextuels

Les Indicateurs donnent assez de contexte pour le diagnostic quotidien sans
ouvrir systématiquement Zabbix, Uptime Kuma ou PatchMon. Ils ne remplacent ni
les tableaux de bord complets du produit d'origine, ni ses seuils, ni ses
alertes. Une valeur d'Indicateur ne modifie jamais la santé, la disponibilité
ou les Incidents calculés par CairnOps.

## Catalogue permis

Le catalogue volontairement court évite de transformer CairnOps en outil de
séries temporelles généraliste :

- Zabbix : utilisation CPU et mémoire, occupation des volumes confirmés,
  débits entrant et sortant des interfaces confirmées ;
- Uptime Kuma : temps de réponse, jours avant expiration et validité du
  certificat, seulement lorsque `/metrics` les publie ;
- PatchMon : mises à jour, correctifs de sécurité, redémarrage requis et âge de
  la dernière remontée.

La configuration conserve l'identifiant externe exact. Si un item Zabbix, un
monitor Kuma ou un hôte PatchMon disparaît, CairnOps affiche « À vérifier » et
demande une nouvelle confirmation. Il n'en choisit jamais un autre à partir du
seul nom.

## Configuration et portée

Un Administrateur ouvre **Indicateurs** depuis la fiche du Connecteur. La grande
modale redécouvre les capacités, propose une présélection, puis attend une
confirmation explicite. Les profils nommés facilitent une sélection répétée,
mais ne s'appliquent pas automatiquement aux nouvelles Cibles. Une Cible
nouvellement découverte reste hors périmètre jusqu'à sa confirmation.

Cette portée ne commande que les Indicateurs. Elle est indépendante de
l'import opérationnel du Connecteur : un périmètre contextuel seul ne crée pas
de Source d'Incident, et sa désactivation n'arrête pas les Incidents déjà
synchronisés.

La sauvegarde applique le périmètre, les sélections et les profils dans une
seule transaction. L'historique local de la fiche indique qui a modifié quoi et
quand. La configuration complexe reste Web ; la consultation utilise le même
contrat sur le Web, iOS et un futur client Android.

## Collecte et rétention

La collecte s'effectue au plus une fois par minute. Les points détaillés sont
conservés vingt-quatre heures, puis consolidés par heure jusqu'à sept jours. Ils
expirent ensuite automatiquement.

Désactiver un périmètre ou un Indicateur arrête la collecte et le masque
immédiatement. Les points déjà enregistrés ne sont pas supprimés sur-le-champ :
ils expirent naturellement selon les durées ci-dessus. Cette propriété évite
qu'une simple erreur de configuration ne devienne un effacement irréversible.

## Affichage contextuel

La fiche d'une Cible affiche tous ses Indicateurs. Chaque personne peut en
épingler jusqu'à quatre sur sa vue d'ensemble ; ce choix est enregistré côté
serveur et suit donc son compte sur mobile. La liste compacte n'en montre qu'un
par Cible.

La fiche d'un Incident affiche les valeurs contemporaines de son ouverture et
une courte courbe environnante. La mention « corrélation temporelle uniquement »
reste visible : ce rapprochement aide le diagnostic, mais ne prouve pas une
cause.

Une panne de la seule capacité Indicateurs n'abaisse pas artificiellement le
Connecteur entier. L'interface distingue par exemple « Incidents synchronisés ·
Indicateurs indisponibles » et conserve la dernière valeur avec sa date.
