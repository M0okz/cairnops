<script lang="ts">
  import { untrack } from 'svelte';
  import IndicatorHistoryChart from './IndicatorHistoryChart.svelte';
  import SegmentedControl from './ui/SegmentedControl.svelte';
  import { session } from '$lib/session.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { indicatorTimeBounds, indicatorWindowPoints, type IndicatorPeriod } from '$lib/indicator-history';
  import { severityTone, since, stamp } from '$lib/format';
  import { t } from '$lib/i18n.svelte';
  import type { ContextIndicator, Incident } from '$lib/api';

  let { targetId, incident = null }: { targetId: string; incident?: Incident | null } = $props();
  const id = $props.id();
  let period = $state<IndicatorPeriod>('24h');
  let now = $state(new Date());
  let loading = $state(true);
  let failed = $state(false);
  let retry = $state(0);
  let pinning = $state(false);
  const window = $derived(period === '7d' ? '7d' : '24h');
  const detail = $derived(session.indicatorDetails[`${targetId}:${window}`] ?? null);
  const timeBounds = $derived(indicatorTimeBounds(detail?.generated_at ?? '', period));
  const markerVisible = $derived(Boolean(incident && timeBounds && Date.parse(incident.opened_at) >= timeBounds[0] && Date.parse(incident.opened_at) <= timeBounds[1]));
  const marker = $derived(incident && markerVisible ? {
    at: incident.opened_at,
    label: t('incidents.detail.openingMarker'),
    tone: severityTone(incident.severity) as 'info' | 'warn' | 'crit'
  } : null);

  $effect(() => {
    void retry;
    const target = targetId;
    const selectedWindow = window;
    let disposed = false;
    loading = !untrack(() => session.indicatorDetails[`${target}:${selectedWindow}`]);
    failed = false;
    void session.loadTargetIndicators(target, selectedWindow).then((result) => {
      if (disposed) return;
      failed = !result;
      loading = false;
    });
    return () => { disposed = true; };
  });
  $effect(() => { const timer = setInterval(() => (now = new Date()), 30_000); return () => clearInterval(timer); });

  function connectorAddress(connectorId: string): string | null {
    const endpoint = session.connectors.find((connector) => connector.id === connectorId)?.endpoint;
    if (!endpoint) return null;
    try { const url = new URL(endpoint); url.pathname = '/'; url.search = ''; url.hash = ''; return url.toString(); } catch { return endpoint; }
  }

  async function togglePin(indicator: ContextIndicator) {
    if (pinning) return;
    pinning = true;
    try {
      const saved = await session.toggleIndicatorPin(indicator);
      // The shared pin action refreshes 24 h; also refresh the displayed weekly projection.
      if (saved && window === '7d') failed = !await session.loadTargetIndicators(indicator.target_id, '7d');
    } finally { pinning = false; }
  }
</script>

