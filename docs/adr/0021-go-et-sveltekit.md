---
status: accepted
---

# Construire le serveur en Go et le Web avec SvelteKit

Le serveur et le worker CairnOps sont développés en Go, dont le modèle de concurrence et le déploiement en binaire autonome conviennent aux nombreux contrôles réseau et Connecteurs. L'interface Web utilise Svelte et SvelteKit avec TypeScript afin de construire une application temps réel réactive, soignée et propice aux transitions ciblées, sans partager de logique autoritative avec le serveur. SvelteKit produit des artefacts statiques intégrés et servis par Go ; l'application devient dynamique dans le navigateur par l'API et le WebSocket, sans processus Node.js en production.
