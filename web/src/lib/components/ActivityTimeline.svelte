<script lang="ts">
  import Icon from './Icon.svelte';
  import { activityDays, activityMarker, activityOrigin, type TimelineEntry } from '$lib/activity-timeline';
  import { localeTag, t, type MessageKey } from '$lib/i18n.svelte';

  let { entries }: { entries: TimelineEntry[] } = $props();
  const days = $derived(activityDays(entries));
  const detailLabels: Record<string, MessageKey> = {
    evidence_added: 'timeline.evidence_added', evidence_resolved: 'timeline.evidence_resolved',
    invalidated: 'timeline.invalidated', impact_joined: 'timeline.impact_joined', impact_reopened: 'timeline.impact_reopened'
  };
  const date = (at: string) => new Intl.DateTimeFormat(localeTag(), {
    day: 'numeric', month: 'long', year: 'numeric'
  }).format(new Date(at));
  const time = (at: string) => new Intl.DateTimeFormat(localeTag(), {
    hour: '2-digit', minute: '2-digit'
  }).format(new Date(at));
</script>

<div class="activity-timeline">
  {#each days as day (day.key)}
    <section class="timeline-day" aria-label={date(day.at)}>
      <h4 class="day-heading"><span>{date(day.at)}</span></h4>
      <ol class="timeline-list" aria-label={t('target.activityLog')}>
        {#each day.entries as entry (entry.id)}
          {@const marker = activityMarker(entry.kind)}
          <li class="timeline-event" data-kind={entry.kind}>
            <time class="event-time num" datetime={entry.occurred_at} aria-label={`${date(entry.occurred_at)}, ${time(entry.occurred_at)}`}>
              {time(entry.occurred_at)}
            </time>
            <span class="event-marker" class:restored={marker.restored} aria-hidden="true"><Icon name={marker.icon} size={16} /></span>
            <div class="event-body">
              <p class="event-message">{entry.message}</p>
              <div class="event-meta">
                {#if detailLabels[entry.kind]}
                  <span>{t(detailLabels[entry.kind])}</span>
                {/if}
                {#if entry.context}<span>{entry.context}</span>{/if}
                {#if entry.origin}<span class="event-origin">{entry.origin === 'user' ? entry.actor_name || t('timeline.userAction') : activityOrigin(entry.origin)}</span>{/if}
                {#if entry.actor_name && entry.origin !== 'user'}<span>{entry.actor_name}</span>{/if}
              </div>
            </div>
          </li>
        {/each}
      </ol>
    </section>
  {:else}
    <p class="timeline-empty">{t('target.noEntries')}</p>
  {/each}
</div>

<style>
  .activity-timeline { --event-marker-size: 1.5rem; container-type: inline-size; padding: var(--s5); }
  .timeline-day + .timeline-day { margin-top: var(--s5); }
  .day-heading { display: flex; align-items: center; gap: var(--s4); margin: 0 0 var(--s4); color: var(--ink); font-size: var(--text-xs); font-weight: var(--weight-semibold); }
  .day-heading::after { content: ''; height: 1px; background: var(--line); flex: 1; }
  .timeline-list { list-style: none; margin: 0; padding: 0; }
  .timeline-event { position: relative; display: grid; grid-template-columns: 3rem var(--event-marker-size) minmax(0, 1fr); gap: var(--s4); padding-bottom: var(--s5); }
  .timeline-event:last-child { padding-bottom: 0; }
  .timeline-event:not(:last-child)::before { content: ''; position: absolute; width: 1px; background: var(--line-strong); left: calc(3rem + var(--s4) + var(--event-marker-size) / 2); top: calc(var(--event-marker-size) + var(--s2)); bottom: var(--s2); }
  .event-time { padding-top: var(--s2); font-size: var(--chart-text-size); line-height: 1.5; color: var(--faint); white-space: nowrap; }
  .event-marker { position: relative; display: grid; place-items: center; width: var(--event-marker-size); height: var(--event-marker-size); border: 1px solid var(--line-strong); border-radius: 50%; color: var(--faint); background: var(--surface); }
  .event-marker.restored { color: var(--ok); border-color: var(--ok-line); background: var(--ok-bg); }
  .event-body { min-width: 0; padding-top: var(--s1); }
  .event-message { margin: 0; color: var(--ink); font-size: var(--text-sm); font-weight: var(--weight-medium); line-height: 1.5; overflow-wrap: anywhere; }
  .event-meta { display: flex; flex-wrap: wrap; column-gap: var(--s3); row-gap: var(--s1); margin-top: var(--s2); color: var(--faint); font-size: var(--chart-text-size); line-height: 1.5; overflow-wrap: anywhere; }
  .event-meta > span + span::before { content: '·'; margin-right: var(--s3); color: var(--faint); }
  .timeline-empty { margin: 0; color: var(--faint); font-size: var(--text-sm); }
  @container (max-width: 25rem) {
    .timeline-event { grid-template-columns: var(--event-marker-size) minmax(0, 1fr); column-gap: var(--s3); row-gap: var(--s1); }
    .event-marker { grid-column: 1; grid-row: 1 / 3; }
    .event-time { grid-column: 2; grid-row: 1; padding-top: 0; }
    .event-body { grid-column: 2; grid-row: 2; }
    .timeline-event:not(:last-child)::before { left: calc(var(--event-marker-size) / 2); }
  }
  @media (forced-colors: active) { .event-marker, .event-marker.restored { border-color: CanvasText; color: CanvasText; } .timeline-event:not(:last-child)::before { background: CanvasText; } }
</style>
