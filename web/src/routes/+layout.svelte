<script lang="ts">
  import '../styles/app.css';
  import { onMount } from 'svelte';
  import Commissioning from '$lib/components/Commissioning.svelte';
  import Palette from '$lib/components/Palette.svelte';
  import Rail from '$lib/components/Rail.svelte';
  import { session } from '$lib/session.svelte';

  let { children } = $props();

  onMount(() => {
    void session.boot();
    return () => session.teardown();
  });
</script>

{#if session.gate === 'app'}
  <div class="shell">
    <Rail />
    <main>
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
    ontoggletheme={() => session.toggleTheme()}
    onsetup={(input) => session.setup(input)}
    onlogin={(input) => session.login(input)}
    onrecover={(input) => session.recover(input)}
  />
{/if}

<style>
  main {
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
</style>
