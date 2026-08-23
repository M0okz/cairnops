<script lang="ts">
  import { page } from '$app/state';
  import Icon, { type IconName } from './Icon.svelte';
  import Odometer from './Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { i18n, locales, t } from '$lib/i18n.svelte';

  /* Le dépôt ne publie pas encore de site de documentation ni de fichier
   * CHANGELOG : les deux liens pointent donc vers GitHub, seule source
   * réellement consultable aujourd'hui. */
  const DOCS_URL = 'https://github.com/M0okz/cairnops/tree/main/docs';
  const CHANGELOG_URL = 'https://github.com/M0okz/cairnops/releases';

  const roleLabel = $derived(session.user ? t(`role.${session.user.role}`) : '');

  let menuOpen = $state(false);
  let anchor = $state<HTMLDivElement | null>(null);
  let navigation = $state<HTMLElement | null>(null);

  /* Sur une fenêtre étroite, la navigation devient un ruban horizontal. La
   * route active revient au centre à chaque changement plutôt que de rester
   * cachée à l'autre bout du ruban. */
  $effect(() => {
    page.url.pathname;
    if (!window.matchMedia('(max-width: 48rem)').matches) return;
    const frame = requestAnimationFrame(() => {
      const current = navigation?.querySelector<HTMLElement>('[aria-current="page"]');
      if (!navigation || !current) return;
      const navigationBox = navigation.getBoundingClientRect();
      const currentBox = current.getBoundingClientRect();
      navigation.scrollTo({
        left:
          navigation.scrollLeft +
          currentBox.left -
          navigationBox.left -
          (navigationBox.width - currentBox.width) / 2,
        behavior: 'auto'
      });
    });
    return () => cancelAnimationFrame(frame);
  });

  /* Le menu se referme sur Échap et sur tout clic à l'extérieur. */
  $effect(() => {
    if (!menuOpen) return;

    const away = (event: MouseEvent) => {
      if (anchor && !anchor.contains(event.target as Node)) menuOpen = false;
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') menuOpen = false;
    };

    document.addEventListener('pointerdown', away);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', away);
      document.removeEventListener('keydown', escape);
    };
  });

  const initials = $derived(
    (session.user?.display_name ?? 'Homeblack')
      .split(/\s+/)
      .slice(0, 2)
      .map((word) => word.charAt(0).toLocaleUpperCase(i18n.locale))
      .join('')
  );

  type Item = {
    href: string;
    label: string;
    icon: IconName;
    count?: number;
    hot?: boolean;
    dot?: string;
    /* Ouvre un groupe : un filet le sépare de ce qui précède. */
    apart?: boolean;
  };

  const items = $derived<Item[]>([
    { href: '/', label: t('nav.overview'), icon: 'overview' },
    { href: '/cibles', label: t('nav.targets'), icon: 'targets', count: session.targets.length },
    {
      href: '/incidents',
      label: t('nav.incidents'),
      icon: 'incidents',
      count: session.actionable.length,
      hot: session.unacknowledged.length > 0
    },
    {
      href: '/maintenance',
      label: t('nav.maintenance'),
      icon: 'maintenance',
      count: session.visibleMaintenances.length
    },
    {
      href: '/connecteurs',
      label: t('nav.connectors'),
      icon: 'connectors',
      count: session.connectors.length
    },
    {
      href: '/sante',
      label: t('nav.health'),
      icon: 'health',
      dot: session.system?.status === 'operational' ? 'ok' : session.system ? 'warn' : 'idle'
    },
    /* Réglages configure l'instance, il n'observe rien : le filet marque ce
     * changement de registre. */
    { href: '/reglages', label: t('nav.settings'), icon: 'settings', apart: true }
  ]);

  /* Une route est courante pour elle-même et pour ses descendants — le Détail
   * d'une Cible garde donc Cibles allumé. */
  function current(href: string) {
    if (href === '/') return page.url.pathname === '/';
    return page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
  }

  /* La fraîcheur s'égrène : « Temps réel · 8 s » ne vaut que s'il compte. */
  let now = $state(Date.now());
  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(timer);
  });

  const freshness = $derived.by(() => {
    if (session.realtime !== 'online') {
      return session.realtime === 'connecting' ? t('rail.connecting') : t('rail.offline');
    }
    if (!session.lastEventAt) return t('rail.waiting');
    const seconds = Math.max(0, Math.round((now - session.lastEventAt.getTime()) / 1000));
    return seconds < 60
      ? t('duration.seconds', { count: seconds })
      : t('duration.minutes', { count: Math.floor(seconds / 60) });
  });
</script>

