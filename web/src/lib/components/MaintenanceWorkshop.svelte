<script lang="ts">
  import { t } from '$lib/i18n.svelte';
  import Icon from './Icon.svelte';
  import Odometer from './Odometer.svelte';
  import { api, type Maintenance, type Target } from '$lib/api';

  let {
    targets,
    onclose,
    onsuccess
  }: {
    targets: Target[];
    onclose: () => void;
    onsuccess: (maintenance: Maintenance) => Promise<void> | void;
  } = $props();

  let timing = $state<'now' | 'planned'>('now');
  let name = $state('');
  let reason = $state('');
  let selectedTargets = $state<string[]>([]);
  let startsAt = $state(toLocalInput(new Date(Date.now() + 15 * 60 * 1000)));
  let endsAt = $state(toLocalInput(new Date(Date.now() + 60 * 60 * 1000)));
  let busy = $state(false);
  let error = $state('');

  function toLocalInput(date: Date) {
    const shifted = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
    return shifted.toISOString().slice(0, 16);
  }

  function toggleTarget(targetID: string) {
    selectedTargets = selectedTargets.includes(targetID)
      ? selectedTargets.filter((id) => id !== targetID)
      : [...selectedTargets, targetID];
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    if (selectedTargets.length === 0) {
      error = t('workshop.pickTarget');
      return;
    }
    busy = true;
    try {
      const created = await api<Maintenance>('/api/v1/maintenances', {
        method: 'POST',
        body: JSON.stringify({
          name,
          reason,
          target_ids: selectedTargets,
          starts_at: timing === 'planned' ? new Date(startsAt).toISOString() : undefined,
          ends_at: new Date(endsAt).toISOString()
        })
      });
      await onsuccess(created);
      onclose();
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('workshop.maintenanceFailed');
    } finally {
      busy = false;
    }
  }
</script>

<svelte:window onkeydown={(event) => event.key === 'Escape' && onclose()} />

<div class="scrim" role="presentation" onclick={(event) => event.target === event.currentTarget && onclose()}>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="maintenance-title">
    <header>
      <div>
        <h2 id="maintenance-title">{t('workshop.maintenanceTitle')}</h2>
        <p>{t('workshop.maintenanceLead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} aria-label="Fermer">
        <Icon name="close" size={14} />
      </button>
    </header>

    <form onsubmit={submit}>
      <div class="modal-body">
        <div class="segments timing" role="group" aria-label={t('workshop.windowKind')}>
          <button type="button" aria-pressed={timing === 'now'} onclick={() => (timing = 'now')}>
            {t('workshop.immediate')}
          </button>
          <button type="button" aria-pressed={timing === 'planned'} onclick={() => (timing = 'planned')}>
            {t('workshop.scheduled')}
          </button>
        </div>

        <div class="field">
          <label for="mw-name">{t('workshop.interventionName')}</label>
          <input id="mw-name" bind:value={name} minlength="3" maxlength="160" required
            placeholder={t('workshop.interventionPlaceholder')} />
        </div>

        <div class="field">
          <label for="mw-reason">{t('workshop.operationalReason')}</label>
          <textarea id="mw-reason" bind:value={reason} minlength="8" maxlength="500" required rows="3"
            placeholder={t('workshop.reasonPlaceholder')}></textarea>
          <small>{t('workshop.reasonHint')}</small>
        </div>

        <div class="dates">
          {#if timing === 'planned'}
            <div class="field">
              <label for="mw-start">{t('workshop.start')}</label>
              <input id="mw-start" type="datetime-local" bind:value={startsAt} required />
            </div>
          {/if}
          <div class="field">
            <label for="mw-end">{t('workshop.end')}</label>
            <input id="mw-end" type="datetime-local" bind:value={endsAt} required />
          </div>
        </div>

        <fieldset class="targets">
          <legend>
            {t('workshop.neutralisedTargets')}
            <span class="faint num"><Odometer value={`${selectedTargets.length} / ${targets.length}`} /></span>
          </legend>
          <div class="chips">
            {#each targets as target (target.id)}
              <button
                class="chip-toggle"
                type="button"
                aria-pressed={selectedTargets.includes(target.id)}
                onclick={() => toggleTarget(target.id)}
              >
                <i class="mark" aria-hidden="true">{selectedTargets.includes(target.id) ? '✓' : '+'}</i>
                {target.name}
              </button>
            {/each}
          </div>
          {#if targets.length === 0}
            <p class="faint empty-note">{t('workshop.noTargetToNeutralise')}</p>
          {/if}
        </fieldset>

        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>

      <footer>
        <button class="btn" type="button" onclick={onclose}>Annuler</button>
        <button class="btn primary" type="submit" disabled={busy}>
          {busy ? t('workshop.logging') : t('workshop.activateWindow')}
        </button>
      </footer>
    </form>
  </div>
</div>

<style>
  .timing {
    width: 100%;
    margin-bottom: var(--s5);
  }

  .timing button {
    flex: 1;
  }

  .dates {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
    gap: var(--s4);
  }

  .targets {
    margin: var(--s2) 0 0;
    padding: 0;
    border: 0;
  }

  legend {
    padding: 0;
    margin-bottom: var(--s3);
    color: var(--muted);
    font-size: 0.75rem;
    font-weight: 500;
  }

  legend span {
    margin-left: 0.3125rem;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .chip-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.3125rem 0.625rem;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-pill);
    background: var(--bg);
    color: var(--muted);
    font-size: 0.75rem;
    transition: border-color var(--d1) var(--ease), color var(--d1) var(--ease);
  }

  .chip-toggle:hover {
    border-color: var(--line-strong);
    color: var(--ink);
  }

  .chip-toggle[aria-pressed='true'] {
    border-color: var(--accent);
    background: var(--surface-2);
    color: var(--ink);
  }

  .mark {
    font-style: normal;
    color: var(--faint);
  }

  .chip-toggle[aria-pressed='true'] .mark {
    color: var(--accent);
  }

  .empty-note {
    font-size: 0.75rem;
  }
</style>
