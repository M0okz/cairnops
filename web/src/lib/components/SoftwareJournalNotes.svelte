<script lang="ts">
  import { t } from "$lib/i18n.svelte";
  import { safeReleaseURL, type ReleaseNote } from "$lib/software-updates";

  let { notes }: { notes: ReleaseNote[] } = $props();
  let showAll = $state(false);
  const visible = $derived(showAll ? notes : notes.slice(0, 3));
  const excerpt = (body: string) => body
    .replace(/\[([^\]]+)\]\(https?:\/\/[^)]+\)/g, "$1")
    .replace(/^\s{0,3}(?:#{1,6}\s+|[-*]\s+)/gm, "")
    .replace(/[*_`]/g, "")
    .replace(/\s+/g, " ").trim().slice(0, 180);
</script>

{#if notes.length}
  <ul class="notes">
    {#each visible as note (note.version)}
      <li>
        <div class="note-head">
          <strong class="mono">{note.version}</strong>
          {#if note.missing}<span class="muted">{t("updates.missing")}</span>{/if}
          {#if safeReleaseURL(note.url)}<a href={safeReleaseURL(note.url)} target="_blank" rel="noreferrer noopener">{t("updates.original")} ↗</a>{/if}
        </div>
        {#if note.body}
          <p class="excerpt">{excerpt(note.body)}{note.body.length > 180 ? "…" : ""}</p>
          {#if note.body.length > 180}
            <details class="full-note"><summary>{t("updates.readFullNote")}</summary><pre>{note.body}</pre></details>
          {/if}
        {/if}
      </li>
    {/each}
  </ul>
  {#if notes.length > 3}
    <button class="more-notes" type="button" onclick={() => (showAll = !showAll)}>
      {showAll ? t("updates.showFewerVersions") : t("updates.showAllVersions", { count: notes.length })}
    </button>
  {/if}
{/if}

<style>
  .notes { list-style: none; padding: 0; margin: var(--s3) 0 0; border-top: 1px solid var(--line); }
  .notes li { padding-block: var(--s3); border-bottom: 1px solid var(--line); min-width: 0; }
  .note-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--s2); font-size: var(--text-sm); }
  .note-head strong { color: var(--ink); }
  .note-head a { margin-inline-start: auto; color: var(--muted); }
  .muted, .excerpt { color: var(--muted); }
  .excerpt { margin-top: var(--s2); font-size: var(--text-sm); line-height: 1.5; overflow-wrap: anywhere; }
  .full-note summary { margin-top: var(--s2); color: var(--ink); font-size: var(--text-sm); font-weight: var(--weight-medium); cursor: pointer; }
  .full-note pre { white-space: pre-wrap; overflow-wrap: anywhere; font-family: inherit; font-size: var(--text-sm); line-height: 1.55; color: var(--muted); }
  .more-notes { margin-top: var(--s3); padding: var(--s2) 0; border: 0; background: none; color: var(--ink); font: inherit; font-size: var(--text-sm); font-weight: var(--weight-semibold); cursor: pointer; }
  .more-notes:hover, .note-head a:hover { text-decoration: underline; }
  summary:focus-visible, button:focus-visible, a:focus-visible { outline: 2px solid var(--ink); outline-offset: 2px; }
</style>
