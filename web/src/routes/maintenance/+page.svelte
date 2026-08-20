<script lang="ts">
  /* Écran 4h — Maintenance.
   * Une fenêtre suspend le routage des notifications sans effacer les preuves :
   * la collecte continue, seule la projection est neutralisée. */

  import Topbar from '$lib/components/Topbar.svelte';
  import MaintenanceWorkshop from '$lib/components/MaintenanceWorkshop.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { since, stamp, until } from '$lib/format';
  import { t } from '$lib/i18n.svelte';
  import type { Maintenance } from '$lib/api';

  let scope = $state<'planned' | 'past'>('planned');
  let workshopOpen = $state(false);
  let now = $state(new Date());

  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });

  const running = $derived(session.maintenances.filter((item) => item.state === 'active'));
  const planned = $derived(
    session.maintenances
      .filter((item) => item.state === 'active' || item.state === 'upcoming')
      .sort((a, b) => new Date(a.starts_at).getTime() - new Date(b.starts_at).getTime())
  );
  const past = $derived(
    session.maintenances
      .filter((item) => item.state === 'ended' || item.state === 'cancelled')
      .sort((a, b) => new Date(b.ends_at).getTime() - new Date(a.ends_at).getTime())
  );

  const shown = $derived(scope === 'planned' ? planned : past);

  const windowStates = $derived<Record<Maintenance['state'], { label: string; tone: string }>>({
    active: { label: t('maintenance.state.active'), tone: 'info' },
    upcoming: { label: t('maintenance.state.planned'), tone: 'idle' },
    ended: { label: t('maintenance.state.ended'), tone: 'ok' },
    cancelled: { label: t('maintenance.state.cancelled'), tone: 'idle' }
  });

  function duration(item: Maintenance) {
    return since(item.starts_at, new Date(item.ends_at));
  }

  /* Le fuseau affiché est celui du navigateur ; le stockage reste en UTC. */
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
</script>

<svelte:head><title>{t('nav.maintenance')} — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.maintenance') }]} />

<div class="page">
  <div class="page-head">
    <div>
      <h1>{t('nav.maintenance')}</h1>
      <p>{t('maintenance.lead')}</p>
    </div>
    <div class="page-actions">
      <button class="btn primary" type="button" onclick={() => (workshopOpen = true)}>
        {t('maintenance.plan')}
      </button>
    </div>
  </div>

  {#each running as item (item.id)}
    <div class="banner info current">
      <i class="dot info"></i>
      <div class="banner-copy">
        <strong>
          {t('maintenance.current')} · {item.targets.map((target) => target.name).join(', ')} · {item.name}
        </strong>
        <p>
          {t('maintenance.currentRange', {
            start: stamp(item.starts_at),
            end: stamp(item.ends_at),
            remaining: until(item.ends_at, now)
          })}
          {t('maintenance.currentSay')}
        </p>
      </div>
      <button class="btn sm" type="button" onclick={() => session.cancelMaintenance(item)}>
        {t('maintenance.endNow')}
      </button>
    </div>
  {/each}

  <div class="filters">
    <div class="segments" role="group" aria-label={t('targets.scope')}>
      <button type="button" aria-pressed={scope === 'planned'} onclick={() => (scope = 'planned')}>
        {t('maintenance.scope.planned')} <b><Odometer value={planned.length} /></b>
      </button>
      <button type="button" aria-pressed={scope === 'past'} onclick={() => (scope = 'past')}>
        {t('maintenance.scope.past')} <b><Odometer value={past.length} /></b>
      </button>
    </div>
    <span class="note">{t('maintenance.timezone', { zone })}</span>
  </div>

  <div class="card cols">
    <div class="thead">
      <span>{t('maintenance.column.reason')}</span>
      <span>{t('targets.column.state')}</span>
      <span class="hide-sm">{t('maintenance.column.range')}</span>
      <span class="hide-sm">{t('incidents.column.duration')}</span>
      <span class="hide-sm">{t('maintenance.column.targets')}</span>
      <span class="hide-sm">{t('maintenance.column.createdBy')}</span>
      <span></span>
    </div>

    {#each shown as item (item.id)}
      {@const state = windowStates[item.state]}
      <div class="trow">
        <span class="cell-name">
          <i class="dot {state.tone}"></i>
          <span>
            <strong>{item.name}</strong>
            {#if item.reason}<small class="reason">{item.reason}</small>{/if}
          </span>
        </span>

        <span class="pill {state.tone}">{state.label}</span>

        <span class="num hide-sm">{stamp(item.starts_at)} → {stamp(item.ends_at)}</span>

        <span class="num hide-sm"><Odometer value={duration(item)} /></span>

        <span class="muted hide-sm targets">
          {item.targets.length > 0
            ? item.targets.map((target) => target.name).join(', ')
            : t('maintenance.allTargets')}
        </span>

        <span class="faint hide-sm">{item.created_by ?? t('common.none')}</span>

        {#if item.state === 'active' || item.state === 'upcoming'}
          <button class="btn sm" type="button" onclick={() => session.cancelMaintenance(item)}>
            {item.state === 'active' ? t('maintenance.end') : t('common.cancel')}
          </button>
        {:else}
          <span></span>
        {/if}
      </div>
    {:else}
      <div class="empty">
        {#if scope === 'planned'}
          <strong>{t('maintenance.emptyPlanned')}</strong>
          {t('maintenance.emptyPlannedHint')}
        {:else}
          <strong>{t('maintenance.emptyPast')}</strong>
          {t('maintenance.emptyPastHint')}
        {/if}
      </div>
    {/each}
  </div>
</div>

{#if workshopOpen}
  <MaintenanceWorkshop
    targets={session.targets}
    onclose={() => (workshopOpen = false)}
    onsuccess={async (maintenance) => {
      await Promise.all([session.loadMaintenances(), session.loadIncidents()]);
      session.showNotice(
        maintenance.state === 'active'
          ? t('maintenance.noticeActive')
          : t('maintenance.noticePlanned')
      );
    }}
  />
{/if}

<style>
  .cols {
    --cols: minmax(0, 1.3fr) 6.75rem 11.875rem 4.125rem minmax(0, 1fr) 6rem auto;
  }

  .current {
    margin-bottom: var(--s5);
    align-items: center;
  }

  .banner-copy {
    flex: 1;
    min-width: 0;
  }

  .banner-copy strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .banner-copy p {
    margin-top: 0.25rem;
    color: var(--muted);
    font-size: 0.75rem;
  }

  .reason {
    font-family: var(--font);
    color: var(--faint);
    font-size: 0.6875rem;
  }

  .targets {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
