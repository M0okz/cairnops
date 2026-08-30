<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import Odometer from './Odometer.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { plural, t } from '$lib/i18n.svelte';
  import { pinnedIndicatorIDs } from '$lib/overview';
  import { session } from '$lib/session.svelte';

  let { ondismiss }: { ondismiss: () => void } = $props();

  let dialog = $state<HTMLDialogElement | null>(null);
  let closeButton = $state<HTMLButtonElement | null>(null);
  let search = $state('');
  let selectedIDs = $state<string[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let loadFailed = $state(false);
  let selectionError = $state('');

  const rows = $derived(
    Object.values(session.indicatorCatalog).flatMap((target) =>
      target.indicators.map((indicator) => ({
        indicator,
        targetName:
          session.targets.find((targetItem) => targetItem.id === target.target_id)?.name ??
          t('overview.indicators.unknownTarget')
      }))
    )
  );

  const filteredRows = $derived.by(() => {
    const query = search.trim().toLocaleLowerCase();
    if (!query) return rows;
    return rows.filter(({ indicator, targetName }) =>
      [indicator.label, indicator.dimension, targetName, indicator.semantic_key]
        .filter(Boolean)
        .some((value) => value!.toLocaleLowerCase().includes(query))
    );
  });

  function toggle(indicatorID: string) {
    selectionError = '';
    if (selectedIDs.includes(indicatorID)) {
      selectedIDs = selectedIDs.filter((id) => id !== indicatorID);
      return;
    }
    if (selectedIDs.length >= 4) {
      selectionError = t('overview.indicators.selectionLimit');
      return;
    }
    selectedIDs = [...selectedIDs, indicatorID];
  }

  async function save() {
    if (saving) return;
    saving = true;
    const saved = await session.setIndicatorPins(selectedIDs);
    saving = false;
    if (saved) ondismiss();
  }

  onMount(() => {
    selectedIDs = pinnedIndicatorIDs(
      Object.values(session.indicatorOverview).flatMap((target) => target.indicators)
    );
    requestAnimationFrame(() => {
      dialog?.showModal();
      closeButton?.focus();
    });
    void session.loadIndicatorCatalog().then((loaded) => {
      loading = false;
      loadFailed = !loaded;
    });
    return () => {
      if (dialog?.open) dialog.close();
    };
  });
</script>

<dialog
  bind:this={dialog}
  class="personalizer"
  aria-labelledby="indicator-personalizer-title"
  aria-describedby="indicator-personalizer-hint"
  oncancel={(event) => {
    event.preventDefault();
    ondismiss();
  }}
  onclick={(event) => event.currentTarget === event.target && ondismiss()}
>
  <header>
    <div>
      <h2 id="indicator-personalizer-title">{t('overview.indicators.personalizerTitle')}</h2>
      <p id="indicator-personalizer-hint">{t('overview.indicators.personalizerHint')}</p>
    </div>
    <button
      bind:this={closeButton}
      class="close"
      type="button"
      aria-label={t('common.close')}
      onclick={ondismiss}
    >
      <Icon name="close" size={14} />
    </button>
  </header>

  <div class="personalizer-tools">
    <input
      type="search"
      bind:value={search}
      placeholder={t('overview.indicators.personalizerSearch')}
      aria-label={t('overview.indicators.personalizerSearch')}
    />
    <span class="selection-count">{plural('overview.indicators.selected', selectedIDs.length)}</span>
  </div>

  <div class="catalog" aria-live="polite">
    {#if loading}
      <div class="catalog-state">{t('overview.indicators.catalogLoading')}</div>
    {:else if loadFailed}
      <div class="catalog-state error">{t('overview.indicators.catalogFailed')}</div>
    {:else if filteredRows.length === 0}
      <div class="catalog-state">{t('overview.indicators.catalogEmpty')}</div>
    {:else}
      {#each filteredRows as row (row.indicator.id)}
        {@const selected = selectedIDs.includes(row.indicator.id)}
        <button
          class="indicator-choice"
          class:selected
          type="button"
          aria-pressed={selected}
          onclick={() => toggle(row.indicator.id)}
        >
          <span class="choice-copy">
            <strong>{row.indicator.label}</strong>
            <small>{row.targetName}{row.indicator.dimension ? ` · ${row.indicator.dimension}` : ''}</small>
          </span>
          <b class="num"><Odometer value={formatIndicator(row.indicator.last_value, row.indicator.unit)} /></b>
          <i aria-hidden="true">{selected ? '✓' : '+'}</i>
        </button>
      {/each}
    {/if}
  </div>

  {#if selectionError}
    <p class="selection-error" role="alert">{selectionError}</p>
  {/if}

  <footer>
    <button
      class="btn automatic"
      type="button"
      onclick={() => {
        selectedIDs = [];
        selectionError = '';
      }}
    >
      {t('overview.indicators.automatic')}
    </button>
    <button class="btn" type="button" onclick={ondismiss}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={saving || loading || loadFailed} onclick={save}>
      {t('overview.indicators.save')}
    </button>
  </footer>
</dialog>

<style>
  .personalizer {
    width: calc(100vw - 2 * var(--s5));
    max-width: 52rem;
    max-height: calc(100dvh - 2 * var(--s5));
    padding: 0;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--surface);
    color: var(--ink);
    box-shadow: var(--shadow);
    overflow: hidden;
    overscroll-behavior: contain;
  }

  .personalizer[open] {
    display: flex;
    flex-direction: column;
  }

  .personalizer::backdrop {
    background: rgb(0 0 0 / 0.55);
  }

  header {
    display: flex;
    align-items: flex-start;
    gap: var(--s4);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line);
  }

  header > div {
    min-width: 0;
    flex: 1;
  }

  header h2 {
    font-size: 1rem;
  }

  header p {
    margin-top: var(--s1);
    color: var(--muted);
    font-size: var(--text-sm);
    text-wrap: pretty;
  }

  .close {
    position: relative;
    width: var(--ctl-h-lg);
    height: var(--ctl-h-lg);
    display: grid;
    place-items: center;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: none;
    color: var(--muted);
    flex: none;
  }

  .close:hover {
    background: var(--surface-2);
    color: var(--ink);
  }

  .personalizer-tools {
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line-row);
    background: var(--bg);
  }

  .personalizer-tools input {
    min-width: 0;
    flex: 1;
  }

  .selection-count {
    color: var(--faint);
    font-family: var(--font-num);
    font-size: 0.6875rem;
    white-space: nowrap;
  }

  .catalog {
    min-height: 14rem;
    padding: var(--s4);
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    align-content: start;
    gap: var(--s3);
    overflow-y: auto;
    background: var(--bg);
  }

  .catalog-state {
    grid-column: 1 / -1;
    min-height: 12rem;
    display: grid;
    place-items: center;
    color: var(--faint);
    font-size: var(--text-sm);
    text-align: center;
  }

  .catalog-state.error,
  .selection-error {
    color: var(--crit);
  }

  .indicator-choice {
    min-width: 0;
    min-height: 3.5rem;
    padding: var(--s3) var(--s4);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto var(--ctl-h);
    align-items: center;
    gap: var(--s3);
    border: 1px solid var(--line);
    border-radius: var(--r-m);
    background: var(--surface);
    color: var(--ink);
    text-align: left;
  }

  .indicator-choice:hover {
    border-color: var(--line-strong);
    background: var(--surface-2);
  }

  .indicator-choice.selected {
    border-color: var(--accent);
  }

  .choice-copy,
  .choice-copy strong,
  .choice-copy small {
    min-width: 0;
    display: block;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .choice-copy strong {
    font-size: 0.75rem;
  }

  .choice-copy small {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .indicator-choice > b {
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .indicator-choice > i {
    width: var(--ctl-h);
    height: var(--ctl-h);
    display: grid;
    place-items: center;
    border-radius: var(--r-m);
    background: var(--surface-3);
    color: var(--faint);
    font-size: 0.75rem;
    font-style: normal;
  }

  .indicator-choice.selected > i {
    background: var(--surface-3);
    color: var(--accent);
  }

  .selection-error {
    padding: var(--s3) var(--s5) 0;
    font-size: 0.6875rem;
  }

  footer {
    padding: var(--s4) var(--s5);
    display: flex;
    justify-content: flex-end;
    gap: var(--s3);
    border-top: 1px solid var(--line);
  }

  footer .automatic {
    margin-right: auto;
  }

  @media (max-width: 48rem) {
    .personalizer-tools {
      align-items: stretch;
      flex-direction: column;
      gap: var(--s2);
    }

    .catalog {
      grid-template-columns: minmax(0, 1fr);
    }

    footer {
      flex-wrap: wrap;
    }

    footer .automatic {
      width: 100%;
      margin-right: 0;
    }
  }
</style>
