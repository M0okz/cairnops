---
status: accepted
---

# Une Intégration produit des Observations, pas seulement des Incidents

Le langage de CairnOps dit qu'une Source de signal est « une sonde exécutée par
CairnOps ou une intégration externe » qui « produit des observations sur son
état ». Le code, lui, ne connaissait les Sources d'une Intégration que comme un
compteur de liaisons : elles ouvraient des Incidents et rien d'autre. Une
installation entièrement alimentée par un Connecteur n'avait donc ni
Disponibilité, ni Couverture, ni latence — les colonnes restaient vides sans que
rien n'explique pourquoi.

Une liaison de Connecteur porte désormais une véritable Source de signal, dotée
d'une origine. Un Contrôle natif est exécuté par le worker ; une Source
d'Intégration ne l'est jamais, mais elle vit dans la même table, produit des
Observations dans la même table, et se mesure exactement par les mêmes agrégats
horaires que l'[ADR 0029](0029-mesurer-sur-des-agregats-horaires.md). Une seule
machinerie de mesure, quelle que soit la provenance de la preuve.

Chaque cycle de synchronisation enregistre une Observation par Source liée. Pour
Uptime Kuma, DOWN conclut à l'indisponibilité, UP à la disponibilité avec le
temps de réponse publié par le produit, tandis que PENDING et MAINTENANCE ne
concluent rien — ils restent neutres ici comme partout ailleurs dans le
Connecteur, et font seulement baisser la Couverture. La cadence de
synchronisation du Connecteur tient lieu d'Observations attendues.

Ces Observations n'entrent pas dans la Politique de déclenchement. L'Incident
d'une Intégration est décidé par le rapprochement de ses propres signaux, qui
sait des choses qu'une suite d'Observations ignore : identité de l'événement
distant, gravité d'origine, acquittement amont. Confronter en plus la Source à
un seuil dédoublerait la décision. La mesure observe ; l'Intégration conclut.

La mesure ne commande pas la synchronisation. Une Observation qui ne s'enregistre
pas laisse un trou dans la Couverture — ce qui est exactement ce qu'un trou doit
faire — mais ne dégrade pas le Connecteur et ne réécrit pas un Incident déjà
rapproché.

Un Signal entrant poussé par un webhook générique ne promet aucune cadence :
personne ne lui a demandé de parler à un rythme donné. Rien n'y est donc attendu,
et sa Couverture reste absente plutôt que nulle. Une Source dont le Connecteur
est suspendu cesse pareillement d'être attendue : suspendre n'efface rien, mais
n'accuse pas non plus un silence que l'on a soi-même demandé.
