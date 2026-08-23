<script lang="ts">
  /* Écran 4a — Cibles.
   * Colonnes, largeurs et ordre repris des Écrans. Le tri par gravité est le
   * comportement par défaut annoncé en bas de la barre de filtres. */

  import { goto } from '$app/navigation';
  import Topbar from '$lib/components/Topbar.svelte';
  import Spark from '$lib/components/Spark.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import TargetWorkshop from '$lib/components/TargetWorkshop.svelte';
  import ConnectorChooser from '$lib/components/ConnectorChooser.svelte';
  import { session } from '$lib/session.svelte';
  import {
    inWindow,
    latency,
    leadIncident,
    ratio,
    severityLabel,
    severityTone,
    severityWeight,
    since,
    stateLabel,
    stateTones,
    type TargetState
  } from '$lib/format';
  import { i18n, plural, t } from '$lib/i18n.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import type { Measure, Outcome, SourceMeasures, Target } from '$lib/api';
  import { reconciliationState } from '$lib/reconciliation.svelte';

  /* Les Contrôles natifs portent le nom de leur protocole : il ne se traduit
   * pas, et « Heartbeat » est le mot des Écrans dans les deux langues. */
  const kindLabels: Record<string, string> = {
    http: 'HTTP',
    tcp: 'TCP',
    dns: 'DNS',
    icmp: 'ICMP',
    heartbeat: 'Heartbeat',
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    generic_webhook: 'Webhook'
  };

  const outcomeTones: Record<Outcome, string> = {
    healthy: 'ok',
    unhealthy: 'crit',
    unknown: 'idle'
  };

  function sourceIdentity(source: Pick<SourceMeasures, 'kind' | 'origin'>): string {
    return source.origin === 'native' ? 'cairnops' : source.kind;
  }

  function sourceLabel(source: Pick<SourceMeasures, 'kind' | 'origin'>): string {
    const kind = kindLabels[source.kind] ?? source.kind;
    return source.origin === 'native' ? `CairnOps · ${kind}` : kind;
  }

  function outcomeLabel(outcome: Outcome | undefined): string {
    if (outcome === 'healthy') return t('state.ok');
    if (outcome === 'unhealthy') return t('target.failing');
    return t('state.unknown');
  }

  /* Réduire la liste affichée, pas chercher dans l'instance : ce champ vit
   * avec les autres filtres, sous le titre. La recherche globale, elle, est
   * dans la Palette (⌘K). */
  let filter = $state('');
  let scope = $state<'all' | 'problems' | 'maintenance'>('all');
  let divergentOnly = $state(false);
  let workshopOpen = $state(false);
  let chooserOpen = $state(false);

  type Row = {
    target: Target;
    state: TargetState;
    lead: ReturnType<typeof leadIncident>;
    measure: Measure;
    trend: number[];
    divergent: boolean;
    sourceCount: number;
    sources: SourceMeasures[];
    contextual: { label: string; value: string } | null;
  };

  const rows = $derived.by<Row[]>(() => {
    const query = filter.trim().toLocaleLowerCase(i18n.locale);

    return session.targets
      .map((target) => {
        const measured = session.measuresFor(target.id);
        /* Avant la première réponse de métriques, les Contrôles natifs restent
         * identifiables. Les Sources d'Intégration arrivent avec cette réponse,
         * qui porte aussi leurs mesures et évite tout appel par ligne. */
        const sources = measured?.sources ?? target.sources.map((source) => ({
          source_id: source.id,
          name: source.name,
          kind: source.kind,
          origin: 'native' as const,
          measures_availability: true,
          latest_outcome: source.latest_outcome,
          latest_observed_at: source.last_observed_at,
          measures: []
        }));
        return {
          target,
          state: session.targetState(target),
          lead: leadIncident(session.incidentsFor(target.id)),
          measure: inWindow(measured, '24h'),
          trend: measured?.latency_trend ?? [],
          divergent: session.hasDivergence(target),
          sourceCount: target.sources.length + target.external_source_count,
          sources,
          contextual: (() => {
            const indicator = session.indicatorOverview[target.id]?.indicators[0];
            return indicator ? { label: indicator.label, value: formatIndicator(indicator.last_value, indicator.unit) } : null;
          })()
        };
      })
      .filter((row) => {
        if (scope === 'problems' && (row.state === 'ok' || row.state === 'maintenance')) return false;
        if (scope === 'maintenance' && row.state !== 'maintenance') return false;
        if (divergentOnly && !row.divergent) return false;
        if (!query) return true;
        return [row.target.name, row.target.description, ...row.target.aliases, ...row.target.sources.map((source) => source.name)]
          .some((value) => value.toLocaleLowerCase(i18n.locale).includes(query));
      })
      .sort((a, b) => {
        const weight = (row: Row) => (row.lead ? severityWeight(row.lead.effective_severity) : 0);
        return weight(b) - weight(a) || a.target.name.localeCompare(b.target.name, i18n.locale);
      });
  });

  const problemCount = $derived(
    session.targets.filter((target) => {
      const state = session.targetState(target);
      return state !== 'ok' && state !== 'maintenance';
    }).length
  );

  const maintenanceCount = $derived(
    session.targets.filter((target) => session.targetState(target) === 'maintenance').length
  );

  const decisionCount = $derived(session.unacknowledged.length);
