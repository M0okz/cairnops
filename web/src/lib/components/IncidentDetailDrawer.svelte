<script lang="ts">
  import { onMount } from 'svelte';
  import { prefersReducedMotion } from 'svelte/motion';
  import Icon from './Icon.svelte';
  import InfoHint from './InfoHint.svelte';
  import ActivityTimeline from './ActivityTimeline.svelte';
  import IndicatorHistoryChart from './IndicatorHistoryChart.svelte';
  import { APIError, api, type Incident, type IncidentEvidence, type IncidentIndicators } from '$lib/api';
  import {
    diverges,
    natureLabel,
    severityLabel,
    severityTone,
    since,
    stamp
  } from '$lib/format';
  import { formatIndicator } from '$lib/indicator-format';
  import {
    findingShape,
    incidentActivity,
    incidentIndicatorRows,
    measurable,
    primaryEvidence,
    splitIndicatorRows,
    type IncidentIndicatorRow
  } from '$lib/incident-detail';
  import { i18n, plural, t } from '$lib/i18n.svelte';
  import { messageFrom, session } from '$lib/session.svelte';

  let {
    incidentId,
    seed = null,
    ondismiss
  }: {
    incidentId: string;
    seed?: Incident | null;
    ondismiss: () => void;
  } = $props();

  const origins: Record<IncidentEvidence['origin'], string> = {
    native: 'CairnOps',
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    argus: 'Argus',
    proxmox: 'Proxmox VE',
    webhook: 'Webhook'
  };

  let dialog = $state<HTMLDialogElement | null>(null);
  let closeButton = $state<HTMLButtonElement | null>(null);
  let reasonField = $state<HTMLTextAreaElement | null>(null);
  let incident = $state<Incident | null>(null);
  let indicators = $state<IncidentIndicators | null>(null);
  let incidentLoading = $state(true);
  let indicatorsLoading = $state(true);
  let incidentError = $state('');
  let indicatorsError = $state('');
  let acknowledging = $state(false);
  let invalidating = $state(false);
  let invalidationFor = $state<IncidentEvidence | null>(null);
  let invalidationReason = $state('');
  let invalidationError = $state('');
  let invalidationTrigger = $state<HTMLButtonElement | null>(null);
  let projectedOnce = $state(false);
  let now = $state(new Date());
  let requestVersion = 0;
  let closing = $state(false);
  let dismissed = false;
  let dismissTimer: ReturnType<typeof setTimeout> | undefined;
  const metricTimeBounds = $derived.by((): [number, number] | null => {
    const opening = Date.parse(indicators?.opened_at ?? incident?.opened_at ?? '');
    return Number.isFinite(opening) ? [opening - 2 * 3_600_000, opening + 2 * 3_600_000] : null;
  });

  const titleID = $derived(`incident-detail-title-${incidentId}`);
  const descriptionID = $derived(`incident-detail-description-${incidentId}`);
  const activity = $derived(incident ? incidentActivity(incident) : []);
  const metricRows = $derived(
    indicators ? incidentIndicatorRows(indicators) : { captured: [], additional: [] }
  );
  const projected = $derived(session.incidents.find((item) => item.id === incidentId) ?? null);
  const evidence = $derived(incident?.impacts.flatMap((impact) => impact.evidence) ?? []);
  const maintainedImpacts = $derived(incident?.impacts.filter((impact) => impact.maintenance_active) ?? []);
  const primary = $derived(incident ? primaryEvidence(incident) : null);
  const primaryText = $derived(primary?.presentation?.[i18n.locale] ?? null);
  const shape = $derived(findingShape(primary?.fact));
  const indicatorSplit = $derived(splitIndicatorRows(metricRows, primary?.fact));
  const otherPreview = $derived.by(() => {
    const captured = indicatorSplit.others.filter((row) => row.snapshot);
    if (captured.length === 0) return String(indicatorSplit.others.length);
    return captured
      .slice(0, 3)
      .map((row) => `${row.label} ${formatIndicator(row.snapshot!.value, row.unit)}`)
      .join(' · ');
  });
  const sourceNames = $derived(
    [...new Set(evidence.map((signal) => signal.connector_name ?? origins[signal.origin]))].join(', ')
  );
  const hasNotes = $derived(Boolean(
    incident && (
      incident.impact_count > 1 ||
      incident.acknowledgement_sync_status === 'pending' ||
      incident.acknowledgement_sync_status === 'failed' ||
      maintainedImpacts[0]?.maintenance_ends_at
    )
  ));
  const firstImpact = $derived(incident?.impacts[0] ?? null);
  const incidentTitle = $derived(incident
    ? incident.affected_target_count > 1
      ? plural('incidents.targetsAffected', incident.affected_target_count)
      : (firstImpact?.target_name ?? t('nav.incidents'))
    : t('nav.incidents'));
  const marker = $derived(
    incident
      ? {
          at: incident.opened_at,
          label: t('incidents.detail.openingMarker'),
          tone: severityTone(incident.severity) as 'info' | 'warn' | 'crit'
        }
      : null
  );

  async function loadIncident(showLoading = true) {
    const version = ++requestVersion;
    if (showLoading && !incident) incidentLoading = true;
    incidentError = '';
    try {
      const loaded = await api<Incident>(`/api/v1/incidents/${encodeURIComponent(incidentId)}`);
      if (version === requestVersion) incident = loaded;
    } catch (cause) {
      if (version !== requestVersion) return;
      incidentError =
        cause instanceof APIError && cause.status === 404
          ? t('incidents.detail.notFound')
          : t('incidents.detail.loadFailed', { error: messageFrom(cause) });
    } finally {
      if (version === requestVersion) incidentLoading = false;
    }
  }

  async function loadIndicators() {
    indicatorsLoading = true;
    indicatorsError = '';
    try {
      indicators = await api<IncidentIndicators>(
        `/api/v1/incidents/${encodeURIComponent(incidentId)}/indicators`
      );
    } catch (cause) {
      indicatorsError = t('incidents.detail.metricsLoadFailed', { error: messageFrom(cause) });
    } finally {
      indicatorsLoading = false;
    }
  }

  async function acknowledge() {
    if (!incident || acknowledging) return;
    acknowledging = true;
    try {
      await session.acknowledge(incident);
      await loadIncident(false);
    } finally {
      acknowledging = false;
    }
  }

  function beginInvalidation(signal: IncidentEvidence, trigger: HTMLButtonElement) {
    invalidationFor = signal;
    invalidationReason = '';
    invalidationError = '';
    invalidationTrigger = trigger;
    requestAnimationFrame(() => reasonField?.focus());
  }

  function cancelInvalidation() {
    invalidationFor = null;
    invalidationReason = '';
    invalidationError = '';
    requestAnimationFrame(() => invalidationTrigger?.focus());
  }

  async function confirmInvalidation(event: SubmitEvent) {
    event.preventDefault();
    if (!incident || !invalidationFor || invalidating) return;
    const reason = invalidationReason.trim();
    if (reason.length < 8) {
      invalidationError = t('incidents.detail.reasonError');
      requestAnimationFrame(() => reasonField?.focus());
      return;
    }

    invalidationError = '';
    invalidating = true;
    const done = await session.invalidate(incident.id, invalidationFor.id, reason);
    invalidating = false;
    if (!done) return;
    invalidationFor = null;
    invalidationReason = '';
    await loadIncident(false);
  }

  function finishDismiss() {
    if (dismissed) return;
    dismissed = true;
    clearTimeout(dismissTimer);
    ondismiss();
  }

  function requestDismiss() {
    if (invalidationFor) {
      cancelInvalidation();
      return;
    }
    if (closing || dismissed) return;
    if (prefersReducedMotion.current) { finishDismiss(); return; }
    closing = true;
    // Fallback for a removed/overridden CSS transition; normally transitionend finishes first.
    dismissTimer = setTimeout(finishDismiss, 250);
  }

  function restoreFocus(dismissedIncidentID: string) {
    const trigger = Array.from(
      document.querySelectorAll<HTMLElement>('[data-incident-trigger]')
    ).find((candidate) => candidate.dataset.incidentTrigger === dismissedIncidentID);
    (trigger ?? document.getElementById('main-content'))?.focus();
  }

  function trapFocus(event: KeyboardEvent) {
    if (event.key !== 'Tab' || !dialog) return;
    const controls = Array.from(dialog.querySelectorAll<HTMLElement>(
      'button, a[href], input, select, textarea, summary, [tabindex]'
    )).filter((element) => {
      if (element.tabIndex < 0 || element.matches(':disabled') || element.closest('[inert]')) return false;
      const hiddenDetails = element.closest('details:not([open])');
      if (hiddenDetails && hiddenDetails.querySelector('summary') !== element) return false;
      const rect = element.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && getComputedStyle(element).visibility !== 'hidden';
    });
    const first = controls[0];
    const last = controls.at(-1);
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault(); last?.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault(); first?.focus();
    }
  }

  $effect(() => {
    if (projected) {
      incident = projected;
      projectedOnce = true;
      return;
    }
    /* Une Résolution retire l'Incident de la projection active. Le détail le
       relit alors par son identifiant et reste ouvert sur son état Résolu. */
    if (projectedOnce) {
      projectedOnce = false;
      void loadIncident(false);
    }
  });

  onMount(() => {
    const mountedIncidentID = incidentId;
    if (seed?.id === incidentId) {
      incident = seed;
      incidentLoading = false;
      projectedOnce = seed.status === 'active';
    }
    const timer = setInterval(() => (now = new Date()), 30_000);
    requestAnimationFrame(() => {
      dialog?.showModal();
      closeButton?.focus();
    });
    void Promise.all([loadIncident(false), loadIndicators()]);
    return () => {
      requestVersion += 1;
      clearInterval(timer);
      clearTimeout(dismissTimer);
      if (dialog?.open) dialog.close();
      requestAnimationFrame(() => restoreFocus(mountedIncidentID));
    };
  });
