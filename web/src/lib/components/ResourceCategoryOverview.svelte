<script lang="ts">
  import { onMount } from 'svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import { api, type IncidentSeverity, type ResourceCategory } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { dashboardCategoryHealth, type CategoryHealth, type HealthState } from '$lib/dashboard';
  import { isVersionNotice, resourceCategories } from '$lib/resources';
  import { updateTargetIds, type SoftwareService } from '$lib/software-updates';
  import { severityWeight, stateTones } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  const segmentOrder: HealthState[] = ['down', 'degraded', 'unknown', 'maintenance', 'ok'];
  const categoryIcons: Record<ResourceCategory, IconName> = {
    virtual_machine: 'monitor', container: 'cube', virtualization_host: 'server', storage: 'database',
    network: 'signal', host: 'server', service: 'worker', application: 'overview',
    scheduled_task: 'activity', software: 'changelog', infrastructure: 'connectors', unclassified: 'targets'
  };
  const problemSeverities = $derived.by(() => {
    const severities = new Map<string, IncidentSeverity>();
    for (const incident of session.incidents) {
      if (incident.status !== 'active' || isVersionNotice(incident)) continue;
      for (const impact of incident.impacts) {
        if (impact.status !== 'active') continue;
        const previous = severities.get(impact.target_id);
        if (!previous || severityWeight(impact.effective_severity) > severityWeight(previous)) {
          severities.set(impact.target_id, impact.effective_severity);
        }
      }
    }
    return severities;
  });
  // Les mises à jour disponibles viennent du suivi des versions : elles
  // n'ouvrent plus d'Incident, sauf correctif de sécurité.
  let software = $state<SoftwareService[]>([]);
  const updateTargets = $derived(updateTargetIds(software));
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
    const timer = setInterval(loadVersions, 60_000);
    return () => { disposed = true; clearInterval(timer); };
  });
  const groups = $derived(dashboardCategoryHealth(session.targets.map((target) => ({
    category: target.category,
    state: session.targetState(target),
    problem: problemSeverities.has(target.id),
    problemSeverity: problemSeverities.get(target.id),
    update: updateTargets.has(target.id)
  })), resourceCategories));
  const virtualMachinesUnidentified = $derived(!groups.some((group) => group.category === 'virtual_machine'));

  function details(counts: Record<HealthState, number>, problems: number): string {
    const parts = [
      counts.down && plural('dashboard.category.down', counts.down),
      counts.degraded && plural('dashboard.category.degraded', counts.degraded),
      counts.unknown && plural('dashboard.category.unknown', counts.unknown),
      counts.maintenance && plural('dashboard.category.maintenance', counts.maintenance)
    ].filter(Boolean);
    const state = parts.length ? parts.join(' · ') : t('dashboard.category.allAvailable');
    return problems ? `${state} · ${plural('dashboard.category.problems', problems)}` : state;
  }

  function trackingDetails(group: CategoryHealth): string {
    const parts = [
      group.problems && plural('dashboard.category.problems', group.problems),
      group.category === 'software' && group.updates && plural('dashboard.category.updates', group.updates)
    ].filter(Boolean);
    return parts.length ? parts.join(' · ') : t(group.category === 'software' ? 'resources.versionTracking' : 'resources.taskTracking');
  }

  function segments(counts: Record<HealthState, number>, total: number) {
    let offset = 0;
    return segmentOrder.map((state) => {
      const width = total ? counts[state] / total * 1000 : 0;
      const segment = { state, offset, width };
      offset += width;
      return segment;
    });
  }

  function trackingSegments(group: CategoryHealth) {
    let offset = 0;
    return ([['crit', group.attention.crit], ['warn', group.attention.warn], ['info', group.attention.info], ['update', group.tracking.updates]] as const).map(([tone, count]) => {
      const width = count / group.total * 1000;
      const segment = { tone, offset, width };
      offset += width;
      return segment;
    });
  }

  function attentionSegments(group: CategoryHealth) {
    let offset = 0;
    return ([['crit', group.attention.crit], ['warn', group.attention.warn], ['info', group.attention.info]] as const).map(([tone, count]) => {
      const width = count / group.total * 1000;
      const segment = { tone, offset, width };
      offset += width;
      return segment;
    });
  }
</script>

