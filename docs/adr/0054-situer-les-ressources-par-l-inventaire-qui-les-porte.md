---
status: accepted
---

# Situer les Ressources par l'inventaire qui les porte

La liste des Ressources présentait une catégorie par onglet. Douze onglets,
dont plusieurs vides, mêlaient ce qu'est une Ressource et ce qu'on en suit, et
ne disaient pas où elle s'exécute. Dans un homelab, la question qui précède le
diagnostic est souvent « qu'est-ce qui tourne sur ce nœud ? ».

CairnOps expose désormais la Ressource hôte de chaque Ressource lorsqu'un
inventaire l'atteste. Pour Proxmox VE, une machine virtuelle, un conteneur ou un
stockage nomme le nœud qui le porte ; la liaison de ce nœud donne la Ressource
hôte. Le serveur la projette avec la catégorie, en une seule lecture, et les
clients ne la déduisent jamais eux-mêmes.

Trois règles bornent cette relation :

- seule une preuve d'inventaire l'établit : une ressemblance de nom, une
  adresse partagée ou un regroupement temporel ne suffisent pas ;
- deux inventaires qui se contredisent n'établissent aucune Ressource hôte ;
- elle situe la Ressource sans constituer une Dépendance. Elle n'alimente ni
  les regroupements d'Incidents ni une hypothèse de cause.

La liste regroupe les Ressources par Ressource hôte ou par catégorie au choix
de l'utilisateur. Les catégories se rangent en deux familles, Infrastructure
d'une part, Services et applications d'autre part. Les Tâches planifiées
forment une troisième famille lorsqu'elles existent. Les Logiciels suivis et
les Ressources à classer quittent la navigation principale pour des accès
dédiés : les premiers relèvent du suivi des versions, les secondes d'un
classement à compléter.
