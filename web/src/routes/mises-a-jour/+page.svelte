<script lang="ts">
  import Button from '$lib/components/ui/Button.svelte';
  import Topbar from "$lib/components/Topbar.svelte";
  import SoftwareUpdates from "$lib/components/SoftwareUpdates.svelte";
  import { t } from "$lib/i18n.svelte";
  import { session } from "$lib/session.svelte";
</script>

<svelte:head
  ><title>{t("updates.title")} — {session.instanceLabel}</title></svelte:head
>
<Topbar crumbs={[{ label: t("updates.title") }]} />
<div class="page">
  <div class="page-head">
    <div>
      <h1>{t("updates.title")}</h1>
      <p>{t("updates.intro")}</p>
    </div>
    {#if session.user?.role === "administrator"}<Button
        href="/reglages#software-analysis">{t("updates.settings")}</Button>{/if}
  </div>
  <SoftwareUpdates />
</div>
