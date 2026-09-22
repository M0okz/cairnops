# Supervision et opérations quotidiennes

## État de santé et ressources sans état connu

Le verdict de la vue d'ensemble conduit directement aux ressources sans état
connu lorsque la supervision est incomplète. Ce filtre est aussi disponible
dans la page Ressources. En présence d'une indisponibilité, un lien distinct
conserve l'accès aux ressources sans état connu.

La fraîcheur des observations et les dates de maintenance continuent d'être
évaluées même si les requêtes échouent. Une maintenance couvre les ressources
sélectionnées sans exiger qu'un incident existe. Les fenêtres connues sont
conservées en cas de réponse tronquée ou d'échec réseau ; les projections des
incidents servent alors de repli jusqu'à leur date de fin de maintenance.

## Triage et historique

Les incidents non acquittés passent avant les incidents acquittés, puis sont
classés par gravité décroissante et ancienneté. La recherche, les filtres et
l'historique paginé sont décrits dans [Triage et historique](incident-triage-history.md).

## Modifier une connexion existante

Dans Connecteurs, **Connexion** permet de modifier le nom, l'adresse ou les
accès d'une intégration Zabbix, Uptime Kuma, PatchMon, Argus ou Proxmox VE.
Les champs d'accès laissés vides conservent leur valeur enregistrée.

**Tester la connexion** vérifie les réglages avant de proposer leur
enregistrement. Toute modification du formulaire exige un nouveau test. Le
résultat du test expire après quinze minutes. Une synchronisation en cours
demande de réessayer l'enregistrement ; un changement concurrent de connexion
demande un nouveau test.

Cette opération conserve les ressources, les contrôles, les associations,
l'historique et une éventuelle suspension. Elle sert notamment au déplacement
de la même instance externe : elle ne réimporte pas son inventaire. Pour un
compte Proxmox géré par CairnOps, son adresse d'origine reste celle du nettoyage
lors d'une suppression ultérieure et apparaît dans la confirmation.

## Maintenances hebdomadaires

Une maintenance peut se répéter chaque semaine au même jour et à la même heure
locale, dans le fuseau choisi, avec une date de fin obligatoire située entre
une semaine et un an après son début. Chaque fenêtre récurrente dure au maximum
vingt-quatre heures. Les occurrences sont créées ensemble ; au changement
d'heure, une heure inexistante est sautée et une heure répétée utilise sa
première occurrence. La durée écoulée reste identique.

Une fenêtre active peut être prolongée de trente minutes. La prolongation est
liée à la fin affichée pour empêcher qu'une répétition de requête ou deux
opérateurs la prolongent deux fois par erreur. Elle ne peut pas empiéter sur la
fenêtre suivante de la série. L'annulation d'une fenêtre et celle de la série
sont des actions distinctes ; l'annulation de série concerne les fenêtres
actives et futures.

Voir [la décision de projection des maintenances](adr/0026-maintenance-projection.md).
