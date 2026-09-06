<script lang="ts">
  import Icon from './Icon.svelte';
  import Inbox from './Inbox.svelte';
  import { palette } from '$lib/palette.svelte';
  import { session } from '$lib/session.svelte';
  import { t } from '$lib/i18n.svelte';
  import ThemePicker from './ThemePicker.svelte';
  import ReconciliationProgress from './ReconciliationProgress.svelte';

  /* Le déclencheur de la Palette est le même sur tous les écrans : la barre
   * supérieure n'a plus de champ propre. Filtrer une liste est le travail de
   * l'écran qui l'affiche, pas d'un champ qui change de sens selon la page. */
  let { crumbs = [] }: { crumbs?: Array<{ label: string; href?: string }> } = $props();

</script>

<header class="topbar">
  <nav class="crumb shown" aria-label={t('topbar.breadcrumb')}>
    <a class="instance-crumb" href="/">{session.instanceLabel}</a>
    <span class="sep" aria-hidden="true">/</span>
    {#each crumbs as crumb, index (crumb.label)}
      {#if index > 0}<span class="sep" aria-hidden="true">/</span>{/if}
      {#if crumb.href}
        <a href={crumb.href}>{crumb.label}</a>
      {:else}
        <span>{crumb.label}</span>
      {/if}
    {/each}
  </nav>

  <div class="topbar-right">
    <ReconciliationProgress />
    <button class="search" type="button" aria-label={t('common.search')} onclick={() => palette.show()}>
      <Icon name="search" size={20} />
      <span>{t('common.search')}</span>
      <kbd>⌘K</kbd>
    </button>
    <ThemePicker />
    <Inbox />
  </div>
</header>