</script>

<svelte:head><title>{t('nav.targets')} — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.targets') }]} />

<div class="page">
  <div class="page-head">
    <div>
      <h1>{t('nav.targets')}</h1>
      <p>
        {plural('targets.supervised', session.targets.length)}
        {#if decisionCount > 0}· {plural('targets.awaitingDecision', decisionCount)}{/if}
      </p>
    </div>
    <div class="page-actions">
      {#if session.user?.role === 'administrator'}
        <a class="btn" href="/cibles/rapprochements">
          {t('reconciliation.title')}
          {#if reconciliationState.actionable.length > 0}<b class="action-count">{reconciliationState.actionable.length}</b>{/if}
        </a>
      {/if}
      <button class="btn" type="button" onclick={() => (chooserOpen = true)}>
        {t('targets.importFromConnector')}
      </button>
      <button class="btn primary" type="button" onclick={() => (workshopOpen = true)}>
        {t('targets.new')}
      </button>
    </div>
  </div>

  <div class="filters">
    <div class="segments" role="group" aria-label={t('targets.scope')}>
      <button type="button" aria-pressed={scope === 'all'} onclick={() => (scope = 'all')}>
        {t('targets.scope.all')} <b><Odometer value={session.targets.length} /></b>
      </button>
      <button type="button" aria-pressed={scope === 'problems'} onclick={() => (scope = 'problems')}>
        {t('targets.scope.problems')} <b><Odometer value={problemCount} /></b>
      </button>
      <button type="button" aria-pressed={scope === 'maintenance'} onclick={() => (scope = 'maintenance')}>
        {t('nav.maintenance')} <b><Odometer value={maintenanceCount} /></b>
      </button>
    </div>
    <button class="btn sm" type="button" aria-pressed={divergentOnly} onclick={() => (divergentOnly = !divergentOnly)}>
      {t('targets.divergence')} {divergentOnly ? '·' : '⌄'}
    </button>
    <label class="filter">
      <span class="visually-hidden">{t('targets.filterLabel')}</span>
      <input bind:value={filter} type="search" placeholder={t('targets.filterPlaceholder')} />
    </label>
    <span class="note">{t('targets.sortedBySeverity')}</span>
  </div>

  <div class="card cols">
    <div class="thead">
      <span>{t('targets.column.target')}</span>
      <span>{t('targets.column.state')}</span>
      <span class="hide-sm">{t('targets.column.natureSeverity')}</span>
      <span class="hide-sm">{t('targets.column.averageLatency')}</span>
      <span class="hide-sm">{t('targets.column.availabilityCoverage')}</span>
      <span class="hide-sm">{t('targets.column.sources')}</span>
      <span class="hide-sm">{t('targets.column.trend')}</span>
      <span></span>
    </div>

    {#each rows as row, index (row.target.id)}
      <a class="trow" href="/cibles/{row.target.id}">
        <span class="cell-name">
          <i class="dot {stateTones[row.state]}"></i>
          <span>
            <strong>{row.target.name}</strong>
            {#if row.target.description}<small>{row.target.description}</small>{/if}
            {#if row.contextual}<small class="contextual">{row.contextual.label} · <b class="num">{row.contextual.value}</b></small>{/if}
          </span>
        </span>

        <span class="pill {stateTones[row.state]}">{stateLabel(row.state)}</span>

        <span class="muted hide-sm nature">
          {#if row.lead}
            {row.lead.nature_label} · <span class={severityTone(row.lead.effective_severity)}>{severityLabel(row.lead.effective_severity)}</span>
          {:else}
            {t('common.none')}
          {/if}
        </span>

        <span
          class="num hide-sm"
          class:dim={row.measure.average_latency_milliseconds === null}
          title={t('targets.latencyTitle')}
        >
          <Odometer value={latency(row.measure.average_latency_milliseconds)} />
        </span>

        <!-- Une Disponibilité ne vaut que ce que vaut sa Couverture : les deux
             se lisent ensemble, jamais l'une sans l'autre. -->
        <span class="num hide-sm stacked" class:dim={row.measure.availability === null}>
          <Odometer value={ratio(row.measure.availability)} />
          <small class="faint" title={t('targets.coverageTitle')}>
            <Odometer value={ratio(row.measure.coverage)} />
          </small>
        </span>

        <!-- Une pastille par Source, à la couleur stable de sa provenance :
             rouge Zabbix, vert Uptime Kuma, violet Webhook, cuivre CairnOps.
             L'État garde ainsi ses propres couleurs sémantiques. -->
        <span class="hide-sm sources">
          {#if row.sourceCount === 0}
            <span class="dim">{t('common.none')}</span>
          {:else}
            {#each row.sources.slice(0, 5) as source (source.source_id)}
              <i
                class="dot source-dot {sourceIdentity(source)}"
                title={sourceLabel(source)}
                aria-hidden="true"
              ></i>
            {/each}
            {#each { length: Math.max(0, Math.min(row.sourceCount, 5) - row.sources.length) } as _, dot (dot)}
              <i class="dot source-dot pending" aria-hidden="true"></i>
            {/each}
            {#if row.sourceCount > 5}<small class="faint">+{row.sourceCount - 5}</small>{/if}
          {/if}
          {#if row.divergent}<span class="crit" title={t('overview.divergence')}>≠</span>{/if}

          <!-- Le détail au survol nomme la provenance et donne les preuves
               opérationnelles de chaque Source sur la même fenêtre de 24 h. -->
          <span class="tip" class:up={rows.length > 3 && index >= rows.length - 2} role="tooltip">
            <span class="tip-head">
              <b><Odometer value={plural('palette.sources', row.sourceCount)} /></b>
              <small class="faint">{t('targets.sourceWindow')}</small>
            </span>
            {#each row.sources.slice(0, 4) as source (source.source_id)}
              {@const measured = inWindow(source, '24h')}
              <span class="tip-source">
                <span class="tip-source-head">
                  <i class="dot source-dot {sourceIdentity(source)}" aria-hidden="true"></i>
                  <span class="tip-name">{source.name}</span>
                  <small class="faint">{sourceLabel(source)}</small>
                  <small class="source-outcome {outcomeTones[source.latest_outcome ?? 'unknown']}">
                    {outcomeLabel(source.latest_outcome)}
                  </small>
                </span>
                <span class="tip-metrics">
                  <span>
                    <small>{t('targets.sourceLastObservation')}</small>
                    <b><Odometer value={source.latest_observed_at ? since(source.latest_observed_at) : t('common.none')} /></b>
                  </span>
                  <span>
                    <small>{t('targets.sourceAvailability')}</small>
                    <b class:dim={measured.availability === null}><Odometer value={ratio(measured.availability)} /></b>
                  </span>
                  <span>
                    <small>{t('targets.sourceCoverage')}</small>
                    <b class:dim={measured.coverage === null}><Odometer value={ratio(measured.coverage)} /></b>
                  </span>
                  <span>
                    <small>{t('targets.sourceLatency')}</small>
                    <b class:dim={measured.average_latency_milliseconds === null}>
                      <Odometer value={latency(measured.average_latency_milliseconds)} />
                    </b>
                  </span>
                </span>
              </span>
            {/each}
            {#if row.sources.length > 4}
              <small class="faint">{plural('targets.moreChecks', row.sources.length - 4)}</small>
            {:else if row.sources.length < row.sourceCount}
              <small class="faint">{t('targets.loadingMeasures')}</small>
            {/if}
            {#if row.divergent}
              <small class="crit">{t('targets.divergenceHint')}</small>
            {/if}
            {#if row.sourceCount === 0}
              <small class="faint">{t('targets.noChecks')}</small>
            {/if}
          </span>
        </span>

        <span class="hide-sm trend {stateTones[row.state]}">
          <Spark values={row.trend} />
        </span>

        <span class="caret" aria-hidden="true">›</span>
      </a>
    {:else}
      <div class="empty">
        {#if session.targets.length === 0}
          <strong>{t('targets.emptyTitle')}</strong>
          {t('targets.emptyHint')}
        {:else}
          <strong>{t('targets.noMatchTitle')}</strong>
          {t('targets.noMatchHint')}
        {/if}
      </div>
    {/each}
  </div>

  {#if rows.length > 0 && Object.keys(session.measures).length === 0}
    <p class="loading">{t('targets.loadingMeasures')}</p>
  {/if}
</div>

{#if workshopOpen}
  <TargetWorkshop
    onclose={() => (workshopOpen = false)}
    onsuccess={async (_target, created) => {
      await session.loadTargets();
      session.showNotice(
        created.heartbeat_path
          ? t('targets.createdHeartbeat')
          : t('targets.created')
      );
    }}
  />
{/if}

{#if chooserOpen}
  <ConnectorChooser
    onclose={() => (chooserOpen = false)}
    onselect={(kind) => {
      chooserOpen = false;
      void goto(`/connecteurs/${kind.replace('_', '-')}`);
    }}
  />
{/if}

<style>
  .cols {
    --cols: minmax(0, 1.5fr) 7.25rem 8.5rem 5.25rem 7.25rem 5.5rem 5.125rem 1.25rem;
    /* L'infobulle des Sources doit pouvoir sortir de la dalle. Les deux bords
       qui vivaient du rognage — le fond de l'en-tête et celui de la dernière
       ligne survolée — reprennent l'arrondi à leur compte. */
    overflow: visible;
  }

  .thead {
    border-radius: var(--r-l) var(--r-l) 0 0;
  }

  .trow:last-child {
    border-radius: 0 0 var(--r-l) var(--r-l);
  }

  /* La Couverture se range sous la Disponibilité qu'elle qualifie, sans
     ajouter de colonne à la grille commune aux écrans. */
  .stacked small {
    display: block;
    margin-top: 2px;
    font-size: 0.6875rem;
  }

  .nature {
    font-size: 0.75rem;
  }

  .cell-name .contextual { color: var(--accent); font-size: .625rem; }
  .cell-name .contextual b { font-weight: 600; }

  .sources {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .sources {
    position: relative;
  }

  .sources > small {
    font-size: 0.6875rem;
  }

  .source-dot.cairnops { background: var(--source-cairnops) }
  .source-dot.zabbix { background: var(--source-zabbix) }
  .source-dot.uptime_kuma { background: var(--source-uptime-kuma) }
  .source-dot.patchmon { background: var(--source-patchmon) }
  .source-dot.generic_webhook { background: var(--source-webhook) }
  .source-dot.pending { background: var(--dim) }

  .tip {
    position: absolute;
    right: 0;
    top: calc(100% + 0.5rem);
    z-index: 5;
    display: grid;
    gap: 0.5rem;
    width: 27rem;
    padding: 0.625rem;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--surface);
    box-shadow: var(--shadow);
    font-size: 0.6875rem;
    line-height: 1.35;
    opacity: 0;
    visibility: hidden;
    pointer-events: none;
    transition: opacity var(--d1) var(--ease);
  }

  .tip.up {
    top: auto;
    bottom: calc(100% + 0.5rem);
  }

  .sources:hover .tip,
  .trow:focus-visible .tip {
    opacity: 1;
    visibility: visible;
  }

  .tip b {
    font-weight: 600;
  }

  .tip-head,
  .tip-source-head {
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }

  .tip-head {
    justify-content: space-between;
  }

  .tip-source {
    display: grid;
    gap: 0.375rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--line);
  }

  .tip-source-head {
    min-width: 0;
  }

  .tip-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--ink);
  }

  .source-outcome {
    margin-left: auto;
    white-space: nowrap;
  }

  .tip-metrics {
    display: grid;
    grid-template-columns: 1.25fr repeat(3, 1fr);
    gap: 0.5rem;
    padding-left: 0.75rem;
  }

  .tip-metrics > span {
    display: grid;
    gap: 1px;
    min-width: 0;
  }

  .tip-metrics small {
    overflow: hidden;
    color: var(--faint);
    font-size: 0.625rem;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tip-metrics b {
    font-family: var(--font-num);
    font-size: 0.6875rem;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .trend {
    color: var(--ok);
  }

  .trend.crit { color: var(--crit) }
  .trend.warn { color: var(--warn) }
  .trend.info { color: var(--info) }
  .trend.idle { color: var(--dim) }

  .loading {
    margin-top: var(--s4);
    color: var(--faint);
    font-size: 0.75rem;
  }

  .action-count {
    min-width: 1.125rem;
    height: 1.125rem;
    display: inline-grid;
    place-items: center;
    margin-left: var(--s1);
    border-radius: var(--r-pill);
    background: var(--accent);
    color: var(--accent-ink);
    font: 0.625rem var(--font-num);
  }
</style>
