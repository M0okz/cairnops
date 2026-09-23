<script lang="ts">
  import Icon from './Icon.svelte';
  import HealthTrend from './HealthTrend.svelte';
  import { session } from '$lib/session.svelte';
  import { inWindow, latency, ratio } from '$lib/format';
  import { t } from '$lib/i18n.svelte';

  let selectedID = $state('');
  const candidates = $derived(session.targets.filter((target) => session.measures[target.id]));
  const selected = $derived(candidates.find((target) => target.id === selectedID) ?? candidates[0]);
  const measured = $derived(selected ? session.measures[selected.id] : undefined);
  const summary = $derived(inWindow(measured, '24h'));
  const availability = $derived((measured?.trend ?? []).filter(Number.isFinite));
  const latencyTrend = $derived((measured?.latency_trend ?? []).filter(Number.isFinite));
</script>

<section class="infrastructure-health card" aria-labelledby="infrastructure-health-title">
  <header>
    <div>
      <h2 id="infrastructure-health-title">{t('dashboard.infrastructureHealth')}</h2>
      <p>{t('dashboard.infrastructureHealthLead')}{#if selected} · {selected.name}{/if}</p>
    </div>
    <div class="health-controls">
      {#if candidates.length > 1}
        <label>
          <span class="visually-hidden">{t('dashboard.healthTarget')}</span>
          <select value={selected?.id ?? ''} onchange={(event) => selectedID = event.currentTarget.value}>
            {#each candidates as target (target.id)}<option value={target.id}>{target.name}</option>{/each}
          </select>
        </label>
      {/if}
      <span class="period">{t('dashboard.24h')}</span>
    </div>
  </header>
  <div class="health-panels">
    <article class="health-panel">
      <div class="panel-heading"><span><i class="indicator ok"></i>{t('dashboard.availability')}</span><Icon name="health" size={16} /></div>
      <strong class="num">{ratio(summary.availability)}</strong>
      <div class="health-visual availability-visual">
        <HealthTrend values={availability} kind="availability" />
        <p>{availability.length ? t('dashboard.hourlyAvailability') : t('dashboard.noSeries')}</p>
      </div>
    </article>
    <article class="health-panel">
      <div class="panel-heading"><span><i class="indicator neutral"></i>{t('dashboard.responseTime')}</span><Icon name="activity" size={16} /></div>
      <strong class="num">{latency(summary.average_latency_milliseconds)}</strong>
      <div class="health-visual response-visual">
        <HealthTrend values={latencyTrend} kind="latency" />
        <p>{latencyTrend.length ? t('dashboard.hourlyLatency') : t('dashboard.noSeries')}</p>
      </div>
    </article>
  </div>
  {#if selected}<footer><a href="/cibles/{selected.id}">{t('dashboard.details')} <span aria-hidden="true">↗</span></a></footer>{/if}
</section>

<style>
  .infrastructure-health { grid-column: 1 / -1; min-width: 0; padding: var(--s5); border-radius: var(--r-overview); }
  header { display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: var(--s4); margin-bottom: var(--s4); }
  h2 { font-size: 1.125rem; font-weight: 600; }
  header p { color: var(--faint); font-size: var(--text-sm); margin-top: var(--s2); }
  .health-controls { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s3); }
  select, .period { display: block; min-height: var(--ctl-h); padding: var(--s3) var(--s4); border: 1px solid var(--line); border-radius: var(--r-m); background: var(--surface); color: var(--ink); font: inherit; font-size: var(--text-xs); }
  select { max-width: 16rem; }
  .period { color: var(--muted); background: var(--surface-2); }
  .health-panels { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s4); }
  .health-panel { min-width: 0; display: flex; flex-direction: column; gap: var(--s3); padding: var(--s4); border: 1px solid var(--line); border-radius: var(--r-overview); }
  .panel-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); color: var(--muted); font-size: var(--text-sm); }
  .panel-heading span { display: inline-flex; align-items: center; gap: var(--s3); }
  .indicator { display: inline-block; width: var(--s2); height: var(--s4); border-radius: var(--r-s); }
  .indicator.ok { background: var(--ok); }
  .indicator.neutral { background: var(--chart-series); }
  strong { font-size: 1.5rem; font-weight: 600; letter-spacing: -0.04em; }
  .health-visual { display: flex; flex-direction: column; justify-content: end; gap: var(--s3); min-height: 7rem; color: var(--muted); }
  .health-visual p { font-size: var(--text-xs); color: var(--faint); }
  footer { display: flex; justify-content: flex-end; padding-top: var(--s4); font-size: var(--text-xs); color: var(--muted); }
  footer a:hover { color: var(--ink); }
  @media (max-width: 48rem) { .infrastructure-health { padding: var(--s4); } .health-panels { grid-template-columns: minmax(0, 1fr); } }
</style>
