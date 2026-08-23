<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import { session } from '$lib/session.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  let { incidentId }: { incidentId: string } = $props();
  const detail = $derived(session.incidentIndicatorDetails[incidentId] ?? null);
  $effect(() => { void session.loadIncidentIndicators(incidentId); });
</script>

{#if detail && detail.indicators.length > 0}
  <section class="incident-context card" aria-labelledby="incident-context-title">
    <header><div><h2 id="incident-context-title">Contexte à l’ouverture</h2><p>{detail.disclaimer}</p></div><span class="pill">± 2 h</span></header>
    <div class="context-grid">
      {#each detail.indicators as indicator (indicator.id)}
        {@const snapshot = detail.snapshots.find((item) => item.indicator_id === indicator.id || item.semantic_key === indicator.semantic_key)}
        <article>
          <div><strong>{indicator.label}</strong><b class="num">{formatIndicator(snapshot?.value ?? indicator.last_value, indicator.unit)}</b></div>
          <IndicatorAreaChart compact interactive points={detail.series[indicator.id] ?? []} unit={indicator.unit} label={`${indicator.label} autour de l’ouverture`} />
        </article>
      {/each}
    </div>
  </section>
{/if}

<style>
  .incident-context { margin-bottom: var(--s5); border-color: var(--line-strong); }
  header { display: flex; align-items: center; gap: var(--s4); padding: var(--s4) var(--s5); border-bottom: 1px solid var(--line); }
  header > div { flex: 1; }
  h2 { font-size: .8125rem; }
  header p { margin-top: var(--s1); color: var(--faint); font-size: .6875rem; }
  .context-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr)); }
  article { padding: var(--s4); border-right: 1px solid var(--line-row); }
  article > div { display: flex; align-items: baseline; gap: var(--s3); }
  article strong { min-width: 0; flex: 1; font-size: .6875rem; }
  article b { font-size: .75rem; }
</style>
