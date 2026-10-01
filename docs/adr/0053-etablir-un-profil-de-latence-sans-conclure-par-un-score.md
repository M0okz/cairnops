---
status: accepted
---

# Établir un profil de latence sans conclure par un score

Les Contrôles natifs mesurent la latence de chaque Observation depuis l'origine,
mais elle ne conclut rien : seuls l'échec du transport, le statut et le contenu
décident de la conclusion d'une Observation. Une Cible qui répond en trois
secondes au lieu de quatre-vingts millisecondes reste donc Opérationnelle, alors
que « des réponses lentes » figure parmi les Atteintes que CairnOps dit savoir
décrire. La latence est mesurée, affichée, et sans effet.

Un seuil fixe par Source ne comble pas ce manque. Il demande un réglage manuel
pour chaque Source, il ignore que la latence habituelle d'un service dépend de
l'heure, et il oblige l'Administrateur à connaître d'avance ce que CairnOps
observe déjà. CairnOps établit donc, pour chaque Source de Contrôle natif, un
Profil de latence : la latence habituelle de cette Source, heure par heure,
apprise sur ses propres Observations.

## Ce que le Profil apprend, et sur quoi

Le Profil est calculé sur une fenêtre glissante d'Observations récentes et ne
retient que les Observations **saines**. Une Observation défavorable porte le
délai écoulé avant son échec, souvent le délai d'attente entier : l'inclure
apprendrait à CairnOps qu'un service indisponible est un service lent. Une
Observation Inconnue ne mesure rien. Le Profil décrit donc la latence d'un
service qui fonctionne, ce qui est précisément ce qu'une réponse lente
contredit.

Il retient par heure la latence médiane et deux quantiles hauts, plutôt qu'une
moyenne et un écart type. Une moyenne suit les valeurs extrêmes qu'elle est
censée aider à reconnaître ; la médiane et les quantiles résistent à quelques
Observations aberrantes sans qu'il faille les écarter au préalable. Les heures
sont celles d'UTC, comme tout horodatage stocké : le cycle quotidien d'un
service se retrouve dans les mêmes seaux d'un jour à l'autre, quel que soit le
fuseau d'affichage.

## Ce qu'il ne fait pas

Le Profil est un artefact persisté et consultable, pas un modèle opaque. Il
porte sa fenêtre, son nombre d'Observations et sa date de calcul, il se relit
tel quel dans l'interface, et un même historique produit toujours le même
Profil. Il n'appelle aucun service extérieur et n'emploie ni modèle génératif,
ni ressemblance de texte.

Il ne conclut rien par lui-même. À cette étape, il ne produit ni Observation, ni
Atteinte, ni Incident, ni notification : il se calcule et se montre, afin que sa
justesse soit vérifiable sur une installation réelle avant qu'une décision s'y
appuie. La règle de dérivation qui en découle appartient au domaine, comme celle
de la Disponibilité et de la Couverture : elle traduit un seau du Profil en un
seuil au-delà duquel une latence cesse d'être habituelle, et reste une fonction
pure, explicable et testable.

Ce seuil n'est pas le quantile haut seul. Une Source très régulière verrait
sinon une variation ordinaire le franchir, puisque son quantile haut est proche
de sa médiane. Le seuil exige donc aussi un écart franc à la médiane et une
marge absolue : une réponse lente doit l'être assez pour qu'un Opérateur le
reconnaisse. Aucune de ces bornes n'est apprise ; elles sont nommées dans le
domaine et se lisent dans le Profil.

## Ce qu'un Profil insuffisant vaut

Un seau qui ne réunit pas assez d'Observations ne fournit pas de seuil. Le seau
de l'heure concernée est utilisé lorsqu'il est assez fourni ; sinon, celui qui
réunit toutes les heures de la fenêtre, ce qui laisse une Source à cadence
lente disposer d'un Profil sans attendre que chaque heure se remplisse. Si ni
l'un ni l'autre n'y parvient, le Profil reste absent pour cette Observation et
rien n'est conclu. Une mesure absente vaut mieux qu'une mesure inventée, et
l'absence de preuve n'établit jamais qu'une latence est normale.

