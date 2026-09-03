---
status: accepted
---

# Partager les adapters des Connecteurs entre connexion guidée et Supervision

Chaque Connecteur officiel porte un adapter produit unique. Cet adapter satisfait plusieurs petites interfaces selon les capacités réelles du produit : découvrir les cibles pendant la connexion guidée, observer leur état pendant la Supervision et, seulement lorsque le produit le permet, lire des Indicateurs ou acquitter un Burst.

Le module de connexion guidée et le runtime de Supervision conservent leurs workflows respectifs. Ils possèdent notamment l'ouverture des secrets, les baux d'exécution et la persistance ; l'adapter ne possède que le dialogue avec le produit externe et sa traduction vers le langage CairnOps. Le catalogue reste fermé aux Connecteurs officiels et n'introduit ni interface universelle fourre-tout ni mécanisme générique de plugin, conformément aux ADR 0013 et 0014.

Un résultat d'observation distingue une lecture complète d'une lecture partielle. Seule une lecture complète peut conclure qu'une condition précédemment active a disparu ; en cas de réponse partielle, de délai dépassé ou de permissions insuffisantes, les éléments non vérifiés conservent leur état précédent et la synchronisation est signalée comme incomplète.

Les exécutions continues sont supervisées et isolées par famille de Connecteurs dans le worker. Une erreur propre à un produit interrompt uniquement la boucle concernée, qui se signale en échec et réessaie avec temporisation ; les autres Connecteurs, les contrôles natifs et les notifications continuent. Seule une défaillance d'une dépendance commune au worker peut arrêter l'ensemble du runtime.

La connexion guidée distingue la capacité principale de Supervision des capacités facultatives découvertes par l'adapter. Une capacité facultative indisponible, comme la lecture d'Indicateurs ou l'acquittement distant, est annoncée sans bloquer l'import ; la connexion est refusée seulement lorsque le produit ne permet pas une Supervision fiable des cibles sélectionnées.

La confirmation finale applique la sélection de manière atomique : toutes les cibles et leurs liaisons sont importées, ou aucune ne l'est. Un conflit ou une cible devenue indisponible annule l'ensemble de l'import et renvoie l'utilisateur à une sélection corrigée, sans laisser de Connecteur partiellement configuré.

Les rapprochements entre les objets découverts et les Cibles existantes restent des propositions accompagnées de leurs éléments de correspondance. CairnOps ne crée aucune association silencieuse à partir d'un nom, d'une adresse ou d'une autre ressemblance ; l'utilisateur confirme les propositions dans le récapitulatif final, éventuellement en une seule action pour l'ensemble de la sélection.

La connexion guidée privilégie l'amorçage automatique des accès permanents. L'utilisateur fournit l'autorisation temporaire la plus simple acceptée par le produit ; l'adapter crée un jeton dédié à CairnOps avec les permissions minimales, le vérifie, puis CairnOps chiffre et conserve uniquement ce jeton. Les identifiants temporaires, notamment le mot de passe, ne sont jamais persistés. Lorsque le produit ne permet pas cet amorçage, le parcours explique l'opération manuelle minimale et demande un jeton existant.

Argus constitue une exception explicite : il ne propose ni jeton ni gestion distante d'identités, seulement une authentification HTTP Basic configurée sur l'instance. CairnOps peut donc conserver, chiffré, un secret Basic technique dédié à Argus après l'avoir clairement annoncé ; il refuse d'utiliser à cette fin un mot de passe personnel ou administrateur.

Uptime Kuma constitue une seconde exception maîtrisée : son adapter peut utiliser l'interface Socket.IO interne, après contrôle de version, uniquement pour ouvrir une session temporaire et créer ou révoquer la clé dédiée à `/metrics`. La Supervision n'utilise jamais cette interface et reste fondée sur `/metrics` ; une incompatibilité future peut interrompre l'amorçage ou le nettoyage, mais pas un Connecteur déjà actif.

Lorsque les capacités officielles du produit l'autorisent, cet amorçage crée également une identité technique CairnOps et son rôle minimal plutôt que d'attacher le jeton permanent au compte personnel ou administrateur utilisé pour l'installation. Le récapitulatif indique les objets et permissions qui seront créés avant la confirmation ; l'adapter conserve les références nécessaires à leur gestion, jamais l'autorisation administrative temporaire.

CairnOps gère jusqu'à leur révocation les accès distants qu'il a lui-même créés. La suppression d'un Connecteur le désactive d'abord, puis demande à l'adapter de révoquer son jeton et de retirer son identité technique avant d'effacer l'état local. Ce nettoyage ne s'applique jamais automatiquement aux comptes ou aux jetons préexistants fournis par l'utilisateur.

Si le produit externe est inaccessible pendant ce nettoyage, le Connecteur reste désactivé dans un état de suppression en attente et le worker réessaie avec temporisation. L'utilisateur peut demander l'abandon définitif de l'état local après un avertissement explicite indiquant que les accès distants pourraient subsister ; cet abandon est tracé et ne prétend pas que la révocation a réussi.

Le parcours principal ne demande que l'adresse du produit et l'autorisation temporaire nécessaire à l'amorçage, puis masque la création du compte technique, du rôle et du jeton derrière une seule action de connexion. La fourniture d'un jeton existant reste disponible comme mode manuel avancé, ou comme repli explicite lorsque l'adapter établit que l'amorçage automatique n'est pas pris en charge.

La vérification TLS n'est jamais désactivée globalement. Lorsqu'un produit présente un certificat autosigné ou issu d'une autorité inconnue, le parcours expose son identité et son empreinte puis permet de l'approuver explicitement ; CairnOps épingle alors ce certificat précis. Toute modification ultérieure bloque les échanges concernés et demande une nouvelle approbation au lieu d'être acceptée silencieusement.

Après l'installation, une découverte complète inscrit automatiquement tout nouvel objet externe qui ne présente aucune correspondance plausible avec une Cible existante : CairnOps crée sa Cible, sa liaison et démarre sa Supervision. Si une correspondance plausible existe, la découverte reste en attente de confirmation afin de ne jamais rapprocher silencieusement deux identités potentiellement distinctes.

Lorsqu'une découverte complète établit qu'un objet précédemment lié a disparu du produit externe, CairnOps conserve la Cible et son historique, marque la liaison comme introuvable et signale cette disparition comme une condition anormale. Cette absence ne vaut ni rétablissement ni autorisation de supprimer la Cible ; une lecture partielle ne peut jamais produire cette transition.
