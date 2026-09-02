<script lang="ts">
  /* Écran 4c — Incidents.
   * Un Incident par Cible et par Nature. Les Résolus sont chargés à part : la
   * projection partagée ne retient que les actifs. */

  import { afterNavigate, goto } from '$app/navigation';
  import { page } from '$app/state';
  import IncidentDetailModal from '$lib/components/IncidentDetailModal.svelte';
  import IncidentBurstRows from '$lib/components/IncidentBurstRows.svelte';
  import Topbar from '$lib/components/Topbar.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import { session, messageFrom } from '$lib/session.svelte';
  import { api, type Incident, type IncidentBurst } from '$lib/api';
  import { incidentHref } from '$lib/incident-detail';
  import { shouldLoadResolvedIncidents, type IncidentScope } from '$lib/resolved-incidents';
  import {
    activeSignalRatio,
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
  let resolvedBursts = $state<IncidentBurst[]>([]);
  let linkedBurst = $state<IncidentBurst | null>(null);
  let requestedBurstID = '';
  let resolvedRevision = $state(-1);
  let resolvedRequest = 0;
  let resolvedError = $state('');
  let acknowledging = $state('');
  let now = $state(new Date());
  const incidentIDFrom = (url: URL) => url.searchParams.get('incident')?.trim() ?? '';
  const burstIDFrom = (url: URL) => url.searchParams.get('burst')?.trim() ?? '';
  let selectedIncidentID = $state(incidentIDFrom(page.url));
  let selectedBurstID = $state(burstIDFrom(page.url));

  afterNavigate(({ to }) => {
    selectedIncidentID = to ? incidentIDFrom(to.url) : '';
    selectedBurstID = to ? burstIDFrom(to.url) : '';
  });

  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });

  async function loadLinkedBurst(burstID: string) {
    try {
      const item = await api<IncidentBurst>(`/api/v1/incident-bursts/${burstID}`);
      if (selectedBurstID !== burstID) return;
      linkedBurst = item;
      scope = item.status === 'resolved' ? 'resolved' : 'active';
    } catch (cause) {
      if (selectedBurstID !== burstID) return;
      resolvedError = messageFrom(cause);
    }
  }

  $effect(() => {
    const burstID = selectedBurstID;
    if (!burstID || session.bursts.some((burst) => burst.id === burstID)) {
      linkedBurst = null;
      requestedBurstID = '';
      return;
    }
    if (requestedBurstID === burstID) return;
    requestedBurstID = burstID;
    void loadLinkedBurst(burstID);
  });

  async function loadResolved(revision: number) {
    const request = ++resolvedRequest;
    resolvedError = '';
    try {
      const [response, burstResponse] = await Promise.all([
        api<{ incidents: Incident[] }>('/api/v1/incidents?status=resolved&limit=100'),
        api<{ bursts: IncidentBurst[] }>('/api/v1/incident-bursts?status=resolved&limit=100')
      ]);
      if (request !== resolvedRequest) return;
      resolved = response.incidents;
      resolvedBursts = burstResponse.bursts;
      resolvedRevision = revision;
    } catch (cause) {
      if (request !== resolvedRequest) return;
      resolvedError = messageFrom(cause);
    }
  }

  /* La projection reste paresseuse, puis tout changement d'Incident la rend
   * périmée. Une Résolution reçue pendant que la route reste montée apparaît
   * ainsi dès l'ouverture du filtre, ou immédiatement s'il est déjà ouvert. */
  $effect(() => {
    const revision = session.incidentRevision;
    session.bursts.map((burst) => `${burst.id}:${burst.revision}`).join(',');
    if (!shouldLoadResolvedIncidents(scope, resolvedRevision, revision)) return;
    void loadResolved(revision);
  });

  const active = $derived(
    [...session.actionable].sort((a, b) => {
      if (Boolean(a.acknowledged_at) !== Boolean(b.acknowledged_at)) return a.acknowledged_at ? 1 : -1;
      return new Date(a.opened_at).getTime() - new Date(b.opened_at).getTime();
    })
  );

  const candidateIncidents = $derived(
    scope === 'resolved'
      ? resolved
      : scope === 'unacknowledged'
        ? active.filter((incident) => !incident.acknowledged_at)
        : active
  );
  const candidateBursts = $derived.by(() => {
    const candidates = scope === 'resolved'
      ? resolvedBursts
      : scope === 'unacknowledged'
        ? session.bursts.filter((burst) => !burst.acknowledged_at)
        : session.bursts;
    if (
      linkedBurst &&
      linkedBurst.id === selectedBurstID &&
      (scope === 'resolved') === (linkedBurst.status === 'resolved') &&
      !candidates.some((burst) => burst.id === linkedBurst?.id)
    ) {
      return [...candidates, linkedBurst];
    }
    return candidates;
  });
  const shown = $derived.by(() => {
    const grouped = new Set(candidateBursts.flatMap((burst) => burst.members.map((member) => member.incident_id)));
    const rows: Array<
      | { kind: 'incident'; id: string; openedAt: string; incident: Incident }
      | { kind: 'burst'; id: string; openedAt: string; burst: IncidentBurst }
    > = candidateIncidents
      .filter((incident) => !grouped.has(incident.id))
      .map((incident) => ({ kind: 'incident' as const, id: incident.id, openedAt: incident.opened_at, incident }));
    rows.push(
      ...candidateBursts.map((burst) => ({
        kind: 'burst' as const,
        id: burst.id,
        openedAt: burst.opened_at,
        burst
      }))
    );
    return rows.sort((left, right) => new Date(left.openedAt).getTime() - new Date(right.openedAt).getTime());
  });
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

  async function acknowledgeBurst(burst: IncidentBurst) {
    acknowledging = burst.id;
    try {
      await session.acknowledgeBurst(burst);
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
    <div class="segments" role="group" aria-label={t('targets.scope')}>
      <button type="button" aria-pressed={scope === 'active'} onclick={() => (scope = 'active')}>
        {t('incidents.scope.active')} <b><Odometer value={session.actionable.length} /></b>
      </button>
      <button type="button" aria-pressed={scope === 'unacknowledged'} onclick={() => (scope = 'unacknowledged')}>
        {t('incidents.scope.unacknowledged')} <b><Odometer value={session.unacknowledged.length} /></b>
      </button>
      <button type="button" aria-pressed={scope === 'resolved'} onclick={() => (scope = 'resolved')}>
        {t('incidents.scope.resolved')} <b><Odometer value={resolved.length} /></b>
      </button>
    </div>
    <span class="note">
      {scope === 'resolved' ? t('incidents.lastThirtyDays') : t('incidents.unacknowledgedFirst')}
    </span>
  </div>

  <div class="card cols">
    <div class="thead">
      <span>{t('incidents.column.targetNature')}</span>
      <span>{t('incidents.column.severity')}</span>
      <span class="hide-sm">
        {scope === 'resolved' ? t('incidents.column.resolved') : t('incidents.column.acknowledgement')}
      </span>
      <span class="hide-sm">{t('incidents.column.duration')}</span>
      <span class="hide-sm">{t('targets.column.sources')}</span>
      <span class="hide-sm">{t('incidents.column.lastEntry')}</span>
      <span></span>
    </div>

    {#each shown as row (`${row.kind}:${row.id}`)}
      {#if row.kind === 'burst'}
        <IncidentBurstRows
          burst={row.burst}
          resolved={scope === 'resolved'}
          {now}
          openInitially={selectedBurstID === row.burst.id}
          canAcknowledge={session.user?.role !== 'observer'}
          acknowledging={acknowledging === row.burst.id}
          onacknowledge={() => acknowledgeBurst(row.burst)}
        />
      {:else}
      {@const incident = row.incident}
      <div class="trow">
        <span class="cell-name">
          <i class="dot {scope === 'resolved' ? 'ok' : severityTone(incident.effective_severity)}"></i>
          <span>
            <a
              class="incident-link"
              href={incidentHref(incident.id)}
              data-incident-trigger={incident.id}
            >
              <strong>{incident.target_name}</strong>
              <small class="nature">{natureLabel(incident)}</small>
            </a>
          </span>
        </span>

        <span class="pill {severityTone(incident.effective_severity)}">
          {severityLabel(incident.effective_severity)}
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

        <span class="num hide-sm sources">
          <Odometer value={activeSignalRatio(incident)} />
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
      {/if}
    {:else}
      <div class="empty">
        {#if scope === 'resolved' && resolvedError}
          <strong>{t('incidents.logUnread')}</strong>
          {resolvedError}
        {:else if scope === 'resolved'}
          <strong>{t('incidents.emptyResolved')}</strong>
          {t('incidents.emptyResolvedHint')}
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

  {#if session.incidents.length > session.actionable.length}
    <p class="under">
      {plural('incidents.neutralised', session.incidents.length - session.actionable.length)} —
      <a href="/maintenance">{t('incidents.seeWindows')}</a>.
    </p>
  {/if}
</div>

{#if selectedIncidentID}
  {#key selectedIncidentID}
    <IncidentDetailModal
      incidentId={selectedIncidentID}
      seed={selectedIncident}
      ondismiss={dismissIncident}
    />
  {/key}
{/if}

<style>
  .cols {
    --cols: minmax(0, 1.4fr) 7.25rem 8.75rem 4.125rem 4.375rem minmax(0, 1.1fr) var(--table-action-width);
  }

  .cols :global(.trow > .btn:last-child) {
    justify-self: end;
  }

  @media (max-width: 48rem) {
    .cols :global(.trow > .btn:last-child) {
      justify-self: auto;
    }
  }

  .nature {
    font-family: var(--font);
    color: var(--faint);
    font-size: 0.6875rem;
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
    font-size: 0.75rem;
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
    font-size: 0.5625rem;
    font-style: normal;
  }

  .sources {
    display: flex;
    align-items: center;
    gap: 0.3125rem;
  }

  .log {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .under {
    margin-top: var(--s4);
    color: var(--faint);
    font-size: 0.75rem;
  }

  .under a {
    color: var(--accent);
  }
</style>
