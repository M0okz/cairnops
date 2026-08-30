<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import Odometer from './Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { since } from '$lib/format';
  import { t } from '$lib/i18n.svelte';

  let now = $state(new Date());
  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });

  const pinned = $derived(
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
          (left.indicator.pin_position ?? 99) - (right.indicator.pin_position ?? 99)
      )
      .slice(0, 4)
  );
</script>

{#if pinned.length > 0}
  <section class="indicator-overview" aria-labelledby="overview-indicators-title">
    <div class="band">
      <div>
        <h2 id="overview-indicators-title">{t('overview.indicators.title')}</h2>
        <p>{t('overview.indicators.hint')}</p>
      </div>
      <span class="tally">{pinned.length}/4</span>
    </div>
    <div class="card indicator-rack">
      {#each pinned as row (row.indicator.id)}
        {@const value = formatIndicator(row.indicator.last_value, row.indicator.unit)}
        <a
          class="indicator-cell"
          href="/cibles/{row.target.target_id}"
          aria-label={`${row.indicator.label}, ${row.name}, ${value}`}
        >
          <div class="indicator-head">
            <span><strong>{row.indicator.label}</strong><small>{row.name}</small></span>
            <b class="num"><Odometer {value} /></b>
          </div>
          <IndicatorAreaChart
            compact
            points={row.target.series?.[row.indicator.id] ?? []}
            unit={row.indicator.unit}
            label={t('overview.indicators.chartLabel', { indicator: row.indicator.label })}
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

  .band h2 {
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .band > div {
    min-width: 0;
  }

  .band p {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.6875rem;
  }

  .tally {
    min-width: var(--counter-pill-h);
    height: var(--counter-pill-h);
    margin-left: auto;
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