<section class="category-overview card" aria-labelledby="category-overview-title">
  <header>
    <h2 id="category-overview-title">{t('dashboard.category.title')}</h2>
    <a href="/cibles">{t('dashboard.viewTargets')} <span aria-hidden="true">→</span></a>
    <p>{t('dashboard.category.subtitle')}</p>
  </header>
  <p class="sr-only">{t('dashboard.category.hint')}</p>
  {#if groups.length}
    <div class="category-list">
      {#each groups as group (group.category)}
        <a class="category-row" href={`/cibles?category=${group.category}`}
          aria-label={group.category === 'software' || group.category === 'scheduled_task'
            ? `${t(`resources.category.${group.category}`)} · ${plural('resources.results', group.total)} · ${trackingDetails(group)}`
            : `${t(`resources.category.${group.category}`)} · ${group.counts.ok} / ${group.total} ${plural('dashboard.category.available', group.counts.ok)} · ${details(group.counts, group.problems)}`}
          title={`${t(`resources.category.${group.category}`)} · ${group.category === 'software' || group.category === 'scheduled_task' ? trackingDetails(group) : details(group.counts, group.problems)}`}>
          <span class="category-icon"><Icon name={categoryIcons[group.category]} size={16} /></span>
          <strong class="row-name">{t(`resources.category.${group.category}`)}</strong>
          {#if group.category === 'software' || group.category === 'scheduled_task'}
            <svg class="category-bar" viewBox="0 0 1000 12" preserveAspectRatio="none" aria-hidden="true">
              {#each trackingSegments(group) as segment (segment.tone)}
                {#if segment.width > 0}
                  <rect class={`segment ${segment.tone}`} x={segment.offset} width={segment.width} height="12" />
                {/if}
              {/each}
            </svg>
            <span class="row-count"><b>{group.total}</b></span>
          {:else}
            <svg class="category-bar" viewBox="0 0 1000 12" preserveAspectRatio="none" aria-hidden="true">
              {#each segments(group.counts, group.total) as segment (segment.state)}
                {#if segment.width > 0}
                  <rect class={`segment ${stateTones[segment.state]}`} x={segment.offset} width={segment.width} height="7" />
                {/if}
              {/each}
              {#each attentionSegments(group) as segment (segment.tone)}
                {#if segment.width > 0}
                  <rect class={`segment ${segment.tone}`} x={segment.offset} y="8" width={segment.width} height="4" />
                {/if}
              {/each}
            </svg>
            <span class="row-count"><b>{group.counts.ok} / {group.total}</b></span>
          {/if}
        </a>
      {/each}
      {#if virtualMachinesUnidentified}
        <a class="category-row" href="/cibles?category=virtual_machine" aria-label={`${t('resources.category.virtual_machine')} · ${t('dashboard.category.noVirtualMachine')}`}>
          <span class="category-icon"><Icon name="monitor" size={16} /></span>
          <strong class="row-name">{t('resources.category.virtual_machine')}</strong>
          <span class="category-bar"></span>
          <span class="row-count"><b>0</b></span>
        </a>
      {/if}
    </div>
  {:else}
    <p class="category-empty">{t('dashboard.noTargets')}</p>
  {/if}
</section>

<style>
  .category-overview { min-width: 0; overflow: hidden; padding: var(--s4); border-radius: var(--r-overview); }
  .category-overview > header { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-content: start; gap: var(--s2) var(--s3); min-height: var(--s7); margin-bottom: var(--s3); padding: 0; border: 0; }
  header h2 { font-size: var(--text-sm); font-weight: 600; }
  header p { grid-column: 1 / -1; color: var(--faint); font-size: var(--text-xs); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  header a { flex: none; color: var(--faint); font-size: var(--text-xs); text-align: end; }
  header a:hover { color: var(--ink); }
  .category-list { display: grid; }
  .category-row { display: grid; grid-template-columns: var(--s4) minmax(0, 1fr) minmax(3.5rem, 25%) minmax(3.25rem, auto); align-items: center; gap: var(--s3); min-height: var(--s6); color: var(--muted); font-size: var(--text-xs); }
  .category-row:hover { color: var(--ink); }
  .category-row:focus-visible { outline: var(--s1) solid var(--ink); outline-offset: var(--s2); }
  .category-icon { display: flex; align-items: center; color: var(--icon); }
  .row-name { min-width: 0; color: var(--ink); font-weight: 500; line-height: 1.2; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .row-count { color: var(--muted); font-size: 0.75rem; text-align: end; white-space: nowrap; }
  .row-count b { color: var(--ink); font-family: var(--font-num); font-variant-numeric: tabular-nums; font-weight: 600; }
  .category-bar { display: block; width: 100%; height: var(--category-overview-bar-height); border-radius: var(--r-pill); overflow: hidden; background: var(--surface-3); }
  .segment.ok { fill: var(--ok); }
  .segment.warn { fill: var(--warn); }
  .segment.crit { fill: var(--crit); }
  .segment.info { fill: var(--info); }
  .segment.idle { fill: var(--dim); }
  .segment.update { fill: var(--accent); }
  .category-empty { padding: var(--s5) 0; color: var(--faint); font-size: var(--text-sm); }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  @media (max-width: 32rem) {
    .category-overview > header { grid-template-columns: minmax(0, 1fr); }
    .category-row { grid-template-columns: var(--s4) minmax(0, 1fr) minmax(3rem, 25%) minmax(3.25rem, auto); }
  }
</style>
