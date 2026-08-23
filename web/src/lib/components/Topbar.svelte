<script lang="ts">
  import Icon from './Icon.svelte';
  import Inbox from './Inbox.svelte';
  import { palette } from '$lib/palette.svelte';
  import { session } from '$lib/session.svelte';
  import { i18n, t } from '$lib/i18n.svelte';
  import ReconciliationProgress from './ReconciliationProgress.svelte';

  /* Le déclencheur de la Palette est le même sur tous les écrans : la barre
   * supérieure n'a plus de champ propre. Filtrer une liste est le travail de
   * l'écran qui l'affiche, pas d'un champ qui change de sens selon la page. */
  let { crumbs = [] }: { crumbs?: Array<{ label: string; href?: string }> } = $props();

  const initials = $derived(
    (session.user?.display_name ?? '')
      .split(/\s+/)
      .slice(0, 2)
      .map((word) => word.charAt(0).toLocaleUpperCase(i18n.locale))
      .join('') || '··'
  );

  /* Le fil ne paraît qu'une fois le titre de l'écran sorti du champ. Tant qu'on
   * voit le h1, le redire dans la barre n'apprend rien ; passé lui, la barre
   * reste le seul endroit qui nomme ce qu'on regarde — et, sur les écrans
   * imbriqués, le seul chemin pour remonter. */
  let titleGone = $state(false);

  $effect(() => {
    /* Les écrans nomment leur titre de deux façons — `.page-head` partout, et
     * `.intro` sur la Vue d'ensemble. Un écran qui n'a ni l'un ni l'autre, comme
     * un Connecteur en cours de raccordement, n'a rien qui redise ce que la
     * barre dirait : le fil y paraît d'emblée. */
    const head = document.querySelector('.page-head, .intro');
    if (!head) {
      titleGone = true;
      return;
    }
    /* Le titre est réputé parti dès qu'il glisse sous la barre, pas quand il
     * quitte l'écran : celle-ci le recouvre déjà. La marge se mesure sur la
     * barre elle-même — `rootMargin` n'accepte que des pixels, jamais un rem,
     * et la hauteur de la barre suit l'échelle de l'écran. */
    const bar = document.querySelector('.topbar');
    const cover = bar ? Math.round(bar.getBoundingClientRect().height) : 48;
    const watcher = new IntersectionObserver(([entry]) => (titleGone = !entry.isIntersecting), {
      rootMargin: `-${cover}px 0px 0px 0px`
    });
    watcher.observe(head);
    return () => watcher.disconnect();
  });
</script>

<header class="topbar">
  <nav
    class="crumb"
    class:shown={titleGone}
    aria-label={t('topbar.breadcrumb')}
    aria-hidden={!titleGone}
  >
    {#each crumbs as crumb, index (crumb.label)}
      {#if index > 0}<span class="sep" aria-hidden="true">/</span>{/if}
      {#if crumb.href}
        <a href={crumb.href} tabindex={titleGone ? undefined : -1}>{crumb.label}</a>
      {:else}
        <span>{crumb.label}</span>
      {/if}
    {/each}
  </nav>

  <div class="topbar-right">
    <ReconciliationProgress />
    {#if session.availableVersion}
      <button
        class="btn sm primary update-cta"
        type="button"
        title={t('topbar.updateAvailableVersion', { version: session.availableVersion })}
        aria-label={t('topbar.updateAvailableVersion', { version: session.availableVersion })}
        onclick={() => session.reloadForUpdate()}
      >
        <span>{t('topbar.updateAction')}</span>
        <small>{session.availableVersion}</small>
      </button>
    {/if}
    <button class="search" type="button" aria-label={t('common.search')} onclick={() => palette.show()}>
      <Icon name="search" size={13} />
      <span>{t('common.search')}</span>
      <kbd>⌘K</kbd>
    </button>
    <Inbox />
    <span class="avatar" title={session.user?.display_name ?? ''}>{initials}</span>
  </div>
</header>
