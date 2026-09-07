# Présentation standard des alertes

Le catalogue `internal/alerttext` décrit des faits de supervision, en français
et en anglais. Les adapters reconnaissent ces faits ; `internal/synthesis`
compose ensuite les titres et le contexte communs aux Synthèses, au Push,
à la boîte intégrée et à Mattermost. Cette couche de présentation ne décide
ni de la Nature, ni du regroupement, ni des seuils, ni de la Gravité, ni des envois.

## Vocabulaire

| Clé de présentation | Français | Anglais |
| --- | --- | --- |
| `availability.unavailable` | Indisponibilité | Unavailability |
| `disk.latency.high` | Latence disque élevée | High disk latency |
| `disk.space.low` | Espace disque insuffisant | Low disk space |
| `disk.inodes.low` | Peu d’inodes disponibles | Low free inodes |
| `cpu.usage.high` | Utilisation CPU élevée | High CPU utilization |
| `system.load.high` | Charge système moyenne élevée | High average system load |
| `memory.usage.high` | Utilisation mémoire élevée | High memory utilization |
| `memory.available.low` | Mémoire disponible insuffisante | Low available memory |
| `swap.space.low` | Espace swap insuffisant | Low swap space |
| `packages.count.changed` | Nombre de paquets installés modifié | Installed package count changed |
| `certificate.expiring` | Expiration de certificat proche | Certificate nearing expiry |
| `certificate.invalid` | Certificat invalide | Invalid certificate |
| `software.security_updates` | Correctifs de sécurité requis | Security updates required |
| `system.reboot_required` | Redémarrage requis | Restart required |
| `software.update_available` | Mise à jour logicielle disponible | Software update available |
| `backup.failure` | Échec de sauvegarde | Backup failure |
| `backup.freshness` | Sauvegarde trop ancienne | Outdated backup |

« Disque » désigne les accès ou l’espace disque, y compris virtuel. Ce mot
n’affirme pas une panne physique. La charge système moyenne n’est pas un
pourcentage d’utilisation CPU. Un manque d’inodes n’est pas un manque d’octets.
Les noms des produits, des ressources et les sigles usuels restent tels quels.
Les deux sens de sauvegarde appartiennent aux Natures canoniques existantes ;
aucun nouvel adapter de sauvegarde n’est introduit ici.

## Reconnaissance Zabbix

Une règle exige l’UUID officiel **et** sa condition vérifiée. Le trigger actif
et chaque maillon de son héritage doivent correspondre à un extrait officiel :
UUID seul, mots-clés et ressemblance de noms ne suffisent jamais. Un titre
renommé reste présentable si la condition structurée est inchangée.

La comparaison développe les identifiants de fonctions à partir des items,
fonctions et paramètres API. Elle neutralise le nom de l’hôte, tout en refusant
les conditions mêlant plusieurs hôtes. Elle conserve les opérateurs, les
périodes, les clés d’items, les macros de seuil et les expressions de
rétablissement. Les substitutions de découverte portent seulement sur la
ressource de la règle vérifiée (disque, système de fichiers, certificat).
Une chaîne incomplète, une fonction non résolue, une condition modifiée ou
une corrélation personnalisée donne une présentation inconnue.

Les valeurs des macros ne sont pas remplacées par une valeur par défaut :
un seuil propre à une installation reste sous l’autorité de Zabbix. Le
catalogue ne prétend pas afficher le seuil numérique ou la mesure courante
lorsque ces données ne sont pas fournies. Les chaînes de paramètres complexes
que le comparateur ne peut pas établir restent inconnues.

### Matrice des fixtures officielles

Les versions ci-dessous désignent les versions des templates. La reconnaissance
ne se fonde pas sur le numéro de version du serveur : un template importé peut
être plus ancien que celui-ci. Les sources sont épinglées à un commit ; le SHA256
complet de chaque fichier source figure dans `standard_alerts.json`.

| Templates / conditions | 6.0 | 7.0 | 7.4 |
| --- | --- | --- | --- |
| Linux agent passif et actif : CPU, charge, mémoire utilisée/disponible, swap | Oui | Oui | Oui |
| Linux agent passif et actif : latence disque, espace disque, inodes | Oui | Oui | Oui |
| Linux agent passif et actif : paquets installés modifiés | Non couvert | Oui | Oui |
| Website certificate agent 2 : expiration et validité | Oui | Oui | Oui, découverte de sites |
| Nombre d’extraits vérifiés | 22 | 24 | 24 |

Sources officielles épinglées :

