---
status: accepted
---

# Combiner comptes locaux et OpenID Connect

CairnOps conserve au moins un compte Administrateur local de secours tout en permettant à un fournisseur OpenID Connect facultatif de devenir le mode de connexion principal. Les compagnons mobiles passent par un flux d'autorisation dans le navigateur et reçoivent chacun un jeton révocable, sans collecter le mot de passe de l'utilisateur ; cette solution évite qu'une panne de l'IdP supervisé interdise l'accès à CairnOps tout en permettant l'intégration aux systèmes d'identité self-hosted.
