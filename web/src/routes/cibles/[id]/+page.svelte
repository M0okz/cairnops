<script lang="ts">
  /* Écran 4b — Détail d'une Cible.
   * Chaque Source garde son verdict, sa fraîcheur et son origine. Une
   * Divergence est signalée sans produire un cinquième État de santé. */

  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import Topbar from '$lib/components/Topbar.svelte';
  import Spark from '$lib/components/Spark.svelte';
  import Uptime from '$lib/components/Uptime.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import MaintenanceWorkshop from '$lib/components/MaintenanceWorkshop.svelte';
  import TargetIndicators from '$lib/components/TargetIndicators.svelte';
  import TargetWorkshop from '$lib/components/TargetWorkshop.svelte';
  import ReconciliationWorkshop from '$lib/components/ReconciliationWorkshop.svelte';
  import { reconciliationState } from '$lib/reconciliation.svelte';
  import { incidentHref } from '$lib/incident-detail';
  import { incidentTimelineForTarget } from '$lib/incident-timeline';
  import { session } from '$lib/session.svelte';
  import {
    diverges,
    inWindow,
    lastObserved,
    latency,
    leadIncident,
    natureLabel,
    ratio,
    severityLabel,
    severityTone,
    since,
    stamp,
    clock,
    stateLabel,
    stateTones,
    windowLabel
  } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';
  import { api, type IncidentSignal, type MeasureWindow, type ReconciliationSourceSummary, type TargetReconciliationActivity } from '$lib/api';

  type Tab = 'view' | 'sources' | 'checks' | 'log' | 'settings';

  let tab = $state<Tab>('view');
  let maintenanceOpen = $state(false);
  let acknowledging = $state(false);
  let invalidating = $state('');
  let invalidationFor = $state<{ incidentId: string; signal: IncidentSignal } | null>(null);
  let invalidationReason = $state('');
  let now = $state(new Date());
  let controlOpen = $state(false);
  let reconciliationOpen = $state(false);
  let sourceForMove = $state<ReconciliationSourceSummary | null>(null);
  let targetActivity = $state<TargetReconciliationActivity[]>([]);
  let resolutionCheckedFor = $state('');

  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 15_000);
    return () => clearInterval(timer);
  });

  const target = $derived(session.targets.find((item) => item.id === page.params.id) ?? null);
  const incidents = $derived(target ? session.incidentsFor(target.id) : []);
  const incidentHistory = $derived(target ? session.incidentHistoryFor(target.id) : []);
  const lead = $derived(leadIncident(incidents));
  const healthState = $derived(target ? session.targetState(target) : 'unknown');
  const structureBusy = $derived(
    target ? reconciliationState.activeOperations.some((operation) =>
      operation.primary_target_id === target.id || operation.secondary_target_id === target.id
    ) : false
  );

  /* Les trois fenêtres viennent du serveur ; la fenêtre choisie gouverne à la
   * fois les chiffres de la Cible et la part de chaque Source. */
  let period = $state<MeasureWindow>('24h');
  const detail = $derived(target ? (session.measureDetails[target.id] ?? null) : null);
  const measure = $derived(inWindow(detail, period));
  const lastObservedAt = $derived(target ? lastObserved(target, detail) : null);

  /* La lecture dépend de l'adresse, pas de la projection : recharger les Cibles
   * ne doit pas redemander les mêmes mesures. Le battement de quinze secondes
   * s'en charge ensuite. */
  $effect(() => {
    const targetId = page.params.id;
    if (targetId) {
      void session.loadMeasureDetail(targetId);
      void session.loadIncidentHistory(targetId);
      void api<{ activity: TargetReconciliationActivity[] }>(`/api/v1/targets/${targetId}/reconciliation-activity`)
        .then((response) => (targetActivity = response.activity))
        .catch(() => (targetActivity = []));
    }
  });

  $effect(() => {
    const targetId = page.params.id;
    if (session.gate !== 'app' || target || !targetId || resolutionCheckedFor === targetId) return;
    resolutionCheckedFor = targetId;
    void api<{ target_id: string }>(`/api/v1/targets/${targetId}/resolution`)
      .then((resolved) => {
        if (resolved.target_id !== targetId) void goto(`/cibles/${resolved.target_id}`, { replaceState: true });
      })
      .catch(() => undefined);
  });

  function sourceMeasure(sourceId: string) {
    return inWindow(detail?.sources.find((source) => source.source_id === sourceId), period);
  }

  /* Les Sources apportées par une Intégration ne sont pas des Contrôles natifs
   * et n'apparaissent donc pas dans la Cible ; la mesure, elle, les connaît. */
  const integrationSources = $derived(
    (detail?.sources ?? []).filter((source) => source.origin === 'integration')
  );

  /* ── Réglages de la Cible ──────────────────────────────────────────────
   * Le brouillon suit la Cible tant que l'utilisateur n'y a pas touché : une
   * projection rechargée en arrière-plan ne doit pas écraser une saisie en
   * cours, ni laisser un champ périmé après un renommage. */
  const admin = $derived(session.user?.role === 'administrator');
  let draftName = $state('');
  let draftDescription = $state('');
  let editedTarget = $state('');
  let saving = $state(false);
  let archiveOpen = $state(false);

  $effect(() => {
    if (target && editedTarget !== target.id) {
      editedTarget = target.id;
      draftName = target.name;
      draftDescription = target.description;
    }
  });

  function resetDraft() {
    if (!target) return;
    draftName = target.name;
    draftDescription = target.description;
  }

  async function saveTarget(event: SubmitEvent) {
    event.preventDefault();
    if (!target || draftName.trim().length === 0) return;
    saving = true;
    await session.renameTarget(target.id, draftName.trim(), draftDescription.trim());
    saving = false;
  }

  async function confirmArchive() {
    if (!target) return;
    saving = true;
    const done = await session.archiveTarget(target.id);
    saving = false;
    archiveOpen = false;
    if (done) await goto('/cibles');
  }

  async function toggleSource(source: { id: string; enabled: boolean }) {
    saving = true;
    await session.updateSource(source.id, { enabled: !source.enabled });
    saving = false;
  }

  async function removeSource(sourceId: string) {
    saving = true;
    await session.deleteSource(sourceId);
    saving = false;
    removalFor = null;
  }

  let removalFor = $state<{ id: string; name: string } | null>(null);

  const outcomeLabels = $derived<Record<string, string>>({
    healthy: t('state.ok'),
    unhealthy: t('target.failing'),
    unknown: t('state.unknown')
  });

  const kindLabels: Record<string, string> = {
    http: 'HTTP',
    tcp: 'TCP',
    dns: 'DNS',
    icmp: 'ICMP',
    heartbeat: 'Heartbeat',
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    argus: 'Argus',
    generic_webhook: 'Webhook'
  };

  const originLabels: Record<IncidentSignal['origin'], string> = {
    native: 'CairnOps',
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    argus: 'Argus',
    webhook: 'Webhook'
  };

  /* Les preuves de tous les Incidents de la Cible, à plat : c'est la lecture
   * « par Source » que demande l'écran, pas la lecture par Incident. */
  const proofs = $derived(
    incidents.flatMap((incident) =>
      incident.signals.map((signal) => ({ incident, signal }))
    )
  );

  const failingCount = $derived(
    proofs.filter((proof) => proof.signal.active && !proof.signal.invalidated_at).length
  );

  const liveCount = $derived(proofs.filter((proof) => !proof.signal.invalidated_at).length);

  const divergent = $derived(incidents.some(diverges));

  const journal = $derived(
    target ? incidentTimelineForTarget(incidents, incidentHistory, target.id) : []
  );

  /* La Vue tient sur les 24 heures : c'est la fenêtre qui décrit l'instant.
   * Les trois fenêtres se lisent côte à côte dans la colonne de droite, et la
   * fenêtre choisie ne gouverne plus que la part de chaque Source. */
  const day = $derived(inWindow(detail, '24h'));

  const windows = $derived(
    (['24h', '7d', '30d'] as const).map((value) => ({ value, measure: inWindow(detail, value) }))
  );

  /* Les Sources qui, seules, continuent de conclure à la disponibilité : c'est
   * le désaccord lui-même, nommé, plutôt qu'un simple avertissement. */
  const dissenting = $derived(
    proofs
      .filter((proof) => !proof.signal.invalidated_at && !proof.signal.active)
      .map((proof) => proof.signal.connector_name ?? proof.signal.name)
  );

  /* La fenêtre de maintenance qui couvre cette Cible, en cours ou annoncée. */
  const window_ = $derived(
    session.visibleMaintenances.find((maintenance) =>
      maintenance.targets.some((covered) => covered.id === target?.id)
    ) ?? null
  );

  /* Le Journal résume ; les Observations montrent chaque relevé tel qu'il a
   * été écrit. On ne les charge qu'à la demande. */
  let observationsOpen = $state(false);
  const observations = $derived(target ? (session.observations[target.id] ?? []) : []);

  async function showObservations() {
    if (!target) return;
    observationsOpen = true;
    await session.loadObservations(target.id);
  }

  function httpURL(value: unknown): string {
    if (typeof value !== 'string') return '';
    try {
      const parsed = new URL(value);
      return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : '';
    } catch {
      return '';
    }
  }

  function argusPosture(details: Record<string, unknown>) {
    const instanceURL = httpURL(details.argus_url);
    if (typeof details.service_id !== 'string' || !instanceURL) return null;
    return {
      deployed: typeof details.deployed_version === 'string' ? details.deployed_version : '',
      latest: typeof details.latest_version === 'string' ? details.latest_version : '',
      approved: details.approved === true,
      skipped: details.skipped === true,
      lastChecked: typeof details.last_checked === 'string' ? details.last_checked : '',
      instanceURL,
      versionURL: httpURL(details.version_url)
    };
  }

  const checkKinds = $derived(
    [...new Set((target?.sources ?? []).map((source) => kindLabels[source.kind] ?? source.kind))].join(', ')
  );

  async function acknowledge() {
    if (!lead) return;
    acknowledging = true;
    try {
      await session.acknowledge(lead);
    } finally {
      acknowledging = false;
    }
  }

  async function confirmInvalidation() {
    if (!invalidationFor || invalidationReason.trim().length < 8) return;
    invalidating = invalidationFor.signal.id;
    const done = await session.invalidate(
      invalidationFor.incidentId,
      invalidationFor.signal.id,
      invalidationReason.trim()
    );
    invalidating = '';
    if (done) {
      invalidationFor = null;
      invalidationReason = '';
    }
  }
