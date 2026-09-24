<script lang="ts">
  import { session } from '$lib/session.svelte';
  import { dashboardCategoryHealth, type CategoryHealth, type HealthState } from '$lib/dashboard';
  import { isVersionNotice, resourceCategories } from '$lib/resources';
  import { stateTones } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  const segmentOrder: HealthState[] = ['down', 'degraded', 'unknown', 'maintenance', 'ok'];
  const problemTargets = $derived(new Set(session.incidents.filter((incident) => incident.status === 'active' && !isVersionNotice(incident))
    .flatMap((incident) => incident.impacts.filter((impact) => impact.status === 'active').map((impact) => impact.target_id))));
  const updateTargets = $derived(new Set(session.incidents.filter((incident) => incident.status === 'active' && isVersionNotice(incident))
    .flatMap((incident) => incident.impacts.filter((impact) => impact.status === 'active').map((impact) => impact.target_id))));
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
    <p>{t('dashboard.category.hint')}</p>
  </header>
  {#if groups.length}
    <div class="category-list">
      {#each groups as group (group.category)}
        <a class="category-row" href={`/cibles?category=${group.category}`}>
          <span class="row-top">
            <strong>{t(`resources.category.${group.category}`)}</strong>
            <span class="row-count">
              {#if group.category === 'software' || group.category === 'scheduled_task'}
                <b>{plural('resources.results', group.total)}</b>
              {:else}
                <b>{group.counts.ok} / {group.total}</b> {plural('dashboard.category.available', group.counts.ok)}
              {/if}
              <span aria-hidden="true">→</span>
            </span>
          </span>
          {#if group.category === 'software' || group.category === 'scheduled_task'}
            <span class="row-details">{trackingDetails(group)}</span>
          {:else}
            <svg class="category-bar" viewBox="0 0 1000 12" preserveAspectRatio="none" aria-hidden="true">
              {#each segments(group.counts, group.total) as segment (segment.state)}
                {#if segment.width > 0}
                  <rect class={`segment ${stateTones[segment.state]}`} x={segment.offset} width={segment.width} height="12" />
                {/if}
              {/each}
            </svg>
            <span class="row-details">{details(group.counts)}</span>
          {/if}
        </a>
      {/each}
      {#if virtualMachinesUnidentified}
        <a class="category-row" href="/cibles?category=virtual_machine">
          <span class="row-top">
            <strong>{t('resources.category.virtual_machine')}</strong>
            <span class="row-count"><b>0</b> <span aria-hidden="true">→</span></span>
          </span>
          <span class="row-details">{t('dashboard.category.noVirtualMachine')}</span>
        </a>
      {/if}
    </div>
  {:else}
    <p class="category-empty">{t('dashboard.noTargets')}</p>
  {/if}
</section>

<style>
  .category-overview { min-width: 0; overflow: hidden; }
  header { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: start; gap: var(--s2) var(--s4); padding: var(--s4); }
  header h2 { font-size: var(--text-sm); font-weight: 600; }
  header p { grid-column: 1 / -1; font-size: var(--text-xs); color: var(--faint); }
  header a { color: var(--muted); font-size: var(--text-xs); text-align: end; }
  header a:hover { color: var(--ink); }
  .category-list { display: grid; }
  .category-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--s2) var(--s3); padding: var(--s2) var(--s4); border-top: var(--line-width) solid var(--line); }
  .category-row:hover { background: var(--surface-2); }
  .category-row:focus-visible { outline: var(--s1) solid var(--ink); outline-offset: calc(-1 * var(--s1)); }
  .row-top { grid-column: 1 / -1; display: flex; align-items: baseline; justify-content: space-between; gap: var(--s3); min-width: 0; font-size: var(--text-xs); line-height: 1.2; }
  .row-top strong { min-width: 0; font-weight: 600; overflow-wrap: anywhere; }
  .row-count { color: var(--muted); font-size: 0.75rem; text-align: end; }
  .row-count b { color: var(--ink); font-family: var(--font-num); font-variant-numeric: tabular-nums; font-weight: 600; }
  .row-count > span { margin-inline-start: var(--s2); color: var(--faint); }
  .category-bar { display: block; width: 100%; height: var(--s2); align-self: center; border-radius: var(--r-pill); overflow: hidden; background: var(--surface-3); }
  .segment.ok { fill: var(--ok); }
  .segment.warn { fill: var(--warn); }
  .segment.crit { fill: var(--crit); }
  .segment.info { fill: var(--info); }
  .segment.idle { fill: var(--dim); }
  .row-details { grid-column: 2; color: var(--muted); font-size: 0.75rem; line-height: 1.2; text-align: end; overflow-wrap: anywhere; }
  .category-empty { padding: var(--s5); border-top: var(--line-width) solid var(--line); color: var(--faint); font-size: var(--text-sm); }
  @media (max-width: 48rem) { header { grid-template-columns: minmax(0, 1fr); } header p { grid-column: 1; } header a { grid-row: 2; text-align: start; } }
</style>
