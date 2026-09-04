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
`occurred_at`, `instance_url` et `presentation`. La présentation porte seulement
`title` et `body` ; son niveau de détail dépend du mode `complete`, `discreet` ou
`masked` enregistré pour l'appareil.

Le Relais Push ne participe à aucune de ces opérations cryptographiques. Il
reçoit l'enveloppe telle quelle et la remet à APNs ou FCM pour le destinataire
opaque indiqué par l'instance.

Une priorité `high` produit une alerte visible. Une priorité `normal` reste une
mise à jour d'état silencieuse : sur APNs, le Relais emploie une notification
d'arrière-plan sans `alert` ni `sound`. Les révisions ordinaires d'un Incident et
les Résolutions utilisent cette voie ; seules l'ouverture, une hausse de
Gravité encore jamais notifiée ou la première Propagation étendue interrompent
l'utilisateur.