<aside class="rail">
  <div class="rail-brand">
    <span class="cairn" aria-hidden="true"><i></i><i></i><i></i></span>
    <!-- Le rail nomme l'instance, pas le produit : c'est ce qui distingue deux
         onglets ouverts côte à côte. -->
    <strong title={session.instanceLabel}>{session.instanceLabel}</strong>
    <span class="version">v{session.version}</span>
  </div>

  <nav bind:this={navigation}>
    {#each items as item (item.href)}
      {#if item.apart}<span class="rule" role="separator"></span>{/if}
      <a href={item.href} aria-current={current(item.href) ? 'page' : undefined}>
        <Icon name={item.icon} />
        {item.label}
        {#if item.dot || item.count !== undefined}
          <span class="rail-indicator">
            {#if item.dot}
              <i class="dot {item.dot}"></i>
            {:else}
              <b class="num" class:hot={item.hot}><Odometer value={item.count ?? 0} /></b>
            {/if}
          </span>
        {/if}
      </a>
    {/each}
  </nav>

  <div class="rail-foot">
    <div class="realtime">
      <i class="dot" class:ok={session.realtime === 'online'} class:warn={session.realtime === 'connecting'} class:crit={session.realtime === 'offline'}></i>
      <span>{t('rail.realtime')} · {freshness}</span>
    </div>

    <!-- Le compte et ses réglages de confort. Rien ici n'agit sur la
         supervision : thème, documentation, session. -->
    <div class="account" bind:this={anchor}>
      {#if menuOpen}
        <div class="menu" role="menu" aria-label={t('rail.account')}>
          <div class="menu-head">
            <span class="workspace-mark">{initials}</span>
            <span class="who">
              <strong>{session.user?.display_name ?? '—'}</strong>
              <small class="faint">{session.user?.username ?? ''}</small>
            </span>
          </div>

          <div class="menu-row theme">
            <span
              ><Icon name={session.lightTheme ? 'sun' : 'moon'} size={15} />{t('rail.theme')}</span
            >
            <div class="segments" role="group" aria-label={t('rail.theme')}>
              <button type="button" aria-pressed={!session.lightTheme}
                onclick={() => session.lightTheme && session.toggleTheme()}>{t('rail.dark')}</button>
              <button type="button" aria-pressed={session.lightTheme}
                onclick={() => !session.lightTheme && session.toggleTheme()}>{t('rail.light')}</button>
            </div>
          </div>

          <!-- La langue vit à côté du thème : ce sont les deux réglages qui
               changent l'écran sans rien changer à la supervision. -->
          <div class="menu-row theme">
            <span><Icon name="book" size={15} />{t('rail.language')}</span>
            <div class="segments" role="group" aria-label={t('rail.language')}>
              {#each locales as choice (choice.value)}
                <button
                  type="button"
                  lang={choice.value}
                  aria-pressed={i18n.locale === choice.value}
                  onclick={() => i18n.choose(choice.value)}>{choice.label}</button
                >
              {/each}
            </div>
          </div>

          <a class="menu-row" role="menuitem" href={CHANGELOG_URL} target="_blank" rel="noreferrer noopener">
            <span><Icon name="changelog" size={15} />Changelog</span>
            <i class="ext" aria-hidden="true">↗</i>
          </a>

          <a class="menu-row" role="menuitem" href={DOCS_URL} target="_blank" rel="noreferrer noopener">
            <span><Icon name="book" size={15} />Documentation</span>
            <i class="ext" aria-hidden="true">↗</i>
          </a>

          <button class="menu-row danger" role="menuitem" type="button"
            onclick={() => { menuOpen = false; void session.logout(); }}>
            <span><Icon name="logout" size={15} />{t('rail.logout')}</span>
          </button>
        </div>
      {/if}

      <button class="account-button" type="button" aria-expanded={menuOpen} aria-haspopup="menu"
        onclick={() => (menuOpen = !menuOpen)}>
        <span class="workspace-mark">{initials}</span>
        <span class="who">
          <strong>{session.user?.display_name ?? t('rail.account')}</strong>
          <small class="faint">{roleLabel}</small>
        </span>
        <span class="chev" aria-hidden="true">⌄</span>
      </button>
    </div>
  </div>
</aside>

<style>
  .rail-foot {
    display: grid;
    gap: 0.25rem;
  }

  /* Le filet est tiré à l'aplomb des libellés, pas des bords du rail : il
     sépare deux groupes de la même liste, il ne coupe pas le volet en deux. */
  .rule {
    height: 1px;
    margin: 0.4375rem 0.5rem;
    background: var(--line);
  }

  .account {
    position: relative;
  }

  .account-button {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 0;
    border-radius: var(--r-m);
    background: none;
    text-align: left;
    transition: background var(--d1) var(--ease);
  }

  .account-button:hover,
  .account-button[aria-expanded='true'] {
    background: var(--surface-2);
  }

  .who {
    flex: 1;
    min-width: 0;
  }

  .who strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .who small {
    display: block;
    font-size: 0.6875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .chev {
    color: var(--dim);
    font-size: 0.6875rem;
  }

  /* Le menu déborde volontairement la largeur du rail : la ligne Thème ne
     tient pas dans 204 px une fois son étiquette et ses deux segments posés. */
  .menu {
    position: absolute;
    bottom: calc(100% + 0.375rem);
    left: 0;
    width: 15.5rem;
    max-width: calc(100vw - 1.5rem);
    z-index: 40;
    display: grid;
    gap: 1px;
    padding: 0.25rem;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--surface);
    box-shadow: var(--shadow);
  }

  .menu-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    margin-bottom: 0.25rem;
    border-bottom: 1px solid var(--line);
  }

  .menu-head .who strong {
    font-size: 0.8125rem;
  }

  .menu-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 0;
    border-radius: var(--r-s);
    background: none;
    color: var(--muted);
    font-size: 0.8125rem;
    text-align: left;
    transition: background var(--d1) var(--ease), color var(--d1) var(--ease);
  }

  .menu-row > span:first-child {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .menu-row:hover {
    background: var(--surface-2);
    color: var(--ink);
  }

  .menu-row.danger:hover {
    color: var(--crit);
  }

  .theme:hover {
    background: none;
    color: var(--muted);
  }

  .theme .segments {
    height: 1.5rem;
    flex: none;
  }

  .theme .segments button {
    padding: 0 0.5rem;
    font-size: 0.6875rem;
  }

  .ext {
    color: var(--dim);
    font-size: 0.6875rem;
    font-style: normal;
  }
</style>
