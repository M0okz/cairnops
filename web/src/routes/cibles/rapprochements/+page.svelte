<script lang="ts">
  import Topbar from '$lib/components/Topbar.svelte';
  import ReconciliationWorkshop from '$lib/components/ReconciliationWorkshop.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import { reconciliationState } from '$lib/reconciliation.svelte';
  import type { ReconciliationSuggestion } from '$lib/api';
  import { session, messageFrom } from '$lib/session.svelte';
  import { plural, t, type MessageKey } from '$lib/i18n.svelte';

  let review = $state<ReconciliationSuggestion | null>(null);
  let manualOpen = $state(false);
  let acting = $state('');
  let error = $state('');
  let snoozeFor = $state<ReconciliationSuggestion | null>(null);
  let snoozeUntil = $state('');

  const evidenceLabels: Record<string, MessageKey> = {
    same_machine_id: 'reconciliation.evidence.machine',
    same_name: 'reconciliation.evidence.name',
    same_hostname: 'reconciliation.evidence.hostname',
    same_ip: 'reconciliation.evidence.ip',
    similar_name: 'reconciliation.evidence.similarName',
    different_machine_id: 'reconciliation.evidence.machineConflict'
  };

  const stageLabels: Record<string, MessageKey> = {
    preparing: 'reconciliation.stage.preparing',
    consolidating: 'reconciliation.stage.consolidating',
    reconciling_incidents: 'reconciliation.stage.incidents',
    recalculating_metrics: 'reconciliation.stage.metrics',
    finalizing: 'reconciliation.stage.finalizing',
    completed: 'reconciliation.stage.completed',
    failed: 'reconciliation.stage.failed'
  };

  $effect(() => {
    if (session.user?.role === 'administrator') void reconciliationState.load();
  });

  async function reject(item: ReconciliationSuggestion) {
    acting = item.id; error = '';
    try { await reconciliationState.reject(item.id); }
    catch (cause) { error = messageFrom(cause); }
    finally { acting = ''; }
  }

  async function snooze(item: ReconciliationSuggestion, days: number) {
    acting = item.id; error = '';
    try {
      const until = new Date(); until.setDate(until.getDate() + days);
      await reconciliationState.snooze(item.id, until);
    } catch (cause) { error = messageFrom(cause); }
    finally { acting = ''; }
  }

  function openCustomSnooze(item: ReconciliationSuggestion) {
    const defaultDate = new Date();
    defaultDate.setDate(defaultDate.getDate() + 30);
    snoozeUntil = defaultDate.toISOString().slice(0, 10);
    snoozeFor = item;
  }

  async function snoozeCustom() {
    if (!snoozeFor || !snoozeUntil) return;
    const until = new Date(`${snoozeUntil}T23:59:59`);
    if (Number.isNaN(until.getTime())) return;
    acting = snoozeFor.id; error = '';
    try {
      await reconciliationState.snooze(snoozeFor.id, until);
      snoozeFor = null;
    } catch (cause) { error = messageFrom(cause); }
    finally { acting = ''; }
  }
</script>

<svelte:head><title>{t('reconciliation.title')} — {session.instanceLabel}</title></svelte:head>
<Topbar crumbs={[{ label: t('nav.targets'), href: '/cibles' }, { label: t('reconciliation.title') }]} />

