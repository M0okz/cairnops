<script lang="ts">
  import Icon from './Icon.svelte';
  import { api, type ReconciliationOperation, type ReconciliationPreview, type ReconciliationSourceSummary } from '$lib/api';
  import { reconciliationState } from '$lib/reconciliation.svelte';
  import { session, messageFrom } from '$lib/session.svelte';
  import { t } from '$lib/i18n.svelte';
  import { natureLabel } from '$lib/format';
  import Checkbox from './ui/Checkbox.svelte';

  let {
    primaryTargetId = '',
    secondaryTargetId = '',
    source = null,
    suggestionId = '',
    onclose
  }: {
    primaryTargetId?: string;
    secondaryTargetId?: string;
    source?: ReconciliationSourceSummary | null;
    suggestionId?: string;
    onclose: () => void;
  } = $props();

  const kind = $derived(source ? 'source_move' : 'target_merge');
  let primaryID = $state('');
  let secondaryID = $state('');
  let preview = $state<ReconciliationPreview | null>(null);
  let loading = $state(false);
  let submitting = $state(false);
  let error = $state('');
  let reason = $state('');
  let confirmation = $state('');
  let archiveOrigin = $state(false);
  let adopted = $state(false);

  $effect(() => {
    if (!primaryID) primaryID = primaryTargetId;
    if (!secondaryID) secondaryID = secondaryTargetId || source?.target_id || '';
  });

  const availablePrimary = $derived(session.targets.filter((target) => target.id !== secondaryID));
  const availableSecondary = $derived(session.targets.filter((target) => target.id !== primaryID));
  const valid = $derived(Boolean(
    preview && reason.trim().length >= 3 && confirmation === preview.primary.name && !submitting
  ));

  $effect(() => {
    if (!primaryID || !secondaryID || primaryID === secondaryID) {
      preview = null;
      return;
    }
    void loadPreview();
  });

  async function loadPreview() {
    loading = true;
    error = '';
    preview = null;
    try {
      preview = source
        ? await api<ReconciliationPreview>('/api/v1/source-reassignments/preview', {
            method: 'POST',
            body: JSON.stringify({ source_id: source.id, destination_target_id: primaryID })
          })
        : await api<ReconciliationPreview>('/api/v1/target-reconciliation/preview', {
            method: 'POST',
            body: JSON.stringify({ primary_target_id: primaryID, secondary_target_id: secondaryID })
          });
      if (!source && !adopted && preview.suggested_primary_id === secondaryID) {
        adopted = true;
        [primaryID, secondaryID] = [secondaryID, primaryID];
        return;
      }
      adopted = true;
      confirmation = '';
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      loading = false;
    }
  }

  function chooseSurvivor(id: string) {
    if (id === primaryID || source) return;
    [primaryID, secondaryID] = [secondaryID, primaryID];
    confirmation = '';
  }

  function choosePreviewSide(side: 'primary' | 'secondary') {
    if (preview) chooseSurvivor(preview[side].id);
  }

  async function submit() {
    if (!preview || !valid) return;
    submitting = true;
    error = '';
    try {
      const operation = await api<ReconciliationOperation>('/api/v1/target-reconciliation/operations', {
        method: 'POST',
        body: JSON.stringify({
          kind,
          primary_target_id: primaryID,
          secondary_target_id: secondaryID,
          source_id: source?.id,
          suggestion_id: suggestionId || undefined,
          archive_origin: source ? archiveOrigin : false,
          reason: reason.trim(),
          confirmation
        })
      });
      reconciliationState.operations = [operation, ...reconciliationState.operations.filter((item) => item.id !== operation.id)];
      reconciliationState.suggestions = reconciliationState.suggestions.filter((item) => item.id !== suggestionId);
      session.showNotice(source ? t('reconciliation.sourceStarted', { name: source.name }) : t('reconciliation.mergeStarted'));
      onclose();
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:window onkeydown={(event) => event.key === 'Escape' && !submitting && onclose()} />

<div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && !submitting && onclose()}>
  <div class="modal reconciliation-modal" role="dialog" aria-modal="true" aria-labelledby="reconciliation-title">
    <header>
      <div>
        <h2 id="reconciliation-title">{source ? t('reconciliation.attachSource') : t('reconciliation.mergeTargets')}</h2>
        <p>{source ? t('reconciliation.sourceLead') : t('reconciliation.mergeLead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={submitting} aria-label="Fermer"><Icon name="close" size={14} /></button>
    </header>

    <div class="modal-body">
      {#if source}
        <section class="source-line">
          <span class="key">{source.kind}</span>
          <div><strong>{source.name}</strong><small>{source.origin === 'native' ? t('reconciliation.nativeSource') : t('reconciliation.integrationSource')}</small></div>
        </section>
        <div class="field">
          <label for="destination-target">{t('reconciliation.destination')}</label>
          <select id="destination-target" bind:value={primaryID} disabled={submitting}>
            <option value="">{t('reconciliation.chooseTarget')}</option>
            {#each availablePrimary as target (target.id)}<option value={target.id}>{target.name}</option>{/each}
          </select>
        </div>
      {:else}
        <div class="pickers">
          <div class="field">
            <label for="primary-target">{t('reconciliation.firstTarget')}</label>
            <select id="primary-target" bind:value={primaryID} disabled={submitting}>
              <option value="">{t('reconciliation.choose')}</option>
              {#each session.targets as target (target.id)}<option value={target.id}>{target.name}</option>{/each}
            </select>
          </div>
          <div class="field">
            <label for="secondary-target">{t('reconciliation.secondTarget')}</label>
            <select id="secondary-target" bind:value={secondaryID} disabled={submitting}>
              <option value="">{t('reconciliation.choose')}</option>
              {#each availableSecondary as target (target.id)}<option value={target.id}>{target.name}</option>{/each}
            </select>
          </div>
        </div>
      {/if}

      {#if loading}
        <div class="working" role="status"><i></i><span><strong>{t('reconciliation.previewLoading')}</strong><small>{t('reconciliation.previewLoadingHint')}</small></span></div>
      {:else if preview}
        <section class="comparison">
          <article class:survivor={true}>
            <label>
              {#if !source}<input type="radio" name="survivor" checked onchange={() => choosePreviewSide('primary')} />{/if}
              <span><strong>{preview.primary.name}</strong><small>{t('reconciliation.survivor')}</small></span>
            </label>
            <dl>
              <div><dt>{t('reconciliation.sources')}</dt><dd>{preview.primary.source_count}</dd></div>
              <div><dt>{t('reconciliation.incidents')}</dt><dd>{preview.primary.incident_count}</dd></div>
              <div><dt>{t('reconciliation.observations')}</dt><dd>{preview.primary.observation_count}</dd></div>
              <div><dt>{t('reconciliation.identity')}</dt><dd>{preview.primary.human_managed ? t('reconciliation.managed') : t('reconciliation.discovered')}</dd></div>
            </dl>
          </article>
          <span class="direction">←</span>
          <article>
            <label>
              {#if !source}<input type="radio" name="survivor" onchange={() => choosePreviewSide('secondary')} />{/if}
              <span><strong>{preview.secondary.name}</strong><small>{source ? t('reconciliation.origin') : t('reconciliation.absorbed')}</small></span>
            </label>
            <dl>
              <div><dt>{t('reconciliation.sources')}</dt><dd>{preview.secondary.source_count}</dd></div>
              <div><dt>{t('reconciliation.incidents')}</dt><dd>{preview.secondary.incident_count}</dd></div>
              <div><dt>{t('reconciliation.observations')}</dt><dd>{preview.secondary.observation_count}</dd></div>
              <div><dt>{t('reconciliation.identity')}</dt><dd>{preview.secondary.human_managed ? t('reconciliation.managed') : t('reconciliation.discovered')}</dd></div>
            </dl>
          </article>
        </section>

        {#if preview.warnings.length > 0}
          <div class="warnings">{#each preview.warnings as warning}<p><i></i>{warning}</p>{/each}</div>
        {/if}
        {#if preview.incident_conflicts.length > 0}
          <section class="conflicts"><strong>{t('reconciliation.incidentsCombined')}</strong>{#each preview.incident_conflicts as conflict}<span>{natureLabel(conflict)}</span>{/each}</section>
        {/if}

        {#if source && preview.secondary.source_count === 1}
          <div class="archive-option"><Checkbox bind:checked={archiveOrigin}>{t('reconciliation.archiveEmpty', { name: preview.secondary.name })}</Checkbox></div>
        {/if}

        <div class="field">
          <label for="reconciliation-reason">{t('reconciliation.reason')}</label>
          <textarea id="reconciliation-reason" bind:value={reason} maxlength="1000" rows="3" placeholder={t('reconciliation.reasonPlaceholder')}></textarea>
        </div>
        <div class="field confirmation">
          <label for="reconciliation-confirmation">{t('reconciliation.confirm', { name: preview.primary.name })}</label>
          <input id="reconciliation-confirmation" bind:value={confirmation} autocomplete="off" />
          <small>{t('reconciliation.irreversible')}</small>
        </div>
      {/if}
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>

    <footer>
      <span class="note">{t('reconciliation.supervisionContinues')}</span>
      <button class="btn" type="button" onclick={onclose} disabled={submitting}>{t('common.cancel')}</button>
      <button class="btn primary" type="button" onclick={submit} disabled={!valid}>
        {submitting ? t('reconciliation.starting') : source ? t('reconciliation.attachDefinitely') : t('reconciliation.mergeDefinitely')}
      </button>
    </footer>
  </div>
</div>

<style>
  .reconciliation-modal { width: min(52rem, calc(100vw - 2 * var(--s5))); }
  .pickers, .comparison { display: grid; grid-template-columns: 1fr 1fr; gap: var(--s4); }
  .field { display: grid; gap: var(--s2); }
  .field label { color: var(--muted); font-size: .6875rem; }
  select, input, textarea { width: 100%; border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--surface); color: var(--ink); font: inherit; }
  select, input { height: var(--ctl-h); padding: 0 var(--s3); }
  textarea { padding: var(--s3); resize: vertical; }
  .source-line { display: flex; align-items: center; gap: var(--s3); margin-bottom: var(--s4); padding: var(--s3); border: 1px solid var(--line); border-radius: var(--r-m); background: var(--surface-2); }
  .source-line div, .source-line strong, .source-line small { display: block; }
  .source-line small { color: var(--faint); font-size: .6875rem; }
  .comparison { grid-template-columns: minmax(0, 1fr) 2rem minmax(0, 1fr); align-items: stretch; margin-top: var(--s5); }
  .comparison article { padding: var(--s4); border: 1px solid var(--line-strong); border-radius: var(--r-l); background: var(--surface); }
  .comparison article.survivor { border-color: color-mix(in srgb, var(--accent) 55%, var(--line-strong)); }
  .comparison label { display: flex; align-items: center; gap: var(--s3); }
  .comparison label span, .comparison label strong, .comparison label small { display: block; }
  .comparison label small { color: var(--accent); font-size: .625rem; }
  .comparison dl { display: grid; grid-template-columns: 1fr 1fr; gap: var(--s3); margin-top: var(--s4); }
  .comparison dl div { display: flex; justify-content: space-between; gap: var(--s2); color: var(--faint); font-size: .6875rem; }
  .comparison dd { color: var(--ink); font-family: var(--font-num); }
  .direction { align-self: center; color: var(--accent); text-align: center; }
  .working { display: flex; align-items: center; gap: var(--s3); min-height: 6rem; margin-top: var(--s4); padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r-l); }
  .working i { width: .625rem; height: .625rem; border-radius: 50%; background: var(--accent); }
  .working span, .working strong, .working small { display: block; }
  .working small { margin-top: var(--s1); color: var(--faint); }
  .warnings { margin: var(--s4) 0; padding: var(--s3); border: 1px solid color-mix(in srgb, var(--warn) 35%, var(--line)); border-radius: var(--r-m); }
  .warnings p { display: flex; gap: var(--s2); color: var(--muted); font-size: .6875rem; }
  .warnings i { width: .375rem; height: .375rem; margin-top: .3rem; border-radius: 50%; background: var(--warn); }
  .conflicts { display: flex; align-items: center; gap: var(--s2); flex-wrap: wrap; margin-bottom: var(--s4); }
  .conflicts strong { font-size: .6875rem; }
  .conflicts span { padding: var(--s1) var(--s2); border-radius: var(--r-pill); background: var(--surface-2); color: var(--muted); font-size: .625rem; }
  .archive-option { display: flex; align-items: center; gap: var(--s2); margin-bottom: var(--s4); color: var(--muted); font-size: .6875rem; }
  .comparison input { width: auto; height: auto; }
  .confirmation { margin-top: var(--s4); }
  .confirmation small { color: var(--crit); font-size: .625rem; }
  .error { margin-top: var(--s4); color: var(--crit); font-size: .6875rem; }
  @media (max-width: 48rem) {
    .pickers, .comparison { grid-template-columns: 1fr; }
    .direction { transform: rotate(-90deg); }
  }
</style>
