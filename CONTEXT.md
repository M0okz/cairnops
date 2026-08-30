# CairnOps

CairnOps centralise la supervision continue de cibles et la coordination des incidents depuis plusieurs interfaces partageant le même état.

## Langage

**Espace opérationnel** :
Ensemble isolé des Cibles, Sources de signal, Incidents, utilisateurs et appareils appartenant à une unique installation CairnOps. Une installation ne contient qu'un Espace opérationnel.
_Éviter_ : Tenant, organisation, workspace

**Utilisateur** :
Personne autorisée à accéder à l'Espace opérationnel, porteuse d'un rôle, d'un historique d'actions et de préférences propres. Ses moyens d'authentification peuvent évoluer sans changer son identité CairnOps.
_Éviter_ : Compte OIDC, Session

**Fournisseur d'identité** :
Service externe configuré auquel CairnOps délègue l'authentification de certaines Identités externes par OpenID Connect. Ses Groupes d'accès externes ne déterminent l'accès et le rôle CairnOps qu'après leur mise en correspondance explicite par un Administrateur.
_Éviter_ : IdP, annuaire CairnOps, autorité des rôles

**Identité externe** :
Identité reconnue par le Fournisseur d'identité et associable à un Utilisateur comme moyen de connexion. Elle ne crée pas à elle seule un Utilisateur et ne porte aucun rôle CairnOps.
_Éviter_ : Utilisateur, Compte OIDC

**Groupe d'accès externe** :
Groupe déclaré par le Fournisseur d'identité et explicitement mis en correspondance avec un rôle CairnOps. Une appartenance reconnue autorise le premier accès ; lorsque plusieurs correspondances s'appliquent, la plus puissante détermine le rôle.
_Éviter_ : Groupe de notification, rôle CairnOps

**Régime d'autorisation** :
Autorité exclusive qui gouverne l'accès et le rôle d'un Utilisateur : CairnOps en régime local, ou les Groupes d'accès externes en régime externe. Un Utilisateur ne relève jamais simultanément des deux.
_Éviter_ : Méthode de connexion, double autorité

**Désactivation d'un Utilisateur** :
Décision administrative qui retire tout accès à un Utilisateur sans effacer son identité ni son historique. Elle reste prioritaire sur son Régime d'autorisation et seule une nouvelle décision administrative peut la lever.
_Éviter_ : Suppression, Suspension d'accès externe

**Suspension d'accès externe** :
Retrait automatique et réversible de l'accès d'un Utilisateur externe lorsque ses groupes ne l'autorisent plus ou que leur vérification a dépassé sa grâce. Elle préserve son identité, son historique et ses appareils, et ne peut jamais annuler une Désactivation d'Utilisateur.
_Éviter_ : Désactivation d'un Utilisateur, révocation d'un appareil

**Administrateur** :
Utilisateur habilité à configurer l'Espace opérationnel, ses utilisateurs, appareils, intégrations, Cibles et Sources de signal. Il possède également tous les pouvoirs d'un Opérateur.
_Éviter_ : Super-utilisateur, propriétaire

**Administrateur local de secours** :
Administrateur actif qui dispose d'un moyen de connexion local indépendant du Fournisseur d'identité. L'Espace opérationnel doit toujours en conserver au moins un.
_Éviter_ : Jeton d'amorçage, Administrateur du fournisseur

**Jeton d'amorçage** :
Secret temporaire défini lors du déploiement qui autorise exclusivement l'initialisation sécurisée du premier Administrateur et des réglages indispensables. Il cesse d'être un moyen d'accès dès que l'Espace opérationnel est initialisé.
_Éviter_ : Compte Administrateur, jeton API permanent, mot de passe par défaut

**Session** :
Accès authentifié, temporaire et révocable d'un utilisateur à une interface CairnOps. Son secret reste propre au client, n'est jamais enregistré en clair par le serveur et ne constitue pas un Jeton d'amorçage.
_Éviter_ : Jeton API permanent, état partagé

**Opérateur** :
Utilisateur habilité à prendre en charge les Incidents, notamment par Acquittement, assignation, commentaire et Invalidation motivée d'une Source de signal.
_Éviter_ : Administrateur, intervenant

**Observateur** :
Utilisateur habilité uniquement à consulter l'état et l'historique de l'Espace opérationnel.
_Éviter_ : Invité, lecteur

**Supervision** :
Observation continue de ressources, centralisation de leurs signaux et suivi des incidents associés, sans action directe sur l'infrastructure supervisée.
_Éviter_ : Administration distante, pilotage d'infrastructure

