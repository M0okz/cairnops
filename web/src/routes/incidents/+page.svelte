<script lang="ts">
  /* Écran 4c — Incidents. Un Incident peut porter plusieurs Atteintes. */

  import { afterNavigate, goto } from '$app/navigation';
  import { page } from '$app/state';
  import IncidentDetailDrawer from '$lib/components/IncidentDetailDrawer.svelte';
  import Topbar from '$lib/components/Topbar.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import SegmentedControl from '$lib/components/ui/SegmentedControl.svelte';
  import { session, messageFrom } from '$lib/session.svelte';
  import { api, type Incident, type IncidentSeverity, type ResolvedIncidentPage } from '$lib/api';
  import { incidentHref } from '$lib/incident-detail';
  import { resolvedHistoryChanged, type IncidentScope } from '$lib/resolved-incidents';
  import {
    compareActiveIncidents, historyBounds, incidentFilterOptions, localDateValue,
    matchesIncidentFilters, resolvedIncidentQuery, type HistoryPeriod
  } from '$lib/incident-list';
  import {
    activeImpactRatio,
    diverges,
    natureLabel,
    severityLabel,
    severityTone,
    since,
    stamp
  } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  let scope = $state<IncidentScope>('active');
  let resolved = $state<Incident[]>([]);
  let query = $state('');
  let targetID = $state('');
  let natureKey = $state('');
  let severity = $state<IncidentSeverity | ''>('');
  let period = $state<HistoryPeriod>('30');
  const initialDate = new Date();
  let fromDate = $state(localDateValue(new Date(initialDate.getFullYear(), initialDate.getMonth(), initialDate.getDate() - 29)));
  let throughDate = $state(localDateValue(initialDate));
  let historyFilters = $state<ResolvedIncidentPage['filters']>();
  let resolvedLoading = $state(false);
  let nextCursor = $state('');
  let resolvedRevision = $state(-1);
  let resolvedRequest = 0;
  let resolvedError = $state('');
  let failedResolvedRequest: { url: string; append: boolean } | null = null;
  let acknowledging = $state('');
  let now = $state(new Date());
  const incidentIDFrom = (url: URL) => url.searchParams.get('incident')?.trim() ?? '';
  let selectedIncidentID = $state(incidentIDFrom(page.url));

  afterNavigate(({ to }) => {
    selectedIncidentID = to ? incidentIDFrom(to.url) : '';
  });

  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });

  const filters = $derived({ query, targetID, natureKey, severity });
  const hasFilters = $derived(Boolean(query.trim() || targetID || natureKey || severity));
  const historyDay = $derived(localDateValue(now));
  const bounds = $derived(historyBounds(period, fromDate, throughDate, new Date(`${historyDay}T12:00:00`)));
  const historyURL = $derived(bounds ? resolvedIncidentQuery(filters, bounds) : '');
  const activeOptions = $derived(incidentFilterOptions(session.incidents));
  const options = $derived(scope === 'resolved' ? (historyFilters ?? activeOptions) : activeOptions);
  const historyChanged = $derived(resolvedHistoryChanged(resolvedRevision, session.incidentRevision));

  async function loadResolved(url: string, append = false) {
    const request = ++resolvedRequest;
    const revision = session.incidentRevision;
    resolvedLoading = true;
    resolvedError = '';
    failedResolvedRequest = null;
    try {
      const response = await api<ResolvedIncidentPage>(url);
      if (request !== resolvedRequest) return;
      resolved = append ? [...resolved, ...response.incidents] : response.incidents;
      if (!append) resolvedRevision = revision;
      nextCursor = response.next_cursor ?? '';
      if (response.filters) historyFilters = response.filters;
    } catch (cause) {
      if (request !== resolvedRequest) return;
      resolvedError = messageFrom(cause);
      failedResolvedRequest = { url, append };
    } finally {
      if (request === resolvedRequest) resolvedLoading = false;
    }
  }

  /* Only a filter/date change starts a new snapshot automatically. Realtime
   * changes offer an explicit refresh and leave the pages/cursor untouched. */
  $effect(() => {
    const currentScope = scope;
    const url = historyURL;
    resolvedRequest += 1;
    if (currentScope !== 'resolved') {
      resolvedLoading = false;
      return;
    }
    resolved = [];
    resolvedRevision = -1;
    nextCursor = '';
    resolvedError = '';
    resolvedLoading = Boolean(url);
    if (!url) return;
    const timer = setTimeout(() => void loadResolved(url), 200);
    return () => clearTimeout(timer);
  });

  function loadMore() {
    if (!bounds || !nextCursor || resolvedLoading) return;
    void loadResolved(resolvedIncidentQuery(filters, bounds, nextCursor), true);
  }

  function clearFilters() {
    query = '';
    targetID = '';
    natureKey = '';
    severity = '';
  }

  const shown = $derived(scope === 'resolved' ? resolved : session.actionable
    .filter((incident) => (scope !== 'unacknowledged' || !incident.acknowledged_at) && matchesIncidentFilters(incident, filters))
    .sort(compareActiveIncidents));
  const selectedIncident = $derived(
    [...session.incidents, ...resolved].find((incident) => incident.id === selectedIncidentID) ?? null
  );

  async function acknowledge(incident: Incident) {
    acknowledging = incident.id;
    try {
      await session.acknowledge(incident);
    } finally {
      acknowledging = '';
    }
  }

  function lastEntry(incident: Incident) {
    return incident.activity.at(-1)?.message ?? t('common.none');
  }

  async function dismissIncident() {
    const dismissedIncidentID = selectedIncidentID;
    const url = new URL(page.url);
    url.searchParams.delete('incident');
    await goto(`${url.pathname}${url.search}${url.hash}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
    const trigger = Array.from(
      document.querySelectorAll<HTMLElement>('[data-incident-trigger]')
    ).find((candidate) => candidate.dataset.incidentTrigger === dismissedIncidentID);
    (trigger ?? document.getElementById('main-content'))?.focus();
  }
</script>

<svelte:head><title>{t('nav.incidents')} — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.incidents') }]} />

<div class="page">
  <div class="page-head">
    <div>
      <h1>{t('nav.incidents')}</h1>
      <p>
        {plural('incidents.active', session.actionable.length)}
        {#if session.unacknowledged.length > 0}
          {plural('incidents.ofWhichUnacknowledged', session.unacknowledged.length)}
        {/if}
        · {t('incidents.onePerNature')}
      </p>
    </div>
  </div>

  <div class="filters">
    <SegmentedControl
      label={t('targets.scope')}
      value={scope}
      items={[
        { value: 'active', label: t('incidents.scope.active'), count: session.actionable.length },
        {
          value: 'unacknowledged',
          label: t('incidents.scope.unacknowledged'),
          count: session.unacknowledged.length
        },
        { value: 'resolved', label: t('incidents.scope.resolved') }
      ]}
      onValueChange={(value) => (scope = value)}
    />
    <span class="note">
      {scope === 'resolved' ? t('incidents.history.order') : t('incidents.priorityOrder')}
    </span>
  </div>

  <div class="incident-filter-grid">
    <div class="field search-field">
      <label for="incident-search">{t('incidents.filters.search')}</label>
      <input id="incident-search" name="incident-search" type="search" bind:value={query} maxlength="200" placeholder={t('incidents.filters.searchHint')} />
    </div>
    <div class="field">
      <label for="incident-target">{t('incidents.filters.target')}</label>
      <select id="incident-target" bind:value={targetID}>
        <option value="">{t('incidents.filters.allTargets')}</option>
        {#if targetID && !options.targets.some((option) => option.value === targetID)}
          <option value={targetID}>{session.targets.find((target) => target.id === targetID)?.name ?? t('incidents.filters.selectedTarget')}</option>
        {/if}
        {#each options.targets as option (option.value)}<option value={option.value}>{option.label}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label for="incident-nature">{t('incidents.filters.nature')}</label>
      <select id="incident-nature" bind:value={natureKey}>
        <option value="">{t('incidents.filters.allNatures')}</option>
        {#if natureKey && !options.natures.some((option) => option.value === natureKey)}
          <option value={natureKey}>{natureKey}</option>
        {/if}
        {#each options.natures as option (option.value)}<option value={option.value}>{option.label}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label for="incident-severity">{t('incidents.column.severity')}</label>
      <select id="incident-severity" bind:value={severity}>
        <option value="">{t('incidents.filters.allSeverities')}</option>
        {#each ['critical', 'major', 'warning', 'information'] as value}
          <option {value}>{severityLabel(value as IncidentSeverity)}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if scope === 'resolved'}
    <div class="history-filters">
      <div class="field">
        <label for="incident-period">{t('incidents.history.period')}</label>
        <select id="incident-period" bind:value={period}>
          <option value="7">{t('incidents.history.sevenDays')}</option>
          <option value="30">{t('incidents.lastThirtyDays')}</option>
          <option value="90">{t('incidents.history.ninetyDays')}</option>
          <option value="all">{t('incidents.history.all')}</option>
          <option value="custom">{t('incidents.history.custom')}</option>
        </select>
      </div>
      {#if period === 'custom'}
        <div class="field">
          <label for="incident-from">{t('incidents.history.from')}</label>
          <input id="incident-from" type="date" bind:value={fromDate} aria-invalid={!bounds} aria-describedby={!bounds ? 'incident-date-error' : undefined} />
        </div>
        <div class="field">
          <label for="incident-through">{t('incidents.history.through')}</label>
          <input id="incident-through" type="date" bind:value={throughDate} aria-invalid={!bounds} aria-describedby={!bounds ? 'incident-date-error' : undefined} />
        </div>
      {/if}
      <span class="history-note">{t('incidents.history.dateNote')}</span>
    </div>
    {#if !bounds}<p id="incident-date-error" class="filter-error" role="alert">{t('incidents.history.invalidDates')}</p>{/if}
  {/if}

  <div class="results-summary">
    <p role="status">{scope === 'resolved' && resolvedLoading ? t('incidents.history.loading') : plural('incidents.results', shown.length)}</p>
    {#if hasFilters}<button class="btn sm" type="button" onclick={clearFilters}>{t('incidents.filters.clear')}</button>{/if}
  </div>

  {#if scope === 'resolved' && historyChanged}
    <div class="history-refresh">
      <p role="status">{t('incidents.history.changed')}</p>
      <button class="btn sm" type="button" disabled={resolvedLoading} onclick={() => loadResolved(historyURL)}>{t('incidents.history.refresh')}</button>
    </div>
  {/if}

  <div class="card cols" aria-busy={scope === 'resolved' && resolvedLoading}>
    <div class="thead">
      <span>{t('incidents.column.targetNature')}</span>
      <span>{t('incidents.column.severity')}</span>
      <span class="hide-sm">
        {scope === 'resolved' ? t('incidents.column.resolved') : t('incidents.column.acknowledgement')}
      </span>
      <span class="hide-sm">{t('incidents.column.duration')}</span>
      <span class="hide-sm">{t('incidents.column.activeImpacts')}</span>
      <span class="hide-sm log">{t('incidents.column.lastEntry')}</span>
      <span></span>
    </div>

    {#each shown as incident (incident.id)}
      <div class="trow">
        <span class="cell-name">
          <i class="dot {scope === 'resolved' ? 'ok' : severityTone(incident.severity)}"></i>
          <span>
            <a
              class="incident-link"
              href={incidentHref(incident.id)}
              data-incident-trigger={incident.id}
            >
              <strong>{incident.affected_target_count > 1
                ? plural('incidents.targetsAffected', incident.affected_target_count)
                : (incident.impacts[0]?.target_name ?? t('nav.incidents'))}</strong>
              <small class="nature">
                {natureLabel(incident)}
                {#if incident.extended} · {t('incidents.extended')}{/if}
              </small>
            </a>
          </span>
        </span>

        <span class="pill {severityTone(incident.severity)}">
          {severityLabel(incident.severity)}
        </span>

        <span class="hide-sm ack-cell">
          {#if scope === 'resolved'}
            <span class="muted">
              {incident.resolved_at ? stamp(incident.resolved_at) : t('common.none')}
            </span>
          {:else if incident.acknowledged_at}
            <span class="ack">
              <i class="mark">✓</i>{incident.acknowledged_by ?? t('overview.acknowledgedShort')}
              {#if incident.acknowledgement_sync_status === 'failed'}
                <span class="warn" title={incident.acknowledgement_sync_error}>sync ✕</span>
              {/if}
            </span>
          {:else}
            <span class="crit">{t('overview.fig.unacknowledged')}</span>
          {/if}
        </span>

        <span class="num hide-sm">
          <Odometer value={incident.resolved_at
            ? since(incident.opened_at, new Date(incident.resolved_at))
            : since(incident.opened_at, now)} />
        </span>

        <span
          class="num hide-sm sources"
          title={t('incidents.impactsRatio', { ratio: activeImpactRatio(incident) })}
          aria-label={t('incidents.impactsRatio', { ratio: activeImpactRatio(incident) })}
        >
          <Odometer value={activeImpactRatio(incident)} />
          {#if diverges(incident)}<span class="crit" title={t('overview.divergence')}>≠</span>{/if}
        </span>

        <span class="faint log hide-sm">{lastEntry(incident)}</span>

        {#if scope !== 'resolved' && !incident.acknowledged_at && session.user?.role !== 'observer'}
          <button
            class="btn primary sm"
            type="button"
            disabled={acknowledging === incident.id}
            onclick={() => acknowledge(incident)}
          >
            {acknowledging === incident.id ? '…' : t('incident.acknowledge')}
          </button>
        {:else}
          <a
            class="btn sm"
            href={incidentHref(incident.id)}
            data-incident-trigger={incident.id}
          >{t('incidents.detail.open')}</a>
        {/if}
      </div>
    {:else}
      <div class="empty">
        {#if scope === 'resolved' && resolvedLoading}
          <strong>{t('incidents.history.loading')}</strong>
        {:else if scope === 'resolved' && !bounds}
          <strong>{t('incidents.history.chooseDates')}</strong>
        {:else if scope === 'resolved' && resolvedError}
          <strong>{t('incidents.logUnread')}</strong>
        {:else if hasFilters || scope === 'resolved'}
          <strong>{t('incidents.filters.empty')}</strong>
          {t('incidents.filters.emptyHint')}
        {:else if scope === 'unacknowledged'}
          <strong>{t('incidents.emptyUnacknowledged')}</strong>
          {t('incidents.emptyUnacknowledgedHint')}
        {:else}
          <strong>{t('incidents.emptyActive')}</strong>
          {t('incidents.emptyActiveHint')}
        {/if}
      </div>
    {/each}
  </div>

  {#if scope === 'resolved' && resolvedError}
    <div class="history-error" role="alert">
      <span>{resolvedError}</span>
      <button class="btn" type="button" onclick={() => failedResolvedRequest && loadResolved(failedResolvedRequest.url, failedResolvedRequest.append)}>{t('common.retry')}</button>
    </div>
  {/if}
  {#if scope === 'resolved' && nextCursor}
    <div class="history-pagination">
      <button class="btn" type="button" disabled={resolvedLoading} onclick={loadMore}>
        {resolvedLoading ? t('incidents.history.loading') : t('incidents.history.loadMore')}
      </button>
    </div>
  {/if}

  {#if session.incidents.length > session.actionable.length}
    <p class="under">
      {plural('incidents.neutralised', session.incidents.length - session.actionable.length)} —
      <a href="/maintenance">{t('incidents.seeWindows')}</a>.
    </p>
  {/if}
</div>

{#if selectedIncidentID}
  {#key selectedIncidentID}
    <IncidentDetailDrawer
      incidentId={selectedIncidentID}
      seed={selectedIncident}
      ondismiss={dismissIncident}
    />
  {/key}
{/if}

<style>
  .incident-filter-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) repeat(3, minmax(0, 1fr));
    gap: var(--s4);
    margin-bottom: var(--s4);
  }

  .incident-filter-grid .field, .history-filters .field { min-width: 0; margin: 0; }
  .history-filters { display: flex; align-items: end; flex-wrap: wrap; gap: var(--s4); margin-bottom: var(--s4); }
  .history-filters .field { flex: 1 1 11rem; }
  .history-note { color: var(--muted); font-size: var(--text-sm); flex: 1 1 16rem; padding-bottom: var(--s2); }
  .results-summary, .history-error, .history-refresh { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: var(--s3); margin-block: var(--s4); }
  .results-summary p, .history-refresh p { color: var(--muted); font-size: var(--text-sm); }
  .filter-error, .history-error { color: var(--crit); font-size: var(--text-sm); }
  .history-pagination { display: flex; justify-content: center; margin-top: var(--s5); }

  @media (max-width: 70rem) {
    .incident-filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
  @media (max-width: 38rem) {
    .incident-filter-grid { grid-template-columns: minmax(0, 1fr); }
    .history-filters .field { flex-basis: 100%; }
  }

  .cols {
    container-type: inline-size;
    --cols: minmax(0, 1.4fr) 7.25rem 8.75rem 4.125rem 5.5rem minmax(0, 1.1fr) var(--table-action-width);
  }

  .cols :global(.trow > .btn:last-child) {
    justify-self: end;
  }

  @media (max-width: 100rem) {
    .cols {
      --cols: minmax(0, 1fr) 7.25rem 7.5rem 4.125rem 5.5rem var(--table-action-width);
    }

    .cols :global(.log) {
      display: none;
    }
  }

  @media (max-width: 48rem) {
    .cols :global(.trow > .btn:last-child) {
      grid-column: 1 / -1;
      justify-self: end;
    }
  }

  @container (max-width: 44rem) {
    .thead,
    .trow > .hide-sm {
      display: none;
    }

    .trow {
      grid-template-columns: minmax(0, 1fr) auto;
      row-gap: var(--s3);
    }

    .cols :global(.trow > .btn:last-child) {
      grid-column: 1 / -1;
      justify-self: end;
    }
  }

  .nature {
    font-family: var(--font);
    color: var(--faint);
    font-size: var(--text-xs);
  }

  .incident-link {
    display: block;
    min-width: 0;
    border-radius: var(--r-s);
  }

  .incident-link:hover strong {
    color: var(--accent);
  }

  .ack-cell {
    font-size: var(--text-sm);
  }

  .ack {
    display: inline-flex;
    align-items: center;
    gap: 0.3125rem;
    color: var(--muted);
  }

  .mark {
    width: 0.875rem;
    height: 0.875rem;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--ok);
    font-size: 0.75rem;
    font-style: normal;
  }

  .sources {
    display: flex;
    align-items: center;
    gap: 0.3125rem;
  }

  .log {
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .under {
    margin-top: var(--s4);
    color: var(--faint);
    font-size: var(--text-sm);
  }

  .under a {
    color: var(--accent);
  }
</style>
