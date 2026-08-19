# Contribuer à CairnOps

Merci de vouloir améliorer CairnOps. Le projet est encore dans sa phase de
fondation : une proposition doit donc préserver la cohérence du modèle et du
périmètre avant d'ajouter de la surface fonctionnelle.

## Avant de commencer

1. Vérifiez les issues existantes et ouvrez-en une pour les changements
   fonctionnels ou architecturaux significatifs.
2. Lisez [CONTEXT.md](CONTEXT.md), [docs/V1-SCOPE.md](docs/V1-SCOPE.md) et les
   [ADR](docs/adr/) concernés.
3. Gardez les contributions focalisées : un problème ou une décision par pull
   request.

## Principes du projet

- Le serveur est la source de vérité de l'état partagé.
- Une absence de preuve n'est jamais un rétablissement.
- La supervision n'autorise pas l'administration distante.
- Les secrets ne doivent apparaître ni dans les journaux, ni dans les captures,
  ni dans l'historique Git.
- L'accessibilité, la lisibilité et la réduction des mouvements doivent rester
  des propriétés du produit, pas des corrections tardives.

## Proposer une modification

- Créez une branche courte depuis `main`.
- Ajoutez ou adaptez les tests pertinents dès que le code concerné existe.
- Mettez à jour la documentation lorsque le comportement ou une décision
  change.
- Utilisez des messages de commit impératifs et explicites.
- Décrivez dans la pull request le problème, la solution et la manière dont elle
  a été vérifiée.

## Lancer les tests

`go test ./...` suffit pour les tests unitaires. Les tests d'intégration, eux,
ont besoin d'une base PostgreSQL et **se sautent en silence** sans elle : une
suite entièrement verte ne prouve donc rien tant que `CAIRNOPS_TEST_DATABASE_URL`
n'est pas renseignée. Ils appliquent les migrations puis écrivent dans les
tables — pointez-les vers une base jetable, jamais vers une instance réelle.

```bash
docker run -d --name cairnops-test -e POSTGRES_PASSWORD=test \
  -e POSTGRES_USER=test -e POSTGRES_DB=test -p 55432:5432 postgres:18-alpine
export CAIRNOPS_TEST_DATABASE_URL="postgres://test:test@127.0.0.1:55432/test?sslmode=disable"
make test
docker rm -f cairnops-test
```

Chaque test d'intégration reçoit sa propre base. `testsupport.Pool(t)` crée un
schéma PostgreSQL qui n'appartient qu'à ce test, y applique les migrations, puis
le supprime avec tout ce qu'il contient :

```go
func TestQuelqueChose(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	// …
}
```

Cette isolation n'est pas un confort, c'est ce qui rend les tests justes. CairnOps
repose sur des files de travail — les Incidents actifs, la boîte d'envoi des
notifications, les Sources dues — qu'un test interroge globalement parce que
c'est ainsi que le code s'en sert. Dans une base partagée, il y verrait ce que
ses voisins viennent d'y déposer, et Go exécute les paquets en parallèle.

Il en découle qu'un test **n'a pas** à nommer ses fixtures de façon unique, à les
effacer en fin de parcours, ni à filtrer ce qu'il lit. S'il vous faut l'une de
ces précautions, c'est que quelque chose échappe au schéma du test : cherchez là
plutôt que d'ajouter un contournement.

Deux chantiers menés en parallèle méritent chacun leur `git worktree`, avec sa
branche, sa base de test et son port : c'est le seul moyen de ne pas mêler deux
décisions dans un même arbre.

Pour l'interface, `npm run check` dans `web/` type-vérifie l'ensemble et
`npm run build` produit le bundle servi par le serveur Go.

Les contributions acceptées sont distribuées sous la licence AGPL-3.0 du
projet. Aucun CLA séparé n'est demandé à ce stade.