Le Profil n'est jamais présenté comme un score de confiance, ni converti en
Gravité, ni comparé entre Sources. Il se lit en latences, dans l'unité que
l'Opérateur voit déjà partout ailleurs, et l'explication d'un futur
déclenchement se formule en Observations : une latence, la latence habituelle de
cette heure, et le nombre d'Observations consécutives qui l'ont dépassée.

## Conséquences

Le worker porte une passe de calcul supplémentaire, bornée par passe comme la
consolidation des Observations, qui reprend les Sources dont le Profil a vieilli.
Elle n'observe rien et ne retarde ni l'ordonnanceur, ni la projection des
Incidents. Le calcul lit les Observations brutes : sa fenêtre reste donc
inférieure à leur rétention, et une rétention raccourcie réduit la profondeur du
Profil sans le rendre faux.

Ce Profil ouvre la voie à l'Atteinte « réponses lentes », qui s'appuiera sur la
Politique de déclenchement existante — un nombre d'Observations consécutives
au-delà du seuil — plutôt que sur une conclusion tirée d'une seule Observation.
Cette étape reste à décider séparément, activable par Source et inactive par
défaut : elle change ce que CairnOps affirme, alors que le Profil ne fait que
décrire ce qu'il a vu.

## Évaluation des anomalies de latence

L'étape suivante utilise le Profil comme modèle non supervisé et explicable.
Chaque calcul apprend les quantiles sur les 28 jours précédant la dernière
journée ; les Observations saines de cette dernière journée restent hors de
l'apprentissage. Une Observation dont la latence dépasse le seuil du seau de
son heure devient une **candidate à examiner**. Le seuil conserve la marge
proportionnelle et absolue définie ci-dessus. Aucun score opaque n'est créé.

La fiche de la Ressource montre les vingt candidates les plus récentes avec
leur mesure, leur médiane habituelle et leur seuil. La lecture des Observations
est bornée à 5 000 par fiche et signale si elle a été tronquée. Un Profil absent
ou insuffisant ne produit aucune candidate ; une erreur de lecture est affichée
comme une évaluation indisponible, jamais comme une absence d'anomalie.

Cette évaluation reste informative. Elle ne change pas le verdict du Contrôle,
l'État de santé, les Incidents ou les notifications. Elle permet de comparer
les candidates à l'expérience de l'Opérateur avant de décider séparément si une
séquence de réponses lentes doit devenir une Atteinte.

## Extension au temps de réponse Uptime Kuma

Le temps de réponse importé d'Uptime Kuma est un Indicateur contextuel, et non
une Observation d'un Contrôle natif. Sa série détaillée ne dure que 24 heures ;
les six jours précédents ne sont disponibles que sous forme d'une dernière
valeur par heure. Sa carte établit donc un repère distinct sur au moins trente
heures de ces valeurs, sans reprendre le Profil natif de 28 jours. La dernière
valeur, tenue hors apprentissage avec toute la journée récente, est comparée
au plus grand du 99e centile, du double de la médiane et de la médiane plus
50 ms. Le collecteur ne retient cette mesure que lorsque le monitor annonce
un état disponible. Une mesure périmée, absente ou nulle ne reçoit aucune
conclusion.

Ce calcul de présentation utilise la projection hebdomadaire déjà servie à la
fiche de la Ressource. Il n'ajoute ni rétention, ni collecte, ni appel à Uptime
Kuma. Il montre une candidate à examiner avec ses valeurs explicatives ; il ne
change pas l'Indicateur contextuel en Source de signal et ne crée aucun
Incident ou notification. Le repère plus court et échantillonné à l'heure ne
prétend pas à la même précision que le Profil des Contrôles natifs.
