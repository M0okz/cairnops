<script lang="ts">
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
  import Odometer from './Odometer.svelte';
  import SegmentedControl from './ui/SegmentedControl.svelte';
  import { session } from '$lib/session.svelte';
  import { formatIndicator, incidentMarkerIsVisible } from '$lib/indicator-format';
  import { clock, severityTone, since, stamp } from '$lib/format';
  import type { Incident } from '$lib/api';

  let { targetId, incident = null }: { targetId: string; incident?: Incident | null } = $props();
  let window = $state<'24h' | '7d'>('24h');
  let now = $state(new Date());
  const detail = $derived(session.indicatorDetails[`${targetId}:${window}`] ?? null);
  const markerVisible = $derived(Boolean(incident && detail && incidentMarkerIsVisible(incident.opened_at, detail.generated_at, window)));
  const marker = $derived(incident && markerVisible ? {
    at: incident.opened_at,
    label: `Ouverture · ${clock(incident.opened_at)}`,
    tone: severityTone(incident.severity) as 'info' | 'warn' | 'crit'
  } : null);
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
      <SegmentedControl
        label="Fenêtre des Indicateurs"
        value={window}
        items={[
          { value: '24h', label: '24 h' },
          { value: '7d', label: '7 j' }
        ]}
        onValueChange={(value) => (window = value)}
      />
    </header>
    <div class="indicator-list">
      {#each detail.indicators as indicator (indicator.id)}
        <article class="indicator-row">
          <div class="indicator-copy">
            <div class="indicator-title">
              <span><strong>{indicator.label}</strong>{#if indicator.dimension}<small>{indicator.dimension}</small>{/if}</span>
              <b class="num"><Odometer value={formatIndicator(indicator.last_value, indicator.unit)} /></b>
            </div>
            <IndicatorAreaChart interactive points={detail.series?.[indicator.id] ?? []} unit={indicator.unit} label={`${indicator.label} · ${window}`} {marker} />
            <div class="indicator-meta">
              <span class:warn={Boolean(indicator.last_error)}>{indicator.last_observed_at ? `Relevé ${since(indicator.last_observed_at, now)}` : 'Aucun relevé'}{indicator.last_error ? ` · ${indicator.last_error}` : ''}</span>
              {#if connectorAddress(indicator.connector_id)}<a href={connectorAddress(indicator.connector_id)!} target="_blank" rel="noreferrer">Ouvrir la source ↗</a>{/if}
            </div>
          </div>
          <button class="pin" class:active={indicator.pinned} type="button" aria-pressed={indicator.pinned} aria-label={indicator.pinned ? `Désépingler ${indicator.label}` : `Épingler ${indicator.label}`} onclick={() => session.toggleIndicatorPin(indicator)} title="Au plus quatre épingles personnelles">{indicator.pinned ? '◆' : '◇'}</button>
        </article>
      {/each}
    </div>
    <footer>
      <span>Contexte uniquement · les seuils et alertes restent sous l’autorité du produit d’origine. Les relevés détaillés expirent après 24 h et les agrégats horaires après 7 j.</span>
      {#if incident}
        <span class="incident-marker-note">
          <i class={severityTone(incident.severity)} aria-hidden="true"></i>
          {markerVisible
            ? `Ouverture de l’Incident · ${stamp(incident.opened_at)} · repère temporel, sans lien de cause établi.`
            : `Ouverture de l’Incident · ${stamp(incident.opened_at)} · repère hors de la fenêtre affichée.`}
        </span>
      {/if}
    </footer>
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
  .target-indicators > footer { display: flex; align-items: center; gap: var(--s4); padding: var(--s3) var(--s5); color: var(--faint); font-size: .625rem; }
  .target-indicators > footer > span:first-child { min-width: 0; flex: 1; }
  .incident-marker-note { display: inline-flex; align-items: center; gap: var(--s2); white-space: nowrap; }
  .incident-marker-note i { width: var(--s3); height: var(--s3); border: 1px solid currentColor; border-radius: var(--r-pill); background: var(--surface); }
  .incident-marker-note i.info { color: var(--info); }
  .incident-marker-note i.warn { color: var(--warn); }
  .incident-marker-note i.crit { color: var(--crit); }
  @media (max-width: 48rem) { .indicator-list { grid-template-columns: minmax(0, 1fr); } .indicator-row:nth-child(odd) { border-right: 0; } }
  @media (max-width: 68rem) { .target-indicators > footer { align-items: flex-start; flex-direction: column; gap: var(--s2); } .incident-marker-note { white-space: normal; } }
</style>
