<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import Odometer from './Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { since } from '$lib/format';

  let now = $state(new Date());
  $effect(() => { const timer = setInterval(() => (now = new Date()), 30_000); return () => clearInterval(timer); });
  const pinned = $derived(Object.values(session.indicatorOverview).flatMap((target) => target.indicators.map((indicator) => ({ indicator, target, name: session.targets.find((item) => item.id === target.target_id)?.name ?? 'Cible' }))).sort((left, right) => (left.indicator.pin_position ?? 99) - (right.indicator.pin_position ?? 99)).slice(0, 4));
</script>

{#if pinned.length > 0}
  <section class="indicator-overview" aria-labelledby="overview-indicators-title">
    <div class="band">
      <div>
        <h2 id="overview-indicators-title">Indicateurs épinglés</h2>
        <p>Métriques contextuelles importées depuis les Connecteurs · jamais utilisées comme seuils CairnOps</p>
      </div>
      <span class="tally">{pinned.length}/4</span>
    </div>
    <div class="indicator-grid">
      {#each pinned as row (row.indicator.id)}
        <a class="card indicator-card" href="/cibles/{row.target.target_id}">
          <div class="indicator-head">
            <span><strong>{row.indicator.label}</strong><small>{row.name}</small></span>
            <b class="num"><Odometer value={formatIndicator(row.indicator.last_value, row.indicator.unit)} /></b>
          </div>
          <IndicatorAreaChart compact points={row.target.series?.[row.indicator.id] ?? []} unit={row.indicator.unit} label={`${row.indicator.label} sur 24 heures`} />
          <small class:stale={Boolean(row.indicator.last_error)}>{row.indicator.last_observed_at ? `Relevé ${since(row.indicator.last_observed_at, now)}` : 'Aucun relevé'}{row.indicator.last_error ? ` · ${row.indicator.last_error}` : ''}</small>
        </a>
      {/each}
    </div>
  </section>
{/if}

<style>
  .indicator-overview { margin-bottom: var(--s5); }
  .band > div { min-width: 0; }
  .band p { margin-top: var(--s1); color: var(--faint); font-size: .6875rem; }
  .indicator-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--s4); }
  .indicator-card { padding: var(--s4); transition: border-color var(--d1) var(--ease), background var(--d1) var(--ease); }
  .indicator-card:hover { border-color: var(--accent); background: var(--surface-2); }
  .indicator-head { display: flex; align-items: flex-start; gap: var(--s3); }
  .indicator-head > span { min-width: 0; flex: 1; }
  .indicator-head strong, .indicator-head small { display: block; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .indicator-head strong { font-size: .75rem; }
  .indicator-head small, .indicator-card > small { color: var(--faint); font-size: .625rem; }
  .indicator-head b { color: var(--ink); font-size: .8125rem; white-space: nowrap; }
  .indicator-card > small.stale { color: var(--warn); }
  @media (max-width: 68rem) { .indicator-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 34rem) { .indicator-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
