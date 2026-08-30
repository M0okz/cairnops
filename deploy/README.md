# Déploiement de production

Le déploiement retenu est **pull-based** : GitHub Actions teste CairnOps puis
publie les images `server`, `worker` et `relay` dans GHCR. La VM ne reçoit aucune
connexion de déploiement entrante. Watchtower interroge GHCR toutes les cinq
minutes et ne redéploie que les conteneurs portant son label explicite.

## Images publiées

Chaque push sur `main` qui passe les contrôles publie :

- `ghcr.io/m0okz/cairnops-server:<version>`, `:<sha>` et `:latest` ;
- `ghcr.io/m0okz/cairnops-worker:<version>`, `:<sha>` et `:latest` ;
- `ghcr.io/m0okz/cairnops-relay:<version>`, `:<sha>` et `:latest`.

La version suit le format `<majeure>.<mineure>.<build>`. La série
`majeure.mineure` est conservée dans le fichier `VERSION` ; GitHub Actions
ajoute son numéro de build, croissant, à chaque publication. Le tag SHA reste
disponible pour un retour arrière manuel et le déploiement automatique suit
`latest`.

## Releases GitHub

Après la publication réussie des trois images, le même workflow crée une
Release GitHub `v<majeure>.<mineure>.<build>` sur le commit exact de `main`.
La première Release présente le périmètre initial de CairnOps ; les suivantes
classent les changements utiles depuis le tag précédent et écartent les
modifications purement internes. Chaque note rappelle les images immuables, la
sauvegarde recommandée, le traitement des migrations et les incompatibilités
déclarées.

Une Release prouve que les contrôles et la publication des images ont réussi.
Elle ne prouve pas que Watchtower a déjà remplacé les conteneurs de production :
le SHA de `/api/v1/version` reste la référence du déploiement réellement servi.

## Première publication GHCR

GHCR crée normalement un nouveau package avec une visibilité privée, même si
le dépôt source est public. Après la première exécution réussie du workflow,
ouvrir les paramètres de chacun des trois packages et choisir **Change
visibility → Public** :

- `cairnops-server` ;
- `cairnops-worker` ;
- `cairnops-relay`.

Cette opération unique permet ensuite à la VM de puller anonymement les images,
sans token GitHub durable.

## Variables de la VM

Le fichier `/opt/stacks/cairnops/.env`, généré par Ansible et lisible uniquement
par `root`, fournit :

```dotenv
CAIRNOPS_POSTGRES_PASSWORD=<secret>
CAIRNOPS_BOOTSTRAP_TOKEN=<secret>
CAIRNOPS_HTTP_PORT=8080
CAIRNOPS_PUBLIC_URL=https://cairnops.int.homeblack.fr
CAIRNOPS_UPDATE_INTERVAL=300
CAIRNOPS_PUSH_RELAY_URL=https://cairnops-push.int.homeblack.fr
CAIRNOPS_RELAY_BIND=0.0.0.0:8082
CAIRNOPS_RELAY_APNS_KEY_HOST_PATH=/opt/stacks/cairnops/secrets/AuthKey.p8
CAIRNOPS_RELAY_APNS_KEY_ID=<key-id-apple>
CAIRNOPS_RELAY_APNS_TEAM_ID=<team-id-apple>
```

Les secrets sont conservés dans Bitwarden et ne sont jamais versionnés. La clé
APNs `.p8` est matérialisée par Ansible dans un répertoire `0700`, puis montée
en lecture seule dans le Relais.
Le jeton d'amorçage doit être gardé après la création du premier Administrateur,
car il permet aussi le rétablissement d'un accès perdu.

## Commandes opératoires

Sur `trust-cairnops-01`, le stack est installé dans
`/opt/stacks/cairnops` :

```shell
cd /opt/stacks/cairnops
docker compose config --quiet
docker compose pull
docker compose up -d --wait
docker compose ps
```

Watchtower utilise le fork maintenu `nickfedor/watchtower:1.21.0`. Le projet
historique `containrrr/watchtower` est archivé et n'est pas utilisé.

## Retour arrière

Pour revenir temporairement à une révision connue, remplacer `latest` par le
SHA Git voulu pour `server`, `worker` et `relay`, puis relancer :

```shell
docker compose pull
docker compose up -d --wait
```

Watchtower ne remplace pas un tag SHA immuable tant que le compose ne repasse
pas sur `latest`.
