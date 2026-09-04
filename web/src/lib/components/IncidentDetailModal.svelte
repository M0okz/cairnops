<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import IndicatorAreaChart from './IndicatorAreaChart.svelte';
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
  import { incidentActivity, incidentIndicatorRows } from '$lib/incident-detail';
  import { plural, t } from '$lib/i18n.svelte';
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

  const titleID = $derived(`incident-detail-title-${incidentId}`);
  const descriptionID = $derived(`incident-detail-description-${incidentId}`);
  const activity = $derived(incident ? incidentActivity(incident) : []);
  const metricRows = $derived(
    indicators ? incidentIndicatorRows(indicators) : { captured: [], additional: [] }
  );
  const projected = $derived(session.incidents.find((item) => item.id === incidentId) ?? null);
  const evidence = $derived(incident?.impacts.flatMap((impact) => impact.evidence) ?? []);
  const maintainedImpacts = $derived(incident?.impacts.filter((impact) => impact.maintenance_active) ?? []);
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

  const origins: Record<IncidentEvidence['origin'], string> = {
    native: 'CairnOps',
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    argus: 'Argus',
    webhook: 'Webhook'
  };

  function activeEvidenceCount(impact: Incident['impacts'][number]): number {
    return impact.evidence.filter((item) => item.active && !item.invalidated_at).length;
  }

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

  function requestDismiss() {
    if (invalidationFor) {
      cancelInvalidation();
      return;
    }
    ondismiss();
  }

  function restoreFocus() {
    const trigger = Array.from(
      document.querySelectorAll<HTMLElement>('[data-incident-trigger]')
    ).find((candidate) => candidate.dataset.incidentTrigger === incidentId);
    (trigger ?? document.getElementById('main-content'))?.focus();
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
      if (dialog?.open) dialog.close();
      requestAnimationFrame(restoreFocus);
    };
  });
</script>

<svelte:head>
  <title>{incident
    ? `${natureLabel(incident)} · ${incidentTitle} — ${session.instanceLabel}`
    : `${t('incidents.detail.title')} — ${session.instanceLabel}`}</title>
</svelte:head>

<dialog
  bind:this={dialog}
  class="incident-modal"
  aria-labelledby={titleID}
  aria-describedby={descriptionID}
  aria-busy={incidentLoading || acknowledging || invalidating}
  oncancel={(event) => {
    event.preventDefault();
    requestDismiss();
  }}
  onclick={(event) => event.currentTarget === event.target && !invalidationFor && ondismiss()}
