---
status: accepted
---

# Présenter les alertes à partir des faits des Connecteurs

## Constat

La traduction de formulations Zabbix connues rendait certaines notifications
lisibles, mais elle dépendait du texte reçu. Un nom de trigger est modifiable,
peut être personnalisé et ne constitue pas une preuve du sens de sa condition.
CairnOps doit présenter les alertes standard indépendamment de l’installation,
sans introduire de nouvelles règles de regroupement.

## Décision

`internal/alerttext` porte un catalogue de sens de présentation et leurs textes
FR/EN. Les adapters fournissent une `Fact` typée, distincte de la Nature et des
métadonnées libres. La reconnaissance précède le rendu. Le module ignore les
connecteurs, la base de données, la Gravité et les décisions de notification.

L’adapter Zabbix vérifie les UUID et les conditions de toute la chaîne
trigger/prototype/template contre des extraits officiels épinglés. Les noms
ne participent jamais à cette vérification. Les autres adapters utilisent
uniquement les conditions structurées qu’ils qualifient déjà. Un webhook
libre reste libre : ses métadonnées ne sont pas un canal d’injection de sens.

La Preuve conserve séparément le message original, sa provenance et les faits
de présentation. L’Incident expose un sens commun seulement si toutes ses
Preuves contributrices en partagent un. Une Nature canonique existante conserve
sa traduction explicite ; la liste fermée des six Natures n’est pas élargie.
Le catalogue des textes est commun aux deux chemins.

Le rendu serveur compose la Synthèse, les contrats Web/mobile et le gabarit
compact des notifications. Les paramètres individuels restent dans les Preuves.
Les clients choisissent une langue, sans deviner ni retraduire le sens.
Les champs JSON de présentation sont additifs et les messages sources restent
consultables. Le détail garde le texte original même lorsque le titre est traduit.

Les faits sont stockés dans `cairnops_incident_evidence.alert_facts` et le sens
commun dans `cairnops_incidents.alert_kind`. Une relecture peut actualiser ces
champs sans changer la révision opérationnelle, la propagation ou les envois.
L’outbox et la boîte intégrée figent le sens avec le reste du contexte de leur
notification. Le Push lit cet instantané et ne réinterprète pas une ancienne
notification à partir du sens actuel de l’Incident.

## Conséquences

Un cas inconnu ou ambigu reste dans la langue de sa Source. Le catalogue est
volontairement limité aux conditions vérifiées ; son extension exige une
preuve structurée, des contre-exemples et une couverture documentée. Aucun
service de traduction automatique, appel réseau au rendu ou apprentissage
sur les messages d’une installation n’est nécessaire.

Le changement de vocabulaire ne doit jamais créer une nouvelle Nature ni une
notification. L’historique n’est pas retraduit par migration. Les clients
mobiles anciens peuvent ignorer les champs ajoutés ; leur adaptation visuelle
peut suivre indépendamment, alors que le Push reçoit déjà le rendu serveur.

La [matrice et la méthode d’extension](../notification-lexicon.md) précisent
les versions des templates, les paramètres et les limites. Les tests couvrent
les fixtures officielles, la persistance des Preuves, l’absence de révision
opérationnelle supplémentaire et la stabilité des instantanés de notification.