**État partagé** :
État de référence maintenu par le serveur et projeté vers les interfaces Web, iOS et Android. Une copie locale ou une action en attente de synchronisation ne constitue pas l'état de référence.
_Éviter_ : État local, vérité du client

**Cible** :
Chose identifiable et durable dont l'état opérationnel importe à l'utilisateur, par exemple un service, un équipement ou une tâche planifiée. Une Cible peut être observée par plusieurs Sources de signal ; son identité, son historique et son cycle de vie appartiennent à CairnOps, même lorsqu'elle a été découverte par une Intégration.
_Éviter_ : Monitor, moniteur, ressource

**Suggestion de rapprochement** :
Proposition non contraignante selon laquelle deux Cibles pourraient représenter la même chose. Elle ne modifie leur identité ni leur historique tant qu'un utilisateur ne l'a pas confirmée.
_Éviter_ : Fusion automatique, déduplication automatique

**Source de signal** :
Mécanisme rattaché à une Cible qui produit des observations sur son état, qu'il s'agisse d'une sonde exécutée par CairnOps ou d'une intégration externe.
_Éviter_ : Monitor, fournisseur

**Intégration** :
Connexion configurée à un système externe qui peut découvrir des Cibles, créer des Sources de signal ou transmettre des Observations. Elle conserve la propriété de ses Sources importées et de leurs liens externes, mais jamais celle des Cibles CairnOps auxquelles elles sont rattachées.
_Éviter_ : Fournisseur, monitor

**Connecteur** :
Adaptateur sur mesure fourni par CairnOps pour créer et exploiter simplement une Intégration avec un produit donné. Il encapsule l'autorisation initiale, la validation de compatibilité et de sécurité, la découverte et la configuration spécifique au produit.
_Éviter_ : Intégration, configuration manuelle

**Cible découverte** :
Entité trouvée dans l'inventaire d'une Intégration et présentée à un Administrateur avant son import comme Cible. Plusieurs Cibles découvertes peuvent être importées en une seule validation, mais jamais silencieusement.
_Éviter_ : Cible active, import automatique

**Signal à rapprocher** :
Signal entrant muni d'une identité externe stable mais ne pouvant pas encore être rattaché avec certitude à une Cible. Il reste hors du tableau opérationnel et ne déclenche aucune escalade jusqu'à sa création ou son rapprochement par un Administrateur ; ses signaux conservés sont alors rattachés rétroactivement, sans notifier un problème déjà terminé.
_Éviter_ : Cible, Incident actif

**Contrôle natif** :
Source de signal interrogée ou exécutée périodiquement par CairnOps depuis son propre point de vue réseau.
_Éviter_ : Script, agent

**Signal entrant** :
Source de signal dont les Observations sont envoyées à CairnOps par un système externe, notamment par intégration ou webhook.
_Éviter_ : Contrôle natif, notification

**Relais Push** :
Service officiel extérieur à l'installation qui transmet aux services de notification mobiles un message chiffré par l'instance pour un appareil donné. Il peut également constater l'expiration d'un Bail de présence opaque, mais ne détient ni l'état opérationnel, ni un accès à l'instance, ni la capacité de lire le contenu transmis.
_Éviter_ : Serveur CairnOps, source de vérité

**Bail de présence** :
Preuve opaque renouvelée périodiquement par une instance auprès du Relais Push. Son expiration déclenche la livraison d'un message préalablement chiffré indiquant seulement que l'instance ne donne plus signe de vie.
_Éviter_ : Contrôle de Cible, accès distant

**Canal de notification** :
Destination configurée par laquelle CairnOps informe un utilisateur ou un système externe d'un événement opérationnel, notamment dans l'application, par Push, navigateur, courrier électronique, webhook signé ou Mattermost.
_Éviter_ : Source de signal, Intégration entrante

**Politique de notification** :
Suite d'étapes temporisées qui sélectionne les destinataires et Canaux de notification selon les caractéristiques d'un Incident. La progression s'arrête à son Acquittement ou à sa Résolution et les destinataires déjà alertés sont informés du rétablissement.
_Éviter_ : Politique de déclenchement, rappel

**Groupe de notification** :
Ensemble statique d'utilisateurs pouvant être désigné comme destinataire d'une Politique de notification. Il ne représente ni une équipe d'astreinte, ni une rotation planifiée.
_Éviter_ : Astreinte, planning, rôle

