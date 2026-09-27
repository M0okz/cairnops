<script lang="ts">
  import { localeTag, t, type MessageKey } from "$lib/i18n.svelte";
  import { clock } from "$lib/format";
  import { safeReleaseURL, softwareJournal, type SoftwareJournalEntry, type SoftwareService, type VersionEvent } from "$lib/software-updates";
  import Icon from "./Icon.svelte";
  import ReleasePoints from "./ReleasePoints.svelte";
  import JournalNotes from "./SoftwareJournalNotes.svelte";
  import JournalAnalysisDetails from "./SoftwareJournalAnalysisDetails.svelte";

  let { service }: { service: SoftwareService } = $props();
  const entries = $derived(softwareJournal(service));
  const days = $derived.by(() => {
    const groups: { key: string; at: string; entries: SoftwareJournalEntry[] }[] = [];
    for (const entry of entries) {
      const key = new Intl.DateTimeFormat(localeTag(), { year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date(entry.at));
      if (groups.at(-1)?.key !== key) groups.push({ key, at: entry.at, entries: [] });
      groups.at(-1)!.entries.push(entry);
    }
    return groups;
  });
  const day = (at: string) => new Intl.DateTimeFormat(localeTag(), { day: "numeric", month: "long", year: "numeric" }).format(new Date(at));
  function eventLabel(event: VersionEvent) {
    const key = event.kind === "installed"
      ? `updates.event.${event.direction ?? "changed"}`
      : `updates.event.${event.kind}`;
    return t(key as MessageKey, {
      version: event.version,
      previous: event.previous ?? "—",
      target: event.target ?? "—",
    });
  }
</script>

<section class="journal" aria-labelledby="software-journal-title">
  <header class="journal-head">
    <h3 id="software-journal-title">{t("updates.journal")}</h3>
    <p>{t("updates.historyHint")}</p>
  </header>
  {#each days as group (group.key)}
    <section class="journal-day" aria-label={day(group.at)}>
      <h4 class="day-heading"><span>{day(group.at)}</span></h4>
      <ol class="journal-list">
        {#each group.entries as entry (entry.id)}
          <li class="journal-event" data-kind={entry.kind}>
            <time class="event-time mono" datetime={entry.at}>{clock(entry.at)}</time>
            <span class="event-marker" class:observed={entry.kind === "observation" && entry.event.kind === "installed" && entry.event.direction === "upgrade"} aria-hidden="true">
              <Icon name={entry.kind === "observation" ? "activity" : entry.kind === "notes" ? "book" : "changelog"} size={15} />
            </span>
            <div class="event-body">
              {#if entry.kind === "observation"}
                <p class="event-title">{eventLabel(entry.event)}</p>
                <p class="event-meta">Argus</p>
              {:else if entry.kind === "notes"}
                <p class="event-title">{t("updates.journalNotes", { installed: entry.archive.installed_version, target: entry.archive.target_version })}</p>
                <p class="event-meta">{t("updates.journalNotesCount", { count: entry.archive.notes.length })}{entry.current ? ` · ${t("updates.journalCurrent")}` : ""}</p>
                {#if entry.archive.incomplete}<p class="journal-caution">{t("updates.catalogueIncomplete")}</p>{/if}
                <JournalNotes notes={entry.archive.notes} />
              {:else}
                <p class="event-title">{t("updates.journalSummary", { installed: entry.analysis.installed_version, target: entry.analysis.target_version })}</p>
                <p class="event-meta">{t("updates.summaryFrench")}{entry.current ? ` · ${t("updates.journalCurrent")}` : ""}</p>
                <ReleasePoints points={entry.analysis.result.overview} notes={entry.analysis.notes} source={entry.analysis.source} />
                {#if !entry.analysis.result.overview.length}<p class="event-meta">{t("updates.noChanges")}</p>{/if}
                {#if entry.analysis.result.details.length}<JournalAnalysisDetails analysis={entry.analysis} />{/if}
              {/if}
            </div>
          </li>
        {/each}
      </ol>
    </section>
  {:else}
    <p class="journal-empty">{t("updates.journalEmpty")}</p>
  {/each}
</section>

<style>
  .journal { min-width: 0; container-type: inline-size; }
  .journal-head { margin-bottom: var(--s5); }
  .journal-head h3 { margin: 0; font-size: var(--text-lg); }
  .journal-head p { margin-top: var(--s2); color: var(--muted); font-size: var(--text-sm); line-height: 1.5; }
  .journal-day + .journal-day { margin-top: var(--s5); }
  .day-heading { display: flex; align-items: center; gap: var(--s4); margin: 0 0 var(--s4); font-size: var(--text-sm); font-weight: var(--weight-semibold); }
  .day-heading::after { content: ''; height: 1px; background: var(--line); flex: 1; }
  .journal-list { list-style: none; margin: 0; padding: 0; }
  .journal-event { position: relative; display: grid; grid-template-columns: 3rem 1.5rem minmax(0, 1fr); gap: var(--s4); padding-bottom: var(--s5); }
  .journal-event:last-child { padding-bottom: 0; }
  .journal-event:not(:last-child)::before { content: ''; position: absolute; left: calc(3rem + var(--s4) + .75rem); top: calc(1.5rem + var(--s2)); bottom: var(--s2); width: 1px; background: var(--line-strong); }
  .event-time { padding-top: var(--s1); color: var(--faint); font-size: var(--text-xs); white-space: nowrap; }
  .event-marker { position: relative; display: grid; place-items: center; width: 1.5rem; height: 1.5rem; border-radius: 50%; border: 1px solid var(--line-strong); color: var(--muted); background: var(--surface); }
  .event-marker.observed { color: var(--ok); border-color: var(--ok-line); background: var(--ok-bg); }
  .event-body { min-width: 0; padding-top: var(--s1); }
  .event-title { margin: 0; font-weight: var(--weight-semibold); line-height: 1.45; overflow-wrap: anywhere; }
  .event-meta { margin-top: var(--s1); color: var(--muted); font-size: var(--text-sm); line-height: 1.5; }
  .journal-caution { margin-top: var(--s2); color: var(--muted); font-size: var(--text-sm); }
  .journal-empty { color: var(--muted); font-size: var(--text-sm); }
  @container (max-width: 28rem) {
    .journal-event { grid-template-columns: 1.5rem minmax(0, 1fr); gap: var(--s2) var(--s3); }
    .event-marker { grid-column: 1; grid-row: 1 / 3; }
    .event-time { grid-column: 2; grid-row: 1; padding-top: 0; }
    .event-body { grid-column: 2; grid-row: 2; }
    .journal-event:not(:last-child)::before { left: .75rem; }
  }
  @media (forced-colors: active) { .event-marker { border-color: CanvasText; } .journal-event:not(:last-child)::before { background: CanvasText; } }
</style>
