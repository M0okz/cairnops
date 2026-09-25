<script lang="ts">
  import { onMount } from 'svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import { api, type ResourceCategory } from '$lib/api';
  import { session } from '$lib/session.svelte';
  import { dashboardCategoryHealth, type CategoryHealth, type HealthState } from '$lib/dashboard';
  import { isVersionNotice, resourceCategories } from '$lib/resources';
  import { updateTargetIds, type SoftwareService } from '$lib/software-updates';
  import { stateTones } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  const segmentOrder: HealthState[] = ['down', 'degraded', 'unknown', 'maintenance', 'ok'];
  const categoryIcons: Record<ResourceCategory, IconName> = {
    virtual_machine: 'monitor', container: 'cube', virtualization_host: 'server', storage: 'database',
    network: 'signal', host: 'server', service: 'worker', application: 'overview',
    scheduled_task: 'activity', software: 'changelog', infrastructure: 'connectors', unclassified: 'targets'
  };
  const problemTargets = $derived(new Set(session.incidents.filter((incident) => incident.status === 'active' && !isVersionNotice(incident))
    .flatMap((incident) => incident.impacts.filter((impact) => impact.status === 'active').map((impact) => impact.target_id))));
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
    problem: problemTargets.has(target.id),
    update: updateTargets.has(target.id)
  })), resourceCategories));
  const virtualMachinesUnidentified = $derived(!groups.some((group) => group.category === 'virtual_machine'));

  function details(counts: Record<HealthState, number>): string {
    const parts = [
      counts.down && plural('dashboard.category.down', counts.down),
      counts.degraded && plural('dashboard.category.degraded', counts.degraded),
      counts.unknown && plural('dashboard.category.unknown', counts.unknown),
      counts.maintenance && plural('dashboard.category.maintenance', counts.maintenance)
    ].filter(Boolean);
    return parts.length ? parts.join(' · ') : t('dashboard.category.allAvailable');
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
</script>

<section class="category-overview card" aria-labelledby="category-overview-title">
  <header>
    <h2 id="category-overview-title">{t('dashboard.category.title')}</h2>
    <a href="/cibles">{t('dashboard.viewTargets')} <span aria-hidden="true">→</span></a>
  </header>
  <p class="sr-only">{t('dashboard.category.hint')}</p>
  {#if groups.length}
    <div class="category-list">
      {#each groups as group (group.category)}
        <a class="category-row" href={`/cibles?category=${group.category}`}
          aria-label={group.category === 'software' || group.category === 'scheduled_task'
            ? `${t(`resources.category.${group.category}`)} · ${plural('resources.results', group.total)} · ${trackingDetails(group)}`
            : `${t(`resources.category.${group.category}`)} · ${group.counts.ok} / ${group.total} ${plural('dashboard.category.available', group.counts.ok)} · ${details(group.counts)}`}
          title={group.category === 'software' || group.category === 'scheduled_task' ? trackingDetails(group) : details(group.counts)}>
          <span class="category-icon"><Icon name={categoryIcons[group.category]} size={16} /></span>
          <strong class="row-name">{t(`resources.category.${group.category}`)}</strong>
          {#if group.category === 'software' || group.category === 'scheduled_task'}
            <span class="row-details">{trackingDetails(group)}</span>
            <span class="row-count"><b>{group.total}</b></span>
          {:else}
            <svg class="category-bar" viewBox="0 0 1000 12" preserveAspectRatio="none" aria-hidden="true">
              {#each segments(group.counts, group.total) as segment (segment.state)}
                {#if segment.width > 0}
                  <rect class={`segment ${stateTones[segment.state]}`} x={segment.offset} width={segment.width} height="12" />
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
          <span class="row-details">{t('dashboard.category.noVirtualMachine')}</span>
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
  .category-overview > header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--s3); min-height: var(--s7); margin-bottom: var(--s3); padding: 0; border: 0; }
  header h2 { font-size: var(--text-sm); font-weight: 600; }
  header a { flex: none; color: var(--faint); font-size: var(--text-xs); text-align: end; }
  header a:hover { color: var(--ink); }
  .category-list { display: grid; }
  .category-row { display: grid; grid-template-columns: var(--s4) minmax(0, 1fr) minmax(var(--s6), 0.55fr) auto; align-items: center; gap: var(--s3); min-height: var(--s6); color: var(--muted); font-size: var(--text-xs); }
  .category-row:hover { color: var(--ink); }
  .category-row:focus-visible { outline: var(--s1) solid var(--ink); outline-offset: var(--s2); }
  .category-icon { display: flex; align-items: center; color: var(--icon); }
  .row-name { min-width: 0; color: var(--ink); font-weight: 500; line-height: 1.2; overflow-wrap: anywhere; }
  .row-count { min-width: 5ch; color: var(--muted); font-size: 0.75rem; text-align: end; white-space: nowrap; }
  .row-count b { color: var(--ink); font-family: var(--font-num); font-variant-numeric: tabular-nums; font-weight: 600; }
  .category-bar { display: block; width: 100%; height: var(--incident-rank-gauge-height); border-radius: var(--r-pill); overflow: hidden; background: var(--surface-3); }
  .segment.ok { fill: var(--ok); }
  .segment.warn { fill: var(--warn); }
  .segment.crit { fill: var(--crit); }
  .segment.info { fill: var(--info); }
  .segment.idle { fill: var(--dim); }
  .row-details { min-width: 0; color: var(--muted); font-size: 0.75rem; text-align: end; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .category-empty { padding: var(--s5) 0; color: var(--faint); font-size: var(--text-sm); }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  @media (max-width: 32rem) {
    .category-overview > header { flex-wrap: wrap; }
    .category-row { grid-template-columns: var(--s4) minmax(0, 1fr) minmax(var(--s6), 0.65fr) auto; }
  }
</style>