**Rappel** :
Notification répétée explicitement configurée pour signaler qu'un Incident demeure actif, y compris après son Acquittement. Il ne constitue pas une étape supplémentaire d'escalade.
_Éviter_ : Escalade, nouvel Incident

**Heartbeat** :
Signal entrant périodique attestant qu'une tâche externe continue de fonctionner ; son absence au-delà de la tolérance configurée constitue une Observation défavorable.
_Éviter_ : Ping, ICMP

**Observation** :
Résultat daté produit par une Source de signal au sujet d'une Cible. Une observation isolée ne constitue pas nécessairement un changement d'état ni un Incident.
_Éviter_ : Incident, alerte

**Indicateur contextuel** :
Valeur numérique ou booléenne, explicitement sélectionnée depuis un Connecteur, qui aide à comprendre la situation d'une Cible sans participer à son État de santé, à sa Disponibilité ni à l'ouverture d'un Incident. CairnOps en conserve un historique court et borné ; le produit d'origine reste l'autorité sur ses seuils et ses alertes.
_Éviter_ : Source de signal, Observation, seuil CairnOps, cause d'Incident

**Divergence de Sources** :
Situation temporaire dans laquelle des Sources valides rattachées à une même Cible soutiennent des conclusions opposées. Elle qualifie les preuves sans constituer un État de santé supplémentaire ni bloquer un Incident.
_Éviter_ : État contradictoire, consensus

**Politique de déclenchement** :
Règle propre à une Source de signal qui détermine quand une suite d'Observations devient suffisamment certaine pour signaler une dégradation ou un rétablissement. L'absence d'Observation ne constitue jamais un rétablissement.
_Éviter_ : Vote, consensus des sources

**Incident** :
Situation opérationnelle d'une Nature donnée affectant une Cible, ouverte ou alimentée dès qu'une Source de signal satisfait sa Politique de déclenchement. Une Cible peut avoir plusieurs Incidents actifs de Natures différentes, mais au plus un Incident actif par Nature ; l'Incident reste actif tant qu'au moins une Source qui l'alimente demeure déclenchée et il est automatiquement résolu après confirmation du rétablissement par chacune d'elles.
_Éviter_ : Observation, panne d'une source

**Dépendance** :
Relation explicite indiquant que le fonctionnement d'une Cible dépend de celui d'une autre. Elle fournit un indice de causalité aux regroupements sans constituer une preuve certaine de cause.
_Éviter_ : Cause confirmée, parenté

**Événement opérationnel** :
Regroupement corrélé d'Incidents affectant plusieurs Cibles, destiné à représenter et notifier un phénomène commun sans fusionner les Incidents ni leurs preuves. CairnOps peut le créer provisoirement lorsque des Dépendances explicites et la proximité temporelle l'expliquent ; en l'absence de Dépendance, il ne fait que suggérer le regroupement, et sa cause reste probable tant qu'un Opérateur ne l'a pas confirmée.
_Éviter_ : Incident, cause certaine

**Nature d'incident** :
Catégorie stable du problème opérationnel rencontré par une Cible, telle qu'une indisponibilité ou une expiration TLS imminente. Elle détermine quelles Sources de signal alimentent le même Incident sans préjuger de sa cause profonde.
_Éviter_ : Cause, gravité, message d'alerte

**Gravité** :
Importance de l'impact associé à un Incident, qualifiée par ordre croissant comme Information, Avertissement, Majeur ou Critique. La Gravité source continue de refléter la Source de signal ; la Gravité effective utilisée par CairnOps peut être requalifiée par un Opérateur, avec justification et trace d'audit, jusqu'à la Résolution ou au retrait explicite de cette requalification.
_Éviter_ : État de santé, priorité, statut

**Acquittement** :
Déclaration traçable qu'un utilisateur a pris connaissance d'un Incident et le prend en charge. Elle peut provenir de CairnOps ou d'un système intégré ; sa propagation dans l'autre système est suivie séparément et un échec de synchronisation ne l'annule pas dans son système d'origine. Elle ne modifie ni l'état de la Cible, ni les preuves, ni les conditions de Résolution.
_Éviter_ : Résolution, fermeture

**Invalidation** :
Décision motivée et traçable d'écarter une Source de signal des preuves actives de l'Incident courant, notamment pour faux positif, défaillance ou doublon. La Source continue ses Observations, son prochain cycle sain met fin à l'Invalidation et un déclenchement ultérieur peut alimenter un nouvel Incident.
_Éviter_ : Acquittement, rétablissement, suspension

