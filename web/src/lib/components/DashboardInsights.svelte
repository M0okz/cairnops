<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import ResourceCategoryOverview from './ResourceCategoryOverview.svelte';
  import { api, type Incident } from '$lib/api';
  import { dashboardIncidentLeaders, dashboardRecentActivity } from '$lib/dashboard';
  import { localeTag, plural, t } from '$lib/i18n.svelte';
  import { session } from '$lib/session.svelte';
  import { severityTone } from '$lib/format';

  let resolved = $state<Incident[]>([]);
  let loading = $state(true);
  let failed = $state(false);
  let gaugeWidth = $state(0);
  let gaugeHeight = $state(0);
  const allIncidents = $derived([...new Map([...resolved, ...session.incidents].map((incident) => [incident.id, incident] as const)).values()]);
  const leaders = $derived(dashboardIncidentLeaders(session.incidents, session.targets));
  const activity = $derived(dashboardRecentActivity(allIncidents));
  const maxIncidents = $derived(Math.max(1, ...leaders.map((row) => row.count)));

  onMount(() => {
    let disposed = false;
    let inFlight = false;
    async function loadHistory() {
      if (inFlight) return;
      inFlight = true;
      try {
        const from = new Date(Date.now() - 7 * 86_400_000).toISOString();
        let cursor = '';
        const incidents: Incident[] = [];
        do {
          const query = new URLSearchParams({ status: 'resolved', page: 'true', limit: '100', resolved_from: from });
          if (cursor) query.set('cursor', cursor);
          const response = await api<{ incidents: Incident[]; next_cursor?: string }>(`/api/v1/incidents?${query}`);
          incidents.push(...response.incidents);
          cursor = response.next_cursor ?? '';
        } while (cursor && !disposed);
        if (!disposed) { resolved = incidents; failed = false; }
      } catch {
        if (!disposed) failed = true;
      } finally {
        inFlight = false;
        if (!disposed) loading = false;
      }
    }
    void loadHistory();
    const historyTimer = setInterval(loadHistory, 60_000);
    return () => { disposed = true; clearInterval(historyTimer); };
  });

  const time = (value: string) => new Intl.DateTimeFormat(localeTag(), { hour: '2-digit', minute: '2-digit' }).format(new Date(value));
  const activityName = (incident: Incident) => incident.impacts[0]?.target_name ?? t('nav.incidents');
  const activityTone = (incident: Incident, kind: string) => kind.includes('resolv') || kind.includes('recover') ? 'ok' : severityTone(incident.severity);
</script>

