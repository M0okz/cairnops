<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import Odometer from './Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { healthyEvidenceWindow } from '$lib/overview';
  import { ratio, stateLabel, stateTones, type TargetState } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  const states: TargetState[] = ['ok', 'degraded', 'down', 'maintenance', 'unknown'];
  const ringRadius = 47;
  const ringCircumference = 2 * Math.PI * ringRadius;
  const hourMilliseconds = 60 * 60 * 1_000;

  const distribution = $derived.by(() =>
    states.map((state) => ({
      state,
      tone: stateTones[state],
      label: stateLabel(state),
      count: session.targets.filter((target) => session.targetState(target) === state).length
    }))
  );

  const operational = $derived(distribution.find((entry) => entry.state === 'ok')?.count ?? 0);
  const total = $derived(session.targets.length);

  const ringSegments = $derived.by(() => {
    let offset = 0;
    return distribution
      .filter((entry) => entry.count > 0 && total > 0)
      .map((entry) => {
        const length = (entry.count / total) * ringCircumference;
        const segment = { ...entry, length, offset };
        offset += length;
        return segment;
      });
  });

  const distributionLabel = $derived(
    t('overview.fleet.distributionLabel', {
      details: distribution.map((entry) => `${entry.label} : ${entry.count}`).join(', ')
    })
  );

  const healthyHours = $derived(
    healthyEvidenceWindow(session.system?.hours ?? [], session.system?.checked_at)
  );

  const trend = $derived.by(() => {
    const checkedAt = session.system?.checked_at
      ? new Date(session.system.checked_at).getTime()
      : Number.NaN;
    const currentHour = Number.isFinite(checkedAt)
      ? Math.floor(checkedAt / hourMilliseconds) * hourMilliseconds
      : Number.NaN;
    const firstHour = currentHour - Math.max(0, healthyHours.length - 1) * hourMilliseconds;
    const points = healthyHours
      .map((value, index) =>
        value === null || !Number.isFinite(firstHour)
          ? null
          : {
              at: new Date(firstHour + index * hourMilliseconds).toISOString(),
              value: value * 100
            }
      )
      .filter((point): point is { at: string; value: number } => point !== null);

    if (points.length === 0) {
      return {
        points,
        current: null as number | null,
        low: null as number | null,
        bounds: null as [number, number] | null
      };
    }

    const values = points.map((point) => point.value / 100);
    const low = Math.min(...values);

    return {
      points,
      current: values.at(-1) ?? null,
      low,
      bounds: [Math.max(0, low * 100 - 5), 100] as [number, number]
    };
  });

  const trendLabel = $derived(
    trend.current === null || trend.low === null
      ? t('overview.fleet.noHourlyEvidence')
      : t('overview.fleet.healthyEvidenceLabel', {
          current: ratio(trend.current),
          low: ratio(trend.low)
        })
  );
</script>

