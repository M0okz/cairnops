<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import IndicatorPersonalizer from './IndicatorPersonalizer.svelte';
  import Icon from './Icon.svelte';
  import Odometer from './Odometer.svelte';
  import SegmentedControl from './ui/SegmentedControl.svelte';
  import type { ContextIndicator } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { since } from '$lib/format';
  import { t } from '$lib/i18n.svelte';

  let now = $state(new Date());
  let personalizing = $state(false);
  let personalizerTrigger = $state<HTMLButtonElement | null>(null);
  let selectedID = $state('');
  let period = $state<'24h' | '7d'>('24h');
  let loading = $state(false);
  let failed = $state(false);
  let request = 0;
  const selectorID = $props.id();
  const displayed = $derived(Object.values(session.indicatorOverview)
    .flatMap((target) => target.indicators.map((indicator) => ({ indicator, target,
      name: session.targets.find((item) => item.id === target.target_id)?.name ?? t('overview.indicators.unknownTarget') })))
    .sort((a, b) => (a.indicator.overview_position ?? 99) - (b.indicator.overview_position ?? 99)).slice(0, 4));
  const selected = $derived(displayed.find((row) => row.indicator.id === selectedID) ?? displayed[0]);
  const targetID = $derived(selected?.target.target_id);
  const points = $derived(selected ? (period === '24h' ? selected.target : session.indicatorDetails[`${selected.target.target_id}:7d`])?.series?.[selected.indicator.id] ?? [] : []);
  const label = $derived(selected ? suggestedLabel(selected.indicator) : t('overview.indicators.title'));

  $effect(() => {
    const selectedTargetID = targetID;
    const window = period;
    let disposed = false;
    const load = async (initial = false) => {
      const current = ++request;
      failed = false;
      loading = initial && window === '7d' && Boolean(selectedTargetID);
      if (!selectedTargetID || window !== '7d') return;
      const result = await session.loadTargetIndicators(selectedTargetID, window);
      if (disposed || current !== request) return;
      failed = !result;
      loading = false;
    };
    void load(true);
    const timer = setInterval(() => { if (window === '7d') void load(); }, 60_000);
    return () => { disposed = true; clearInterval(timer); };
  });

  $effect(() => { const timer = setInterval(() => now = new Date(), 30_000); return () => clearInterval(timer); });

  function dismissPersonalizer() {
    personalizing = false;
    requestAnimationFrame(() => personalizerTrigger?.focus());
  }
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


</script>

