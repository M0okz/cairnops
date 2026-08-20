<script lang="ts">
  import type { ReconciliationCounts } from '$lib/reconciliation';
  import { t } from '$lib/i18n.svelte';
  import Odometer from './Odometer.svelte';

  let { counts }: { counts: ReconciliationCounts } = $props();
</script>

<section class="reconciliation" class:attention={counts.review > 0} aria-live="polite">
  <div class="summary-copy">
    <strong>{counts.review > 0 ? t('wizard.reviewNeeded') : t('wizard.reconciliationReady')}</strong>
    <small>{counts.review > 0 ? t('wizard.reviewNeededNote') : t('wizard.reconciliationReadyNote')}</small>
  </div>
  <dl>
    <div>
      <dt>{t('wizard.existingTargets')}</dt>
      <dd><Odometer value={counts.reused} /></dd>
    </div>
    <div>
      <dt>{t('wizard.newTargets')}</dt>
      <dd><Odometer value={counts.created} /></dd>
    </div>
    <div class:warn={counts.review > 0}>
      <dt>{t('wizard.toConfirm')}</dt>
      <dd><Odometer value={counts.review} /></dd>
    </div>
  </dl>
</section>

<style>
  .reconciliation {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s4);
    padding: var(--s3) var(--s4);
    margin-bottom: var(--s3);
    border: 1px solid var(--ok-line);
    border-left: 0.1875rem solid var(--ok);
    border-radius: var(--r-m);
    background: var(--ok-bg);
  }

  .reconciliation.attention {
    border-color: var(--warn-line);
    border-left-color: var(--warn);
    background: var(--warn-bg);
  }

  .summary-copy strong,
  .summary-copy small {
    display: block;
  }

  .summary-copy strong {
    font-size: 0.75rem;
    font-weight: 600;
  }

  .summary-copy small {
    margin-top: 0.125rem;
    color: var(--muted);
    font-size: 0.625rem;
  }

  dl {
    display: flex;
    align-items: stretch;
    gap: var(--s2);
    margin: 0;
  }

  dl div {
    min-width: 4.25rem;
    padding: 0.3125rem var(--s2);
    border-left: 1px solid var(--line-strong);
    text-align: right;
  }

  dt {
    color: var(--faint);
    font-size: 0.5625rem;
  }

  dd {
    margin: 0;
    color: var(--ink);
    font-family: var(--font-num);
    font-size: 0.875rem;
    font-weight: 650;
  }

  dl div.warn dt,
  dl div.warn dd {
    color: var(--warn);
  }

  @media (max-width: 40rem) {
    .reconciliation {
      align-items: stretch;
      flex-direction: column;
    }

    dl {
      justify-content: space-between;
    }

    dl div {
      flex: 1;
      text-align: left;
    }
  }
</style>
