<script lang="ts">
  /* La boîte de réception.
   *
   * Elle ne décide de rien : elle dit ce qui est arrivé à cette personne, et
   * conduit à l'endroit où l'on décide. Chaque entrée garde l'identité de son
   * Incident et ouvre donc son détail, sans le diluer dans la Cible.
   *
   * Ouvrir la boîte marque son contenu comme lu. La vider est un geste séparé :
   * il retire le bruit accumulé du volet sans perdre le routage d'une future
   * Résolution. */

  import Icon from './Icon.svelte';
  import { incidentHref } from '$lib/incident-detail';
  import { session } from '$lib/session.svelte';
  import { natureLabel, severityTone, since, stamp } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  let open = $state(false);
  let anchor = $state<HTMLDivElement | null>(null);
  let triggerElement = $state<HTMLButtonElement | null>(null);
  let clearing = $state(false);
  let status = $state('');

  /* Le panneau se referme sur Échap et sur tout clic à l'extérieur, comme le
   * menu du compte. */
  $effect(() => {
    if (!open) return;

    const away = (event: MouseEvent) => {
      if (anchor && !anchor.contains(event.target as Node)) open = false;
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      open = false;
      requestAnimationFrame(() => triggerElement?.focus());
    };

    document.addEventListener('pointerdown', away);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', away);
      document.removeEventListener('keydown', escape);
    };
  });

  function toggle() {
    open = !open;
    if (open && session.unread > 0) void session.markInboxRead();
  }

  async function clearInbox() {
    if (clearing || (session.inbox.length === 0 && session.unread === 0)) return;
    status = '';
    clearing = true;
    const cleared = await session.dismissInbox();
    if (cleared) status = t('inbox.cleared');
    clearing = false;
  }

  /* Au-delà d'une centaine, le compte exact n'aide plus personne à décider. */
  const badge = $derived(session.unread > 99 ? '99+' : String(session.unread));
  const entryHref = (entry: { incident_id: string }) => incidentHref(entry.incident_id);
</script>

<div class="inbox" bind:this={anchor}>
  <button
    class="trigger"
    type="button"
    aria-expanded={open}
    aria-haspopup="dialog"
    aria-label={session.unread > 0
      ? plural('inbox.unreadLabel', session.unread)
      : t('inbox.title')}
    bind:this={triggerElement}
    onclick={toggle}
  >
    <Icon name="bell" size={14} />
    {#if session.unread > 0}<span class="count">{badge}</span>{/if}
  </button>

  {#if open}
    <div class="panel" role="dialog" aria-modal="false" aria-label={t('inbox.title')}>
      <header>
        <span class="heading">
          <strong>{t('inbox.title')}</strong>
          <span class="faint">{t('inbox.note')}</span>
        </span>
        <button
          class="btn sm quiet clear"
          type="button"
          disabled={clearing || (session.inbox.length === 0 && session.unread === 0)}
          aria-busy={clearing}
          onclick={clearInbox}
        >{t('inbox.clear')}</button>
      </header>

      <div class="entries">
        {#each session.inbox as entry (entry.id)}
          <a
            class="entry"
            class:fresh={!entry.read_at}
            href={entryHref(entry)}
            onclick={() => (open = false)}
          >
            <i
              class="dot {entry.event_kind === 'resolved' ? 'ok' : severityTone(entry.severity)}"
            ></i>
            <span class="what">
              <strong>{entry.target_name}</strong>
              <small class="faint">
                {entry.event_kind === 'resolved'
                  ? t('inbox.resolved', { nature: natureLabel(entry) })
                  : t('inbox.opened', { nature: natureLabel(entry) })}
              </small>
            </span>
            <span class="when num faint" title={stamp(entry.occurred_at)}>
              {since(entry.occurred_at)}
            </span>
          </a>
        {:else}
          <div class="empty">
            <strong>{t('inbox.empty')}</strong>
            <span class="faint">{t('inbox.emptyHint')}</span>
          </div>
        {/each}
      </div>
    </div>
  {/if}
  <span class="visually-hidden" role="status">{status}</span>
</div>

<style>
  .inbox {
    position: relative;
  }

  .trigger {
    position: relative;
    display: grid;
    place-items: center;
    width: 1.75rem;
    height: 1.75rem;
    border: 1px solid var(--line);
    border-radius: var(--r-m);
    background: none;
    color: var(--muted);
    transition:
      background var(--d1) var(--ease),
      color var(--d1) var(--ease);
  }

  .trigger:hover,
  .trigger[aria-expanded='true'] {
    background: var(--surface-2);
    color: var(--ink);
  }

  /* La pastille déborde volontairement le bouton : elle doit se voir sans
     agrandir la cible, qui garde sa place dans la barre. */
  .count {
    position: absolute;
    top: -0.3125rem;
    right: -0.3125rem;
    min-width: 0.9375rem;
    padding: 0 0.25rem;
    border-radius: var(--r-pill);
    background: var(--crit);
    color: var(--on-accent, #fff);
    font-family: var(--font-num);
    font-size: 0.5625rem;
    line-height: 0.9375rem;
    font-weight: 600;
  }

  .panel {
    position: absolute;
    top: calc(100% + 0.375rem);
    right: 0;
    width: 22rem;
    max-width: calc(100vw - 1.5rem);
    z-index: 40;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--surface);
    box-shadow: var(--shadow);
    overflow: hidden;
  }

  header {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--s3);
    padding: 0.625rem 0.75rem;
    border-bottom: 1px solid var(--line);
  }

  .heading {
    display: flex;
    align-items: baseline;
    gap: var(--s3);
    min-width: 0;
  }

  header strong {
    font-size: 0.8125rem;
    font-weight: 600;
  }

  header .faint {
    font-size: 0.6875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .clear {
    flex: none;
  }

  .entries {
    max-height: 24rem;
    overflow-y: auto;
  }

  .entry {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid var(--line-row);
  }

  .entry:last-child {
    border-bottom: 0;
  }

  .entry:hover {
    background: var(--surface-2);
  }

  /* Une entrée non lue se marque par un fond, pas par une couleur de texte :
     la Gravité est déjà dite par la pastille, et deux signaux pour la même
     chose se contrediraient au premier coup d'œil. */
  .entry.fresh {
    background: var(--bg);
  }

  .what {
    flex: 1;
    min-width: 0;
  }

  .what strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .what small {
    display: block;
    font-size: 0.6875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .when {
    flex: none;
    font-size: 0.6875rem;
  }

  .empty {
    display: grid;
    gap: 0.25rem;
    padding: 1.25rem 0.75rem;
    text-align: center;
  }

  .empty strong {
    font-size: 0.75rem;
    font-weight: 600;
  }

  .empty .faint {
    font-size: 0.6875rem;
  }

  @media (max-width: 48rem) {
    /* Le panneau s'aligne sur la fenêtre, au-delà de l'avatar et de la
       gouttière de la barre, au lieu de déborder à gauche de l'écran. */
    .panel {
      right: calc(-1.625rem - var(--s3) - var(--s5));
      width: calc(100vw - var(--s4) - var(--s4));
      max-width: none;
    }
  }
</style>