- [Zabbix 6.0, Linux](https://github.com/zabbix/zabbix/blob/4519972900f52106a17bf1a223f4e4029165970e/templates/os/linux/template_os_linux.yaml)
- [Zabbix 7.0, Linux](https://github.com/zabbix/zabbix/blob/66dddcedc71f98b20acf630f9b15b0e955fd41e9/templates/os/linux/template_os_linux.yaml)
- [Zabbix 7.4, Linux](https://github.com/zabbix/zabbix/blob/6c0e5ee881b6556ef74c040d4825e5f140df9489/templates/os/linux/template_os_linux.yaml)
- [Zabbix 7.4, certificats](https://github.com/zabbix/zabbix/blob/6c0e5ee881b6556ef74c040d4825e5f140df9489/templates/app/certificate_agent2/template_app_certificate_agent2.yaml)

Cette matrice n’annonce pas la couverture de tous les templates Zabbix. Les
conditions QEMU, Docker, les triggers maison et les autres versions non
identiques aux extraits vérifiés gardent leur texte original.

## Autres adapters

| Adapter | Fait structuré utilisé | Présentation |
| --- | --- | --- |
| Uptime Kuma | Signal d’indisponibilité déjà produit par l’adapter | Indisponibilité |
| Proxmox VE | Ressource attendue démarrée, constatée indisponible | Indisponibilité |
| Contrôles natifs | Verdict négatif déjà converti en Preuve | Indisponibilité |
| PatchMon | Condition `security_updates` ou `reboot_required` | Correctifs de sécurité / redémarrage requis |
| Argus | Différence de version déjà qualifiée par l’adapter | Mise à jour logicielle disponible |
| Webhook générique | Aucune convention standard de sens | Message original |

Ces mappings s’appliquent aux contrats déjà gérés par les adapters. Ils
n’élargissent pas leur compatibilité produit et ne reclassent aucun verdict.
Les champs libres d’un webhook ne peuvent pas injecter un fait de présentation.

## Paramètres, repli et contrats clients

Une Preuve peut fournir une ressource, un nombre de correctifs de sécurité ou
les versions déployée/disponible. Les champs absents restent absents : aucun
zéro, seuil, durée ou nombre de ressources n’est inventé. Les paramètres restent
attachés à leur Preuve. Un Incident ne reprend jamais ceux d’un membre arbitraire.
Son sens commun exige que toutes les Preuves actives non invalidées portent le
même sens reconnu ; au rétablissement, les Preuves historiques non invalidées
servent à conserver ce contexte. Une Nature canonique garde son sens explicite
indépendamment de cet enrichissement.

`Incident.presentation` et `IncidentEvidence.presentation` exposent des textes
`fr`/`en` avec `title` et une `description` facultative, sans état de résolution.
Un titre vide signifie : utiliser le libellé source. `Incident.summary` ajoute
le contexte opérationnel ; les notifications utilisent son gabarit compact.
Le Web utilise la présentation dans les listes et les Preuves, et propose le
**Message original** dans le volet. `name` et `nature_label` restent inchangés.
Les contrats mobiles sont additifs : les clients peuvent utiliser ces textes
serveur sans embarquer de catalogue ni recalculer le sens. Les anciens clients
qui ne lisent que `name` / `nature_label` conservent leur rendu source. Le Push
complet reçoit directement le titre traduit ; les modes discret/masqué
conservent leurs règles de confidentialité.

Les titres compacts sont bornés à 80 caractères ; la version détaillée conserve
l’attribution. Les messages inconnus restent dans leur langue d’origine. Aucune
traduction générale par IA, reconnaissance approximative ou règle Homeblack
n’intervient. Les nouvelles Preuves sont enrichies lors de la collecte ; les
Preuves déjà actives le sont à leur prochaine relecture. Les anciennes
notifications conservent leur instantané. La migration ne retraduit pas
l’historique et ne modifie pas les Incidents résolus qui ne sont plus collectés.

## Ajouter un cas

1. Définir un sens précis, ses paramètres disponibles et deux titres dans
   `internal/alerttext`. Vérifier qu’il ne confond pas des métriques différentes.
2. Identifier la preuve structurée dans l’adapter. Pour Zabbix, examiner le
   template officiel épinglé, l’UUID, les items, la condition et le rétablissement.
3. Ajouter l’extrait à `scripts/generate-zabbix-alert-fixtures.py`. Les noms de
   cette génération servent uniquement à sélectionner les extraits revus hors
   exécution ; ils ne sont jamais utilisés pour reconnaître une alerte reçue.
   Exécuter le script avec PyYAML dans un environnement Python jetable et revoir
   le diff JSON (source, hash, UUID, expression, sens).
4. Tester un cas connu, un nom modifié, une condition modifiée, un UUID inconnu,
   l’héritage, les paramètres manquants et les variantes de versions annoncées.
5. Rejouer les tests d’intégration d’Incident et de notification : même identité,
   même Gravité, même révision, mêmes décisions d’envoi lors d’un changement de
   présentation seul. Documenter les limites de couverture.
