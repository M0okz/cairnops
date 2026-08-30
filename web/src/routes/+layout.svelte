<script lang="ts">
  import '../styles/app.css';
  import { onMount } from 'svelte';
  import Commissioning from '$lib/components/Commissioning.svelte';
  import Palette from '$lib/components/Palette.svelte';
  import Rail from '$lib/components/Rail.svelte';
  import { t } from '$lib/i18n.svelte';
  import { session } from '$lib/session.svelte';

  let { children } = $props();

  onMount(() => {
    void session.boot();
    return () => session.teardown();
  });
</script>

{#if session.gate === 'app'}
  <a class="skip-link" href="#main-content">{t('common.skipToContent')}</a>
  <div class="shell">
    <Rail />
    <main id="main-content" tabindex="-1">
      {@render children()}
    </main>
  </div>

  <!-- La Palette est montée une fois pour toute l'application : son raccourci
       répond depuis n'importe quel écran, y compris ceux sans barre de
       recherche visible. -->
  <Palette />

  {#if session.notice}
    <p class="notice" role="status">{session.notice}</p>
  {/if}
{:else}
  <Commissioning
    mode={session.gate}
    instance={session.instanceLabel}
    health={session.health}
    version={session.version}
    lightTheme={session.lightTheme}
    error={session.identityError}
    busy={session.identityBusy}
    oidcEnabled={session.oidcEnabled}
    oidcLabel={session.oidcLabel}
    ontoggletheme={() => session.toggleTheme()}
    onsetup={(input) => session.setup(input)}
    onlogin={(input) => session.login(input)}
    onoidc={() => session.startOIDC()}
    onrecover={(input) => session.recover(input)}
  />
{/if}

<style>
  .skip-link {
    position: fixed;
    inset-block-start: var(--s3);
    inset-inline-start: var(--s3);
    z-index: 100;
    padding: var(--s3) var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--surface);
    color: var(--ink);
    box-shadow: var(--shadow);
    font-size: 0.8125rem;
    font-weight: 600;
    transform: translateY(calc(-100% - var(--s5)));
  }

  .skip-link:focus-visible {
    transform: none;
  }

  main {
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
</style>
