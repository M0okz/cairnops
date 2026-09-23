<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '$lib/i18n.svelte';
  import Icon from './Icon.svelte';
  import SegmentedControl from './ui/SegmentedControl.svelte';
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
  let recurrence = $state<'once' | 'weekly'>('once');
  let recurrenceUntil = $state(toLocalInput(new Date(Date.now() + 90 * 24 * 60 * 60 * 1000)).slice(0, 10));
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  let dialog: HTMLDialogElement;
  onMount(() => {
    dialog.showModal();
    return () => dialog.close();
  });
  let busy = $state(false);
  let error = $state('');

  function toLocalInput(date: Date) {
    const shifted = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
    return shifted.toISOString().slice(0, 16);
  }

  function parseLocalTime(value: string) {
    const date = new Date(value);
    if (!Number.isFinite(date.getTime()) || toLocalInput(date) !== value) {
      throw new Error(t('maintenance.invalidLocalTime', { zone: timezone }));
    }
    return date;
  }

  function toggleTarget(targetID: string) {
    selectedTargets = selectedTargets.includes(targetID)
      ? selectedTargets.filter((id) => id !== targetID)
      : [...selectedTargets, targetID];
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;
    error = '';
    if (selectedTargets.length === 0) {
      error = t('workshop.pickTarget');
      return;
    }
    busy = true;
    try {
      const start = timing === 'planned' ? parseLocalTime(startsAt) : undefined;
      const end = parseLocalTime(endsAt);
      const created = await api<Maintenance>('/api/v1/maintenances', {
        method: 'POST',
        body: JSON.stringify({
          name,
          reason,
          target_ids: selectedTargets,
          starts_at: start?.toISOString(),
          ends_at: end.toISOString(),
          recurrence: recurrence === 'weekly' ? { frequency: 'weekly', timezone, until: recurrenceUntil } : undefined
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

<dialog bind:this={dialog} class="modal maintenance-dialog" aria-labelledby="maintenance-title" oncancel={(event) => { event.preventDefault(); if (!busy) onclose(); }}>
    <header>
      <div>
        <h2 id="maintenance-title">{t('workshop.maintenanceTitle')}</h2>
        <p>{t('workshop.maintenanceLead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    <form onsubmit={submit}>
      <div class="modal-body">
        <div class="timing">
          <SegmentedControl value={timing} label={t('workshop.windowKind')}
            items={[{ value: 'now', label: t('workshop.immediate') }, { value: 'planned', label: t('workshop.scheduled') }]}
            onValueChange={(value) => (timing = value)} />
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

        <div class="field">
          <label for="mw-recurrence">{t('maintenance.recurrence')}</label>
          <select id="mw-recurrence" bind:value={recurrence}>
            <option value="once">{t('maintenance.once')}</option>
            <option value="weekly">{t('maintenance.weekly')}</option>
          </select>
        </div>
        {#if recurrence === 'weekly'}
          <div class="field">
            <label for="mw-until">{t('maintenance.repeatUntil')}</label>
            <input id="mw-until" type="date" bind:value={recurrenceUntil} required aria-describedby="mw-recurrence-hint" />
            <small id="mw-recurrence-hint">{t('maintenance.recurrenceHint', { zone: timezone })}</small>
          </div>
        {/if}

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
        <button class="btn" type="button" onclick={onclose} disabled={busy}>{t('common.cancel')}</button>
        <button class="btn primary" type="submit" disabled={busy}>
          {busy ? t('workshop.logging') : recurrence === 'weekly' ? t('maintenance.planSeries') : timing === 'planned' ? t('maintenance.plan') : t('workshop.activateWindow')}
        </button>
      </footer>
    </form>
</dialog>

<style>
  .maintenance-dialog {
    margin: auto;
    padding: 0;
    width: min(42rem, calc(100vw - 2rem));
    max-height: calc(100dvh - 2rem);
    color: var(--ink);
    overscroll-behavior: contain;
  }
  .maintenance-dialog:not([open]) { display: none; }
  .maintenance-dialog::backdrop { background: rgb(0 0 0 / 45%); }

  .timing {
    width: 100%;
    margin-bottom: var(--s5);
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
    border-radius: var(--r-button);
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
