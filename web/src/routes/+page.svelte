<script lang="ts">
  import Topbar from '$lib/components/Topbar.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import Bars from '$lib/components/Bars.svelte';
  import Uptime from '$lib/components/Uptime.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import IndicatorOverview from '$lib/components/IndicatorOverview.svelte';
  import IncidentDetailDrawer from '$lib/components/IncidentDetailDrawer.svelte';
  import SegmentedControl from '$lib/components/ui/SegmentedControl.svelte';
  import { session } from '$lib/session.svelte';
  import { dashboardCoverage, dashboardHealth, type HealthState } from '$lib/dashboard';
  import { i18n, localeTag, plural, t } from '$lib/i18n.svelte';
  import { inWindow, latency, natureLabel, ratio, severityLabel, severityTone, severityWeight, since, stateLabel, stateTones } from '$lib/format';
  import type { Incident } from '$lib/api';

  let now = $state(new Date());
  let filter = $state('');
  let scope = $state<'all' | 'watch'>('all');
  let selectedIncidentID = $state('');
  let acknowledging = $state('');
  $effect(() => { const timer = setInterval(() => now = new Date(), 30_000); return () => clearInterval(timer); });
  const health = $derived(dashboardHealth(session.targets.map((target) => session.targetState(target))));
  const states: HealthState[] = ['ok', 'degraded', 'down', 'maintenance', 'unknown'];
  const distribution = $derived.by(() => {
    let offset = 0;
    return states.map((state) => { const segment = { state, count: health.counts[state], offset }; offset += segment.count; return segment; });
  });
  const distributionLabel = $derived(distribution.map((row) => `${stateLabel(row.state)} : ${row.count}`).join(', '));
  const coverage = $derived(dashboardCoverage(session.targets.map((target) => inWindow(session.measures[target.id], '24h'))));
  const nativeSources = $derived(session.targets.reduce((sum, target) => sum + target.sources.length, 0));
  const externalSources = $derived(session.targets.reduce((sum, target) => sum + target.external_source_count, 0));
  const sourceCount = $derived(nativeSources + externalSources);
  const freshest = $derived(Object.values(session.measures).map((row) => row.latest_observed_at).filter((at): at is string => Boolean(at)).sort().at(-1));
  const openedDays = $derived(session.incidentDays.map((day) => day.opened));
  const active = $derived([...session.actionable].sort((a, b) => Number(Boolean(a.acknowledged_at)) - Number(Boolean(b.acknowledged_at)) || severityWeight(b.severity) - severityWeight(a.severity) || a.opened_at.localeCompare(b.opened_at)));
  const selectedIncident = $derived(session.incidents.find((incident) => incident.id === selectedIncidentID) ?? null);
  const canAcknowledge = $derived(session.user?.role === 'administrator' || session.user?.role === 'operator');
  const verdict = $derived.by(() => {
    const counts = health.counts;
    switch (health.state) {
      case 'down': return { tone: 'crit', title: [plural('dashboard.down', counts.down), counts.degraded ? plural('dashboard.degraded', counts.degraded) : ''].filter(Boolean).join(' · '), href: '/incidents' };
      case 'degraded': return { tone: 'warn', title: plural('dashboard.degraded', counts.degraded), href: '/incidents' };
      case 'unknown': return { tone: 'idle', title: plural('dashboard.unknown', counts.unknown), href: '/cibles' };
      case 'maintenance': return { tone: 'info', title: plural('dashboard.maintenance', counts.maintenance), href: '/maintenance' };
      case 'empty': return { tone: 'idle', title: t('dashboard.empty'), href: '/cibles' };
      default: return { tone: 'ok', title: t('dashboard.allClear'), href: '/cibles' };
    }
  });
  const targetRows = $derived(session.targets.map((target) => ({ target, state: session.targetState(target), measure: inWindow(session.measures[target.id], '24h'), trend: session.measures[target.id]?.trend ?? [] }))
    .filter((row) => (scope === 'all' || row.state !== 'ok' && row.state !== 'maintenance') && `${row.target.name} ${row.target.description}`.toLocaleLowerCase(i18n.locale).includes(filter.toLocaleLowerCase(i18n.locale)))
    .sort((a, b) => ['down', 'degraded', 'unknown', 'maintenance', 'ok'].indexOf(a.state) - ['down', 'degraded', 'unknown', 'maintenance', 'ok'].indexOf(b.state) || a.target.name.localeCompare(b.target.name, i18n.locale)));

  async function acknowledge(incident: Incident) {
    acknowledging = incident.id;
    try { await session.acknowledge(incident); } finally { acknowledging = ''; }
  }
  function dismissIncident() { selectedIncidentID = ''; }
