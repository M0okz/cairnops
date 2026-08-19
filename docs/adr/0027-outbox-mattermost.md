---
status: accepted
---

# Router Mattermost depuis une boîte d'envoi durable

Le worker CairnOps projette les Incidents vers Mattermost au moyen d'une boîte
d'envoi PostgreSQL durable. Une ouverture non acquittée est créée une seule fois
pour chaque canal dont la sélection contient la Gravité effective. Une
maintenance active neutralise cette création sans interrompre les preuves ; à la
fin de la fenêtre, un Incident toujours actif redevient éligible.

Un Acquittement annule les ouvertures encore en attente. Lorsqu'une ouverture a
été livrée, sa ligne devient le reçu qui détermine les destinataires de la
Résolution : le même canal reçoit donc exactement une fermeture, même si la
Gravité de l'Incident a évolué entre-temps. La livraison est louée par un worker,
retentée avec temporisation bornée et idempotente par Incident, canal et type
d'événement ; aucune politique multi-étapes, aucun rappel et aucune astreinte ne
sont introduits en V1.

La connexion Mattermost exige HTTPS et envoie un message de contrôle avant toute
persistance. L'URL complète du webhook est ensuite chiffrée par la clé maîtresse
partagée avec le worker ; l'API et l'interface n'exposent plus que son origine.