<div class="page">
  <div class="page-head">
    <div><h1>{t('reconciliation.title')}</h1><p>{t('reconciliation.lead')}</p></div>
    <div class="page-actions"><button class="btn primary" type="button" onclick={() => (manualOpen = true)}>{t('reconciliation.manual')}</button></div>
  </div>

  {#if session.user?.role !== 'administrator'}
    <div class="card"><div class="empty"><strong>{t('reconciliation.adminRequired')}</strong>{t('reconciliation.adminHint')}</div></div>
  {:else}
    {#if reconciliationState.activeOperations.length > 0}
      <section class="operations" aria-live="polite">
        <header><h2>{t('reconciliation.working')}</h2><span>{reconciliationState.activeOperations.length}</span></header>
        {#each reconciliationState.activeOperations as operation (operation.id)}
          <article class="operation">
            <i></i>
            <span><strong>{t(stageLabels[operation.stage])}</strong><small>{operation.secondary_target_name} → {operation.primary_target_name}</small></span>
            <time>{operation.attempts}/3</time>
          </article>
        {/each}
      </section>
    {/if}

    <div class="summary">
      <article><b><Odometer value={reconciliationState.actionable.length} /></b><span>{t('reconciliation.toReview')}</span></article>
      <article><b><Odometer value={reconciliationState.operations.filter((item) => item.status === 'succeeded').length} /></b><span>{t('reconciliation.recentlyCompleted')}</span></article>
    </div>

    {#if reconciliationState.loading && !reconciliationState.loaded}
      <div class="card"><div class="empty"><strong>{t('reconciliation.analyzing')}</strong>{t('reconciliation.analyzingHint')}</div></div>
    {:else if reconciliationState.actionable.length === 0}
      <div class="card"><div class="empty"><strong>{t('reconciliation.empty')}</strong>{t('reconciliation.emptyHint')}</div></div>
    {:else}
      <section class="queue">
        <header><h2>{t('reconciliation.suggestions')}</h2><span>{t('reconciliation.actionableConfidence')}</span></header>
        {#each reconciliationState.actionable as item (item.id)}
          <article class="suggestion">
            <div class="identity">
              <span><strong>{item.left.name}</strong><small>{plural('reconciliation.sourceCount', item.left.source_count)}</small></span>
              <i>↔</i>
              <span><strong>{item.right.name}</strong><small>{plural('reconciliation.sourceCount', item.right.source_count)}</small></span>
            </div>
            <div class="decision">
              <span class="pill {item.confidence === 'high' ? 'ok' : 'info'}">{item.confidence === 'high' ? t('reconciliation.confidenceHigh') : t('reconciliation.confidenceMedium')}</span>
              <strong>{item.kind === 'target_merge' ? t('reconciliation.targetMerge') : t('reconciliation.correctSource', { name: item.source?.name ?? '' })}</strong>
            </div>
            <div class="evidence">
              {#each item.evidence as proof (proof.kind + proof.value)}
                <span><b>{evidenceLabels[proof.kind] ? t(evidenceLabels[proof.kind]) : proof.kind}</b><code>{proof.value}</code></span>
              {/each}
              {#each item.contradictions as contradiction (contradiction.kind + contradiction.value)}
                <span class="contradiction"><b>{evidenceLabels[contradiction.kind] ? t(evidenceLabels[contradiction.kind]) : contradiction.kind}</b><code>{contradiction.value}</code></span>
              {/each}
            </div>
            <div class="actions">
              <button class="btn sm" type="button" disabled={acting === item.id} onclick={() => reject(item)}>{t('reconciliation.reject')}</button>
              <button class="btn sm" type="button" disabled={acting === item.id} onclick={() => snooze(item, 7)}>{t('reconciliation.snooze7')}</button>
              <button class="btn sm" type="button" disabled={acting === item.id} onclick={() => snooze(item, 30)}>{t('reconciliation.snooze30')}</button>
              <button class="btn sm" type="button" disabled={acting === item.id} onclick={() => openCustomSnooze(item)}>{t('reconciliation.snoozeOther')}</button>
              <button class="btn sm primary" type="button" onclick={() => (review = item)}>{t('reconciliation.review')}</button>
            </div>
          </article>
        {/each}
      </section>
    {/if}

    {#if reconciliationState.weak.length > 0}
      <details class="weak"><summary>{t('reconciliation.weakLeads')}</summary>
        {#each reconciliationState.weak as item (item.id)}<p><strong>{item.left.name}</strong> ↔ <strong>{item.right.name}</strong> · {item.evidence.map((proof) => evidenceLabels[proof.kind] ? t(evidenceLabels[proof.kind]) : proof.kind).join(', ')}</p>{/each}
      </details>
    {/if}

    {#if error || reconciliationState.error}<p class="error" role="alert">{error || reconciliationState.error}</p>{/if}
  {/if}
</div>

{#if manualOpen}<ReconciliationWorkshop onclose={() => (manualOpen = false)} />{/if}
{#if review}
  <ReconciliationWorkshop
    primaryTargetId={review.kind === 'source_move' ? review.right.id : review.left.id}
    secondaryTargetId={review.left.id}
    source={review.source ?? null}
    suggestionId={review.id}
    onclose={() => (review = null)}
  />
{/if}

{#if snoozeFor}
  <div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && (snoozeFor = null)}>
    <div class="modal narrow" role="dialog" aria-modal="true" aria-labelledby="snooze-title">
      <header><div><h2 id="snooze-title">{t('reconciliation.snoozeTitle')}</h2><p>{t('reconciliation.snoozeHint')}</p></div></header>
      <div class="modal-body"><div class="field"><label for="snooze-until">{t('reconciliation.until')}</label><input id="snooze-until" type="date" bind:value={snoozeUntil} required /></div></div>
      <footer><button class="btn" type="button" onclick={() => (snoozeFor = null)}>{t('common.cancel')}</button><button class="btn primary" type="button" disabled={!snoozeUntil || acting !== ''} onclick={snoozeCustom}>{t('reconciliation.snooze')}</button></footer>
    </div>
  </div>
{/if}

<style>
  .operations { margin-bottom: var(--s5); border: 1px solid color-mix(in srgb, var(--accent) 40%, var(--line)); border-radius: var(--r-l); background: var(--surface); overflow: hidden; }
  section > header { display: flex; align-items: center; gap: var(--s3); min-height: 2.75rem; padding: 0 var(--s4); border-bottom: 1px solid var(--line); }
  section > header h2 { font-size: .75rem; }
  section > header span { margin-left: auto; color: var(--faint); font-size: .6875rem; }
  .operation { display: grid; grid-template-columns: 1rem 1fr auto; align-items: center; gap: var(--s3); min-height: 3.5rem; padding: var(--s3) var(--s4); }
  .operation i { width: .625rem; height: .625rem; border-radius: 50%; background: var(--accent); }
  .operation span, .operation strong, .operation small { display: block; }
  .operation small { margin-top: var(--s1); color: var(--faint); }
  .operation time { color: var(--faint); font: .625rem var(--font-num); }
  .summary { display: grid; grid-template-columns: repeat(2, 1fr); gap: var(--s4); margin-bottom: var(--s5); }
  .summary article { display: flex; align-items: baseline; gap: var(--s3); padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r-l); background: var(--surface); }
  .summary b { font: 1.25rem var(--font-num); }
  .summary span { color: var(--muted); font-size: .6875rem; }
  .queue { border: 1px solid var(--line-strong); border-radius: var(--r-l); background: var(--surface); overflow: hidden; }
  .suggestion { display: grid; grid-template-columns: minmax(14rem, 1fr) minmax(10rem, .6fr) minmax(14rem, 1fr) auto; align-items: center; gap: var(--s4); min-height: 6rem; padding: var(--s4); border-bottom: 1px solid var(--line-row); }
  .suggestion:last-child { border-bottom: 0; }
  .identity { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: var(--s2); }
  .identity span, .identity strong, .identity small { display: block; min-width: 0; }
  .identity strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .identity small { color: var(--faint); font-size: .625rem; }
  .identity i { color: var(--accent); font-style: normal; }
  .decision { display: grid; justify-items: start; gap: var(--s2); }
  .decision strong { font-size: .6875rem; }
  .evidence { display: flex; gap: var(--s2); flex-wrap: wrap; }
  .evidence span { display: grid; gap: var(--s1); padding: var(--s2); border: 1px solid var(--line); border-radius: var(--r-m); }
  .evidence b { font-size: .625rem; }
  .evidence code { color: var(--faint); font: .625rem var(--font-num); }
  .evidence .contradiction { border-color: color-mix(in srgb, var(--crit) 40%, var(--line)); }
  .evidence .contradiction b { color: var(--crit); }
  .actions { display: flex; justify-content: flex-end; gap: var(--s2); flex-wrap: wrap; }
  .weak { margin-top: var(--s5); padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r-l); color: var(--muted); }
  .weak summary { cursor: pointer; font-size: .75rem; }
  .weak p { margin-top: var(--s3); font-size: .6875rem; }
  .error { margin-top: var(--s4); color: var(--crit); }
  @media (max-width: 68rem) { .suggestion { grid-template-columns: 1fr 1fr; } .actions { justify-content: flex-start; } }
  @media (max-width: 48rem) { .summary, .suggestion { grid-template-columns: 1fr; } }
</style>
