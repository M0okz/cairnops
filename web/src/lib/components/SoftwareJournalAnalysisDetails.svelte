<script lang="ts">
  import { t } from "$lib/i18n.svelte";
  import type { ReleaseAnalysis } from "$lib/software-updates";
  import ReleasePoints from "./ReleasePoints.svelte";
  let { analysis }: { analysis: ReleaseAnalysis } = $props();
  let expanded = $state(false);
</script>

<button type="button" class="details-button" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
  {expanded ? t("updates.hideSummaryDetails") : t("updates.showSummaryDetails")}
</button>
{#if expanded}<ReleasePoints points={analysis.result.details} notes={analysis.notes} source={analysis.source} />{/if}

<style>
  .details-button { margin-top: var(--s2); padding: var(--s2) 0; border: 0; background: none; color: var(--ink); font: inherit; font-size: var(--text-sm); font-weight: var(--weight-semibold); cursor: pointer; }
  .details-button:hover { text-decoration: underline; }
  .details-button:focus-visible { outline: 2px solid var(--ink); outline-offset: 2px; }
</style>
