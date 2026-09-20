<script lang="ts">
  import { t } from "$lib/i18n.svelte";
  import {
    safeReleaseURL,
    type ReleasePoint,
    type ReleaseSource,
    type ReleaseNote,
  } from "$lib/software-updates";
  let {
    points,
    notes = [],
    source,
  }: {
    points: ReleasePoint[];
    notes?: ReleaseNote[];
    source: ReleaseSource;
  } = $props();
  const categories = [
    "feature",
    "fix",
    "security",
    "impact",
    "upgrade",
  ] as const;
</script>

{#each categories as category}
  {@const items = points.filter((point) => point.category === category)}
  {#if items.length}
    <section class="point-group">
      <h4>{t(`updates.${category}`)}</h4>
      <ul>
        {#each items as point, i (i)}
          <li>
            <p>{point.text}</p>
            <details>
              <summary>{point.version} · {t("updates.citation")}</summary>
              <blockquote>{point.quote}</blockquote>
              <a
                href={safeReleaseURL(
                  notes.find((n) => n.version === point.version)?.url ??
                    source.url,
                )}
                target="_blank"
                rel="noreferrer noopener">{t("updates.source")} ↗</a
              >
            </details>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
{/each}

<style>
  .point-group {
    margin-block: var(--s4);
  }
  h4 {
    font-size: var(--text-sm);
    color: var(--muted);
    margin-bottom: var(--s2);
  }
  ul {
    padding-left: var(--s5);
    display: grid;
    gap: var(--s3);
  }
  p {
    line-height: 1.65;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  summary {
    color: var(--muted);
    font-size: var(--text-xs);
    cursor: pointer;
    padding-block: var(--s2);
  }
  blockquote {
    border-left: 2px solid var(--line);
    padding: var(--s3);
    color: var(--muted);
    font-size: var(--text-sm);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  a {
    font-size: var(--text-sm);
  }
</style>
