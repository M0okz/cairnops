---
status: accepted
---

# Quarantainer les identités webhook inconnues

Chaque Connecteur webhook générique reçoit une URL publique non devinable et un
secret Bearer aléatoire de 256 bits affiché une seule fois. CairnOps scelle ce
secret avec la clé maîtresse de l'instance et compare l'autorisation en temps
constant ; le secret n'apparaît ensuite ni dans l'API d'administration, ni dans
les journaux.

Le secret authentifie le canal, pas l'identité métier portée par un message. Une
valeur `identity` jamais liée à une Cible est donc placée en quarantaine et ne
modifie aucun Incident, aucune disponibilité et aucune notification. Un
Administrateur choisit explicitement une Cible existante ou autorise CairnOps à
réutiliser un nom exact ou à créer la Cible proposée. Les derniers états retenus
pour cette identité sont alors rejoués de façon idempotente.

Le contrat distingue `firing` et `resolved` et impose un `event_key` stable. Une
Résolution ne peut fermer que le signal actif possédant la même identité et la
même clé ; l'absence de message ne constitue jamais une preuve de rétablissement.
Un nouvel état `firing` après une Résolution ouvre un nouveau cycle historique.
