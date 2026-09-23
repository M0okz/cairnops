<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import Icon from '$lib/components/Icon.svelte';
  import Topbar from '$lib/components/Topbar.svelte';
  import Spark from '$lib/components/Spark.svelte';
  import ResourceTooltip from '$lib/components/ResourceTooltip.svelte';
  import { Input } from '$lib/components/ui/input/index.js';
  import { Popover } from 'bits-ui';
  import TargetWorkshop from '$lib/components/TargetWorkshop.svelte';
  import ConnectorChooser from '$lib/components/ConnectorChooser.svelte';
  import { session } from '$lib/session.svelte';
  import { inWindow, latency, ratio, severityLabel, severityTone, since, stateLabel, stateTones } from '$lib/format';
  import { i18n, plural, t } from '$lib/i18n.svelte';
  import { formatIndicator } from '$lib/indicator-format';
  import { resourceCategories, resourceProblems, problemText, problemOrigin, isVersionNotice, fresh } from '$lib/resources';
  import { api, type ResourceCategory, type SourceMeasures } from '$lib/api';
  import type { SoftwareService } from '$lib/software-updates';

  let filter = $state('');
  let addOpen = $state(false);
  let category = $state<ResourceCategory | 'all'>('all');
  let scope = $state<'all' | 'problems' | 'maintenance' | 'unknown'>('all');
  $effect(() => {
    const requested = page.url.searchParams.get('scope');
    scope = requested === 'problems' || requested === 'maintenance' || requested === 'unknown' ? requested : 'all';
  });
  const scopeLabel = $derived(scope === 'problems' ? t('targets.scope.problems') : scope === 'unknown' ? t('state.unknown') : t('nav.maintenance'));
  let divergentOnly = $state(false);
  const activeFilterCount = $derived(Number(scope !== 'all') + Number(divergentOnly));
  let workshopOpen = $state(false);
  let chooserOpen = $state(false);
  let software = $state<SoftwareService[]>([]);
  const categoryOf = (target: {category?: ResourceCategory}) => target.category ?? 'unclassified';
  onMount(() => {
    let disposed = false;
    let loading = false;
    async function loadVersions() {
      if (loading) return;
      loading = true;
      try { const result = await api<{services: SoftwareService[]}>('/api/v1/software-updates'); if (!disposed) software = result.services ?? []; }
      catch { if (!disposed) software = []; }
      finally { loading = false; }
    }
    void loadVersions();
    const timer = setInterval(loadVersions, 30_000);
    return () => { disposed = true; clearInterval(timer); };
  });
  function sourceLabel(source: SourceMeasures) {
    const names: Record<string, string> = { uptime_kuma: 'Uptime Kuma', zabbix: 'Zabbix', argus: 'Argus', proxmox: 'Proxmox VE', patchmon: 'PatchMon', generic_webhook: 'Webhook' };
    return source.origin === 'native' ? `CairnOps · ${source.kind.toUpperCase()}` : names[source.kind] ?? source.kind;
  }
  const allRows = $derived(session.targets.map((target) => {
    const measured = session.measuresFor(target.id);
    const problems = resourceProblems(target.id, session.incidents);
    const versions = software.filter((service) => service.target_id === target.id);
    const update = session.incidentsFor(target.id).some((incident) => isVersionNotice(incident) && incident.impacts.some((impact) => impact.target_id === target.id && impact.status === 'active'));
    return { target, measured, problems, versions, update, state: session.targetState(target), divergent: session.hasDivergence(target), measure: inWindow(measured, '24h') };
  }));
  const rows = $derived(allRows.filter((row) => {
    if (category !== 'all' && categoryOf(row.target) !== category) return false;
    if (scope === 'problems' && row.problems.length === 0) return false;
    if (scope === 'maintenance' && row.state !== 'maintenance') return false;
    if (scope === 'unknown' && row.state !== 'unknown') return false;
    if (divergentOnly && !row.divergent) return false;
    const query = filter.trim().toLocaleLowerCase(i18n.locale);
    return !query || [row.target.name, row.target.description, ...row.target.aliases, ...row.problems.map((problem) => problemText(problem, i18n.locale))].some((value) => value.toLocaleLowerCase(i18n.locale).includes(query));
  }).sort((a,b) => {
    const weight = { critical: 4, major: 3, warning: 2, information: 1 };
    return (b.problems[0] ? weight[b.problems[0].impact.effective_severity] : 0) - (a.problems[0] ? weight[a.problems[0].impact.effective_severity] : 0) || a.target.name.localeCompare(b.target.name, i18n.locale);
  }));
  const scoped = $derived(allRows.filter((row) => category === 'all' || categoryOf(row.target) === category));
