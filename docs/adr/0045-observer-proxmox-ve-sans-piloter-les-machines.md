---
status: accepted
---

# Observer Proxmox VE sans piloter les machines

Le premier Connecteur d’hyperviseur observe Proxmox VE 8 et 9 par son API
officielle. PBS constitue un futur Connecteur distinct. Ce chantier ne crée ni
seuils de métriques, ni diagnostic causal, ni fonctions d’administration de VM.

## Inventaire et conditions

- Les identités `node/<nom>`, `qemu/<VMID>`, `lxc/<VMID>` et
  `storage/<nœud>/<stockage>` sont propres à cette instance de Connecteur. Une
  migration conserve la liaison de la VM et actualise son hôte dans les
  Observations et les métadonnées. Deux vues d’un stockage partagé ne sont pas
  automatiquement fusionnées.
- Les modèles sont exclus. L’import propose un rapprochement explicable et
  demande sa confirmation ; une affectation vide crée une Cible distincte,
  même lorsqu’un nom identique existe. L’import est atomique.
- Un nœud explicitement hors ligne produit une Preuve canonique
  `availability`. Un arrêt de VM ou de conteneur ne produit cette Preuve que si
  l’Administrateur a demandé « Signaler un arrêt » pour cet objet. Ce choix est
  désactivé par défaut et modifiable depuis l’inventaire. Le retrait de ce choix
  retire la condition d’arrêt au prochain cycle vérifié.
- Une machine démarrée atteste son état d’exécution, pas celui de ses
  applications. Les sources de VM sans exigence d’exécution et celles de
  stockage ne mesurent pas la Disponibilité.
- Un état inconnu, une lecture échouée, une permission perdue ou un objet absent
  ne résout jamais une Preuve. L’objet absent reste lié, avec son historique et
  l’indication `missing`. La Santé de l’Intégration signale la lecture incomplète.
- Toutes les Preuves passent par le cycle Incident commun : quinze arrêts
  voisins forment un Incident pendant sa Propagation. Ce Connecteur ne possède
  aucune voie de notification directe et n’utilise pas d’IA.
- Le worker interroge toutes les minutes, dans sa propre famille supervisée.
  Un bail encore valide est requis à l’écriture des Preuves ; suspendre ou
  reconfigurer le Connecteur invalide un cycle ancien.
- L’inventaire initial mémorise aussi les objets non sélectionnés. Les objets
  réellement nouveaux et sans rapprochement plausible sont importés ensuite ;
  les autres restent à rapprocher dans l’inventaire. Un ancien objet ignoré
  n’est pas réimporté automatiquement au cycle suivant.
  Un objet à rapprocher qui disparaît quitte le compteur des décisions en
  attente ; sa décision reste mémorisée et redevient visible s’il réapparaît.

## Indicateurs et contexte

CPU et mémoire, ainsi que l’occupation des stockages et du système de fichiers
du nœud, sont des Indicateurs explicitement sélectionnés. La mémoire d’une VM
est étiquetée « vue par l’hôte ». Une valeur absente reste absente. Le champ
`qemu.disk`, souvent nul, n’est jamais présenté comme l’occupation des systèmes
de fichiers invités. Les compteurs cumulés de réseau et disque ne sont pas
transformés en débits sans historique approprié.

L’hôte d’exécution constitue le premier fait de topologie. L’attribution des
disques invités à leurs stockages et la corrélation par Dépendances restent des
travaux ultérieurs : aucun lien causal n’est déduit du seul nom d’un objet.

## Autorisation et TLS

L’aperçu utilise uniquement des GET. En mode automatique, l’Administrateur
fournit un jeton d’installation temporaire autorisé à créer des utilisateurs,
jetons et ACL ; l’aperçu vérifie ces droits et annonce les objets à créer. À la
confirmation, CairnOps crée un utilisateur du realm PVE sans mot de passe,
un jeton avec séparation de privilèges et le rôle propagé `PVEAuditor` sur `/`
pour l’utilisateur **et** le jeton. Il vérifie l’accès produit, puis conserve
uniquement ce jeton chiffré. Le mode avancé accepte un jeton technique fourni.

Les permissions effectives `Sys.Audit`, `VM.Audit` et `Datastore.Audit` sont
vérifiées à chaque inventaire : `/cluster/resources` filtre silencieusement
les objets interdits. Les omissions restent conservatrices même si une ACL
plus spécifique change entre la vérification et la lecture.

Les échanges exigent HTTPS, refusent les redirections et bornent les délais
et la taille des réponses. Pour un certificat privé, un premier handshake sans
identifiants expose son identité et son empreinte SHA-256. L’Administrateur
l’approuve explicitement ; seule cette empreinte est acceptée ensuite. Un
certificat changé bloque la lecture jusqu’à une nouvelle approbation vérifiée
avec le jeton existant. Les certificats expirés sont refusés.

## Exception de nettoyage à l’ADR 0041

Proxmox VE exige des droits d’administration d’identités pour supprimer un
utilisateur technique ; `PVEAuditor` ne les possède pas. Nous ne donnons pas
ces droits au jeton de Supervision pour lui permettre de supprimer son compte.

La suppression d’un accès géré demande donc une **nouvelle autorisation
administrative temporaire**, annoncée dès l’aperçu. Le Connecteur est d’abord
suspendu ; le compte géré, ses jetons et ses ACL sont retirés avant suppression
locale. Le préfixe aléatoire et le commentaire d’appartenance doivent
correspondre. Un échec conserve les identifiants techniques chiffrés et le
Connecteur suspendu ; l’Administrateur peut réessayer dans ce même parcours.
Contrairement au nettoyage autonome prévu par l’ADR 0041, le worker ne réessaie
pas cette opération administrative sans nouvelle autorisation. Aucun jeton
d’installation n’est conservé pour rendre cette relance automatique.

Un jeton fourni et son compte préexistant ne sont jamais supprimés. Un échec
avant l’import local tente de retirer immédiatement l’accès nouvellement créé
avec l’autorisation d’installation encore disponible, et indique son identité
si ce nettoyage échoue.

## Références officielles

- [Gestion des utilisateurs et jetons](https://github.com/proxmox/pve-docs/blob/master/pveum.adoc)
- [Inventaire du cluster et filtrage des permissions](https://github.com/proxmox/pve-manager/blob/master/PVE/API2/Cluster.pm)
- [Création et suppression des identités et jetons](https://github.com/proxmox/pve-access-control/blob/master/src/PVE/API2/User.pm)
