<script lang="ts">
  import type { IncidentBurst } from '$lib/api';
  import { incidentHref } from '$lib/incident-detail';
  import { severityLabel, severityTone, since, stamp } from '$lib/format';
  import { t } from '$lib/i18n.svelte';
  import Odometer from './Odometer.svelte';

  type Props = {
    burst: IncidentBurst;
    resolved: boolean;
    now: Date;
    openInitially?: boolean;
    canAcknowledge: boolean;
    acknowledging: boolean;
    onacknowledge: () => void;
  };

  let {
    burst,
    resolved,
    now,
    openInitially = false,
    canAcknowledge,
    acknowledging,
    onacknowledge
  }: Props = $props();

  const stateLabel = $derived(
    burst.status === 'resolved'
      ? t('bursts.resolved')
      : burst.status === 'sealed'
        ? t('bursts.sealed')
        : t('bursts.propagating')
  );
  const impact = $derived(
    burst.status === 'resolved'
      ? t('bursts.maxImpact', { targets: burst.max_affected_targets, total: burst.incident_count })
      : t('bursts.impact', {
          active: burst.active_incident_count,
          targets: burst.affected_target_count,
          total: burst.incident_count
        })
  );
</script>

<details class="burst" id={`burst-${burst.id}`} open={openInitially}>
  <summary class="trow burst-summary" aria-label={`${t('bursts.expand')} — ${burst.nature_label}`}>
    <span class="cell-name">
      <i class="dot {resolved ? 'ok' : severityTone(burst.severity)}"></i>
      <span class="identity">
        <strong><span class="kind">{t('bursts.label')}</span> · {burst.nature_label}</strong>
        <small>{impact}</small>
      </span>
    </span>

    <span class="pill {severityTone(burst.severity)}">{severityLabel(burst.severity)}</span>

    <span class="hide-sm acknowledgement">
      {#if resolved}
        <span class="muted">{burst.resolved_at ? stamp(burst.resolved_at) : t('common.none')}</span>
      {:else if burst.acknowledged_at}
        <span class="ack"><i>✓</i>{burst.acknowledged_by ?? t('overview.acknowledgedShort')}</span>
      {:else}
        <span class="crit">{t('overview.fig.unacknowledged')}</span>
      {/if}
    </span>

    <span class="num hide-sm">
      <Odometer value={burst.resolved_at
        ? since(burst.opened_at, new Date(burst.resolved_at))
        : since(burst.opened_at, now)} />
    </span>

    <span class="num hide-sm impact-count" aria-label={t('bursts.members')}>—</span>

    <span class="faint log hide-sm">
      {burst.extended ? t('bursts.extended') : stateLabel}
    </span>

    <span class="chevron" aria-hidden="true">⌄</span>
  </summary>

  <div class="burst-body">
    <div class="decision">
      <div>
        <strong>{t('bursts.explanation')}</strong>
        <p>{burst.explanation}</p>
      </div>
      {#if canAcknowledge && !burst.acknowledged_at && burst.status !== 'resolved'}
        <button
          class="btn primary sm"
          type="button"
          disabled={acknowledging}
          onclick={onacknowledge}
        >{acknowledging ? '…' : t('bursts.acknowledge')}</button>
      {/if}
    </div>

    <div class="members" aria-label={t('bursts.members')}>
      {#each burst.members as member (member.incident_id)}
        <a class="member" href={incidentHref(member.incident_id)} data-incident-trigger={member.incident_id}>
          <i class="dot {member.status === 'resolved' ? 'ok' : severityTone(member.effective_severity)}"></i>
          <span>
            <strong>{member.target_name}</strong>
            <small class="faint">
              {member.status === 'resolved'
                ? t('incidents.detail.resolvedStatus')
                : member.maintenance_active
                  ? t('state.maintenance')
                  : t('incidents.detail.activeStatus')}
            </small>
          </span>
          <span class="pill {severityTone(member.effective_severity)}">
            {severityLabel(member.effective_severity)}
          </span>
          <span class="num faint">{stamp(member.opened_at)}</span>
          <span aria-hidden="true">→</span>
        </a>
      {/each}
    </div>
  </div>
</details>

<style>
  .burst {
    border-bottom: 1px solid var(--line-row);
    background: var(--surface);
  }

  .burst:last-child {
    border-bottom: 0;
  }

  summary {
    list-style: none;
    cursor: pointer;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  .burst-summary {
    border-bottom: 0;
    background: color-mix(in srgb, var(--surface-2) 48%, var(--surface));
  }

  .burst-summary:hover,
  .burst-summary:focus-visible {
    background: var(--surface-2);
  }

  .identity,
  .identity strong,
  .identity small {
    display: block;
    min-width: 0;
  }

  .identity strong,
  .identity small,
  .log {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .identity small {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.6875rem;
    font-variant-numeric: tabular-nums;
  }

  .kind {
    color: var(--accent);
    font-size: 0.625rem;
    font-weight: 650;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .ack {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
    color: var(--muted);
  }

  .ack i {
    color: var(--ok);
    font-style: normal;
  }

  .impact-count {
    color: var(--ink);
    font-variant-numeric: tabular-nums;
  }

  .log {
    font-size: 0.75rem;
  }

  .chevron {
    justify-self: center;
    color: var(--faint);
    font-size: 1rem;
    transition: transform var(--d1) var(--ease), color var(--d1) var(--ease);
  }

  details[open] .chevron {
    transform: rotate(180deg);
    color: var(--accent);
  }

  .burst-body {
    padding: var(--s3) var(--s4) var(--s4) 2rem;
    border-top: 1px solid var(--line-row);
    background: var(--surface);
  }

  .decision {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s4);
    margin-bottom: var(--s3);
  }

  .decision strong {
    font-size: 0.6875rem;
    font-weight: 650;
  }

  .decision p {
    margin: var(--s1) 0 0;
    color: var(--muted);
    font-size: 0.6875rem;
    text-wrap: pretty;
  }

  .members {
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: var(--r-m);
    background: var(--surface-2);
  }

  .member {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) 7rem 9rem auto;
    align-items: center;
    gap: var(--s3);
    min-height: 2.75rem;
    padding: var(--s2) var(--s3);
    border-bottom: 1px solid var(--line-row);
  }

  .member:last-child {
    border-bottom: 0;
  }

  .member:hover {
    background: var(--surface-3);
  }

  .member strong,
  .member small {
    display: block;
  }

  .member strong {
    font-size: 0.75rem;
    font-weight: 600;
  }

  .member small {
    margin-top: 0.125rem;
    font-size: 0.625rem;
  }

  @media (max-width: 760px) {
    .burst-summary {
      grid-template-columns: minmax(0, 1fr) auto auto;
    }

    .chevron {
      grid-column: 3;
      grid-row: 1;
    }

    .burst-body {
      padding: var(--s3);
    }

    .decision {
      align-items: flex-start;
      flex-direction: column;
    }

    .member {
      grid-template-columns: auto minmax(0, 1fr) auto auto;
    }

    .member > :nth-child(4) {
      display: none;
    }
  }
</style>