>
  <header class="modal-head">
    <div class="title-copy">
      <span class="eyebrow">{t('incidents.detail.title')}</span>
      <h2 id={titleID}>{incidentTitle}</h2>
      <p id={descriptionID}>{incident ? natureLabel(incident) : t('incidents.detail.loading')}</p>
    </div>
    {#if incident}
      <div class="head-status">
        <span class="pill {incident.status === 'resolved' ? 'ok' : severityTone(incident.severity)}">
          <i class="dot {incident.status === 'resolved' ? 'ok' : severityTone(incident.severity)}" aria-hidden="true"></i>
          {incident.status === 'resolved'
            ? t('incidents.detail.resolvedStatus')
            : t('incidents.detail.activeStatus')}
        </span>
        <span class="pill {severityTone(incident.severity)}">
          {severityLabel(incident.severity)}
        </span>
        {#if maintainedImpacts.length > 0}
          <span class="pill info">{t('state.maintenance')}</span>
        {/if}
      </div>
    {/if}
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

  <div class="modal-body">
    {#if incidentLoading && !incident}
      <div class="detail-state" role="status">{t('incidents.detail.loading')}</div>
    {:else if incidentError && !incident}
      <div class="detail-state error-state" role="alert">
        <strong>{incidentError}</strong>
        <button class="btn" type="button" onclick={() => loadIncident()}>{t('common.retry')}</button>
      </div>
    {:else if incident}
      <section class="summary" aria-labelledby="incident-summary-title">
        <h3 id="incident-summary-title" class="visually-hidden">{t('incidents.detail.summary')}</h3>
        <div class="date-grid">
          <div>
            <span>{t('incidents.detail.openedAt')}</span>
            <strong class="num">{stamp(incident.opened_at)}</strong>
          </div>
          <div>
            <span>{t('incidents.detail.acknowledgedAt')}</span>
            <strong class="num">
              {incident.acknowledged_at ? stamp(incident.acknowledged_at) : t('incident.unacknowledged')}
            </strong>
            {#if incident.acknowledged_at}
              <small>
                {incident.acknowledged_by ?? t('target.anOperator')}
                · {incident.acknowledgement_origin === 'connector'
                  ? t('incidents.detail.connectorOrigin')
                  : t('incidents.detail.userOrigin')}
              </small>
            {/if}
          </div>
          <div>
            <span>{t('incidents.detail.resolvedAt')}</span>
            <strong class="num">
              {incident.resolved_at ? stamp(incident.resolved_at) : t('incidents.detail.ongoing')}
            </strong>
          </div>
          <div>
            <span>{t('incidents.column.duration')}</span>
            <strong class="num">
              {incident.resolved_at
                ? since(incident.opened_at, new Date(incident.resolved_at))
                : since(incident.opened_at, now)}
            </strong>
          </div>
        </div>
        <div class="summary-notes">
          <span>{t('incidents.impactsActive', { active: incident.active_impact_count, total: incident.impact_count })}</span>
          <span>{t(`incidents.propagation.${incident.propagation_status}`)}</span>
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
        {#if incident.impact_count > 1}
          <p class="grouping-note">
            {t('incidents.detail.groupingExplanation', {
              nature: natureLabel(incident),
              seconds: incident.propagation_window_seconds
            })}
          </p>
        {/if}
      </section>

      <section class="detail-section metrics" aria-labelledby="incident-metrics-title">
        <div class="section-head">
          <div>
            <h3 id="incident-metrics-title">{t('incidents.detail.metrics')}</h3>
            <p>{t('incidents.detail.metricsNote')}</p>
          </div>
          <span class="pill info">± 2 h</span>
        </div>

        {#if indicatorsLoading}
          <div class="section-state" role="status">{t('incidents.detail.metricsLoading')}</div>
        {:else if indicatorsError}
          <div class="section-state error-state" role="alert">
            <span>{indicatorsError}</span>
            <button class="btn sm" type="button" onclick={loadIndicators}>{t('common.retry')}</button>
          </div>
        {:else if indicators}
          {#if metricRows.captured.length === 0}
            <div class="section-state">
              <strong>{t('incidents.detail.metricsEmpty')}</strong>
              <span>{t('incidents.detail.metricsEmptyHint')}</span>
            </div>
          {:else}
            <div class="metric-grid">
              {#each metricRows.captured as row (row.key)}
                <article class="metric-card">
                  <div class="metric-title">
                    <span>
                      <strong>{row.label}</strong>
                      {#if row.indicator?.dimension}<small>{row.indicator.dimension}</small>{/if}
                    </span>
                    <b class="num">{formatIndicator(row.snapshot?.value, row.unit)}</b>
                  </div>
                  <small class="snapshot-time">
                    {t('incidents.detail.snapshotAt', { date: stamp(row.snapshot!.observed_at) })}
                  </small>
                  {#if row.points.length > 0}
                    <IndicatorAreaChart
                      compact
                      interactive
                      points={row.points}
                      unit={row.unit}
                      label={t('incidents.detail.chartLabel', { label: row.label })}
                      {marker}
                    />
                  {:else}
                    <div class="curve-empty">{t('incidents.detail.curveExpired')}</div>
                  {/if}
                </article>
              {/each}
            </div>
          {/if}

          {#if metricRows.additional.length > 0}
            <details class="additional-metrics">
              <summary>{plural('incidents.detail.moreMetrics', metricRows.additional.length)}</summary>
              <div class="metric-grid">
                {#each metricRows.additional as row (row.key)}
                  <article class="metric-card secondary">
                    <div class="metric-title">
                      <span>
                        <strong>{row.label}</strong>
                        {#if row.indicator?.dimension}<small>{row.indicator.dimension}</small>{/if}
                      </span>
                      <small>{t('incidents.detail.noSnapshot')}</small>
                    </div>
                    {#if row.points.length > 0}
                      <IndicatorAreaChart
                        compact
                        interactive
                        points={row.points}
                        unit={row.unit}
                        label={t('incidents.detail.chartLabel', { label: row.label })}
                        {marker}
                      />
                    {:else}
                      <div class="curve-empty">{t('incidents.detail.curveExpired')}</div>
                    {/if}
                  </article>
                {/each}
              </div>
            </details>
          {/if}

          <p class="correlation-note">{t('incidents.detail.correlationNote')}</p>
        {/if}
      </section>

      <section class="detail-section sources" aria-labelledby="incident-sources-title">
        <div class="section-head">
          <div>
            <h3 id="incident-sources-title">{t('incidents.detail.impactsAndEvidence')}</h3>
            <p>{t('incidents.detail.impactsAndEvidenceNote')}</p>
          </div>
          {#if diverges(incident)}<span class="pill warn">{t('targets.divergence')}</span>{/if}
          <span class="section-count num">{evidence.length}</span>
        </div>

        <div class="impact-list">
          {#each incident.impacts as impact (impact.id)}
            <section class="impact-group">
              <header class="impact-head">
                <div>
                  <a href="/cibles/{impact.target_id}"><strong>{impact.target_name}</strong></a>
                  <small>
                    {t('incidents.detail.impactDates', {
                      opened: stamp(impact.opened_at),
                      resolved: impact.resolved_at ? stamp(impact.resolved_at) : t('incidents.detail.ongoing')
                    })}
                  </small>
                </div>
                <span class="pill {impact.status === 'resolved' ? 'ok' : severityTone(impact.effective_severity)}">
                  {severityLabel(impact.effective_severity)}
                </span>
                <span class="impact-count num">
                  {t('incidents.detail.evidenceRatio', {
                    active: activeEvidenceCount(impact),
                    total: impact.evidence.length
                  })}
                </span>
              </header>

              <div class="source-list">
                {#each impact.evidence as signal (signal.id)}
                  {@const invalidated = Boolean(signal.invalidated_at)}
                  <article class="source-row" class:invalidated>
                    <div class="source-identity">
                      <i
                        class="dot {invalidated ? 'idle' : signal.active ? severityTone(signal.severity) : 'ok'}"
                        aria-hidden="true"
                      ></i>
                      <span>
                        <strong>{signal.name}</strong>
                        <small>{signal.connector_name ?? origins[signal.origin]}</small>
                      </span>
                    </div>
                    <span class="pill {invalidated ? '' : signal.active ? severityTone(signal.severity) : 'ok'}">
                      {invalidated
                        ? t('target.verdict.invalidated')
                        : signal.active
                          ? t('target.failing')
                          : t('target.verdict.recovered')}
                    </span>
                    <dl class="source-dates">
                      <div>
                        <dt>{t('incidents.detail.sourceOpened')}</dt>
                        <dd class="num">{stamp(signal.opened_at)}</dd>
                      </div>
                      <div>
                        <dt>{t('incidents.detail.sourceRecovered')}</dt>
                        <dd class="num">{signal.resolved_at ? stamp(signal.resolved_at) : t('common.none')}</dd>
                      </div>
                      <div>
                        <dt>{t('incidents.detail.upstreamAck')}</dt>
                        <dd
                          class:crit={signal.acknowledgement_sync_status === 'failed'}
                          title={signal.acknowledgement_sync_error}
                        >
                          {t(`incidents.detail.ackSync.${signal.acknowledgement_sync_status}`)}
                        </dd>
                      </div>
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
                    {:else if incident.status === 'active' && signal.active && session.user?.role !== 'observer'}
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
                {:else}
                  <div class="section-state compact">{t('incidents.detail.sourcesEmpty')}</div>
                {/each}
              </div>
            </section>
          {:else}
            <div class="section-state">{t('incidents.detail.sourcesEmpty')}</div>
          {/each}
        </div>
      </section>

      <section class="detail-section activity" aria-labelledby="incident-activity-title">
        <div class="section-head">
          <div>
            <h3 id="incident-activity-title">{t('target.activityLog')}</h3>
            <p>{t('incidents.detail.activityNote')}</p>
          </div>
          <span class="section-count num">{activity.length}</span>
        </div>
        <div class="timeline">
          {#each activity as entry (entry.id)}
            <article class="timeline-entry">
              <i aria-hidden="true"></i>
              <time class="num" datetime={entry.occurred_at}>{stamp(entry.occurred_at)}</time>
              <div>
                <strong>{entry.message}</strong>
                <small>
                  {t('target.origin', { origin: entry.origin })}
                  {#if entry.actor_name}· {entry.actor_name}{/if}
                </small>
              </div>
            </article>
          {:else}
            <div class="section-state">{t('target.noEntries')}</div>
          {/each}
        </div>
      </section>
    {/if}
  </div>

  {#if incident}
    <footer class="modal-actions">
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
  .incident-modal {
    width: calc(100vw - 2 * var(--s5));
    max-width: 64rem;
    max-height: calc(100vh - 2 * var(--s5));
    max-height: calc(100dvh - 2 * var(--s5));
    padding: 0;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--surface);
    color: var(--ink);
    box-shadow: var(--shadow);
    overflow: hidden;
    overscroll-behavior: contain;
  }

  .incident-modal[open] {
    display: flex;
    flex-direction: column;
  }

  .incident-modal::backdrop {
    background: rgb(0 0 0 / 0.55);
  }

  .modal-head {
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
    margin-bottom: var(--s1);
    color: var(--faint);
    font-size: 0.625rem;
    font-weight: var(--weight-semibold);
  }

  .modal-head h2 {
    font-size: 1rem;
  }

  .modal-head p {
    margin-top: var(--s1);
    color: var(--muted);
    font-size: var(--text-sm);
  }

  .head-status {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s3);
    flex-wrap: wrap;
  }

  .head-status .pill:first-child {
    display: inline-flex;
    align-items: center;
    gap: var(--s2);
  }

  .close {
    position: relative;
    width: var(--ctl-h-lg);
    height: var(--ctl-h-lg);
    display: grid;
    place-items: center;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: none;
    color: var(--muted);
    flex: none;
  }

  .close:hover {
    background: var(--surface-2);
    color: var(--ink);
  }

  .modal-body {
    min-height: 0;
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

  .error-state {
    color: var(--crit);
  }

  .summary,
  .detail-section {
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--surface);
    overflow: hidden;
  }

  .detail-section {
    margin-top: var(--s5);
  }

  .date-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .date-grid > div {
    min-width: 0;
    padding: var(--s4) var(--s5);
    border-inline-end: 1px solid var(--line-row);
  }

  .date-grid > div:last-child {
    border-inline-end: 0;
  }

  .date-grid span,
  .date-grid strong,
  .date-grid small {
    display: block;
  }

  .date-grid span {
    margin-bottom: var(--s2);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .date-grid strong {
    color: var(--ink);
    font-size: var(--text-sm);
    font-weight: var(--weight-semibold);
    white-space: normal;
  }

  .date-grid small {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .summary-notes {
    display: flex;
    align-items: center;
    gap: var(--s4);
    min-height: var(--ctl-h);
    padding: var(--s3) var(--s5);
    border-top: 1px solid var(--line-row);
    color: var(--faint);
    font-size: 0.625rem;
    flex-wrap: wrap;
  }

  .grouping-note {
    padding: var(--s3) var(--s5);
    border-top: 1px solid var(--line-row);
    color: var(--muted);
    font-size: 0.6875rem;
    line-height: 1.5;
  }

  .section-head {
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line);
  }

  .section-head > div {
    min-width: 0;
    flex: 1;
  }

  .section-head h3 {
    font-size: var(--text-md);
  }

  .section-head p {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .section-count {
    color: var(--faint);
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

  .metric-card:nth-child(odd) {
    border-inline-end: 1px solid var(--line-row);
  }

  .metric-title {
    display: flex;
    align-items: baseline;
    gap: var(--s4);
  }

  .metric-title > span {
    min-width: 0;
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
    font-size: 0.625rem;
  }

  .metric-title b {
    font-size: var(--text-md);
  }

  .snapshot-time {
    margin-top: var(--s1);
  }

  .curve-empty {
    min-height: 3.375rem;
    display: grid;
    place-items: center;
    margin-top: var(--s3);
    border: 1px dashed var(--line-strong);
    border-radius: var(--r-m);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .additional-metrics {
    border-top: 1px solid var(--line);
  }

  .additional-metrics summary,
  .source-ids summary {
    cursor: pointer;
    color: var(--muted);
    font-size: var(--text-sm);
  }

  .additional-metrics > summary {
    min-height: var(--ctl-h-lg);
    display: flex;
    align-items: center;
    padding: var(--s3) var(--s5);
  }

  .additional-metrics[open] > summary {
    border-bottom: 1px solid var(--line-row);
  }

  .correlation-note {
    padding: var(--s3) var(--s5);
    border-top: 1px solid var(--line);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .impact-group + .impact-group {
    border-top: 1px solid var(--line-strong);
  }

  .impact-head {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto auto;
    align-items: center;
    gap: var(--s4);
    padding: var(--s3) var(--s5);
    background: var(--surface-2);
  }

  .impact-head > div,
  .impact-head strong,
  .impact-head small {
    display: block;
    min-width: 0;
  }

  .impact-head strong {
    font-size: var(--text-sm);
  }

  .impact-head a:hover strong {
    color: var(--accent);
  }

  .impact-head small,
  .impact-count {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .section-state.compact {
    min-height: 4rem;
  }

  .source-row {
    display: grid;
    grid-template-columns: minmax(10rem, 1fr) auto minmax(22rem, 1.4fr) auto;
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
    font-size: var(--text-sm);
  }

  .source-identity small {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .source-dates {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--s4);
    margin: 0;
  }

  .source-dates dt {
    color: var(--faint);
    font-size: 0.5625rem;
  }

  .source-dates dd {
    margin: var(--s1) 0 0;
    font-size: 0.625rem;
  }

  .source-action {
    justify-self: end;
  }

  .invalidation-copy {
    max-width: 18rem;
    font-size: 0.625rem;
  }

  .invalidation-copy strong,
  .invalidation-copy span {
    display: block;
  }

  .invalidation-copy span {
    margin-top: var(--s1);
    color: var(--faint);
  }

  .source-ids {
    grid-column: 1 / -1;
  }

  .source-ids code {
    display: block;
    margin-top: var(--s2);
    color: var(--faint);
    font-size: 0.625rem;
    overflow-wrap: anywhere;
  }

  .invalidation-form {
    grid-column: 1 / -1;
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

  .timeline {
    padding: var(--s3) var(--s5) var(--s4);
  }

  .timeline-entry {
    position: relative;
    display: grid;
    grid-template-columns: 0.75rem 8.75rem minmax(0, 1fr);
    gap: var(--s3);
    min-height: 3rem;
    padding: var(--s3) 0;
  }

  .timeline-entry::before {
    content: '';
    position: absolute;
    inset-block: 0;
    inset-inline-start: 0.34375rem;
    width: 1px;
    background: var(--line-strong);
  }

  .timeline-entry:first-child::before {
    inset-block-start: 50%;
  }

  .timeline-entry:last-child::before {
    inset-block-end: 50%;
  }

  .timeline-entry > i {
    position: relative;
    z-index: 1;
    width: 0.5rem;
    height: 0.5rem;
    margin-top: var(--s2);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-pill);
    background: var(--surface);
  }

  .timeline-entry time {
    color: var(--faint);
    font-size: 0.625rem;
    white-space: normal;
  }

  .timeline-entry strong,
  .timeline-entry small {
    display: block;
  }

  .timeline-entry strong {
    font-size: var(--text-sm);
    font-weight: var(--weight-medium);
  }

  .timeline-entry small {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.625rem;
  }

  .modal-actions {
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

  .modal-actions .note {
    min-width: 12rem;
    flex: 1;
    margin-inline-end: auto;
    color: var(--faint);
    font-size: 0.625rem;
  }

  @media (max-width: 68rem) {
    .source-row {
      grid-template-columns: minmax(10rem, 1fr) auto minmax(18rem, 1.4fr);
    }

    .source-action,
    .invalidation-copy {
      grid-column: 1 / -1;
      justify-self: start;
    }
  }

  @media (max-width: 48rem) {
    .incident-modal {
      width: 100vw;
      max-width: none;
      height: 100vh;
      height: 100dvh;
      max-height: none;
      border: 0;
      border-radius: 0;
    }

    .modal-head {
      padding-top: max(var(--s4), env(safe-area-inset-top));
    }

    .close {
      width: 2.75rem;
      height: 2.75rem;
    }

    .head-status {
      order: 3;
      width: 100%;
      justify-content: flex-start;
    }

    .modal-head {
      flex-wrap: wrap;
    }

    .modal-body {
      padding: var(--s4);
    }

    .date-grid,
    .metric-grid,
    .source-dates {
      grid-template-columns: minmax(0, 1fr);
    }

    .date-grid > div,
    .metric-card:nth-child(odd) {
      border-inline-end: 0;
    }

    .date-grid > div {
      display: grid;
      grid-template-columns: minmax(7rem, 0.8fr) minmax(0, 1fr);
      gap: var(--s3);
      border-bottom: 1px solid var(--line-row);
    }

    .date-grid > div:last-child {
      border-bottom: 0;
    }

    .date-grid span {
      margin: 0;
    }

    .date-grid small {
      grid-column: 2;
    }

    .section-head {
      align-items: flex-start;
    }

    .impact-head {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .impact-count {
      grid-column: 1 / -1;
      margin-top: 0;
    }

    .source-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .source-dates,
    .source-action,
    .invalidation-copy,
    .source-ids,
    .invalidation-form {
      grid-column: 1 / -1;
    }

    .source-dates {
      gap: var(--s3);
      padding-inline-start: 1.125rem;
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

    .timeline-entry {
      grid-template-columns: 0.75rem minmax(0, 1fr);
    }

    .timeline-entry time {
      grid-column: 2;
    }

    .timeline-entry > div {
      grid-column: 2;
    }

    .modal-actions .note {
      flex-basis: 100%;
    }
  }

  @media (hover: hover) {
    .additional-metrics summary:hover,
    .source-ids summary:hover {
      color: var(--ink);
    }
  }
</style>