<section class="target-indicators" aria-labelledby={`target-indicators-${id}`}>
  <header>
    <div>
      <h2 id={`target-indicators-${id}`}>{t('target.indicators.title')}</h2>
      <p>{t('target.indicators.subtitle')}</p>
    </div>
    <SegmentedControl label={t('dashboard.period')} value={period} items={[
      { value: '1h', label: t('chart.1h') }, { value: '24h', label: t('dashboard.24h') }, { value: '7d', label: t('dashboard.7d') }
    ]} onValueChange={(value) => (period = value)} />
  </header>
  {#if loading || failed || !detail?.indicators.length}
    <div class="series-message card" role="status" aria-busy={loading}>
      <p>{t(failed ? 'dashboard.seriesError' : loading ? 'dashboard.loading' : 'dashboard.noIndicators')}</p>
      {#if failed}<button class="btn" type="button" onclick={() => retry += 1}>{t('chart.retry')}</button>{/if}
    </div>
  {:else}
    <div class="indicator-list">
      {#each detail.indicators as indicator (indicator.id)}
        {@const points = indicatorWindowPoints(detail.series?.[indicator.id] ?? [], timeBounds)}
        {@const hasMaximum = period === '7d' && points.some((point) => point.maximum !== undefined && Number.isFinite(point.maximum))}
        <article class="indicator-card card" aria-label={indicator.label}>
          <div class="indicator-title">
            <div><h3>{indicator.label}</h3>{#if indicator.dimension}<p>{indicator.dimension}</p>{/if}</div>
            <button class="pin" class:active={indicator.pinned} type="button" disabled={pinning} aria-pressed={indicator.pinned} aria-label={t(indicator.pinned ? 'target.indicators.unpin' : 'target.indicators.pin', { label: indicator.label })} onclick={() => togglePin(indicator)}>{indicator.pinned ? '◆' : '◇'}</button>
          </div>
          <div class="indicator-value">
            <div class="chart-legend" aria-label={t('chart.series')}>
              <span><i></i>{t(period === '7d' ? 'chart.hourlyLatestLegend' : 'dashboard.context')}</span>
              {#if hasMaximum}<span class="maximum"><i></i>{t('chart.hourlyMaximum')}</span>{/if}
            </div>
            <span class="latest-value"><span>{t('chart.latest')}</span><b>{formatIndicator(indicator.last_value, indicator.unit)}</b></span>
          </div>
          <div class="indicator-chart">
            {#if points.length}
              <IndicatorHistoryChart {points} {timeBounds} {marker} unit={indicator.unit} label={indicator.label} hourly={period === '7d'} />
            {:else}<div class="series-message" role="status">{t('dashboard.noSeries')}</div>{/if}
          </div>
          <footer class="indicator-meta">
            <span class:warn={Boolean(indicator.last_error)}>{indicator.last_error || (indicator.last_observed_at ? t('dashboard.freshness', { duration: since(indicator.last_observed_at, now) }) : t('overview.indicators.neverObserved'))}</span>
            {#if connectorAddress(indicator.connector_id)}<a href={connectorAddress(indicator.connector_id)!} target="_blank" rel="noreferrer">{t('target.indicators.openSource')} ↗</a>{/if}
          </footer>
        </article>
      {/each}
    </div>
  {/if}
  <footer class="context-note">
    <p>{t('target.indicators.contextNote')}</p>
    {#if incident}
      <p class="incident-marker-note"><i class={severityTone(incident.severity)} aria-hidden="true"></i>{t(markerVisible ? 'target.indicators.markerNote' : 'target.indicators.markerOutside', { date: stamp(incident.opened_at) })}</p>
    {/if}
  </footer>
</section>

<style>
  .target-indicators { margin-bottom: var(--s5); min-width: 0; container-type: inline-size; }
  .target-indicators > header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--s4); margin-bottom: var(--s5); }
  header > div:first-child { min-width: 0; flex: 1; }
  h2 { font-size: var(--text-md); }
  header p { margin-top: var(--s2); color: var(--faint); font-size: var(--text-xs); }
  .indicator-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s5); }
  .indicator-card { min-width: 0; border-radius: var(--r-chart); overflow: hidden; }
  .indicator-card:last-child:nth-child(odd) { grid-column: 1 / -1; }
  .indicator-title { display: flex; align-items: flex-start; gap: var(--s4); padding: var(--s5) var(--s5) var(--s4); }
  .indicator-title > div { min-width: 0; flex: 1; }
  h3 { font-size: var(--text-md); overflow-wrap: anywhere; }
  .indicator-title p { color: var(--faint); font-size: var(--text-xs); margin-top: var(--s2); overflow-wrap: anywhere; }
  .indicator-value { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--s3) var(--s4); padding: 0 var(--s5) var(--s4); }
  .chart-legend { display: flex; flex-wrap: wrap; gap: var(--s3) var(--s4); }
  .chart-legend span { display: flex; align-items: center; gap: var(--s3); color: var(--faint); font-size: var(--text-xs); }
  .chart-legend i { width: var(--s1); height: var(--s3); border-radius: var(--r-s); background: var(--chart-series); }
  .chart-legend .maximum i { background: var(--chart-maximum); }
  .latest-value { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--s3); color: var(--faint); font-size: var(--text-xs); }
  .latest-value b { color: var(--ink); font: var(--weight-medium) var(--text-sm) var(--font-num); font-variant-numeric: tabular-nums; }
  .indicator-chart { padding: 0 var(--s4); }
  .indicator-meta { display: flex; justify-content: space-between; align-items: baseline; flex-wrap: wrap; gap: var(--s3) var(--s4); padding: var(--s4) var(--s5); color: var(--faint); font-size: var(--text-xs); }
  .indicator-meta span { min-width: 0; overflow-wrap: anywhere; }
  .indicator-meta a { color: var(--muted); white-space: nowrap; }
  .pin { width: var(--ctl-h); height: var(--ctl-h); flex: none; border: 1px solid var(--line); border-radius: var(--r-m); background: var(--surface); color: var(--faint); }
  .pin.active { color: var(--accent); border-color: var(--accent); background: var(--surface-2); }
  .series-message { min-height: var(--chart-history-height); display: flex; flex-direction: column; align-items: center; justify-content: center; gap: var(--s4); padding: var(--s5); color: var(--faint); font-size: var(--text-sm); text-align: center; }
  .context-note { display: grid; gap: var(--s3); margin-top: var(--s4); color: var(--faint); font-size: var(--text-xs); line-height: 1.5; }
  .incident-marker-note { display: flex; align-items: baseline; gap: var(--s3); }
  .incident-marker-note i { width: var(--s3); height: var(--s3); flex: none; border: 1px solid currentColor; border-radius: var(--r-pill); }
  .incident-marker-note i.info { color: var(--info); } .incident-marker-note i.warn { color: var(--warn); } .incident-marker-note i.crit { color: var(--crit); }
  @container (max-width: 48rem) { .indicator-list { grid-template-columns: minmax(0, 1fr); } }
  @container (max-width: 28rem) {
    .target-indicators > header { align-items: flex-start; flex-direction: column; }
    .indicator-title { padding: var(--s4); } .indicator-value, .indicator-meta { padding-inline: var(--s4); } .indicator-chart { padding-inline: var(--s3); }
  }
  @media (hover: hover) { .pin:hover { background: var(--surface-2); color: var(--ink); } .indicator-meta a:hover { color: var(--ink); } }
</style>
