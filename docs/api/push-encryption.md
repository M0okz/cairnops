# Enveloppe Push CairnOps v1

Le compagnon génère une paire X25519 et transmet seulement sa clé publique de
32 octets pendant l'appairage. La clé privée reste dans le stockage sécurisé de
l'appareil. Toutes les valeurs binaires du protocole HTTP utilisent base64url
sans remplissage.

Pour chaque notification, l'instance :

1. génère une paire X25519 éphémère ;
2. calcule le secret partagé avec la clé publique de l'appareil ;
3. dérive 32 octets par HKDF-SHA-256, sans sel, avec les octets UTF-8
   `cairnops-push-envelope-v1` comme information ;
4. chiffre le JSON avec XChaCha20-Poly1305 et un nonce aléatoire de 24 octets ;
5. utilise les mêmes octets `cairnops-push-envelope-v1` comme données associées.

L'enveloppe transporte la clé publique éphémère, le nonce et le texte chiffré.
Le JSON déchiffré contient `version`, `event_kind`, `incident_id`, `severity`,
`occurred_at`, `instance_url`, `unread_count` et `presentation`, ainsi que
`acknowledged` lorsque l'Incident actif est acquitté. `unread_count` est le
nombre d'entrées non lues de la boîte du destinataire au moment de l'envoi ; le
compagnon s'en sert comme pastille. La présentation porte seulement
`title` et `body` ; son niveau de détail dépend du mode `complete`, `discreet` ou
`masked` enregistré pour l'appareil.

En mode `complete`, le rendu est commun à la boîte intégrée et à Mattermost.
`title` porte une pastille de gravité, puis « Ressource · gravité » (ou le nombre
de Ressources, et « résolu » pour une Résolution). `body` porte le problème,
puis, après un saut de ligne, le contexte connu lors de la livraison :
hausse de gravité, durée, Ressources, détail et Intégrations d'origine. Le
compagnon affiche ces deux champs tels quels.
Les textes `discreet` et `masked` ne révèlent ni Cible, ni Nature, ni Gravité.

Le Relais Push ne participe à aucune de ces opérations cryptographiques. Il
reçoit l'enveloppe telle quelle et la remet à APNs ou FCM pour le destinataire
opaque indiqué par l'instance.

Une priorité `high` produit une alerte visible. Une priorité `normal` reste une
mise à jour d'état silencieuse : sur APNs, le Relais emploie une notification
d'arrière-plan sans `alert` ni `sound`. Les révisions ordinaires d'un Incident et
les acquittements utilisent cette voie ; l'ouverture, une hausse de Gravité
encore jamais notifiée ou la première Propagation étendue interrompent
l'utilisateur. Une Résolution est visible sur chaque appareil qui a reçu
l'ouverture en alerte, afin de la remplacer par le même identifiant de
regroupement ; le compagnon décide ensuite, selon son réglage de son de
rétablissement, si elle sonne ou s'ajoute discrètement
([ADR 0051](../adr/0051-remplacer-l-ouverture-par-sa-resolution-sur-l-appareil.md)).
