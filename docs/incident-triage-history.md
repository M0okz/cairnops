# Triage et historique des incidents

Les Incidents actifs se présentent d’abord sans acquittement, puis par gravité
décroissante et ancienneté. La même priorité s’applique côté serveur avant la
limite de la projection. La recherche et les filtres Ressource, Nature et Gravité
se combinent ; une Ressource secondaire ou une Preuve d’une Atteinte peut faire
correspondre un Incident sans en masquer les autres Atteintes. Les titres français
et anglais présentés dans l’interface sont recherchés avec le libellé original ;
le serveur utilise le même catalogue de présentation avant de limiter la page.

L’historique Résolus propose 7, 30 ou 90 jours calendaires, tout l’historique
conservé et une période personnalisée. La date utilisée est celle de résolution.
Les jours sélectionnés sont inclus dans le fuseau du navigateur, même lors d’un
changement d’heure. Les résultats sont ordonnés par résolution décroissante et
chargés par pages de 50. Les listes de Ressources et de Natures couvrent tout
l’historique conservé, y compris les Ressources archivées.

`GET /api/v1/incidents?status=resolved&page=true` ajoute les paramètres optionnels
`target_id`, `nature_key`, `severity`, `q`, `resolved_from` (RFC3339 inclusif),
`resolved_before` (RFC3339 exclusif) et `cursor`. Le serveur applique les filtres
avant la limite et renvoie `next_cursor` s’il reste une page. Un curseur appartient
à ses filtres et conserve une borne de lecture ; une nouvelle recherche repart
sans curseur. La pagination repose sur `(resolved_at, id)` décroissant, sans
décalage ni duplication lorsqu’un Incident possède plusieurs Atteintes. Les
Incidents créés après la borne sont exclus même s’ils sont antidatés.

Les appels existants sans paramètres de pagination conservent leur réponse
`{ incidents: [...] }`. Les filtres d’Incidents actifs restent appliqués à la
projection partagée courante, bornée à 200 Incidents ; l’historique paginé parcourt
quant à lui l’ensemble des Incidents résolus conservés. Les changements reçus en
temps réel proposent « Actualiser l’historique » et préservent les pages et le
curseur parcourus. Seule cette action ou un changement de filtres/période repart
de la première page ; charger la suite conserve toujours le même snapshot.

Vérifications : tri avant limite ; égalités de dates de résolution ; bornes de
date inclusives/exclusives ; filtre d’une Ressource secondaire ; archives ;
insertion antidatée pendant un parcours ; invalidation d’un curseur lorsque ses
filtres changent ; recherche littérale ; jours de changement d’heure ; contrat
HTTP historique des clients mobiles.
