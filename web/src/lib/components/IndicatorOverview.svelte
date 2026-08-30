<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import IndicatorPersonalizer from './IndicatorPersonalizer.svelte';
  import Odometer from './Odometer.svelte';
  import type { ContextIndicator } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { since } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  let now = $state(new Date());
  let personalizing = $state(false);
  let personalizerTrigger = $state<HTMLButtonElement | null>(null);
  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });

  const displayed = $derived(
    Object.values(session.indicatorOverview)
      .flatMap((target) =>
        target.indicators.map((indicator) => ({
          indicator,
          target,
          name:
            session.targets.find((item) => item.id === target.target_id)?.name ??
            t('overview.indicators.unknownTarget')
        }))
      )
      .sort(
        (left, right) =>
          (left.indicator.overview_position ?? 99) -
          (right.indicator.overview_position ?? 99)
      )
      .slice(0, 4)
  );

  function suggestedLabel(indicator: ContextIndicator): string {
    if (indicator.pinned) return indicator.label;
    const labels: Partial<Record<ContextIndicator['semantic_key'], string>> = {
      'filesystem.utilization': t('overview.indicators.suggestion.filesystem'),
      'memory.utilization': t('overview.indicators.suggestion.memory'),
      'response.time': t('overview.indicators.suggestion.response'),
      'certificate.days_remaining': t('overview.indicators.suggestion.certificateDays'),
      'security_updates.count': t('overview.indicators.suggestion.securityUpdates'),
      'updates.count': t('overview.indicators.suggestion.updates'),
      'reboot.required': t('overview.indicators.suggestion.reboot'),
      'reporting.age': t('overview.indicators.suggestion.reportingAge'),
      'cpu.utilization': t('overview.indicators.suggestion.cpu'),
      'network.in': t('overview.indicators.suggestion.networkIn'),
      'network.out': t('overview.indicators.suggestion.networkOut'),
      'certificate.valid': t('overview.indicators.suggestion.certificateValid')
    };
    return labels[indicator.semantic_key] ?? indicator.label;
  }

  function dismissPersonalizer() {
    personalizing = false;
    requestAnimationFrame(() => personalizerTrigger?.focus());
  }
</script>

{#if displayed.length > 0}
  <section class="indicator-overview" aria-labelledby="overview-indicators-title">
    <div class="band">
      <div class="band-copy">
        <div class="title-line">
          <h2 id="overview-indicators-title">{t('overview.indicators.title')}</h2>
          <span class="tally">{displayed.length}</span>
        </div>
        <p>{plural('overview.indicators.available', displayed.length)}</p>
      </div>
      <button
        bind:this={personalizerTrigger}
        class="btn sm personalize"
        type="button"
        onclick={() => (personalizing = true)}
      >
        {t('overview.indicators.personalize')}
      </button>
    </div>
    <div class="card indicator-rack">
      {#each displayed as row (row.indicator.id)}
        {@const label = suggestedLabel(row.indicator)}
        {@const value = formatIndicator(row.indicator.last_value, row.indicator.unit)}
        <a
          class="indicator-cell"
          href="/cibles/{row.target.target_id}"
          aria-label={`${label}, ${row.name}, ${value}`}
        >
          <div class="indicator-head">
            <span>
              <strong>{label}</strong>
              <small>{row.name}{row.indicator.dimension ? ` · ${row.indicator.dimension}` : ''}</small>
            </span>
            <b class="num"><Odometer {value} /></b>
          </div>
          <IndicatorAreaChart
            compact
            interactive
            focusable={false}
            points={row.target.series?.[row.indicator.id] ?? []}
            unit={row.indicator.unit}
            label={t('overview.indicators.chartLabel', { indicator: label })}
          />
          <small class:stale={Boolean(row.indicator.last_error)}>
            {row.indicator.last_observed_at
              ? t('overview.indicators.observed', {
                  duration: since(row.indicator.last_observed_at, now)
                })
              : t('overview.indicators.neverObserved')}{row.indicator.last_error
              ? ` · ${row.indicator.last_error}`
              : ''}
          </small>
        </a>
      {/each}
    </div>
  </section>
{/if}

{#if personalizing}
  <IndicatorPersonalizer ondismiss={dismissPersonalizer} />
{/if}

<style>
  .indicator-overview {
    margin-bottom: var(--s5);
  }

  .band {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin: 0 0 var(--s4);
  }

  .band-copy {
    min-width: 0;
    flex: 1;
  }

  .title-line {
    display: flex;
    align-items: center;
    gap: var(--s3);
  }

  .band h2 {
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .band p {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.6875rem;
  }

  .tally {
    min-width: var(--counter-pill-h);
    height: var(--counter-pill-h);
    padding: 0 calc(var(--s3) - var(--s1));
    display: inline-grid;
    place-items: center;
    border-radius: var(--r-pill);
    background: var(--surface-2);
    color: var(--faint);
    font-family: var(--font-num);
    font-size: 0.625rem;
    line-height: 1;
  }

  .personalize {
    flex: none;
  }

  .indicator-rack {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    overflow: hidden;
  }

  .indicator-cell {
    min-width: 0;
    padding: var(--s4);
    border-left: 1px solid var(--line);
    transition: background var(--d1) var(--ease);
  }

  .indicator-cell:first-child {
    border-left: 0;
  }

  .indicator-cell:hover {
    background: var(--surface-2);
  }

  .indicator-head {
    display: flex;
    align-items: flex-start;
    gap: var(--s3);
  }

  .indicator-head > span {
    min-width: 0;
    flex: 1;
  }

  .indicator-head strong,
  .indicator-head small {
    display: block;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .indicator-head strong {
    font-size: 0.75rem;
  }

  .indicator-head small,
  .indicator-cell > small {
    color: var(--faint);
    font-size: 0.625rem;
  }

  .indicator-head b {
    color: var(--ink);
    font-size: 0.8125rem;
    white-space: nowrap;
  }

  .indicator-cell > small.stale {
    color: var(--warn);
  }

  @media (max-width: 68rem) {
    .indicator-rack {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .indicator-cell:nth-child(odd) {
      border-left: 0;
    }

    .indicator-cell:nth-child(n + 3) {
      border-top: 1px solid var(--line);
    }
  }

  @media (max-width: 22rem) {
    .band {
      align-items: flex-start;
      flex-direction: column;
    }

    .indicator-rack {
      grid-template-columns: minmax(0, 1fr);
    }

    .indicator-cell {
      border-top: 1px solid var(--line);
      border-left: 0;
    }

    .indicator-cell:first-child {
      border-top: 0;
    }
  }
</style>