<section class="fleet-overview" aria-labelledby="fleet-overview-title">
  <div class="band">
    <h2 id="fleet-overview-title">{t('overview.fleet.title')}</h2>
  </div>

  <div class="card fleet-card">
    <div class="distribution-panel">
      <div class="panel-copy">
        <span class="eyebrow">{t('overview.fleet.currentState')}</span>
        <div class="ring-layout">
          <svg
            class="state-ring"
            viewBox="0 0 120 120"
            role="img"
            aria-label={distributionLabel}
          >
            <title>{distributionLabel}</title>
            <circle class="ring-track" cx="60" cy="60" r={ringRadius} />
            {#each ringSegments as segment (segment.state)}
              <circle
                class="ring-segment {segment.tone}"
                cx="60"
                cy="60"
                r={ringRadius}
                stroke-dasharray={`${segment.length} ${ringCircumference - segment.length}`}
                stroke-dashoffset={-segment.offset}
                transform="rotate(-90 60 60)"
              />
            {/each}
            <text class="ring-value" x="60" y="57" text-anchor="middle">{operational}</text>
            <text class="ring-caption" x="60" y="72" text-anchor="middle">
              {t('overview.fleet.ofTargets', { count: total })}
            </text>
          </svg>

          <div class="state-legend" aria-hidden="true">
            {#each distribution as entry (entry.state)}
              <span>
                <i class="dot {entry.tone}"></i>
                <span>{entry.label}</span>
                <b class="num"><Odometer value={entry.count} /></b>
              </span>
            {/each}
          </div>
        </div>
      </div>
    </div>

    <div class="trend-panel">
      <div class="trend-head">
        <div>
          <span class="eyebrow">{t('overview.fleet.healthyEvidence')}</span>
          <p>{t('overview.fleet.healthyEvidenceHint')}</p>
        </div>
        <b class="trend-value num" class:dim={trend.current === null}>
          <Odometer value={ratio(trend.current)} />
        </b>
      </div>

      <div class="evidence-chart">
        <IndicatorAreaChart
          interactive
          points={trend.points}
          unit="percent"
          label={trendLabel}
          tone="ok"
          bounds={trend.bounds}
          gapThresholdMilliseconds={90 * 60 * 1_000}
        />
      </div>

      <div class="trend-meta">
        <span>
          {plural(
            'overview.fleet.measuredHours',
            trend.points.length
          )}
        </span>
        {#if trend.low !== null}
          <span>{t('overview.fleet.lowPoint', { value: ratio(trend.low) })}</span>
        {:else}
          <span>{t('overview.fleet.noHourlyEvidence')}</span>
        {/if}
      </div>
    </div>
  </div>
</section>

<style>
  .fleet-overview {
    margin-bottom: var(--s6);
  }

  .band {
    display: flex;
    align-items: center;
    margin: 0 0 var(--s4);
  }

  .band h2 {
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .fleet-card {
    display: grid;
    grid-template-columns: minmax(19rem, 0.82fr) minmax(0, 1.18fr);
    overflow: hidden;
  }

  .distribution-panel,
  .trend-panel {
    min-width: 0;
    padding: var(--s5);
  }

  .trend-panel {
    border-left: 1px solid var(--line);
  }

  .eyebrow {
    display: block;
    color: var(--muted);
    font-size: 0.75rem;
    font-weight: 600;
  }

  .ring-layout {
    display: grid;
    grid-template-columns: 7.5rem minmax(0, 1fr);
    align-items: center;
    gap: var(--s5);
    margin-top: var(--s4);
  }

  .state-ring {
    display: block;
    width: 7.5rem;
    height: 7.5rem;
    overflow: visible;
  }

  .ring-track,
  .ring-segment {
    fill: none;
    stroke-width: 11;
  }

  .ring-track {
    stroke: var(--surface-3);
  }

  .ring-segment {
    stroke-linecap: butt;
  }

  .ring-segment.ok { stroke: var(--ok); }
  .ring-segment.warn { stroke: var(--warn); }
  .ring-segment.crit { stroke: var(--crit); }
  .ring-segment.info { stroke: var(--info); }
  .ring-segment.idle { stroke: var(--dim); }

  .ring-value,
  .ring-caption {
    fill: var(--ink);
    font-family: var(--font-num);
    font-variant-numeric: tabular-nums;
  }

  .ring-value {
    font-size: 1.375rem;
    font-weight: 600;
  }

  .ring-caption {
    fill: var(--faint);
    font-size: 0.5625rem;
  }

  .state-legend {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--s3);
  }

  .state-legend > span {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--s3);
    color: var(--muted);
    font-size: 0.6875rem;
  }

  .state-legend b {
    color: var(--ink);
    font-size: 0.75rem;
    font-weight: 600;
  }

  .trend-head {
    display: flex;
    align-items: flex-start;
    gap: var(--s5);
  }

  .trend-head > div {
    min-width: 0;
    flex: 1;
  }

  .trend-head p {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.6875rem;
  }

  .trend-value {
    color: var(--ink);
    font-size: 1rem;
    font-weight: 600;
    white-space: nowrap;
  }

  .evidence-chart {
    margin-top: var(--s3);
  }

  .trend-meta {
    display: flex;
    justify-content: space-between;
    gap: var(--s4);
    color: var(--faint);
    font-size: 0.6875rem;
  }

  @media (max-width: 68rem) {
    .fleet-card {
      grid-template-columns: minmax(0, 1fr);
    }

    .trend-panel {
      border-top: 1px solid var(--line);
      border-left: 0;
    }
  }

  @media (max-width: 36rem) {
    .distribution-panel,
    .trend-panel {
      padding: var(--s4);
    }

    .ring-layout {
      grid-template-columns: minmax(0, 1fr);
      justify-items: center;
    }

    .state-legend {
      width: 100%;
    }

    .trend-head {
      align-items: end;
    }
  }

</style>