</script>

<svelte:head><title>{t('nav.targets')} — {session.instanceLabel}</title></svelte:head>
<Topbar crumbs={[{ label: t('nav.targets') }]} />
<div class="page">
  <div class="page-head">
    <div><h1>{t('nav.targets')}</h1><p>{plural('targets.supervised', session.targets.length)}{#if session.unacknowledged.length} · {plural('targets.awaitingDecision', session.unacknowledged.length)}{/if}</p></div>
    {#if session.user?.role === 'administrator'}
      <div class="page-actions shadcn-control">
        <Popover.Root bind:open={addOpen}>
          <Popover.Trigger class="resource-add-trigger"><Icon name="plus" size={18} />{t('targets.new')}</Popover.Trigger>
          <Popover.Content class="resource-filter-panel add-panel" align="end" sideOffset={8} collisionPadding={16}>
            <button class="menu-action" onclick={() => { addOpen = false; workshopOpen = true; }}>{t('resources.createManually')}</button>
            <button class="menu-action" onclick={() => { addOpen = false; chooserOpen = true; }}>{t('targets.importFromConnector')}</button>
          </Popover.Content>
        </Popover.Root>
      </div>
    {/if}
  </div>
  <div class="category-tabs" role="group" aria-label={t('resources.categories')}>
    {#each [{value: 'all' as const, label: t('targets.scope.all'), count: allRows.length}, ...resourceCategories.map((value) => ({value, label: t(`resources.category.${value}`), count: allRows.filter((row) => categoryOf(row.target) === value).length}))] as item (item.value)}
      <button class="category-tab" aria-pressed={category === item.value} onclick={() => category = item.value}>
        {item.label}<span class="num">{item.count}</span>
      </button>
    {/each}
  </div>
  <div class="resource-toolbar shadcn-control">
    <label class="target-filter"><span class="visually-hidden">{t('targets.filterLabel')}</span><Input bind:value={filter} type="search" placeholder={t('resources.search')} /></label>
    <Popover.Root>
      <Popover.Trigger class="resource-filter-trigger"><Icon name="settings" size={18} />{t('resources.filters')}{#if activeFilterCount}<span class="num">{activeFilterCount}</span>{/if}</Popover.Trigger>
      <Popover.Content class="resource-filter-panel" align="end" sideOffset={8} collisionPadding={16}>
        <h2>{t('resources.filters')}</h2>
        <fieldset>
          <legend>{t('targets.scope')}</legend>
          {#each [
            {value: 'all' as const, label: t('resources.anyState'), count: scoped.length},
            {value: 'problems' as const, label: t('targets.scope.problems'), count: scoped.filter(row => row.problems.length > 0).length},
            {value: 'maintenance' as const, label: t('nav.maintenance'), count: scoped.filter(row => row.state === 'maintenance').length},
            {value: 'unknown' as const, label: t('state.unknown'), count: scoped.filter(row => row.state === 'unknown').length}
          ] as item (item.value)}
            <label class="filter-option"><input type="radio" name="resource-scope" bind:group={scope} value={item.value} />{item.label}<span class="num">{item.count}</span></label>
          {/each}
        </fieldset>
        <label class="filter-option contradiction-option"><input type="checkbox" bind:checked={divergentOnly} />{t('targets.divergence')}</label>
        <Popover.Close class="filter-done">{t('resources.done')}</Popover.Close>
      </Popover.Content>
    </Popover.Root>
  </div>
  {#if activeFilterCount || filter.trim()}
    <div class="active-filters" aria-label={t('resources.activeFilters')}>
      {#if scope !== 'all'}<button class="filter-chip" aria-label={t('resources.removeFilter', {name: scopeLabel})} onclick={() => scope = 'all'}>{scopeLabel} <span aria-hidden="true">×</span></button>{/if}
      {#if divergentOnly}<button class="filter-chip" aria-label={t('resources.removeFilter', {name: t('targets.divergence')})} onclick={() => divergentOnly = false}>{t('targets.divergence')} <span aria-hidden="true">×</span></button>{/if}
      {#if filter.trim()}<button class="filter-chip" aria-label={t('resources.removeSearch')} onclick={() => filter = ''}>« {filter.trim()} » <span aria-hidden="true">×</span></button>{/if}
    </div>
  {/if}
  <div class="results-context">
    <span role="status">{plural('resources.results', rows.length)} · {category === 'all' ? t('resources.allCategories') : t(`resources.category.${category}`)}</span>
    <span>{t('targets.sortedBySeverity')}</span>
  </div>
  <div class="card cols">
    <div class="thead"><span>{t('targets.column.target')}</span><span>{t('targets.column.state')}</span><span>{t('targets.column.natureSeverity')}</span><span>{t('resources.details')}</span></div>
    {#each rows as row (row.target.id)}
      {@const resourceCategory = categoryOf(row.target)}
      <div class="trow">
        <div class="cell-name"><i class="dot {stateTones[row.state]}"></i><div>
          <a class="resource-link" href="/cibles/{row.target.id}">{row.target.name}</a>
          <small>{t(`resources.category.${resourceCategory}`)}</small>
          {#if row.target.description}<small class="description">{row.target.description}</small>{/if}
        </div></div>
        <div class="resource-state">
          {#if resourceCategory === 'software'}<span class="pill info">{t('resources.versionTracking')}</span>
          {:else if resourceCategory === 'scheduled_task'}<span class="pill {row.problems.length ? 'warn' : 'idle'}">{row.problems.length ? t('target.failing') : t('resources.taskTracking')}</span>
          {:else}<span class="pill {stateTones[row.state]}">{stateLabel(row.state)}</span>{/if}
          {#if row.divergent}<small class="warn">{t('targets.divergence')}</small>{/if}
        </div>
        <div class="problems">
          {#if row.problems[0]}
            <span class="problem-title">{problemText(row.problems[0], i18n.locale) || t('target.failing')}</span>
            <small class={severityTone(row.problems[0].impact.effective_severity)}>{severityLabel(row.problems[0].impact.effective_severity)} · {problemOrigin(row.problems[0].impact)}</small>
            {#if row.problems.length > 1}
              <ResourceTooltip label={plural('resources.moreProblems', row.problems.length - 1)}>
                <ul class="problem-list">{#each row.problems.slice(1) as problem (problem.impact.id)}
                  <li><strong>{problemText(problem, i18n.locale) || t('target.failing')}</strong><small>{severityLabel(problem.impact.effective_severity)} · {problemOrigin(problem.impact)}</small></li>
                {/each}</ul>
              </ResourceTooltip>
            {/if}
          {:else}<span class="muted">{t('resources.noProblem')}</span>{/if}
          {#if row.update}<a class="update-notice" href="/mises-a-jour">{t('nature.softwareUpdateAvailable')}</a>{/if}
        </div>
        <div class="resource-details">
          {#if resourceCategory === 'software'}
            {#each row.versions as version (version.id)}
              <span>{t('resources.installedVersion')} <b class="num">{version.known ? version.installed_version : t('common.none')}</b></span>
              <span>{t('resources.availableVersion')} <b class="num">{version.known ? version.target_version : t('common.none')}</b></span>
            {:else}<span class="muted">{t('resources.noVersion')}</span>{/each}
          {:else if resourceCategory === 'scheduled_task'}
            <span>{t('resources.lastSuccess')} <b>{row.target.last_success_at ? since(row.target.last_success_at) : t('common.none')}</b></span>
            <span>{t('targets.sourceLastObservation')} <b>{row.measured?.latest_observed_at ? since(row.measured.latest_observed_at) : t('common.none')}</b></span>
          {:else}
            {#if resourceCategory === 'service'}<span>{t('targets.column.averageLatency')} <b class="num">{latency(row.measure.average_latency_milliseconds)}</b></span>{/if}
            <span>{t('resources.availability')} <b class="num">{ratio(row.measure.availability)}</b></span>
            <span title={t('targets.coverageTitle')}>{t('resources.observedTime')} <b class="num">{ratio(row.measure.coverage)}</b></span>
            {#if resourceCategory === 'infrastructure'}{#each (session.indicatorOverview[row.target.id]?.indicators ?? []).slice(0, 2) as indicator (indicator.id)}
              <span>{indicator.label} <b class="num">{formatIndicator(indicator.last_value, indicator.unit)}</b></span>
            {/each}{/if}
          {/if}
          {#if row.measured?.sources.length}
            <ResourceTooltip label={plural('palette.sources', row.measured.sources.length)}>
              <ul class="problem-list">{#each row.measured.sources as source (source.source_id)}<li>
                <strong>{source.name}</strong><small>{sourceLabel(source)} · {source.enabled === false ? t('target.suspended') : !fresh(source.latest_observed_at, session.evaluatedAt, source.interval_seconds) ? t('component.status.stale') : source.latest_outcome === 'unhealthy' ? t('target.failing') : source.latest_outcome === 'healthy' ? source.measures_availability ? t('state.ok') : t('resources.noProblem') : t('state.unknown')}</small>
                <small>{t('targets.sourceLastObservation')} : {source.latest_observed_at ? since(source.latest_observed_at) : t('common.none')}</small>
                {#if source.measures_availability}
                  <small>{t('resources.availability')} : {ratio(inWindow(source, '24h').availability)} · {t('resources.observedTime')} : {ratio(inWindow(source, '24h').coverage)}</small>
                  <small>{t('targets.column.averageLatency')} : {latency(inWindow(source, '24h').average_latency_milliseconds)}</small>
                {/if}
              </li>{/each}</ul>
            </ResourceTooltip>
          {/if}
          {#if resourceCategory === 'service'}<span class="trend" aria-hidden="true"><Spark values={row.measured?.latency_trend ?? []} /></span>{/if}
        </div>
      </div>
    {:else}<div class="empty"><strong>{session.targets.length === 0 ? t('targets.emptyTitle') : t('targets.noMatchTitle')}</strong>{session.targets.length === 0 ? t('targets.emptyHint') : t('targets.noMatchHint')}</div>{/each}
  </div>
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
  .category-tabs { display: flex; overflow-x: auto; border-bottom: var(--line-width) solid var(--line); margin-bottom: var(--s4); gap: var(--s4); }
  .category-tab { flex: none; display: flex; align-items: center; gap: var(--s2); padding: var(--s3) var(--s1); min-height: var(--choice-hit-area); border: 0; border-bottom: var(--s1) solid transparent; background: transparent; color: var(--muted); font: inherit; font-size: var(--text-sm); cursor: pointer; }
  .category-tab:hover { color: var(--ink); background: var(--surface); }
  .category-tab[aria-pressed="true"] { color: var(--ink); font-weight: 600; border-bottom-color: var(--ink); }
  .category-tab .num { font-size: var(--text-xs); color: var(--faint); }
  .resource-toolbar { display: flex; align-items: center; gap: var(--s3); }
  .target-filter { width: 24rem; max-width: 100%; min-width: 0; }
  :global(.resource-filter-trigger), :global(.resource-add-trigger) { display: inline-flex; align-items: center; justify-content: center; gap: var(--s2); min-height: var(--choice-hit-area); padding: var(--s2) var(--s3); border: var(--line-width) solid var(--line-strong); border-radius: var(--r-m); color: var(--ink); background: var(--surface); font: inherit; font-size: var(--text-sm); cursor: pointer; white-space: nowrap; }
  :global(.resource-add-trigger) { background: var(--ink); color: var(--bg); }
  :global(.resource-filter-trigger:hover) { background: var(--surface-2); }
  :global(.resource-filter-panel) { z-index: 50; width: 20rem; max-width: calc(100vw - var(--s6)); padding: var(--s4); border: var(--line-width) solid var(--line-strong); border-radius: var(--r-l); background: var(--bg); color: var(--ink); font-size: var(--text-sm); box-shadow: 0 var(--s2) var(--s6) var(--line); }
  :global(.resource-filter-panel h2) { margin: 0 0 var(--s3); font-size: var(--text-sm); }
  fieldset { border: 0; margin: 0; padding: 0; }
  legend { color: var(--muted); font-size: var(--text-xs); margin-bottom: var(--s2); }
  .filter-option { display: flex; align-items: center; gap: var(--s2); min-height: var(--choice-hit-area); cursor: pointer; }
  .filter-option .num { margin-left: auto; color: var(--muted); }
  .filter-option input { accent-color: var(--ink); }
  .contradiction-option { border-top: var(--line-width) solid var(--line); margin-top: var(--s2); padding-top: var(--s2); }
  :global(.filter-done), .menu-action { width: 100%; min-height: var(--choice-hit-area); padding: var(--s2); color: var(--ink); background: var(--surface); border: 0; border-radius: var(--r-button); font: inherit; cursor: pointer; }
  .menu-action { text-align: left; background: transparent; }
  .menu-action:hover, :global(.filter-done:hover) { background: var(--surface-2); }
  .active-filters { display: flex; flex-wrap: wrap; gap: var(--s2); margin-top: var(--s3); }
  .filter-chip { display: inline-flex; align-items: center; gap: var(--s2); min-height: var(--choice-hit-area); max-width: 100%; overflow-wrap: anywhere; border: var(--line-width) solid var(--line); border-radius: var(--r-m); padding: var(--s1) var(--s3); background: var(--surface); color: var(--ink); font: inherit; font-size: var(--text-xs); cursor: pointer; }
  .filter-chip span { font-size: var(--text-base); }
  .results-context { display: flex; flex-wrap: wrap; justify-content: space-between; gap: var(--s2); color: var(--muted); font-size: var(--text-xs); margin: var(--s4) 0 var(--s3); }
  .category-tab:focus-visible, .filter-chip:focus-visible { outline: var(--s1) solid var(--ink); outline-offset: calc(-1 * var(--s1)); }

  .cols { --cols: minmax(0, 24rem) minmax(6rem, 0.65fr) minmax(0, 1.3fr) minmax(0, 1fr); overflow: visible; }
  .trow { align-items: start; padding-block: var(--s4); }
  .resource-link { color: var(--ink); font-weight: 600; text-decoration: none; overflow-wrap: anywhere; }
  .resource-link:hover { text-decoration: underline; }
  .cell-name { align-items: start; min-width: 0; }
  .cell-name .dot { margin-top: var(--s2); flex-shrink: 0; }
  .cell-name > div { min-width: 0; }
  .cell-name small { display: block; color: var(--muted); font-size: var(--text-xs); margin-top: var(--s1); white-space: normal; }
  .description { overflow-wrap: anywhere; }
  .resource-state, .problems, .resource-details { display: flex; flex-direction: column; align-items: flex-start; gap: var(--s2); min-width: 0; }
  .pill { white-space: normal; }
  .problems { font-size: var(--text-sm); overflow-wrap: anywhere; }
  .resource-details, .resource-state small { font-size: var(--text-xs); }
  .resource-details > span { width: 100%; display: flex; justify-content: space-between; gap: var(--s2); flex-wrap: wrap; color: var(--muted); }
  .resource-details b { color: var(--ink); font-weight: 500; }
  .update-notice { color: var(--info); font-size: var(--text-xs); }
  .trend { height: 1.5rem; max-width: 9rem; }
  .problem-list { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--s3); }
  .problem-list li { display: grid; gap: var(--s1); }
  .problem-list small { color: var(--muted); }
  @media (max-width: 80rem) { .cols { --cols: minmax(0, 1fr) minmax(6rem, .6fr) minmax(0, 1fr) minmax(0, 1fr); } }
  @media (max-width: 58rem) {
    .thead { display: none; }
    .trow { grid-template-columns: minmax(0, 1fr) minmax(0, .65fr); gap: var(--s4); }
    .problems { grid-column: 1 / -1; }
    .resource-details { grid-column: 1 / -1; width: 100%; }
    .target-filter { width: 100%; }
  }
</style>
