import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  plugins: [svelte({ configFile: false })],
  base: './',
  publicDir: '../static',
  optimizeDeps: { entries: ['./index.html', './film.html'] },
  server: { fs: { allow: [fileURLToPath(new URL('..', import.meta.url))] } },
  build: {
    outDir: 'build',
    rolldownOptions: {
      input: {
        app: fileURLToPath(new URL('./index.html', import.meta.url)),
        film: fileURLToPath(new URL('./film.html', import.meta.url))
      }
    }
  }
});
