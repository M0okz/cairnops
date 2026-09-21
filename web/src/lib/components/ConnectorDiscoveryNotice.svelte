<script lang="ts">
  import { t } from '$lib/i18n.svelte';
</script>

<p class="discovery-note">{t('connectors.discoveryHint')}</p>

<style>
  .discovery-note {
    flex: none;
    margin: 0 0 var(--s3);
    color: var(--muted);
    font-size: var(--text-xs);
  }
</style>
