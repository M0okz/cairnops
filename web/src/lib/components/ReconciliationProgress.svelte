<script lang="ts">
  import { reconciliationState } from '$lib/reconciliation.svelte';
  import { startReconciliationPolling } from '$lib/reconciliation-polling';
  import { session } from '$lib/session.svelte';
  import type { ReconciliationStage } from '$lib/api';
  import { t, type MessageKey } from '$lib/i18n.svelte';

  const stages: Record<ReconciliationStage, MessageKey> = {
    preparing: 'reconciliation.stage.preparing',
    consolidating: 'reconciliation.stage.consolidating',
    reconciling_incidents: 'reconciliation.stage.incidents',
    recalculating_metrics: 'reconciliation.stage.metrics',
    finalizing: 'reconciliation.stage.finalizing',
    completed: 'reconciliation.stage.completed',
    failed: 'reconciliation.stage.failed'
  };
  const watched = new Set<string>();
  const announced = new Set<string>();
  const hasActiveOperations = $derived(reconciliationState.activeOperations.length > 0);

  $effect(() => {
    if (session.user?.role !== 'administrator') return;
    const activePolling = hasActiveOperations;
    return startReconciliationPolling(
      () => reconciliationState.load(),
      () => activePolling
    );
  });

  const active = $derived(reconciliationState.activeOperations[0] ?? null);

  $effect(() => {
    for (const operation of reconciliationState.activeOperations) watched.add(operation.id);
    for (const operation of reconciliationState.operations) {
      if (operation.status !== 'succeeded' || !watched.has(operation.id) || announced.has(operation.id)) continue;
      announced.add(operation.id);
      session.showNotice(
        operation.kind === 'source_move'
          ? t('reconciliation.sourceCompleted', { name: operation.primary_target_name })
          : t('reconciliation.targetsCompleted', { name: operation.primary_target_name })
      );
      void Promise.all([session.loadTargets(), session.loadIncidents(), session.loadMeasures()]);
    }
  });
</script>

{#if active}
  <a class="progress" href="/cibles/rapprochements" aria-live="polite" title={t(stages[active.stage])}>
    <i></i>
    <span>{t(stages[active.stage])}</span>
  </a>
{:else if reconciliationState.actionable.length > 0}
  <a class="review" href="/cibles/rapprochements" title={t('reconciliation.reviewTitle')}>
    <span>{t('reconciliation.title')}</span><b>{reconciliationState.actionable.length}</b>
  </a>
{/if}

<style>
  .progress, .review { display: flex; align-items: center; gap: var(--s2); min-height: var(--ctl-h); padding: 0 var(--s3); border: 1px solid var(--line-strong); border-radius: var(--r-m); color: var(--muted); font-size: .6875rem; white-space: nowrap; }
  .progress i { width: .5rem; height: .5rem; border-radius: 50%; background: var(--accent); }
  .progress span { max-width: 13rem; overflow: hidden; text-overflow: ellipsis; }
  .review b { min-width: 1.125rem; height: 1.125rem; display: grid; place-items: center; border-radius: var(--r-pill); background: var(--accent); color: var(--accent-ink); font: .625rem var(--font-num); }
  @media (max-width: 68rem) { .progress span, .review span { display: none; } }
</style>