<section class="indicator-overview card" aria-labelledby="overview-indicators-title">
  <div class="analysis-head">
    <div class="analysis-copy">
      <h2 id="overview-indicators-title">{label}</h2>
      {#if selected}<p>{selected.name}{selected.indicator.dimension ? ` · ${selected.indicator.dimension}` : ''}</p>{/if}
    </div>
    <div class="analysis-controls">
      {#if selected}
        <SegmentedControl label={t('dashboard.period')} value={period} items={[
          { value: '24h', label: t('dashboard.24h') }, { value: '7d', label: t('dashboard.7d') }
        ]} onValueChange={(value) => (period = value)} />
      {/if}
      <button class="btn sm personalize" bind:this={personalizerTrigger} type="button" aria-label={t('overview.indicators.personalize')} title={t('overview.indicators.personalize')} onclick={() => (personalizing = true)}><Icon name="settings" size={18} /></button>
    </div>
  </div>
  {#if selected}
    <div class="analysis-value">
      <b><Odometer value={formatIndicator(selected.indicator.last_value, selected.indicator.unit)} /></b>
      <span class="context-legend"><i></i>{t('dashboard.context')}</span>
    </div>
    {#if displayed.length > 1}
      <label class="indicator-selector" for={selectorID}>
        <span class="visually-hidden">{t('dashboard.indicatorChoice')}</span>
        <select id={selectorID} value={selected.indicator.id} onchange={(event) => (selectedID = event.currentTarget.value)}>
          {#each displayed as row (row.indicator.id)}<option value={row.indicator.id}>{suggestedLabel(row.indicator)} · {row.name}</option>{/each}
        </select>
      </label>
    {/if}
    <div class="analysis-chart" aria-busy={loading}>
      {#if failed || loading || points.length === 0}
        <p class="series-message" role="status">{t(failed ? 'dashboard.seriesError' : loading ? 'dashboard.loading' : 'dashboard.noSeries')}</p>
      {:else}
        <IndicatorAreaChart expanded interactive tone="context" {points} unit={selected.indicator.unit} {label} gapThresholdMilliseconds={period === '7d' ? 90 * 60_000 : null} />
      {/if}
    </div>
    <footer>
      <span class:warn={Boolean(selected.indicator.last_error)}>{selected.indicator.last_error || (selected.indicator.last_observed_at ? t('dashboard.freshness', { duration: since(selected.indicator.last_observed_at, now) }) : t('overview.indicators.neverObserved'))}</span>
      <a href="/cibles/{selected.target.target_id}">{t('dashboard.details')} ↗</a>
    </footer>
  {:else}
    <div class="empty-analysis">
      <Icon name="activity" size={32} />
      <strong>{t('dashboard.noIndicators')}</strong>
      <p>{t('dashboard.noIndicatorsHint')}</p>
      <button class="btn" type="button" onclick={() => (personalizing = true)}>{t('overview.indicators.personalize')}</button>
    </div>
  {/if}
</section>

{#if personalizing}<IndicatorPersonalizer ondismiss={dismissPersonalizer} />{/if}

<style>
  .indicator-overview { min-width: 0; display: flex; flex-direction: column; overflow: hidden; }
  .analysis-head { display: flex; align-items: flex-start; flex-wrap: wrap; gap: var(--s4); padding: var(--s5); }
  .analysis-copy { flex: 1; min-width: 0; }
  h2 { font-size: 1.125rem; font-weight: 600; overflow-wrap: anywhere; }
  .analysis-copy p { font-size: var(--text-sm); color: var(--faint); margin-top: var(--s2); }
  .analysis-controls { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); }
  .personalize { width: var(--ctl-h); padding: 0; justify-content: center; }
  .analysis-value { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--s4); padding: 0 var(--s5) var(--s4); }
  .analysis-value b { font-size: 1.75rem; font-weight: 500; font-variant-numeric: tabular-nums; }
  .context-legend { display: flex; align-items: center; gap: var(--s3); font-size: var(--text-xs); color: var(--faint); }
  .context-legend i { width: 1rem; height: 2px; background: var(--chart-context); }
  .indicator-selector { padding: 0 var(--s5) var(--s4); }
  select { width: 100%; min-width: 0; height: var(--ctl-h); padding: 0 var(--s3); border: 1px solid var(--line); border-radius: var(--r-m); font: inherit; font-size: var(--text-sm); background: var(--surface); color: var(--ink); }
  .analysis-chart { padding: var(--s3) var(--s4) var(--s5); margin-top: auto; }
  .series-message { min-height: 17.5rem; display: grid; place-items: center; color: var(--faint); font-size: var(--text-sm); text-align: center; }
  footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--s3) var(--s5); padding: var(--s4) var(--s5); border-top: 1px solid var(--line); color: var(--faint); font-size: var(--text-xs); }
  footer span { min-width: 0; overflow-wrap: anywhere; }
  footer a { flex: none; color: var(--muted); }
  footer a:hover { color: var(--ink); }
  .empty-analysis { flex: 1; min-height: 22rem; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: var(--s4); text-align: center; padding: var(--s6); }
  .empty-analysis p { max-width: 28rem; font-size: var(--text-sm); color: var(--muted); line-height: 1.6; }
  .empty-analysis strong { font-size: 1.125rem; }
  @media (max-width: 48rem) { .analysis-head { padding: var(--s4); } .analysis-value { padding-inline: var(--s4); } .analysis-chart { padding-inline: var(--s2); } footer { padding: var(--s4); } }
</style>
