<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import Odometer from './Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { since } from '$lib/format';

  let { targetId }: { targetId: string } = $props();
  let window = $state<'24h' | '7d'>('24h');
  let now = $state(new Date());
  const detail = $derived(session.indicatorDetails[`${targetId}:${window}`] ?? null);
  $effect(() => { void session.loadTargetIndicators(targetId, window); });
  $effect(() => { const timer = setInterval(() => (now = new Date()), 30_000); return () => clearInterval(timer); });

  function connectorAddress(connectorId: string): string | null {
    const endpoint = session.connectors.find((connector) => connector.id === connectorId)?.endpoint;
    if (!endpoint) return null;
    try { const url = new URL(endpoint); url.pathname = '/'; url.search = ''; url.hash = ''; return url.toString(); } catch { return endpoint; }
  }
</script>

{#if detail && detail.indicators.length > 0}
  <section class="target-indicators card" aria-labelledby="target-indicators-title">
    <header>
      <div>
        <h2 id="target-indicators-title">Indicateurs</h2>
        <p>Métriques contextuelles importées depuis les Connecteurs</p>
      </div>
      <div class="segments" role="group" aria-label="Fenêtre des Indicateurs">
        <button type="button" aria-pressed={window === '24h'} onclick={() => (window = '24h')}>24 h</button>
        <button type="button" aria-pressed={window === '7d'} onclick={() => (window = '7d')}>7 j</button>
      </div>
    </header>
    <div class="indicator-list">
      {#each detail.indicators as indicator (indicator.id)}
        <article class="indicator-row">
          <div class="indicator-copy">
            <div class="indicator-title">
              <span><strong>{indicator.label}</strong>{#if indicator.dimension}<small>{indicator.dimension}</small>{/if}</span>
              <b class="num"><Odometer value={formatIndicator(indicator.last_value, indicator.unit)} /></b>
            </div>
            <IndicatorAreaChart interactive points={detail.series?.[indicator.id] ?? []} unit={indicator.unit} label={`${indicator.label} · ${window}`} />
            <div class="indicator-meta">
              <span class:warn={Boolean(indicator.last_error)}>{indicator.last_observed_at ? `Relevé ${since(indicator.last_observed_at, now)}` : 'Aucun relevé'}{indicator.last_error ? ` · ${indicator.last_error}` : ''}</span>
              {#if connectorAddress(indicator.connector_id)}<a href={connectorAddress(indicator.connector_id)!} target="_blank" rel="noreferrer">Ouvrir la source ↗</a>{/if}
            </div>
          </div>
          <button class="pin" class:active={indicator.pinned} type="button" aria-pressed={indicator.pinned} aria-label={indicator.pinned ? `Désépingler ${indicator.label}` : `Épingler ${indicator.label}`} onclick={() => session.toggleIndicatorPin(indicator)} title="Au plus quatre épingles personnelles">{indicator.pinned ? '◆' : '◇'}</button>
        </article>
      {/each}
    </div>
    <footer>Contexte uniquement · les seuils et alertes restent sous l’autorité du produit d’origine. Les relevés détaillés expirent après 24 h et les agrégats horaires après 7 j.</footer>
  </section>
{/if}

<style>
  .target-indicators { margin-bottom: var(--s5); }
  .target-indicators > header { display: flex; align-items: center; gap: var(--s4); padding: var(--s4) var(--s5); border-bottom: 1px solid var(--line); }
  header > div:first-child { flex: 1; }
  h2 { font-size: .8125rem; }
  header p { margin-top: var(--s1); color: var(--faint); font-size: .6875rem; }
  .indicator-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .indicator-row { display: flex; gap: var(--s3); min-width: 0; padding: var(--s5); border-bottom: 1px solid var(--line-row); }
  .indicator-row:nth-child(odd) { border-right: 1px solid var(--line-row); }
  .indicator-row:last-child:nth-child(odd) { grid-column: 1 / -1; border-right: 0; }
  .indicator-copy { min-width: 0; flex: 1; }
  .indicator-title { display: flex; align-items: baseline; gap: var(--s3); }
  .indicator-title > span { min-width: 0; flex: 1; }
  .indicator-title strong, .indicator-title small { display: block; }
  .indicator-title strong { font-size: .75rem; }
  .indicator-title small { color: var(--faint); font-size: .625rem; }
  .indicator-title b { font-size: .875rem; }
  .indicator-meta { display: flex; gap: var(--s3); color: var(--faint); font-size: .625rem; }
  .indicator-meta span { min-width: 0; flex: 1; }
  .indicator-meta a { color: var(--accent); white-space: nowrap; }
  .pin { width: 1.875rem; height: 1.875rem; border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--surface); color: var(--faint); }
  .pin.active { color: var(--accent); border-color: var(--accent); }
  .target-indicators > footer { padding: var(--s3) var(--s5); color: var(--faint); font-size: .625rem; }
  @media (max-width: 48rem) { .indicator-list { grid-template-columns: minmax(0, 1fr); } .indicator-row:nth-child(odd) { border-right: 0; } }
</style>