</script>

<svelte:head><title>{t('nav.overview')} — {session.instanceLabel}</title></svelte:head>
<Topbar crumbs={[{ label: t('nav.overview') }]} />

<div class="page overview-page">
  <div class="intro">
    <div><span class="eyebrow">{t('dashboard.eyebrow')}</span><h1>{t('overview.title')}</h1><p>{t('dashboard.lead', { targets: health.total, sources: sourceCount })}</p></div>
    <time datetime={now.toISOString()}>{new Intl.DateTimeFormat(localeTag(), { day: 'numeric', month: 'long', year: 'numeric' }).format(now)}</time>
  </div>

  <a class="overview-verdict {verdict.tone}" href={verdict.href}>
    <Icon name={health.state === 'down' || health.state === 'degraded' ? 'incidents' : health.state === 'ok' ? 'state-healthy' : 'health'} size={22} />
    <strong>{verdict.title}</strong>
    <span class="verdict-detail">{health.state === 'empty' ? t('dashboard.emptyHint') : freshest ? t('dashboard.freshness', { duration: since(freshest, now) }) : t('overview.noEvidence')}</span>
    <span class="verdict-action">{t('dashboard.inspect')} <span aria-hidden="true">→</span></span>
  </a>

  <div class="overview-metrics">
    <section class="metric card">
      <div class="metric-label"><h2>{t('dashboard.operational')}</h2><Icon name="targets" size={20} /></div>
      <div class="metric-value"><b><Odometer value={health.total ? health.counts.ok : '—'} /></b><span>/ {health.total}</span></div>
      <svg class="fleet-strip" viewBox="0 0 400 12" preserveAspectRatio="none" role="img" aria-label={distributionLabel}>
        <rect width="400" height="12" class="strip-idle" />
        {#if health.total}
          {#each distribution as row}<rect class="strip-{stateTones[row.state]}" x={row.offset / health.total * 400} width={row.count / health.total * 400} height="12" />{/each}
          {#each { length: Math.min(health.total, 64) } as _, index}<line x1={index * 400 / Math.min(health.total, 64)} x2={index * 400 / Math.min(health.total, 64)} y1="0" y2="12" />{/each}
        {/if}
      </svg>
      <small>{health.watched ? `${health.watched} · ${t('dashboard.watch')}` : health.counts.maintenance ? plural('dashboard.maintenance', health.counts.maintenance) : health.total ? t('dashboard.allClear') : t('dashboard.notMeasured')}</small>
    </section>
    <section class="metric card">
      <div class="metric-label"><h2>{t('dashboard.coverage')}</h2><Icon name="activity" size={20} /></div>
      <div class="metric-value"><b><Odometer value={coverage === null ? '—' : new Intl.NumberFormat(localeTag(), { maximumFractionDigits: 2 }).format(coverage * 100)} /></b>{#if coverage !== null}<span>%</span>{/if}</div>
      <svg class="coverage-strip" viewBox="0 0 400 6" preserveAspectRatio="none" aria-hidden="true"><rect width="400" height="6" class="strip-idle" />{#if coverage !== null}<rect width={coverage * 400} height="6" class="strip-accent" />{/if}</svg>
      <small>{t(coverage === null ? 'dashboard.notMeasured' : 'dashboard.measured')}</small>
    </section>
    <section class="metric card">
      <div class="metric-label"><h2>{t('dashboard.currentIncidents')}</h2><Icon name="incidents" size={20} /></div>
      <div class="metric-value"><b><Odometer value={active.length} /></b><span>{t('dashboard.pending', { count: session.unacknowledged.length })}</span></div>
      <div class="metric-history">{#if openedDays.length}<Bars values={openedDays} label={t('overview.fig.dailyHistory')} />{:else}<Bars mode="rule" />{/if}</div>
      <small>{t('overview.fig.dailyHistory')}</small>
    </section>
    <section class="metric card">
      <div class="metric-label"><h2>{t('dashboard.sourceCount')}</h2><Icon name="connectors" size={20} /></div>
      <div class="metric-value"><b><Odometer value={sourceCount} /></b></div>
      <div class="source-origins"><span><Icon name="signal" size={16} />CairnOps</span><span><Icon name="connectors" size={16} />{session.connectors.length} {t('nav.connectors').toLocaleLowerCase(i18n.locale)}</span></div>
      <small>{t('dashboard.sourceSplit', { native: nativeSources, external: externalSources })}</small>
    </section>
  </div>

  <div class="overview-analysis">
    <IndicatorOverview />
    <section class="card current-incidents" aria-labelledby="current-incidents-title">
      <header><div><h2 id="current-incidents-title">{t('dashboard.currentIncidents')} <span class="tally">{active.length}</span></h2><p>{t('dashboard.pending', { count: session.unacknowledged.length })}</p></div><Icon name="incidents" size={20} /></header>
      <div class="incident-list">
        {#each active.slice(0, 3) as incident (incident.id)}
          {@const name = incident.affected_target_count > 1 ? plural('incidents.targetsAffected', incident.affected_target_count) : incident.impacts[0]?.target_name ?? t('nav.incidents')}
          <article class="incident-summary">
            <div class="incident-meta"><span class="severity {severityTone(incident.severity)}"><i class="dot {severityTone(incident.severity)}"></i>{severityLabel(incident.severity)}</span><time datetime={incident.opened_at}>{since(incident.opened_at, now)}</time></div>
            <button class="incident-open" type="button" data-incident-trigger={incident.id} aria-label={t('dashboard.incidentOpen', { name })} onclick={() => selectedIncidentID = incident.id}><strong>{name}</strong><span>{natureLabel(incident)}</span></button>
            <div class="incident-action"><span>{incident.acknowledged_at ? t('overview.acknowledgedShort') : t('dashboard.pending', { count: 1 })}</span>
              {#if !incident.acknowledged_at && canAcknowledge}<button class="btn sm" type="button" disabled={acknowledging === incident.id} onclick={() => acknowledge(incident)}>{t('incident.acknowledge')}<Icon name="acknowledge" size={16} /></button>{/if}
            </div>
          </article>
        {:else}<div class="incidents-empty"><Icon name="state-healthy" size={28} /><strong>{t('dashboard.noIncidents')}</strong><p>{t('dashboard.noIncidentsHint')}</p></div>{/each}
      </div>
      <footer><a href="/incidents">{t('overview.allIncidents')} →</a></footer>
    </section>
  </div>

  <section class="card overview-targets" aria-labelledby="overview-targets-title">
    <header><div><h2 id="overview-targets-title">{t('nav.targets')}</h2><p>{t('dashboard.targetsLead')}</p></div>
      <div class="target-tools"><SegmentedControl label={t('targets.scope')} value={scope} items={[{ value: 'all', label: t('targets.scope.all'), count: health.total }, { value: 'watch', label: t('dashboard.watch'), count: health.watched }]} onValueChange={(value) => (scope = value)} /><label class="target-search"><Icon name="search" size={18} /><input type="search" bind:value={filter} aria-label={t('targets.filterLabel')} placeholder={t('dashboard.targetSearch')} /></label></div>
    </header>
    <div class="target-head target-grid" aria-hidden="true"><span>{t('targets.column.target')}</span><span>{t('targets.column.state')}</span><span class="target-latency">{t('targets.column.averageLatency')}</span><span class="target-availability">{t('targets.column.availabilityCoverage')}</span><span class="target-sources">{t('targets.column.sources')}</span><span></span></div>
    {#each targetRows.slice(0, 6) as row (row.target.id)}
      <a class="overview-target-row target-grid" href="/cibles/{row.target.id}">
        <span class="target-name"><span class="target-symbol"><Icon name={row.target.sources[0]?.kind === 'heartbeat' ? 'worker' : 'targets'} size={20} /></span><span><strong>{row.target.name}</strong><small>{row.target.description || t('nav.targets')}</small></span></span>
        <span class="target-state {stateTones[row.state]}"><i class="dot {stateTones[row.state]}"></i>{stateLabel(row.state)}</span>
        <span class="target-latency num">{latency(row.measure.average_latency_milliseconds)}</span>
        <span class="target-availability"><span class="availability-top"><span class="num">{ratio(row.measure.availability)}</span><Uptime values={row.trend} /></span><small>{t('dashboard.coverageValue', { value: ratio(row.measure.coverage) })}</small></span>
        <span class="target-sources num">{row.target.sources.length + row.target.external_source_count}</span><span class="row-arrow" aria-hidden="true">↗</span>
      </a>
    {:else}<div class="targets-empty"><strong>{t(health.total ? 'dashboard.noMatch' : 'dashboard.noTargets')}</strong>{#if !health.total}<p>{t('dashboard.emptyHint')}</p><a class="btn" href="/cibles">{t('targets.new')} →</a>{/if}</div>{/each}
    <footer><span>{t('dashboard.results', { shown: Math.min(targetRows.length, 6), total: targetRows.length })}</span><a href="/cibles">{t('dashboard.viewTargets')} →</a></footer>
  </section>

  <a class="instance-health" href="/sante"><span><Icon name="health" size={18} />{t('dashboard.instanceHealth')}</span><span class="components">{#each session.system?.components ?? [] as component}<span><i class="dot {component.status === 'operational' ? 'ok' : component.status === 'stale' ? 'warn' : 'crit'}"></i>{component.name} · {t(`component.status.${component.status}`)}</span>{:else}{t('overview.healthUnread')}{/each}</span><span aria-hidden="true">→</span></a>
</div>
{#if selectedIncidentID}<IncidentDetailDrawer incidentId={selectedIncidentID} seed={selectedIncident} ondismiss={dismissIncident} />{/if}

<style>
  .overview-page { display: flex; flex-direction: column; gap: var(--s5); }
  .intro { display: flex; justify-content: space-between; align-items: center; gap: var(--s5); margin-bottom: var(--s3); }
  .eyebrow { display: block; color: var(--faint); font-size: var(--text-xs); margin-bottom: var(--s3); }
  h1 { font-size: 2rem; letter-spacing: -0.045em; margin-bottom: var(--s3); }
  .intro p { color: var(--muted); font-size: var(--text-sm); }
  .intro time { font-size: var(--text-xs); color: var(--faint); flex: none; }
  .overview-verdict { display: flex; align-items: center; gap: var(--s4); padding: var(--s4) var(--s5); border: 1px solid var(--line-strong); border-radius: var(--r-l); font-size: var(--text-sm); }
  .overview-verdict.crit { background: color-mix(in srgb, var(--crit) 4%, var(--surface)); border-color: color-mix(in srgb, var(--crit) 25%, var(--line)); }
  .overview-verdict.warn { background: color-mix(in srgb, var(--warn) 4%, var(--surface)); }
  .overview-verdict strong { color: var(--ink); font-weight: 600; }
  .verdict-detail { color: var(--faint); font-size: var(--text-xs); }
  .verdict-action { display: flex; gap: var(--s4); margin-left: auto; color: var(--muted); flex: none; font-size: var(--text-xs); }
  .overview-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--s4); }
  .metric { display: flex; flex-direction: column; min-width: 0; padding: var(--s5); gap: var(--s4); }
  .metric-label { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); color: var(--faint); }
  .metric-label h2 { font-size: var(--text-sm); color: var(--muted); font-weight: 500; line-height: 1.5; letter-spacing: 0; }
  .metric-value { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--s3); min-height: 3rem; }
  .metric-value b { font-size: 2.5rem; font-weight: 600; letter-spacing: -0.04em; line-height: 1; font-variant-numeric: tabular-nums; }
  .metric-value > span { color: var(--faint); font-size: var(--text-sm); }
  .metric small { font-size: var(--text-xs); color: var(--faint); margin-top: auto; }
  .fleet-strip, .coverage-strip { width: 100%; height: 0.625rem; display: block; }
  .coverage-strip { height: 0.25rem; margin-block: 0.1875rem; }
  .strip-idle { fill: var(--surface-3); } .strip-ok { fill: var(--ok); } .strip-warn { fill: var(--warn); } .strip-crit { fill: var(--crit); } .strip-info { fill: var(--info); } .strip-accent { fill: var(--accent); }
  .fleet-strip line { stroke: var(--surface); stroke-width: 2; }
  /* Bars owns its height: clipping this row hides zero-incident days. */
  .metric-history { color: var(--faint); }
  .source-origins { display: flex; flex-wrap: wrap; gap: var(--s4); font-size: 0.75rem; color: var(--faint); }
  .source-origins span { display: flex; gap: var(--s2); align-items: center; }
  .overview-analysis { display: grid; grid-template-columns: minmax(0, 1.8fr) minmax(19rem, 1fr); gap: var(--s5); align-items: stretch; }
  .current-incidents { min-width: 0; display: flex; flex-direction: column; }
  .current-incidents > header, .overview-targets > header { padding: var(--s5); display: flex; justify-content: space-between; align-items: flex-start; gap: var(--s4); }
  .current-incidents h2, .overview-targets h2 { font-size: 1.125rem; display: flex; align-items: center; gap: var(--s3); }
  .current-incidents header p, .overview-targets header p { margin-top: var(--s3); color: var(--faint); font-size: var(--text-sm); }
  .tally { font-family: var(--font-num); font-size: var(--text-xs); color: var(--faint); border: 1px solid var(--line); padding: var(--s1) var(--s2); border-radius: var(--r-s); }
  .incident-list { padding: 0 var(--s5); flex: 1; }
  .incident-summary { padding: var(--s4) 0 var(--s5); border-top: 1px solid var(--line); }
  .incident-summary:first-child { border-top: 0; padding-top: var(--s3); }
  .incident-meta, .incident-action { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); font-size: var(--text-xs); color: var(--faint); }
  .severity { display: inline-flex; align-items: center; gap: var(--s2); border: 1px solid var(--line); border-radius: var(--r-s); padding: var(--s1) var(--s2); font-size: 0.75rem; }
  .incident-open { display: block; width: 100%; border: 0; background: none; text-align: left; padding: var(--s4) 0; }
  .incident-open strong { display: block; font-size: var(--text-md); font-weight: 600; overflow-wrap: anywhere; }
  .incident-open span { display: block; color: var(--muted); font-size: var(--text-sm); margin-top: var(--s2); }
  .incident-open:hover strong { text-decoration: underline; text-underline-offset: 3px; }
  .incidents-empty { min-height: 18rem; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: var(--s4); text-align: center; }
  .incidents-empty p { font-size: var(--text-sm); color: var(--faint); }
  footer { display: flex; align-items: center; justify-content: space-between; gap: var(--s4); padding: var(--s4) var(--s5); border-top: 1px solid var(--line); color: var(--faint); font-size: var(--text-xs); }
  footer a { color: var(--muted); } footer a:hover { color: var(--ink); }
  .current-incidents footer { justify-content: flex-end; }
  .overview-targets { overflow: hidden; }
  .overview-targets > header { align-items: center; flex-wrap: wrap; }
  .target-tools { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s4); }
  .target-search { height: var(--ctl-h); display: flex; align-items: center; gap: var(--s3); padding: 0 var(--s3); border: 1px solid var(--line-strong); border-radius: var(--r-m); color: var(--faint); }
  .target-search input { width: 12rem; min-width: 0; border: 0; background: none; color: var(--ink); font: inherit; font-size: var(--text-sm); outline-offset: var(--s2); }
  .target-grid { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(0, 1fr) minmax(0, 0.6fr) minmax(0, 1.25fr) 4.5rem 1rem; gap: var(--s5); align-items: center; padding: var(--s4) var(--s5); }
  .target-head { border-block: 1px solid var(--line); background: color-mix(in srgb, var(--bg) 35%, var(--surface)); font-size: var(--text-xs); color: var(--faint); }
  .overview-target-row { min-height: 5.5rem; border-top: 1px solid var(--line); font-size: var(--text-sm); transition: background var(--d1) var(--ease); }
  .target-head + .overview-target-row { border-top: 0; }
  .overview-target-row:hover { background: var(--surface-2); }
  .target-name { min-width: 0; display: flex; align-items: center; gap: var(--s4); }
  .target-name > span:last-child { min-width: 0; }
  .target-symbol { flex: none; display: grid; place-items: center; width: 2.25rem; height: 2.25rem; border: 1px solid var(--line); border-radius: var(--r-m); color: var(--muted); background: var(--bg); }
  .target-name strong { display: block; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .target-name small, .target-availability small { display: block; margin-top: var(--s2); color: var(--faint); font-size: 0.75rem; }
  .target-name small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .target-state { display: flex; align-items: center; gap: var(--s3); font-size: var(--text-xs); }
  .availability-top { display: flex; align-items: center; gap: var(--s4); }
  .availability-top .num { flex: none; font-size: var(--text-xs); }
  .availability-top :global(.uptime) { max-width: 10rem; height: 0.75rem; min-width: 0; }
  .row-arrow { color: var(--faint); justify-self: end; }
  .targets-empty { text-align: center; padding: var(--s7) var(--s5); display: grid; justify-items: center; gap: var(--s4); }
  .targets-empty p { font-size: var(--text-sm); color: var(--muted); }
  .instance-health { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s4); color: var(--faint); font-size: var(--text-xs); }
  .instance-health > span:first-child { display: flex; align-items: center; gap: var(--s3); }
  .components { margin-left: auto; display: flex; flex-wrap: wrap; gap: var(--s5); }
  .components > span { display: flex; align-items: center; gap: var(--s3); }
  @media (max-width: 100rem) { .overview-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .overview-analysis { grid-template-columns: minmax(0, 1.6fr) minmax(17rem, 1fr); } .target-grid { gap: var(--s4); grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) minmax(0, .6fr) minmax(0, 1fr) 1rem; } .target-sources { display: none; } .availability-top :global(.uptime) { display: none; } }
  @media (max-width: 60rem) { .overview-analysis { grid-template-columns: minmax(0, 1fr); } .incident-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s5); } .incident-summary { border-top: 0; padding-top: var(--s3); } .verdict-detail { display: none; } }
  @media (max-width: 48rem) {
    .overview-page { gap: var(--s4); } .intro { align-items: flex-start; } .intro time { display: none; } h1 { font-size: 1.75rem; }
    .overview-verdict { padding: var(--s4); gap: var(--s3); } .verdict-action { font-size: 0; gap: 0; } .verdict-action > span { font-size: 1rem; }
    .metric { padding: var(--s4); gap: var(--s4); } .metric-value b { font-size: 2.25rem; } .metric-label h2 { font-size: var(--text-sm); } .metric-label :global(svg) { display: none; } .metric-value > span { font-size: 0.75rem; } .metric small { font-size: 0.75rem; }
    .incident-list { display: block; padding-inline: var(--s4); } .incident-summary + .incident-summary { border-top: 1px solid var(--line); padding-top: var(--s4); }
    .current-incidents > header, .overview-targets > header { padding: var(--s4); }
    .target-tools, .target-search { width: 100%; } .target-search input { width: 100%; font-size: 1rem; }
    .target-head { display: none; } .target-grid { grid-template-columns: minmax(0, 1fr) auto; gap: var(--s3); padding: var(--s4); } .target-name { grid-column: 1; gap: var(--s3); } .target-name strong { white-space: normal; overflow-wrap: anywhere; } .target-state { grid-column: 1; margin-left: 3rem; } .row-arrow { grid-column: 2; grid-row: 1 / span 2; } .target-latency, .target-availability { display: none; }
    footer { flex-wrap: wrap; padding: var(--s4); } .components { margin-left: 0; gap: var(--s3); }
  }
</style>
