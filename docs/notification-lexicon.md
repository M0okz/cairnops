# Lexique de supervision

Les notifications emploient le vocabulaire courant de la supervision. Le
lexique appartient au rendu partagé de `internal/synthesis` ; il ne change
ni les identités de Nature, ni les seuils, ni le regroupement des Incidents.

| Sens reconnu | Français | Anglais |
| --- | --- | --- |
| Latence des accès disque (`storage.latency`) | Latence disque élevée | High disk latency |
| Temps de réponse lecture/écriture Linux élevé | Latence disque élevée | High disk latency |
| Espace disque insuffisant (`storage.capacity`) | Espace disque insuffisant | Low disk space |
| Load average Linux | Charge système moyenne élevée | High average system load |
| Utilisation CPU Linux | Utilisation CPU élevée | High CPU utilization |
| Espace libre faible d’un système de fichiers Linux | Espace disque insuffisant | Low disk space |
| Occupation élevée d’un système de fichiers QEMU | Occupation disque élevée | High filesystem usage |
| Changement des paquets installés Linux | Paquets installés modifiés | Installed packages changed |

« Disque » désigne ici les accès ou l’espace disque, y compris un disque
virtuel ; ce mot ne conclut ni à une panne physique ni à une cause. « Stockage »
reste approprié pour un ensemble de ressources de stockage. On ne remplace
donc pas ce mot globalement dans l’interface ou dans les messages sources.
La charge système moyenne n’est pas un pourcentage d’utilisation CPU.
Les sigles usuels (CPU, RAM, DNS, TLS) et les noms de produits restent tels quels.

Pour une Nature canonique, la clé reconnue donne le libellé dans les Synthèses
et les notifications. Pour une Nature locale, seules les formulations complètes
connues du Connecteur sont traduites dans les notifications. Par exemple,
l’utilisation CPU reconnaît le nom du trigger Linux Zabbix et son événement
« over …% for 5m », avec seuil numérique ou macro officielle. Un ajout libre,
une négation ou une autre condition conserve le message source. Ces traductions
ne créent aucune nouvelle Nature canonique.

Les messages inconnus ou personnalisés restent dans leur langue d’origine,
sur une ligne bornée à la longueur du gabarit. Le détail garde l’attribution
et le texte original ; les valeurs et les seuils restent consultables dans
les Preuves. Les traductions sont déterministes, sans traduction automatique
générale ni recherche de mots-clés isolés.

Référence des formulations Linux : [template officiel Zabbix 7.0](https://github.com/zabbix/zabbix/blob/release/7.0/templates/os/linux/template_os_linux.yaml).
