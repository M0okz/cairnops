<script lang="ts">
  import { t } from '$lib/i18n.svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import BrandMark, { type BrandName } from './BrandMark.svelte';

  let {
    onclose,
    onselect
  }: {
    onclose: () => void;
    onselect: (kind: 'zabbix' | 'uptime_kuma' | 'generic_webhook') => void;
  } = $props();

  const choices = [
    {
      kind: 'zabbix' as const,
      icon: null,
      brand: 'zabbix' as BrandName,
      name: 'Zabbix',
      note: t('chooser.zabbix'),
      access: 'Jeton API'
    },
    {
      kind: 'uptime_kuma' as const,
      icon: null,
      brand: 'uptime_kuma' as BrandName,
      name: 'Uptime Kuma',
      note: t('chooser.uptimeKuma'),
      access: t('chooser.readApiKey')
    },
    {
      kind: 'generic_webhook' as const,
      icon: 'webhook' as IconName,
      brand: null,
      name: t('connector.genericWebhook'),
      note: t('chooser.genericWebhook'),
      access: t('chooser.generatedSecret')
    }
  ];

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') onclose();
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && onclose()}>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="chooser-title">
    <header>
      <div>
        <h2 id="chooser-title">{t('chooser.title')}</h2>
        <p>{t('chooser.lead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    <div class="modal-body">
      <div class="choices">
        {#each choices as choice (choice.kind)}
          <button class="choice" type="button" onclick={() => onselect(choice.kind)}>
            {#if choice.brand}
              <BrandMark name={choice.brand} size={38} />
            {:else if choice.icon}
              <span class="key"><Icon name={choice.icon} size={17} /></span>
            {/if}
            <span class="copy">
              <strong>{choice.name}</strong>
              <small class="faint">{choice.note}</small>
            </span>
            <span class="access faint">{choice.access}</span>
            <span class="caret" aria-hidden="true">›</span>
          </button>
        {/each}
      </div>
    </div>

    <footer>
      <span class="faint note">{t('chooser.note')}</span>
    </footer>
  </div>
</div>

<style>
  .modal {
    max-width: 34rem;
  }

  .choices {
    display: grid;
    gap: var(--s3);
  }

  .choice {
    display: flex;
    align-items: center;
    gap: var(--s4);
    width: 100%;
    padding: var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
    text-align: left;
    transition: border-color var(--d1) var(--ease), background var(--d1) var(--ease);
  }

  .choice:hover {
    border-color: var(--accent);
    background: var(--surface-2);
  }

  .key {
    width: 2.375rem;
    height: 2.375rem;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--r-m);
    background: var(--surface-2);
    color: var(--muted);
    font-size: 0.6875rem;
    font-weight: 600;
  }

  .choice:hover .key {
    background: var(--accent);
    color: var(--accent-ink);
  }

  .copy {
    flex: 1;
    min-width: 0;
  }

  .copy strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .copy small {
    display: block;
    margin-top: 2px;
    font-size: 0.6875rem;
  }

  .access {
    flex: none;
    font-size: 0.6875rem;
  }


  @media (max-width: 34rem) {
    .access {
      display: none;
    }
  }
</style>