<div class="dashboard-insights">
  <ResourceCategoryOverview />
  <section class="insight-card card" aria-labelledby="dashboard-leaders-title">
    <header><div><h2 id="dashboard-leaders-title">{t('dashboard.topIncidents')}</h2><p>{t('dashboard.activeIncidentsHint')}</p></div><Icon name="incidents" size={18} /></header>
    {#if leaders.length === 0}
      <p class="empty">{t('dashboard.noActiveIncidents')}</p>
    {:else}
      <div class="leader-list">
        {#each leaders as row (row.target.id)}
          <a href="/cibles/{row.target.id}" class="leader-row" class:unacknowledged={row.unacknowledged > 0} class:crit={severityTone(row.severity) === 'crit'} class:warn={severityTone(row.severity) === 'warn'}>
            <span class="row-name">{row.target.name}</span>
            <span class="leader-gauge" bind:clientWidth={gaugeWidth} bind:clientHeight={gaugeHeight}>
              <svg viewBox="0 0 {gaugeWidth || 100} {gaugeHeight || 6}" role="img" aria-label={plural('dashboard.incidentCount', row.count)}>
                <rect class="track" width={gaugeWidth || 100} height={gaugeHeight || 6} rx={(gaugeHeight || 6) / 2} />
                <rect class="bar" width={row.count / maxIncidents * (gaugeWidth || 100)} height={gaugeHeight || 6} rx={(gaugeHeight || 6) / 2} />
              </svg>
            </span>
            <span class="row-count num">{row.count}</span>
          </a>
        {/each}
      </div>
    {/if}
  </section>

  <section class="insight-card card" aria-labelledby="dashboard-activity-title">
    <header><h2 id="dashboard-activity-title">{t('dashboard.recentActivity')}</h2><a href="/incidents">{t('dashboard.viewAll')} <span aria-hidden="true">→</span></a></header>
    {#if loading}
      <p class="empty" role="status">{t('dashboard.loading')}</p>
    {:else if failed && activity.length === 0}
      <p class="empty" role="status">{t('dashboard.historyUnavailable')}</p>
    {:else if activity.length === 0}
      <p class="empty">{t('dashboard.noRecentActivity')}</p>
    {:else}
      <ol class="activity-list">
        {#each activity as row (`${row.incident.id}:${row.entry.id}`)}
          <li>
            <i class="activity-dot {activityTone(row.incident, row.entry.kind)}"></i>
            <time datetime={row.entry.occurred_at}>{time(row.entry.occurred_at)}</time>
            <a href="/incidents?incident={row.incident.id}"><strong>{activityName(row.incident)}</strong><span>{row.entry.message}</span></a>
          </li>
        {/each}
      </ol>
    {/if}
  </section>
</div>

<style>
  .dashboard-insights { grid-column: 1 / -1; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--s4); min-width: 0; }
  .insight-card { min-width: 0; padding: var(--s4); border-radius: var(--r-overview); }
  .insight-card > header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--s3); min-height: var(--s7); margin-bottom: var(--s3); padding: 0; border: 0; }
  h2 { font-size: var(--text-sm); font-weight: 600; }
  header p, header a { color: var(--faint); font-size: var(--text-xs); }
  header p { margin-top: var(--s2); }
  header a { flex: none; }
  header a:hover, .leader-row:hover, .activity-list a:hover { color: var(--ink); }
  .leader-list { display: grid; gap: var(--s2); }
  .leader-row { display: grid; align-items: center; min-height: 2rem; gap: var(--s3); color: var(--muted); font-size: var(--text-xs); }
  .leader-row { grid-template-columns: minmax(5rem, 1fr) minmax(3rem, 0.8fr) 4ch; }
  .row-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .row-count { justify-self: end; white-space: nowrap; }
  .leader-gauge { display: block; min-width: 0; height: var(--incident-rank-gauge-height); }
  .leader-gauge svg { display: block; width: 100%; height: 100%; }
  .track { fill: var(--line); }
  .bar { fill: var(--info); }
  .leader-row.warn .bar { fill: var(--warn); }
  .leader-row.crit .bar { fill: var(--crit); }
  .leader-row.unacknowledged .row-name, .leader-row.unacknowledged .row-count { color: var(--ink); font-weight: 600; }
  .activity-list { list-style: none; display: grid; }
  .activity-list li { display: grid; grid-template-columns: var(--s3) 2.75rem minmax(0, 1fr); align-items: start; gap: var(--s3); padding: var(--s2) 0; border-left: 1px solid var(--line); margin-inline-start: var(--s2); }
  .activity-dot { display: block; width: var(--s3); height: var(--s3); border-radius: 50%; transform: translateX(calc(-50% - 1px)); background: var(--info); }
  .activity-dot.ok { background: var(--ok); } .activity-dot.warn { background: var(--warn); } .activity-dot.crit { background: var(--crit); }
  time { color: var(--faint); font: var(--text-xs) var(--font-num); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .activity-list a { display: grid; min-width: 0; color: var(--muted); font-size: var(--text-xs); line-height: 1.3; }
  .activity-list strong { color: var(--ink); font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .activity-list span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .empty { color: var(--faint); font-size: var(--text-sm); padding: var(--s5) 0; }
  @media (max-width: 100rem) {
    .dashboard-insights { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .dashboard-insights .insight-card:last-child { grid-column: 1 / -1; }
  }
  @media (max-width: 48rem) {
    .dashboard-insights { grid-template-columns: minmax(0, 1fr); }
    .dashboard-insights .insight-card:last-child { grid-column: auto; }
  }
</style>