</script>

<svelte:head><title>{target?.name ?? 'Cible'} — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.targets'), href: '/cibles' }, { label: target?.name ?? '…' }]} />

{#if !target}
  <div class="page">
    <div class="card">
      <div class="empty">
        <strong>{t('target.notFound')}</strong>
        {t('target.notFoundHint')}
        <p class="back"><a href="/cibles">{t('target.backToTargets')}</a></p>
      </div>
    </div>
  </div>
{:else}
  <div class="page">
    <div class="page-head">
      <div>
        <h1>
          {target.name}
          <span class="pill {stateTones[healthState]}">{stateLabel(healthState)}</span>
          {#if divergent}<span class="pill warn" title={t('targets.divergenceHint')}>{t('targets.divergence')}</span>{/if}
        </h1>
        <p>
          {#if target.description}<span class="mono">{target.description}</span> · {/if}
          {plural('palette.sources', target.sources.length + target.external_source_count)}
          {#if checkKinds}· {t('target.checks', { kinds: checkKinds })}{/if}
        </p>
      </div>
      <div class="page-actions">
        <button class="btn" type="button" onclick={() => (maintenanceOpen = true)}>
          {t('target.putUnderMaintenance')}
        </button>
        {#if admin}
          <button class="btn" type="button" disabled={structureBusy} onclick={() => (reconciliationOpen = true)}>{t('target.reconcile')}</button>
          <button class="btn" type="button" onclick={() => (tab = 'settings')}>{t('target.edit')}</button>
        {/if}
        {#if lead && !lead.acknowledged_at}
          <button class="btn primary" type="button" disabled={acknowledging} onclick={acknowledge}>
            {acknowledging ? t('incident.acknowledging') : t('incident.acknowledge')}
          </button>
        {/if}
      </div>
    </div>

    <div class="tabs" role="tablist">
      {#each [
        ['view', t('target.tab.view')],
        ['sources', t('targets.column.sources')],
        ['checks', t('target.tab.checks')],
        ['log', t('target.tab.log')],
        ['settings', t('nav.settings')]
      ] as [value, label] (value)}
        <button
          role="tab"
          type="button"
          aria-selected={tab === value}
          onclick={() => (tab = value as Tab)}
        >{label}</button>
      {/each}
    </div>

    {#if tab === 'view'}
      {#if lead}
        <div class="banner {severityTone(lead.effective_severity)}">
          <i class="dot {severityTone(lead.effective_severity)}"></i>
          <div class="banner-copy">
            <strong>
              {t('target.ongoingIncident')} · {natureLabel(lead)} ·
              {severityLabel(lead.effective_severity)} ·
              {lead.acknowledged_at
                ? t('target.acknowledgedBy', {
                    who: lead.acknowledged_by ?? t('target.anOperator')
                  })
                : t('incident.unacknowledged')}
            </strong>
            <p>
              {plural('target.failingSources', failingCount, {
                live: liveCount,
                duration: since(lead.opened_at, now)
              })}
              {#if divergent}
                {plural('target.dissenting', dissenting.length, { sources: dissenting.join(', ') })}
              {/if}
            </p>
          </div>
          <a class="btn" href={incidentHref(lead.id)}>{t('target.openIncident')}</a>
        </div>
      {/if}

      <!-- Les cinq chiffres de l'instant : la Cible sur 24 heures. Les trois
           fenêtres et la latence se lisent dans la colonne de droite. -->
      <div class="kpis">
        <div class="card kpi">
          <span>{t('incidents.column.duration')}</span>
          <b class={lead ? severityTone(lead.effective_severity) : ''}>
            <Odometer value={lead ? since(lead.opened_at, now) : t('common.none')} />
          </b>
        </div>
        <div class="card kpi">
          <span>{t('target.failingSourcesLabel')}</span>
          <b class={failingCount > 0 ? 'crit' : ''}><Odometer value={`${failingCount}/${liveCount || 0}`} /></b>
        </div>
        <div class="card kpi">
          <span>{t('target.availability24h')}</span>
          <b class:dim={day.availability === null}><Odometer value={ratio(day.availability)} /></b>
        </div>
        <div class="card kpi">
          <span>{t('overview.fig.coverage')}</span>
          <b class:dim={day.coverage === null}><Odometer value={ratio(day.coverage)} /></b>
        </div>
        <div class="card kpi">
          <span>{t('target.lastObservation')}</span>
          <b><Odometer value={lastObservedAt ? since(lastObservedAt, now) : t('common.none')} /></b>
        </div>
      </div>

      <TargetIndicators targetId={target.id} incident={lead} />

      <div class="split">
        <section>
          <div class="card cols-proof">
            <header>
              <h2>{t('target.proofs')}</h2>
              {#if divergent}<span class="pill warn">{t('targets.divergence')}</span>{/if}
              <span class="note">{t('target.proofsNote')}</span>
            </header>

            <div class="thead">
              <span>{t('target.column.source')}</span>
              <span>{t('target.column.verdict')}</span>
              <span class="hide-sm">{t('target.column.freshness')}</span>
              <span class="hide-sm">{t('target.column.origin')}</span>
              <span></span>
            </div>

            {#each proofs as proof (proof.signal.id)}
              {@const dead = Boolean(proof.signal.invalidated_at)}
              <div class="trow" class:invalidated={dead}>
                <span class="cell-name">
                  <i class="dot {dead ? 'idle' : proof.signal.active ? severityTone(proof.signal.severity) : 'ok'}"></i>
                  <span>
                    <strong>{proof.signal.name}</strong>
                    <small class="nature">{natureLabel(proof.incident)}</small>
                  </span>
                </span>

                <span class="pill {dead ? '' : proof.signal.active ? severityTone(proof.signal.severity) : 'ok'}">
                  {dead
                    ? t('target.verdict.invalidated')
                    : proof.signal.active
                      ? t('target.failing')
                      : t('target.verdict.recovered')}
                </span>

                <span class="num hide-sm"><Odometer value={since(proof.signal.opened_at, now)} /></span>

                <span class="muted hide-sm">
                  {proof.signal.connector_name ?? originLabels[proof.signal.origin]}
                </span>

                {#if !dead && session.user?.role !== 'observer'}
                  <button
                    class="btn sm proof-action"
                    type="button"
                    title={t('target.invalidateHint')}
                    disabled={invalidating === proof.signal.id}
                    onclick={() => {
                      invalidationFor = { incidentId: proof.incident.id, signal: proof.signal };
                      invalidationReason = '';
                    }}
                  >{t('target.invalidate')}</button>
                {:else if dead}
                  <span class="faint reason" title={proof.signal.invalidation_reason}>
                    {proof.signal.invalidation_reason ?? t('target.noReason')}
                  </span>
                {:else}
                  <span></span>
                {/if}
              </div>
            {:else}
              <div class="empty">
                <strong>{t('target.noProof')}</strong>
                {t('target.noProofHint')}
              </div>
            {/each}
          </div>

          <div class="card journal">
            <header>
              <h2>{observationsOpen ? t('target.observations') : t('target.activityLog')}</h2>
              {#if observationsOpen}
                <button class="btn sm" type="button" onclick={() => (observationsOpen = false)}>
                  {t('target.backToLog')}
                </button>
              {:else}
                <button class="btn sm" type="button" onclick={showObservations}>
                  {t('target.showObservations')}
                </button>
              {/if}
            </header>
            <div class="card-body log">
              {#if observationsOpen}
                {#each observations as observation (observation.id)}
                  {@const argus = argusPosture(observation.details)}
                  <div class="entry">
                    <span class="when num">{clock(observation.observed_at)}</span>
                    <span class="what">
                      <strong>{observation.source_name} · {outcomeLabels[observation.outcome]}</strong>
                      <small class="faint">
                        {observation.latency_milliseconds} ms
                        {#if observation.message}· {observation.message}{:else if observation.reason}· {observation.reason}{/if}
                      </small>
                      {#if argus}
                        <small class="faint posture">
                          {t('argus.observationVersions', { deployed: argus.deployed || '—', latest: argus.latest || '—' })}
                          {#if argus.approved}· {t('argus.observationApproved')}{/if}
                          {#if argus.skipped}· {t('argus.observationSkipped')}{/if}
                          {#if argus.lastChecked}· {t('argus.lastChecked', { time: clock(argus.lastChecked) })}{/if}
                        </small>
                        <span class="observation-links">
                          <a href={argus.instanceURL} target="_blank" rel="noreferrer">{t('argus.openInstance')} <span aria-hidden="true">↗</span></a>
                          {#if argus.versionURL}<a href={argus.versionURL} target="_blank" rel="noreferrer">{t('argus.viewVersion')} <span aria-hidden="true">↗</span></a>{/if}
                        </span>
                      {/if}
                    </span>
                    <i class="dot {observation.outcome === 'healthy' ? 'ok' : observation.outcome === 'unhealthy' ? 'crit' : 'idle'}"></i>
                  </div>
                {:else}
                  <p class="faint">{t('target.noObservations')}</p>
                {/each}
              {:else}
                {#each journal.slice(0, 6) as item (item.entry.id)}
                  <div class="entry">
                    <span class="when num">{clock(item.entry.occurred_at)}</span>
                    <span class="what">
                      <strong>{item.entry.message}</strong>
                      <small class="faint">
                        {natureLabel(item.incident)} · {t('target.origin', { origin: item.entry.origin })}
                        {#if item.entry.actor_name}· {item.entry.actor_name}{/if}
                      </small>
                    </span>
                  </div>
                {:else}
                  <p class="faint">{t('target.noEntries')}</p>
                {/each}
              {/if}
            </div>
          </div>
        </section>

        <aside>
          <div class="card">
            <header><h2>{t('target.availability')}</h2></header>
            <div class="card-body">
              {#each windows as entry (entry.value)}
                <div class="window">
                  <span class="faint">
                    {t(`target.window.${entry.value}`)}
                  </span>
                  <b class="num" class:dim={entry.measure.availability === null}>
                    <Odometer value={ratio(entry.measure.availability)} />
                  </b>
                </div>
                {#if entry.value === '24h'}
                  <div class="strip"><Uptime values={detail?.trend ?? []} /></div>
                {/if}
              {/each}

              <div class="latency {stateTones[healthState]}">
                <Spark values={detail?.latency_trend ?? []} width={240} height={30} />
                <small class="faint">
                  {t('target.latencySummary', {
                    average: latency(day.average_latency_milliseconds),
                    maximum: latency(day.maximum_latency_milliseconds)
                  })}
                  · {plural('target.conclusiveOf', day.conclusive_observations, {
                    expected: day.expected_observations
                  })}
                </small>
              </div>
            </div>
          </div>

          <div class="card">
            <header><h2>{t('target.nativeChecks')}</h2></header>
            <div class="card-body checks">
              {#each target.sources as source (source.id)}
                <div class="check" class:suspended={!source.enabled}>
                  <span class="key">{kindLabels[source.kind] ?? source.kind}</span>
                  <span class="check-id">
                    <strong>{source.name}</strong>
                    <small class="faint num">
                      {t('duration.seconds', { count: source.interval_seconds })} ·
                      {t('target.thresholds', {
                        failure: source.failure_threshold,
                        recovery: source.recovery_threshold
                      })}
                    </small>
                  </span>
                  <span class={source.enabled
                    ? source.latest_outcome === 'healthy'
                      ? 'ok'
                      : source.latest_outcome === 'unhealthy'
                        ? 'crit'
                        : 'dim'
                    : 'dim'}>
                    {source.enabled ? outcomeLabels[source.latest_outcome ?? 'unknown'] : t('target.suspended')}
                  </span>
                </div>
              {:else}
                <p class="faint">{t('target.noNativeChecksHint')}</p>
              {/each}
            </div>
          </div>

          <div class="card">
            <header><h2>{t('nav.maintenance')}</h2></header>
            <div class="card-body">
              {#if window_}
                <p class="explain">
                  <strong>{window_.name}</strong> —
                  {window_.state === 'active'
                    ? t('target.windowUntil', { end: stamp(window_.ends_at) })
                    : t('target.windowFrom', { start: stamp(window_.starts_at) })}.
                  {t('target.windowSay')}
                </p>
              {:else}
                <p class="explain">{t('target.noWindow')}</p>
                <button class="btn window-action" type="button" onclick={() => (maintenanceOpen = true)}>
                  {t('maintenance.plan')}
                </button>
              {/if}
            </div>
          </div>
        </aside>
      </div>
    {:else if tab === 'sources'}
      <!-- La fenêtre choisie ne gouverne que cette lecture par Source : c'est
           ici qu'on compare une sonde à l'autre sur la même durée. -->
      <div class="measure-head">
        <div class="segments" role="group" aria-label={t('target.measureWindow')}>
          {#each ['24h', '7d', '30d'] as const as value (value)}
            <button type="button" aria-pressed={period === value} onclick={() => (period = value)}>
              {windowLabel(value)}
            </button>
          {/each}
        </div>
        <span class="note">
          {plural('target.conclusiveOf', measure.conclusive_observations, {
            expected: measure.expected_observations
          })}
        </span>
      </div>

      <!-- Chaque Source porte sa propre mesure sur la fenêtre choisie : c'est
           ainsi qu'une sonde aveugle se distingue d'une Cible en panne. -->
      <div class="card cols-source">
        <div class="thead">
          <span>{t('target.column.source')}</span>
          <span>{t('target.column.nature')}</span>
          <span class="hide-sm">{t('target.column.lastVerdict')}</span>
          <span class="hide-sm">{t('target.column.availability', { window: windowLabel(period) })}</span>
          <span class="hide-sm">{t('target.column.coverage', { window: windowLabel(period) })}</span>
          <span class="hide-sm">{t('targets.column.averageLatency')}</span>
          <span class="hide-sm">{t('target.lastObservation')}</span>
        </div>
        {#each target.sources as source (source.id)}
          {@const measured = sourceMeasure(source.id)}
          <div class="trow">
            <span class="cell-name source-cell">
              <i class="dot {source.latest_outcome === 'healthy' ? 'ok' : source.latest_outcome === 'unhealthy' ? 'crit' : 'idle'}"></i>
              <span><strong>{source.name}</strong></span>
              {#if admin}<button class="btn sm source-move" type="button" disabled={structureBusy} onclick={() => (sourceForMove = { id: source.id, target_id: target.id, name: source.name, kind: source.kind, origin: 'native' })}>{t('target.attachSource')}</button>{/if}
            </span>
            <span class="pill">{kindLabels[source.kind] ?? source.kind}</span>
            <span class="hide-sm">{outcomeLabels[source.latest_outcome ?? 'unknown']}</span>
            <span class="num hide-sm" class:dim={measured.availability === null}><Odometer value={ratio(measured.availability)} /></span>
            <span class="num hide-sm" class:dim={measured.coverage === null}><Odometer value={ratio(measured.coverage)} /></span>
            <span class="num hide-sm" class:dim={measured.average_latency_milliseconds === null}>
              <Odometer value={latency(measured.average_latency_milliseconds)} />
            </span>
            <span class="num hide-sm">
              <Odometer value={source.last_observed_at
                ? t('overview.ago', { duration: since(source.last_observed_at, now) })
                : t('common.none')} />
            </span>
          </div>
        {/each}
        {#each integrationSources as source (source.source_id)}
          {@const measured = inWindow(source, period)}
          <div class="trow">
            <span class="cell-name source-cell">
              <i class="dot {source.latest_outcome === 'healthy' ? 'ok' : source.latest_outcome === 'unhealthy' ? 'crit' : 'idle'}"></i>
              <span>
                <strong>{source.name}</strong>
                <small class="nature">{t('target.fromConnector')}</small>
              </span>
              {#if admin}<button class="btn sm source-move" type="button" disabled={structureBusy} onclick={() => (sourceForMove = { id: source.source_id, target_id: target.id, name: source.name, kind: source.kind, origin: 'integration' })}>{t('target.attachSource')}</button>{/if}
            </span>
            <span class="pill info">{kindLabels[source.kind] ?? source.kind}</span>
            <span class="hide-sm">{outcomeLabels[source.latest_outcome ?? 'unknown']}</span>
            <span class="num hide-sm" class:dim={measured.availability === null}><Odometer value={ratio(measured.availability)} /></span>
            <span class="num hide-sm" class:dim={measured.coverage === null}><Odometer value={ratio(measured.coverage)} /></span>
            <span class="num hide-sm" class:dim={measured.average_latency_milliseconds === null}>
              <Odometer value={latency(measured.average_latency_milliseconds)} />
            </span>
            <span class="num hide-sm">
              <Odometer value={source.latest_observed_at
                ? t('overview.ago', { duration: since(source.latest_observed_at, now) })
                : t('common.none')} />
            </span>
          </div>
        {/each}
        {#if target.sources.length === 0 && integrationSources.length === 0}
          <div class="empty">
            <strong>{t('target.noSource')}</strong>
            {t('target.noSourceHint')}
          </div>
        {/if}
      </div>
    {:else if tab === 'checks'}
      {#if admin}
        <div class="section-actions"><button class="btn primary" type="button" disabled={structureBusy} onclick={() => (controlOpen = true)}>{t('target.addCheck')}</button></div>
      {/if}
      <div class="card cols-check">
        <div class="thead">
          <span>{t('target.column.check')}</span>
          <span>{t('incidents.column.severity')}</span>
          <span class="hide-sm">{t('target.column.interval')}</span>
          <span class="hide-sm">{t('target.column.timeout')}</span>
          <span class="hide-sm">{t('target.column.thresholds')}</span>
          <span class="hide-sm">{t('targets.column.state')}</span>
          <span></span>
        </div>
        {#each target.sources as source (source.id)}
          <div class="trow" class:suspended={!source.enabled}>
            <span class="cell-name">
              <span><strong>{source.name}</strong><small class="nature">{kindLabels[source.kind] ?? source.kind}</small></span>
            </span>
            <span class="pill {severityTone(source.severity)}">{severityLabel(source.severity)}</span>
            <span class="num hide-sm">{t('duration.seconds', { count: source.interval_seconds })}</span>
            <span class="num hide-sm">{source.timeout_milliseconds} ms</span>
            <span class="num hide-sm">{source.failure_threshold} / {source.recovery_threshold}</span>
            <span class="hide-sm {source.enabled ? 'ok' : 'dim'}">
              {source.enabled ? t('target.enabled') : t('target.suspended')}
            </span>
            {#if admin}
              <!-- Suspendre arrête la sonde sans rien perdre ; retirer emporte
                   ses Observations, d'où la confirmation. -->
              <span class="row-actions">
                <button class="btn sm" type="button" disabled={saving || structureBusy} onclick={() => (sourceForMove = { id: source.id, target_id: target.id, name: source.name, kind: source.kind, origin: 'native' })}>{t('target.attachSource')}</button>
                <button class="btn sm" type="button" disabled={saving || structureBusy} onclick={() => toggleSource(source)}>
                  {source.enabled ? t('target.suspend') : t('target.resume')}
                </button>
                <button
                  class="btn sm"
                  type="button"
                  disabled={saving || structureBusy}
                  onclick={() => (removalFor = { id: source.id, name: source.name })}
                >{t('target.remove')}</button>
              </span>
            {:else}
              <span></span>
            {/if}
          </div>
        {:else}
          <div class="empty">
            <strong>{t('target.noNativeChecks')}</strong>
            {t('target.noNativeChecksHint')}
          </div>
        {/each}
      </div>
    {:else if tab === 'log'}
      <div class="card">
        <div class="card-body log">
          {#each targetActivity as entry (`target-${entry.id}`)}
            <div class="entry">
              <span class="when num">{stamp(entry.occurred_at)}</span>
              <span class="what">
                <strong>{entry.message}</strong>
                <small class="faint">{t('target.identity')}{#if entry.actor_name} · {entry.actor_name}{/if}</small>
              </span>
            </div>
          {/each}
          {#each journal as item (item.entry.id)}
            <div class="entry">
              <span class="when num">{stamp(item.entry.occurred_at)}</span>
              <span class="what">
                <strong>{item.entry.message}</strong>
                <small class="faint">
                  {natureLabel(item.incident)} · {t('target.origin', { origin: item.entry.origin })}
                  {#if item.entry.actor_name}· {item.entry.actor_name}{/if}
                </small>
              </span>
            </div>
          {/each}
          {#if targetActivity.length === 0 && journal.length === 0}
            <p class="faint">{t('target.noEntries')}</p>
          {/if}
        </div>
      </div>
    {:else}
      <div class="card">
        <div class="card-body">
          <div class="figures">
            <div class="fig"><b class="mono id">{target.id}</b><span>{t('target.identifier')}</span></div>
            <div class="fig"><b>{stamp(target.created_at)}</b><span>{t('target.createdOn')}</span></div>
          </div>

          {#if admin}
            <form class="settings" onsubmit={saveTarget}>
              <div class="field">
                <label for="target-name">{t('target.name')}</label>
                <input id="target-name" bind:value={draftName} maxlength="160" required />
                <small>{t('target.renameHint')}</small>
              </div>
              <div class="field">
                <label for="target-description">{t('target.description')}</label>
                <input id="target-description" bind:value={draftDescription} maxlength="2000" />
              </div>
              <div class="settings-actions">
                <button class="btn primary" type="submit" disabled={saving || structureBusy || draftName.trim().length === 0}>
                  {saving ? t('common.saving') : t('common.save')}
                </button>
                <button class="btn" type="button" disabled={saving} onclick={resetDraft}>
                  {t('common.cancel')}
                </button>
              </div>
            </form>

            <div class="danger identity-management">
              <div>
                <strong>{t('target.identity')}</strong>
                <p>{t('target.identityHint')}</p>
              </div>
              <button class="btn" type="button" disabled={saving || structureBusy} onclick={() => (reconciliationOpen = true)}>
                {t('target.reconcileAnother')}
              </button>
            </div>

            <div class="danger">
              <div>
                <strong>{t('target.archiveTitle')}</strong>
                <p>{t('target.archiveSay')}</p>
              </div>
              <button class="btn" type="button" disabled={saving || structureBusy} onclick={() => (archiveOpen = true)}>
                {t('target.archive')}
              </button>
            </div>
          {:else}
            <p class="pending">{t('target.adminOnly')}</p>
          {/if}
        </div>
      </div>
    {/if}
  </div>
{/if}

{#if invalidationFor}
  <div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && (invalidationFor = null)}>
    <div class="modal narrow" role="dialog" aria-modal="true" aria-labelledby="invalidation-title">
      <header>
        <div>
          <h2 id="invalidation-title">{t('target.invalidateTitle')}</h2>
          <p>{t('target.invalidateSay')}</p>
        </div>
      </header>
      <div class="modal-body">
        <div class="field">
          <label for="reason">{t('target.reason')}</label>
          <textarea id="reason" bind:value={invalidationReason} rows="3" required minlength="8"
            placeholder={t('target.reasonPlaceholder')}></textarea>
          <small>{t('target.reasonHint')}</small>
        </div>
      </div>
      <footer>
        <button class="btn" type="button" onclick={() => (invalidationFor = null)}>{t('common.cancel')}</button>
        <button
          class="btn primary"
          type="button"
          disabled={invalidationReason.trim().length < 8 || invalidating !== ''}
          onclick={confirmInvalidation}
        >{invalidating ? t('common.saving') : t('target.invalidateConfirm')}</button>
      </footer>
    </div>
  </div>
{/if}

{#if archiveOpen && target}
  <div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && (archiveOpen = false)}>
    <div class="modal narrow" role="dialog" aria-modal="true" aria-labelledby="archive-title">
      <header>
        <div>
          <h2 id="archive-title">{t('target.archiveHeading', { name: target.name })}</h2>
          <p>{t('target.archiveLead')}</p>
        </div>
      </header>
      <div class="modal-body">
        <p class="explain">{t('target.archiveExplain')}</p>
      </div>
      <footer>
        <button class="btn" type="button" disabled={saving} onclick={() => (archiveOpen = false)}>
          {t('common.cancel')}
        </button>
        <button class="btn primary" type="button" disabled={saving || structureBusy} onclick={confirmArchive}>
          {saving ? t('target.archiving') : t('target.archiveConfirm')}
        </button>
      </footer>
    </div>
  </div>
{/if}

{#if removalFor}
  <div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && (removalFor = null)}>
    <div class="modal narrow" role="dialog" aria-modal="true" aria-labelledby="removal-title">
      <header>
        <div>
          <h2 id="removal-title">{t('target.removeHeading', { name: removalFor.name })}</h2>
          <p>{t('target.removeLead')}</p>
        </div>
      </header>
      <div class="modal-body">
        <p class="explain">{t('target.removeExplain')}</p>
      </div>
      <footer>
        <button class="btn" type="button" disabled={saving} onclick={() => (removalFor = null)}>
          {t('common.cancel')}
        </button>
        <button class="btn primary" type="button" disabled={saving || structureBusy} onclick={() => removalFor && removeSource(removalFor.id)}>
          {saving ? t('target.removing') : t('target.removeConfirm')}
        </button>
      </footer>
    </div>
  </div>
{/if}

{#if maintenanceOpen}
  <MaintenanceWorkshop
    targets={session.targets}
    onclose={() => (maintenanceOpen = false)}
    onsuccess={async () => {
      await Promise.all([session.loadMaintenances(), session.loadIncidents()]);
      session.showNotice(t('target.windowSaved'));
    }}
  />
{/if}

{#if controlOpen && target}
  <TargetWorkshop
    target={target}
    onclose={() => (controlOpen = false)}
    onsuccess={async () => {
      await session.loadTargets();
      session.showNotice(t('target.checkAdded'));
    }}
  />
{/if}

{#if reconciliationOpen && target}
  <ReconciliationWorkshop primaryTargetId={target.id} onclose={() => (reconciliationOpen = false)} />
{/if}

{#if sourceForMove}
  <ReconciliationWorkshop source={sourceForMove} secondaryTargetId={sourceForMove.target_id} onclose={() => (sourceForMove = null)} />
{/if}

<style>
  .page-head h1 {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    flex-wrap: wrap;
  }

  .tabs {
    display: flex;
    gap: var(--s5);
    margin-bottom: var(--s5);
    border-bottom: 1px solid var(--line);
  }

  .section-actions { display: flex; justify-content: flex-end; margin-bottom: var(--s4); }
  .source-cell { min-width: 0; }
  .source-move { margin-left: auto; opacity: 0; transition: opacity var(--d1) var(--ease); }
  .trow:hover .source-move, .trow:focus-within .source-move { opacity: 1; }

  .tabs button {
    padding: 0 0 0.625rem;
    border: 0;
    border-bottom: 2px solid transparent;
    background: none;
    color: var(--faint);
    font-size: 0.8125rem;
    font-weight: 500;
    transition: color var(--d1) var(--ease), border-color var(--d1) var(--ease);
  }

  .tabs button:hover {
    color: var(--ink);
  }

  .tabs button[aria-selected='true'] {
    border-bottom-color: var(--accent);
    color: var(--ink);
  }

  .banner {
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

  .banner .btn {
    flex: none;
  }

  /* ── Les cinq chiffres de l'instant ─────────────────────────────────────
     Chacun sur sa dalle : ce sont cinq lectures indépendantes, et une durée
     d'Incident ne se compare pas à une Disponibilité. */
  .kpis {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: var(--s4);
    margin-bottom: var(--s5);
  }

  .kpi {
    padding: var(--s4) 1rem;
  }

  .kpi span {
    display: block;
    color: var(--faint);
    font-size: var(--text-xs);
  }

  .kpi b {
    display: block;
    margin-top: 0.3125rem;
    font-family: var(--font-num);
    font-variant-numeric: tabular-nums;
    font-size: 1.25rem;
    font-weight: 600;
    line-height: 1.1;
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1.9fr) minmax(0, 1fr);
    gap: var(--s5);
    align-items: start;
  }

  .split > section,
  .split > aside {
    display: grid;
    gap: var(--s5);
    min-width: 0;
  }

  .window {
    display: flex;
    align-items: baseline;
    gap: var(--s4);
    font-size: 0.75rem;
  }

  .window + .window,
  .strip + .window {
    margin-top: var(--s4);
  }

  .window b {
    margin-left: auto;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .strip {
    margin-top: 0.5rem;
  }

  .latency {
    margin-top: var(--s4);
    padding-top: var(--s4);
    border-top: 1px solid var(--line);
    color: var(--ok);
  }

  .latency.crit { color: var(--crit) }
  .latency.warn { color: var(--warn) }
  .latency.info { color: var(--info) }
  .latency.idle { color: var(--dim) }

  .latency small {
    display: block;
    margin-top: 0.375rem;
    line-height: 1.45;
  }

  .checks {
    display: grid;
    gap: 0;
  }

  .check {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    padding: 0.5625rem 0;
    border-bottom: 1px solid var(--line-row);
    font-size: 0.75rem;
  }

  .check:first-child {
    padding-top: 0;
  }

  .check:last-child {
    padding-bottom: 0;
    border-bottom: 0;
  }

  .check > span:last-child {
    margin-left: auto;
    white-space: nowrap;
  }

  .key {
    padding: 0 0.375rem;
    height: 1.25rem;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--r-s);
    background: var(--surface-2);
    color: var(--muted);
    font-size: 0.625rem;
    font-weight: 600;
  }

  .check-id {
    min-width: 0;
  }

  .check-id strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .check-id small {
    display: block;
    font-size: 0.6875rem;
  }

  .window-action {
    margin-top: var(--s4);
  }

  .measure-head {
    display: flex;
    align-items: center;
    gap: var(--s4);
    margin-bottom: var(--s5);
  }

  .measure-head .note {
    margin-left: auto;
    color: var(--faint);
    font-size: 0.75rem;
  }

  /* L'en-tête et les lignes sont des grilles indépendantes : une largeur
     stable pour l'action empêche le bouton de décaler les autres colonnes. */
  .cols-proof  { --cols: minmax(0, 1.4fr) 7.5rem 5.625rem 8.125rem 5.5rem }
  .cols-source { --cols: minmax(0, 1.4fr) 6rem 7.5rem 5.25rem 5.25rem 5.75rem 8.125rem }
  .cols-check  { --cols: minmax(0, 1.4fr) 7.5rem 5.25rem 5.25rem 8.75rem 5.25rem auto }

  .proof-action {
    justify-self: end;
  }

  .row-actions {
    display: flex;
    gap: 0.375rem;
  }

  .suspended {
    opacity: 0.6;
  }

  .settings {
    display: grid;
    gap: var(--s4);
    max-width: 34rem;
    margin-top: var(--s5);
    padding-top: var(--s5);
    border-top: 1px solid var(--line);
  }

  .settings-actions {
    display: flex;
    gap: 0.5rem;
  }

  .danger {
    display: flex;
    align-items: flex-start;
    gap: var(--s5);
    margin-top: var(--s6);
    padding-top: var(--s5);
    border-top: 1px solid var(--line);
  }

  .danger strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .danger p {
    margin-top: 0.25rem;
    max-width: 40rem;
    color: var(--muted);
    font-size: 0.75rem;
  }

  .danger button {
    margin-left: auto;
    flex: none;
  }

  .explain {
    color: var(--muted);
    font-size: 0.8125rem;
    line-height: 1.5;
  }

  .nature {
    font-family: var(--font);
    color: var(--faint);
    font-size: 0.6875rem;
  }

  .invalidated {
    opacity: 0.55;
  }

  .reason {
    font-size: 0.6875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .external {
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
  }

  .log {
    display: grid;
    gap: 0;
  }

  .entry {
    display: flex;
    align-items: baseline;
    gap: var(--s4);
    padding: 0.5625rem 0;
    border-bottom: 1px solid var(--line-row);
  }

  .entry:last-child {
    border-bottom: 0;
  }

  .when {
    flex: none;
    width: 8.125rem;
    color: var(--faint);
  }

  .what {
    min-width: 0;
  }

  .posture {
    margin-top: var(--s1);
  }

  .observation-links {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s3);
    margin-top: var(--s1);
    font-size: var(--text-xs);
  }

  .observation-links a {
    color: var(--accent);
    text-decoration: none;
  }

  .observation-links a:hover {
    text-decoration: underline;
  }

  /* Dans la lecture par Observation, le verdict se lit en bout de ligne. */
  .entry .dot {
    margin-left: auto;
  }

  .what strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .what small {
    display: block;
    font-size: 0.6875rem;
  }

  .id {
    font-size: 0.75rem;
  }

  .pending {
    margin-top: var(--s5);
    padding-top: var(--s4);
    border-top: 1px solid var(--line);
    color: var(--faint);
    font-size: 0.75rem;
  }

  .narrow {
    max-width: 32rem;
  }

  .back {
    margin-top: var(--s4);
  }

  .back a {
    color: var(--accent);
  }

  /* L'heure suffit dans la colonne étroite : la date est celle du jour. */
  .journal .when {
    width: 3.75rem;
  }

  .journal header .btn {
    margin-left: auto;
  }

  @media (max-width: 80rem) {
    .kpis {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 68rem) {
    .split {
      grid-template-columns: minmax(0, 1fr);
    }

    .banner {
      flex-wrap: wrap;
    }
  }

  @media (max-width: 48rem) {
    .tabs {
      overflow-x: auto;
      overscroll-behavior-inline: contain;
      scrollbar-width: none;
    }

    .tabs::-webkit-scrollbar {
      display: none;
    }

    .tabs button {
      flex: none;
    }

    .banner {
      display: grid;
      grid-template-columns: auto minmax(0, 1fr);
      align-items: start;
    }

    .banner .btn {
      grid-column: 2;
      justify-self: start;
    }

    .kpis {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
