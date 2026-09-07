<script lang="ts">
  import { page } from '$app/state';
  import { reconciliationState } from '$lib/reconciliation.svelte';
  import { startReconciliationPolling } from '$lib/reconciliation-polling';
  import Icon from './Icon.svelte';
  import Odometer from './Odometer.svelte';
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

{#if session.user?.role === 'administrator'}
  <a
    href="/cibles/rapprochements"
    aria-label={t('reconciliation.title')}
    aria-current={page.url.pathname.startsWith('/cibles/rapprochements') ? 'page' : undefined}
    title={active ? t(stages[active.stage]) : t('reconciliation.reviewTitle')}
  >
    <Icon name="signal" size={20} />
    <span class="nav-item-label">{t('reconciliation.title')}</span>
    {#if active || reconciliationState.actionable.length > 0}
      <span class="rail-indicator">
        {#if active}
          <i class="dot info" aria-hidden="true"></i>
        {:else}
          <b class="num"><Odometer value={reconciliationState.actionable.length} /></b>
        {/if}
      </span>
    {/if}
    <span class="visually-hidden" aria-live="polite">{active ? t(stages[active.stage]) : ''}</span>
  </a>
{/if}
