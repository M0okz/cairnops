# Déploiement de production

Le déploiement retenu est **pull-based** : GitHub Actions teste CairnOps puis
publie les images `server` et `worker` dans GHCR. La VM ne reçoit aucune
connexion de déploiement entrante. Watchtower interroge GHCR toutes les cinq
minutes et ne redéploie que les deux conteneurs portant son label explicite.

## Images publiées

Chaque push sur `main` qui passe les contrôles publie :

- `ghcr.io/m0okz/cairnops-server:<version>`, `:<sha>` et `:latest` ;
- `ghcr.io/m0okz/cairnops-worker:<version>`, `:<sha>` et `:latest`.

La version suit le format `<majeure>.<mineure>.<build>`. La série
`majeure.mineure` est conservée dans le fichier `VERSION` ; GitHub Actions
ajoute son numéro de build, croissant, à chaque publication. Le tag SHA reste
disponible pour un retour arrière manuel et le déploiement automatique suit
`latest`.

## Première publication GHCR

GHCR crée normalement un nouveau package avec une visibilité privée, même si
le dépôt source est public. Après la première exécution réussie du workflow,
ouvrir les paramètres de chacun des deux packages et choisir **Change
visibility → Public** :

- `cairnops-server` ;
- `cairnops-worker`.

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
```

Les deux secrets sont conservés dans Bitwarden et ne sont jamais versionnés.
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
SHA Git voulu pour `server` et `worker`, puis relancer :

```shell
docker compose pull
docker compose up -d --wait
```

Watchtower ne remplace pas un tag SHA immuable tant que le compose ne repasse
pas sur `latest`.