**Suspension** :
Action administrative qui empêche temporairement une Source de signal de participer à la supervision jusqu'à sa réactivation explicite. Si toutes les Sources d'une Cible sont suspendues, celle-ci devient Inconnue et reste incluse dans l'État global, sans calcul de disponibilité pendant cette période.
_Éviter_ : Invalidation, maintenance

**Archivage** :
Retrait volontaire d'une Cible de la supervision active et du calcul de l'État global, sans suppression de son historique. Il exige une action explicite d'un Administrateur.
_Éviter_ : Suspension, suppression

**Résolution** :
Fin d'un Incident lorsque plus aucune Source de signal valide qui l'alimente ne demeure déclenchée. Un utilisateur ne peut pas prononcer une Résolution contre des preuves encore actives.
_Éviter_ : Acquittement, fermeture manuelle

**Journal d'activité** :
Chronologie immuable des événements et décisions qui modifient le traitement d'un Incident, avec leur date, auteur ou origine et valeurs avant/après. Il explique notamment l'ouverture, la Résolution, les Acquittements, synchronisations, requalifications, Invalidations et maintenances, sans constituer un fil de commentaires.
_Éviter_ : Discussion, historique modifiable

**État de santé** :
Synthèse calculée des preuves récentes et des Incidents actifs d'une Cible : Opérationnelle, Dégradée, Indisponible ou Inconnue. Il ne peut pas être modifié directement par un utilisateur.
_Éviter_ : Statut manuel, état d'acquittement

**Disponibilité** :
Part du temps observable pendant laquelle une Cible n'est pas Indisponible. Les périodes Inconnues, suspendues ou Sous maintenance sont exclues du calcul et les valeurs propres aux Sources restent diagnostiques.
_Éviter_ : Couverture, moyenne des Sources

**Couverture** :
Part de la période demandée pour laquelle CairnOps possède assez de preuves valides pour calculer la Disponibilité d'une Cible. Elle accompagne toujours le pourcentage de Disponibilité.
_Éviter_ : Disponibilité, temps supervisé implicite

**Opérationnelle** :
État de santé d'une Cible disposant de preuves suffisamment récentes et d'aucun Incident actif.
_Éviter_ : UP, verte

**Dégradée** :
État de santé d'une Cible affectée par au moins un Incident actif qui n'établit pas son indisponibilité totale.
_Éviter_ : Warning, partiellement UP

**Indisponible** :
État de santé d'une Cible affectée par au moins un Incident actif d'indisponibilité.
_Éviter_ : DOWN, rouge

**Inconnue** :
État de santé d'une Cible pour laquelle aucune preuve valide n'est assez récente pour conclure.
_Éviter_ : Opérationnelle, sans Incident

**État global** :
Synthèse de toutes les Cibles actives de l'Espace opérationnel : Supervision non configurée s'il n'en existe aucune, Incident en cours si l'une est Indisponible, Services dégradés sinon si l'une est Dégradée, Supervision incomplète sinon si l'une est Inconnue, et Tout est opérationnel uniquement si elles sont toutes Opérationnelles.
_Éviter_ : Moyenne, absence d'Incident

**Supervision non configurée** :
État global d'un Espace opérationnel qui ne contient encore aucune Cible active et ne peut donc produire aucune conclusion sur son infrastructure.
_Éviter_ : Tout est opérationnel, Supervision incomplète

**Santé de CairnOps** :
Vue distincte de la capacité du serveur, des workers, du stockage, des Intégrations et du Relais Push à assurer la supervision. Elle ne crée aucune Cible artificielle.
_Éviter_ : État de santé d'une Cible, tableau de bord des Cibles

**Défaillance de supervision** :
Problème affectant un composant de CairnOps ou une Intégration plutôt qu'une Cible. Il peut rendre des Cibles Inconnues et placer l'État global en Supervision incomplète, mais ne résout ni ne duplique leurs Incidents.
_Éviter_ : Incident de Cible, indisponibilité d'une Cible

**Fenêtre de maintenance** :
Période ponctuelle ou récurrente appliquée à une ou plusieurs Cibles durant laquelle les Observations et Incidents restent enregistrés, mais n'affectent ni l'État global, ni la disponibilité, ni les notifications et escalades. Un Incident encore actif à son terme redevient immédiatement visible avec son heure réelle de début.
_Éviter_ : Pause des contrôles, suppression d'Incident

**Sous maintenance** :
Contexte d'un Incident survenu pendant une Fenêtre de maintenance, indiquant que son impact opérationnel est temporairement neutralisé sans altérer les preuves recueillies.
_Éviter_ : Résolu, ignoré