</script>

<svelte:head>
  <title>{incident
    ? `${natureLabel(incident)} · ${incidentTitle} — ${session.instanceLabel}`
    : `${t('incidents.detail.title')} — ${session.instanceLabel}`}</title>
</svelte:head>

{#snippet metricCard(row: IncidentIndicatorRow, named = true)}
  <article class="metric-card" class:secondary={!row.snapshot}>
    <div class="metric-title">
      <span>
        <strong>{row.label}</strong>
        {#if named && row.indicator?.dimension}<small>{row.indicator.dimension}</small>{/if}
      </span>
      {#if row.snapshot}
        <b class="num">{formatIndicator(row.snapshot.value, row.unit)}</b>
      {:else}
        <small>{t('incidents.detail.noSnapshot')}</small>
      {/if}
    </div>
    {#if row.snapshot}
      <small class="snapshot-time">
        {t('incidents.detail.snapshotAt', { date: stamp(row.snapshot.observed_at) })}
      </small>
    {/if}
    {#if row.points.length > 0}
      <IndicatorHistoryChart
        compact
        points={row.points}
        unit={row.unit}
        label={t('incidents.detail.chartLabel', { label: row.label })}
        timeBounds={metricTimeBounds}
        {marker}
      />
    {:else}
      <div class="curve-empty">{t('incidents.detail.curveExpired')}</div>
    {/if}
  </article>
{/snippet}

{#snippet sourceRow(signal: IncidentEvidence)}
  {@const invalidated = Boolean(signal.invalidated_at)}
  {@const presented = signal.presentation?.[i18n.locale]}
  <article class="source-row" class:invalidated>
    <div class="source-identity">
      <i
        class="dot {invalidated ? 'idle' : signal.active ? severityTone(signal.severity) : 'ok'}"
        aria-hidden="true"
      ></i>
      <span>
        <strong>{signal.connector_name ?? origins[signal.origin]}</strong>
        {#if presented?.title}<small>{presented.title}</small>{/if}
      </span>
    </div>
    <span class="pill {invalidated ? '' : signal.active ? severityTone(signal.severity) : 'ok'}">
      {invalidated
        ? t('target.verdict.invalidated')
        : signal.active
          ? t('target.failing')
          : t('target.verdict.recovered')}
    </span>
    <div class="source-original">
      <span>{t('incidents.detail.originalMessage')}</span>
      <p>{signal.name}</p>
    </div>
    <dl class="source-dates">
      <div>
        <dt>{t('incidents.detail.sourceOpened')}</dt>
        <dd class="num">{stamp(signal.opened_at)}</dd>
      </div>
      <div>
        <dt>{t('incidents.detail.sourceRecovered')}</dt>
        <dd class="num">{signal.resolved_at ? stamp(signal.resolved_at) : t('common.none')}</dd>
      </div>
      {#if signal.acknowledgement_sync_status !== 'not_applicable'}
        <div>
          <dt>{t('incidents.detail.upstreamAck')}</dt>
          <dd
            class:crit={signal.acknowledgement_sync_status === 'failed'}
            title={signal.acknowledgement_sync_error}
          >
            {t(`incidents.detail.ackSync.${signal.acknowledgement_sync_status}`)}
          </dd>
        </div>
      {/if}
    </dl>

    {#if invalidated}
      <p class="invalidation-copy">
        <strong>{signal.invalidation_reason ?? t('target.noReason')}</strong>
        <span>
          {t('incidents.detail.invalidatedBy', {
            who: signal.invalidated_by ?? t('target.anOperator'),
            date: signal.invalidated_at ? stamp(signal.invalidated_at) : t('common.none')
          })}
        </span>
      </p>
    {:else if incident?.status === 'active' && signal.active && session.user?.role !== 'observer'}
      <button
        class="btn sm source-action"
        type="button"
        onclick={(event) => beginInvalidation(signal, event.currentTarget)}
      >{t('target.invalidate')}</button>
    {/if}

    {#if signal.external_event_id || signal.external_object_id}
      <details class="source-ids">
        <summary>{t('incidents.detail.externalIdentifiers')}</summary>
        {#if signal.external_event_id}
          <code>{signal.external_event_id}</code>
        {/if}
        {#if signal.external_object_id}
          <code>{signal.external_object_id}</code>
        {/if}
      </details>
    {/if}

    {#if invalidationFor?.id === signal.id}
      <form class="invalidation-form" onsubmit={confirmInvalidation} novalidate>
        <div class="field">
          <label for="incident-invalidation-reason-{signal.id}">{t('target.reason')}</label>
          <textarea
            bind:this={reasonField}
            id="incident-invalidation-reason-{signal.id}"
            bind:value={invalidationReason}
            rows="3"
            required
            minlength="8"
            maxlength="500"
            aria-invalid={invalidationError ? 'true' : undefined}
            aria-describedby="incident-invalidation-hint-{signal.id}{invalidationError ? ` incident-invalidation-error-${signal.id}` : ''}"
            placeholder={t('target.reasonPlaceholder')}
          ></textarea>
          <small id="incident-invalidation-hint-{signal.id}">{t('target.reasonHint')}</small>
          {#if invalidationError}
            <small id="incident-invalidation-error-{signal.id}" class="field-error" role="alert">
              {invalidationError}
            </small>
          {/if}
        </div>
        <div class="form-actions">
          <button class="btn" type="button" onclick={cancelInvalidation}>{t('common.cancel')}</button>
          <button class="btn danger" type="submit" disabled={invalidating}>
            {invalidating ? t('common.saving') : t('target.invalidateConfirm')}
          </button>
        </div>
      </form>
    {/if}
  </article>
{/snippet}

<dialog
  bind:this={dialog}
  class="incident-drawer"
  class:closing
  ontransitionend={(event) => { if (closing && event.target === dialog && event.propertyName === 'opacity') finishDismiss(); }}
  aria-labelledby={titleID}
  aria-describedby={descriptionID}
  aria-busy={incidentLoading || acknowledging || invalidating}
  onkeydown={trapFocus}
  oncancel={(event) => {
    event.preventDefault();
    requestDismiss();
  }}
  onclick={(event) => event.currentTarget === event.target && !invalidationFor && requestDismiss()}
>
  <header class="drawer-head">
    <div class="title-copy">
      <span class="eyebrow">{incidentTitle}</span>
      <h2 id={titleID}>{incident?.summary?.[i18n.locale].title ?? (incident ? natureLabel(incident) : t('incidents.detail.title'))}</h2>
      {#if incident}
        <p id={descriptionID} class="head-meta">
          <span class="pill {severityTone(incident.severity)}">{severityLabel(incident.severity)}</span>
          {#if incident.status === 'resolved'}
            <span class="pill ok">{t('incidents.detail.resolvedStatus')}</span>
          {/if}
          {#if maintainedImpacts.length > 0}
            <span class="pill info">{t('state.maintenance')}</span>
          {/if}
          <!-- Le séparateur reste collé au mot qui le précède : une ligne ne commence jamais par « · ». -->
          <span class="head-facts">
            <span title={stamp(incident.opened_at)}>{incident.resolved_at
              ? t('incidents.detail.resolvedAfter', { duration: since(incident.opened_at, new Date(incident.resolved_at)) })
              : t('incidents.detail.openedAgo', { duration: since(incident.opened_at, now) })}</span>{'\u00a0·'}
            <span
              class:crit={!incident.acknowledged_at && incident.status === 'active'}
              title={incident.acknowledged_at ? stamp(incident.acknowledged_at) : undefined}
            >{incident.acknowledged_at
              ? t('incidents.detail.acknowledgedBy', { who: incident.acknowledged_by ?? t('target.anOperator') })
              : t('incident.unacknowledged')}</span>{#if sourceNames}{'\u00a0·'} <span>{sourceNames}</span>{/if}
          </span>
        </p>
      {:else}
        <p id={descriptionID}>{t('incidents.detail.loading')}</p>
      {/if}
    </div>
    <button
      bind:this={closeButton}
      class="close"
      type="button"
      onclick={requestDismiss}
      aria-label={invalidationFor ? t('incidents.detail.cancelInvalidation') : t('common.close')}
    >
      <Icon name="close" size={14} />
    </button>
  </header>

  <div class="drawer-body">
    {#if incidentLoading && !incident}
      <div class="detail-state" role="status">{t('incidents.detail.loading')}</div>
    {:else if incidentError && !incident}
      <div class="detail-state error-state" role="alert">
        <strong>{incidentError}</strong>
        <button class="btn" type="button" onclick={() => loadIncident()}>{t('common.retry')}</button>
      </div>
    {:else if incident}
      {#if hasNotes}
        <div class="notes">
          {#if incident.impact_count > 1}
            <span class="propagation-state">
              {t(`incidents.propagation.${incident.propagation_status}`)}
              <InfoHint
                id={`propagation-state-hint-${incident.id}`}
                ariaLabel={t('incidents.propagation.help', { state: t(`incidents.propagation.${incident.propagation_status}`) })}
                text={t(`incidents.propagation.${incident.propagation_status}Hint`)}
              />
            </span>
          {/if}
          {#if incident.acknowledgement_sync_status === 'pending'}
            <span class="warn">{t('incidents.detail.syncPending')}</span>
          {:else if incident.acknowledgement_sync_status === 'failed'}
            <span class="crit" title={incident.acknowledgement_sync_error}>
              {t('incidents.detail.syncFailed')}
            </span>
          {/if}
          {#if maintainedImpacts[0]?.maintenance_ends_at}
            <span>{t('incidents.detail.maintenanceUntil', { date: stamp(maintainedImpacts[0].maintenance_ends_at) })}</span>
          {/if}
        </div>
      {/if}

      <section class="finding" aria-labelledby="incident-finding-title">
        <h3 id="incident-finding-title" class="finding-label">
          {t('incidents.detail.finding')}
          {#if shape.kind === 'resource'}
            · {t(`incidents.detail.noun.${shape.noun}`)} <span class="num">{shape.resource}</span>
          {/if}
        </h3>

        {#if shape.kind === 'versions'}
          <div class="versions">
            <div>
              <span>{t('incidents.detail.deployedVersion')}</span>
              <b class="num">{shape.current}</b>
            </div>
            <span class="version-arrow" aria-hidden="true">→</span>
            <div>
              <span>{t('incidents.detail.availableVersion')}</span>
              <b class="num {severityTone(incident.severity)}">{shape.available}</b>
            </div>
          </div>
        {:else if shape.kind === 'count'}
          <p class="finding-value">
            <b class="num">{shape.count}</b>
            {plural('incidents.detail.securityUpdates', shape.count)}
          </p>
        {:else if primary}
          {#if shape.kind === 'plain' && primaryText?.description}
            <p class="finding-text">{primaryText.description}</p>
          {/if}
          <p class="finding-original">{primary.name}</p>
          <small class="finding-source">
            {t('incidents.detail.sourceMessage', { source: primary.connector_name ?? origins[primary.origin] })}
          </small>
        {/if}

        {#if primary?.invalidated_at}
          <p class="finding-invalidated">
            {t('incidents.detail.invalidatedBy', {
              who: primary.invalidated_by ?? t('target.anOperator'),
              date: stamp(primary.invalidated_at)
            })}
          </p>
        {/if}

        {#if indicatorsError}
          <div class="finding-state error-state" role="alert">
            <span>{indicatorsError}</span>
            <button class="btn sm" type="button" onclick={loadIndicators}>{t('common.retry')}</button>
          </div>
        {:else if indicatorsLoading && measurable(primary?.fact)}
          <div class="finding-state" role="status">{t('incidents.detail.metricsLoading')}</div>
        {:else if indicatorSplit.relevant.length > 0}
          <div class="finding-metrics">
            {#each indicatorSplit.relevant as row (row.key)}
              {@render metricCard(row, false)}
            {/each}
          </div>
          <p class="correlation-note">{t('incidents.detail.correlationNote')}</p>
        {:else if indicators && measurable(primary?.fact)}
          <p class="finding-state">{t('incidents.detail.unmeasured')}</p>
        {/if}
      </section>

      {#if incident.impact_count > 1}
        <section class="detail-section impacts" aria-labelledby="incident-impacts-title">
          <div class="section-head">
            <h3 id="incident-impacts-title">{t('incidents.detail.affectedResources')}</h3>
            {#if diverges(incident)}<span class="pill warn">{t('targets.divergence')}</span>{/if}
            <span class="section-count num">{incident.active_impact_count}/{incident.impact_count}</span>
          </div>
          {#each incident.impacts as impact (impact.id)}
            <div class="impact-row">
              <div>
                <a href="/cibles/{impact.target_id}"><strong>{impact.target_name}</strong></a>
                <small>
                  {impact.resolved_at
                    ? t('incidents.detail.resolvedAfter', { duration: since(impact.opened_at, new Date(impact.resolved_at)) })
                    : t('incidents.detail.openedAgo', { duration: since(impact.opened_at, now) })}
                </small>
              </div>
              <span class="pill {impact.status === 'resolved' ? 'ok' : severityTone(impact.effective_severity)}">
                {impact.status === 'resolved' ? t('target.verdict.recovered') : severityLabel(impact.effective_severity)}
              </span>
            </div>
          {/each}
          <p class="grouping-note">
            {t('incidents.detail.groupingExplanation', {
              nature: natureLabel(incident),
              seconds: incident.propagation_window_seconds
            })}
          </p>
        </section>
      {/if}

      <div class="folds">
        {#if indicatorSplit.others.length > 0}
          <details class="fold">
            <summary>
              <span>{t('incidents.detail.otherMetrics')}</span>
              <small class="num">{otherPreview}</small>
            </summary>
            <div class="metric-grid">
              {#each indicatorSplit.others as row (row.key)}
                {@render metricCard(row)}
              {/each}
            </div>
            <p class="correlation-note">{t('incidents.detail.correlationNote')}</p>
          </details>
        {/if}

        <details class="fold" open={invalidationFor ? true : undefined}>
          <summary>
            <span>{plural('incidents.detail.sourceDetails', evidence.length)}</span>
            <small class="num">{plural('incidents.detail.evidenceCount', evidence.length)}</small>
          </summary>
          {#each incident.impacts as impact (impact.id)}
            {#if incident.impact_count > 1}
              <h4 class="source-group">{impact.target_name}</h4>
            {/if}
            {#each impact.evidence as signal (signal.id)}
              {@render sourceRow(signal)}
            {:else}
              <div class="section-state compact">{t('incidents.detail.sourcesEmpty')}</div>
            {/each}
          {:else}
            <div class="section-state compact">{t('incidents.detail.sourcesEmpty')}</div>
          {/each}
        </details>

        <details class="fold">
          <summary>
            <span>{t('target.activityLog')}</span>
            <small class="num">{activity.length}</small>
          </summary>
          <div class="fold-body">
            <ActivityTimeline entries={activity} />
          </div>
        </details>
      </div>
    {/if}
  </div>

  {#if incident}
    <footer class="drawer-actions">
      <span class="note">
        {incident.status === 'active'
          ? t('incidents.detail.liveNote')
          : t('incidents.detail.resolvedNote')}
      </span>
      {#if firstImpact}
        <a class="btn" href="/cibles/{firstImpact.target_id}">{t('incidents.detail.viewTarget')}</a>
      {/if}
      {#if incident.status === 'active' && !incident.acknowledged_at && session.user?.role !== 'observer'}
        <button class="btn primary" type="button" disabled={acknowledging} onclick={acknowledge}>
          {acknowledging ? t('incident.acknowledging') : t('incident.acknowledge')}
        </button>
      {/if}
    </footer>
  {/if}
</dialog>

<style>
  .incident-drawer {
    position: fixed;
    inset-block: 0;
    inset-inline-start: auto;
    inset-inline-end: 0;
    margin: 0;
    width: min(100%, var(--incident-drawer-width));
    max-width: none;
    height: 100vh;
    height: 100dvh;
    max-height: none;
    padding: 0;
    border: 0;
    border-inline-start: 1px solid var(--line-strong);
    border-radius: 0;
    background: var(--surface);
    color: var(--ink);
    box-shadow: var(--shadow);
    overflow: hidden;
    overscroll-behavior: contain;
  }

  .incident-drawer[open] {
    display: flex;
    flex-direction: column;
  }

  .incident-drawer::backdrop {
    background: var(--drawer-backdrop);
  }

  /* En-tête : la Ressource, le problème, puis une phrase d'état. */
  .drawer-head {
    flex: none;
    display: flex;
    align-items: flex-start;
    gap: var(--s4);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line);
    background: var(--surface);
  }

  .title-copy {
    min-width: 0;
    flex: 1;
  }

  .eyebrow {
    display: block;
    overflow-wrap: anywhere;
    color: var(--muted);
    font-size: var(--text-sm);
  }

  .drawer-head h2 {
    margin-top: var(--s1);
    overflow-wrap: anywhere;
    font-size: var(--text-lg);
  }

  .head-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2) var(--s3);
    margin-top: var(--s3);
    color: var(--muted);
    font-size: var(--text-sm);
  }

  .head-facts {
    min-width: 0;
  }

  .close {
    position: relative;
    width: var(--ctl-h-lg);
    height: var(--ctl-h-lg);
    display: grid;
    place-items: center;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-button);
    background: none;
    color: var(--muted);
    flex: none;
  }

  .close:hover {
    background: var(--surface-2);
    color: var(--ink);
  }

  .drawer-body {
    flex: 1;
    min-height: 0;
    container-type: inline-size;
    display: flex;
    flex-direction: column;
    gap: var(--s4);
    padding: var(--s5);
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--bg);
  }

  .detail-state,
  .section-state {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s4);
    min-height: 7rem;
    padding: var(--s5);
    color: var(--faint);
    font-size: var(--text-sm);
    text-align: center;
    flex-direction: column;
  }

  .detail-state {
    min-height: 24rem;
  }

  .section-state.compact {
    min-height: 4rem;
  }

  .error-state {
    color: var(--crit);
  }

  .notes {
    position: relative;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s3) var(--s4);
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .propagation-state {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
  }

  .propagation-state :global(.info-hint) {
    position: static;
  }

  .propagation-state :global(.info-hint .tooltip) {
    top: 100%;
    left: 0;
    width: min(19rem, 100%);
  }

  /* Constat : ce que le fait reconnu établit, avant tout le reste. */
  .finding,
  .detail-section,
  .fold {
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--surface);
  }

  .finding {
    padding: var(--s4) var(--s5);
  }

  .finding-label {
    color: var(--faint);
    font-size: var(--chart-text-size);
    font-weight: var(--weight-medium);
  }

  .finding-label .num {
    color: var(--ink);
  }

  .versions {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--s3) var(--s5);
    margin-top: var(--s3);
  }

  .versions span:not(.version-arrow),
  .versions b {
    display: block;
  }

  .versions span:not(.version-arrow) {
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .versions b,
  .finding-value b {
    margin-top: var(--s1);
    font-size: var(--text-lg);
    font-weight: var(--weight-semibold);
  }

  .version-arrow {
    color: var(--faint);
    font-size: var(--text-lg);
  }

  .finding-value {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--s3);
    margin-top: var(--s3);
    color: var(--muted);
    font-size: var(--text-sm);
  }

  .finding-text,
  .finding-original {
    margin-top: var(--s3);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    font-size: var(--text-sm);
  }

  .finding-source,
  .finding-invalidated {
    display: block;
    margin-top: var(--s2);
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .finding-state {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s3);
    margin-top: var(--s3);
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .finding-state.error-state {
    color: var(--crit);
  }

  .finding-metrics {
    display: grid;
    gap: var(--s4);
    margin-top: var(--s3);
  }

  .finding-metrics .metric-card {
    padding: 0;
    border: 0;
  }

  .finding .correlation-note {
    margin-top: var(--s3);
    padding: 0;
    border: 0;
  }

  /* Plusieurs Ressources : une ligne par Atteinte, sans imbriquer les Preuves. */
  .section-head {
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line);
  }

  .section-head h3 {
    flex: 1;
    min-width: 0;
    font-size: var(--text-sm);
  }

  .section-count {
    color: var(--faint);
  }

  .impact-row {
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: var(--s3) var(--s5);
    border-bottom: 1px solid var(--line-row);
  }

  .impact-row > div {
    min-width: 0;
    flex: 1;
  }

  .impact-row strong,
  .impact-row small {
    display: block;
    overflow-wrap: anywhere;
  }

  .impact-row strong {
    font-size: var(--text-sm);
  }

  .impact-row a:hover strong {
    color: var(--accent);
  }

  .impact-row small {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .grouping-note {
    padding: var(--s3) var(--s5);
    color: var(--muted);
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  /* Replis : tout ce qui sert à vérifier, pas à comprendre. */
  .folds {
    display: flex;
    flex-direction: column;
    gap: var(--s3);
  }

  .fold > summary {
    min-height: var(--ctl-h-lg);
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: var(--s3) var(--s5);
    cursor: pointer;
    font-size: var(--text-sm);
  }

  .fold > summary > span {
    flex: none;
  }

  .fold > summary > small {
    min-width: 0;
    margin-inline-start: auto;
    overflow: hidden;
    color: var(--faint);
    font-size: var(--chart-text-size);
    text-align: end;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .fold[open] > summary {
    border-bottom: 1px solid var(--line-row);
  }

  .fold-body {
    padding: var(--s4) var(--s5);
  }

  .source-group {
    padding: var(--s3) var(--s5);
    border-bottom: 1px solid var(--line-row);
    background: var(--surface-2);
    font-size: var(--text-sm);
  }

  .metric-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .metric-card {
    min-width: 0;
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line-row);
  }

  .metric-grid .metric-card:nth-child(odd) {
    border-inline-end: 1px solid var(--line-row);
  }

  .metric-grid .metric-card:last-child:nth-child(odd) {
    grid-column: 1 / -1;
    border-inline-end: 0;
  }

  .metric-title {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--s4);
  }

  .metric-title > span {
    min-width: 8rem;
    flex: 1;
  }

  .metric-title strong,
  .metric-title small,
  .snapshot-time {
    display: block;
  }

  .metric-title strong {
    font-size: var(--text-sm);
  }

  .metric-title small,
  .snapshot-time {
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .metric-title b {
    font-size: var(--text-md);
  }

  .snapshot-time {
    margin-block: var(--s2) var(--s4);
  }

  .curve-empty {
    min-height: 3.375rem;
    display: grid;
    place-items: center;
    margin-top: var(--s3);
    border: 1px dashed var(--line-strong);
    border-radius: var(--r-m);
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .correlation-note {
    padding: var(--s3) var(--s5);
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .source-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--s4);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line-row);
  }

  .source-row:last-child {
    border-bottom: 0;
  }

  .source-row.invalidated {
    color: var(--faint);
  }

  .source-identity {
    display: flex;
    align-items: center;
    gap: var(--s3);
    min-width: 0;
  }

  .source-identity span,
  .source-identity strong,
  .source-identity small {
    display: block;
    min-width: 0;
  }

  .source-identity strong {
    overflow-wrap: anywhere;
    font-size: var(--text-sm);
  }

  .source-identity small {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .source-original,
  .source-dates,
  .source-action,
  .invalidation-copy,
  .source-ids,
  .invalidation-form {
    grid-column: 1 / -1;
  }

  .source-original span,
  .source-dates dt {
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  .source-original p {
    margin-top: var(--s1);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: var(--muted);
    font-size: var(--text-sm);
  }

  .source-dates {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--s4);
    margin: 0;
  }

  .source-dates dd {
    margin: var(--s1) 0 0;
    font-size: var(--chart-text-size);
  }

  .source-action {
    justify-self: start;
  }

  .invalidation-copy {
    max-width: 100%;
    font-size: var(--chart-text-size);
  }

  .invalidation-copy strong,
  .invalidation-copy span {
    display: block;
  }

  .invalidation-copy span {
    margin-top: var(--s1);
    color: var(--faint);
  }

  .source-ids summary {
    cursor: pointer;
    color: var(--muted);
    font-size: var(--text-sm);
  }

  .source-ids code {
    display: block;
    margin-top: var(--s2);
    color: var(--faint);
    font-size: var(--chart-text-size);
    overflow-wrap: anywhere;
  }

  .invalidation-form {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: end;
    gap: var(--s5);
    padding-top: var(--s4);
    border-top: 1px solid var(--line-row);
  }

  .invalidation-form .field {
    margin: 0;
  }

  .invalidation-form textarea {
    resize: vertical;
  }

  .field-error {
    color: var(--crit) !important;
  }

  .form-actions {
    display: flex;
    gap: var(--s3);
    padding-bottom: var(--s1);
  }

  .drawer-actions {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s3);
    padding: var(--s4) var(--s5);
    padding-bottom: max(var(--s4), env(safe-area-inset-bottom));
    border-top: 1px solid var(--line);
    background: var(--surface);
    flex-wrap: wrap;
  }

  .drawer-actions .note {
    min-width: 12rem;
    flex: 1;
    margin-inline-end: auto;
    color: var(--faint);
    font-size: var(--chart-text-size);
  }

  @media (max-width: 48rem) {
    .incident-drawer {
      width: 100vw;
      border: 0;
    }

    .drawer-head {
      padding-top: max(var(--s4), env(safe-area-inset-top));
    }

    .close {
      width: 2.75rem;
      height: 2.75rem;
    }

    .drawer-body {
      padding: var(--s4);
    }

    .drawer-actions .note { flex-basis: 100%; }
  }

  @container (max-width: 32rem) {
    .metric-grid,
    .source-dates {
      grid-template-columns: minmax(0, 1fr);
    }

    .metric-grid .metric-card:nth-child(odd) {
      border-inline-end: 0;
    }

    .finding,
    .fold > summary,
    .fold-body,
    .source-row,
    .source-group,
    .impact-row,
    .section-head,
    .grouping-note {
      padding-inline: var(--s4);
    }

    .source-dates {
      gap: var(--s3);
    }

    .source-dates > div {
      display: grid;
      grid-template-columns: minmax(7rem, 0.8fr) minmax(0, 1fr);
      gap: var(--s3);
    }

    .source-dates dd {
      margin: 0;
    }

    .invalidation-form {
      grid-template-columns: minmax(0, 1fr);
    }

    .form-actions {
      justify-content: flex-end;
    }
  }

  :global(body:has(.incident-drawer[open])) { overflow: hidden; }

  @media (prefers-reduced-motion: no-preference) {
    .incident-drawer {
      opacity: 1;
      transform: translateX(0);
      transition: transform var(--d2) ease-out, opacity var(--d2) ease-out;
    }
    .incident-drawer::backdrop { transition: background-color var(--d2) ease-out; }
    .incident-drawer.closing {
      opacity: 0;
      transform: translateX(var(--s4));
      transition-duration: var(--d1);
    }
    .incident-drawer.closing::backdrop { background: transparent; }
    @starting-style {
      .incident-drawer[open] { opacity: 0; transform: translateX(var(--s5)); }
      .incident-drawer[open]::backdrop { background: transparent; }
    }
  }

  @media (hover: hover) {
    .fold > summary:hover,
    .source-ids summary:hover {
      color: var(--ink);
    }
  }
</style>
