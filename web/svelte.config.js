import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      pages: 'build',
      assets: 'build',
      // L'application est entièrement cliente et le serveur Go sert ce fichier
      // pour toute route sans extension. Il doit donc s'appeler index.html :
      // newSPAHandler le lit sous ce nom et rien d'autre.
      fallback: 'index.html',
      precompress: true,
      strict: true
    })
  }
};

export default config;
