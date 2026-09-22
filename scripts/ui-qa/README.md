# Audit local des alignements

Les scripts ouvrent Chromium **uniquement sur le serveur local**, remplacent les
API par des données synthétiques et ne nécessitent aucun compte ni secret.
Ils ne servent pas à vérifier la production : celle-ci doit être contrôlée dans
le navigateur externe connecté de l’utilisateur.

## Préparation

Depuis la racine du dépôt, avec Python 3.10+ et Node installés :

```sh
python3 -m venv scripts/ui-qa/.venv
scripts/ui-qa/.venv/bin/pip install -r scripts/ui-qa/requirements.txt
scripts/ui-qa/.venv/bin/playwright install chromium
npm --prefix web ci
npm --prefix web run dev -- --host 127.0.0.1 --port 5199
```

Dans un autre terminal :

```sh
scripts/ui-qa/.venv/bin/python scripts/ui-qa/alignment.py --stage pages
scripts/ui-qa/.venv/bin/python scripts/ui-qa/states.py --stage states
```

Les captures et `geometry.json` vont dans `/tmp/cairnops-alignment-evidence/<stage>`.
Un code de sortie non nul indique une erreur JavaScript, un état inaccessible,
un débordement ou un défaut d’alignement mesuré. Les captures restent nécessaires
pour juger les regroupements et les espacements ; les mesures ne remplacent pas
la revue visuelle.

## Couverture

- `alignment.py` : 16 routes, deux thèmes, largeurs CSS 1920/1280/768/390/320,
  hauteur 900. Mesure les bords du bouton de mise à jour, le centrage de son icône
  dans le rail compact, l’accès au compte, les actions des réglages et les débordements.
- `states.py` : 19 états (menus, recherche, personnalisation, détail d’incident,
  création de ressource, rapprochement, version logicielle, maintenance,
  connecteurs, compte, association mobile, options OIDC), deux thèmes,
  largeurs 1440/768/390/320. Attend le panneau attendu et, pour l’association,
  le QR code ; contrôle ses limites et les actions de pied de fenêtre.
- Les rubans d’onglets et la navigation mobile défilent intentionnellement.
  Ils sont exclus du test de débordement horizontal des commandes.

Variantes ciblées :

```sh
scripts/ui-qa/.venv/bin/python scripts/ui-qa/alignment.py --stage short --height 600 --widths 1440,1280 --routes /,/reglages,/sante
scripts/ui-qa/.venv/bin/python scripts/ui-qa/states.py --stage short-states --height 600 --widths 1440,390,320 --only personalizer,new-target,maintenance,incident,device-pairing,connector-config
scripts/ui-qa/.venv/bin/python scripts/ui-qa/alignment.py --stage empty --scenario empty --widths 1440,390 --routes /,/cibles,/incidents,/maintenance,/connecteurs,/mises-a-jour
scripts/ui-qa/.venv/bin/python scripts/ui-qa/alignment.py --stage error --scenario error --widths 1440,390 --routes /reglages,/mises-a-jour
scripts/ui-qa/.venv/bin/python scripts/ui-qa/alignment.py --stage long --scenario long --widths 390,320 --routes /,/cibles,/sante,/connecteurs
scripts/ui-qa/.venv/bin/python scripts/ui-qa/alignment.py --stage english --locale en --widths 1440,390 --routes /,/incidents,/reglages
```

Les fixtures couvrent les états visuels nommés, pas tous les contrats métier.
Les écritures des formulaires sont interceptées ; cet audit n’est pas un test
d’intégration des connecteurs, de l’authentification ou du temps réel.
