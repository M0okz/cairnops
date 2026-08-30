<script lang="ts">
  import Odometer from './Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { healthyEvidenceWindow } from '$lib/overview';
  import { ratio, stateLabel, stateTones, type TargetState } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  const states: TargetState[] = ['ok', 'degraded', 'down', 'maintenance', 'unknown'];
  const ringRadius = 47;
  const ringCircumference = 2 * Math.PI * ringRadius;
  const chartWidth = 640;
  const chartHeight = 112;
  const chartInsetX = 8;
  const chartInsetY = 10;

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
    const points = healthyHours
      .map((value, index) => (value === null ? null : { value, index }))
      .filter((point): point is { value: number; index: number } => point !== null);

    if (points.length === 0) {
      return {
        paths: [] as string[],
        points: [] as Array<{ x: number; y: number; value: number; index: number }>,
        current: null as number | null,
        low: null as number | null
      };
    }

    const low = Math.min(...points.map((point) => point.value));
    const lowerBound = Math.max(0, low - 0.05);
    const span = Math.max(0.05, 1 - lowerBound);
    const slots = Math.max(2, healthyHours.length);
    const coordinates = points.map((point) => ({
      ...point,
      x: chartInsetX + (point.index / (slots - 1)) * (chartWidth - chartInsetX * 2),
      y:
        chartHeight -
        chartInsetY -
        ((point.value - lowerBound) / span) * (chartHeight - chartInsetY * 2)
    }));

    const groups: typeof coordinates[] = [];
    for (const point of coordinates) {
      const group = groups.at(-1);
      if (!group || point.index !== group.at(-1)!.index + 1) groups.push([point]);
      else group.push(point);
    }

    return {
      paths: groups.map((group) =>
        group
          .map((point, index) =>
            `${index === 0 ? 'M' : 'L'}${point.x.toFixed(2)},${point.y.toFixed(2)}`
          )
          .join('')
      ),
      points: coordinates,
      current: coordinates.at(-1)?.value ?? null,
      low
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

      <svg
        class="evidence-chart"
        viewBox={`0 0 ${chartWidth} ${chartHeight}`}
        role="img"
        aria-label={trendLabel}
        preserveAspectRatio="none"
      >
        <title>{trendLabel}</title>
        <path class="guide" d={`M${chartInsetX},${chartInsetY}H${chartWidth - chartInsetX}`} />
        <path
          class="guide lower"
          d={`M${chartInsetX},${chartHeight - chartInsetY}H${chartWidth - chartInsetX}`}
        />
        {#if trend.paths.length > 0}
          {#each trend.paths as path, index (`${index}-${path}`)}
            <path class="trend-line" class:sparse={path.indexOf('L') === -1} d={path} />
          {/each}
          {#if trend.points.length > 0}
            {@const point = trend.points.at(-1)!}
            <circle class="current-point" cx={point.x} cy={point.y} r="3" />
          {/if}
        {:else}
          <path
            class="empty-line"
            d={`M${chartInsetX},${chartHeight / 2}H${chartWidth - chartInsetX}`}
          />
        {/if}
      </svg>

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
    display: block;
    width: 100%;
    height: 7rem;
    margin-top: var(--s3);
    overflow: visible;
    color: var(--ok);
  }

  .guide {
    fill: none;
    stroke: var(--line-row);
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }

  .guide.lower {
    stroke: var(--line);
  }

  .trend-line {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.5;
    stroke-linecap: round;
    stroke-linejoin: round;
    vector-effect: non-scaling-stroke;
  }

  .trend-line.sparse {
    stroke-dasharray: 3 4;
  }

  .current-point {
    fill: var(--surface);
    stroke: currentColor;
    stroke-width: 1.75;
    vector-effect: non-scaling-stroke;
  }

  .empty-line {
    fill: none;
    stroke: var(--dim);
    stroke-width: 1;
    stroke-dasharray: 4 5;
    vector-effect: non-scaling-stroke;
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
