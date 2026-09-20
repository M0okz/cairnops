<script lang="ts">
  import { api } from "$lib/api";
  import { t } from "$lib/i18n.svelte";
  import { messageFrom } from "$lib/session.svelte";
  import type { SoftwareAIConfig } from "$lib/software-updates";
  import { Input } from "./ui/input";
  import { Button } from "./ui/button";
  import Switch from "./ui/Switch.svelte";
  let config = $state<SoftwareAIConfig | null>(null);
  let key = $state("");
  let busy = $state(false);
  let error = $state("");
  let saved = $state(false);
  async function load() {
    try {
      config = await api<SoftwareAIConfig>("/api/v1/software-update-settings");
      error = "";
    } catch (e) {
      error = messageFrom(e);
    }
  }
  $effect(() => {
    void load();
  });
  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!config || busy) return;
    busy = true;
    error = "";
    saved = false;
    try {
      config = await api<SoftwareAIConfig>("/api/v1/software-update-settings", {
        method: "PUT",
        body: JSON.stringify({ ...config, api_key: key }),
      });
      key = "";
      saved = true;
    } catch (e) {
      error = messageFrom(e);
    } finally {
      busy = false;
    }
  }
</script>

<section class="card software-settings" id="software-analysis">
  <h2>{t("updates.aiTitle")}</h2>
  <p class="muted">{t("updates.aiHint")}</p>
  {#if error}<p role="alert">{error}</p>{/if}
  {#if config}
    <form onsubmit={save} class="shadcn-control">
      <div class="switch-row">
        <Switch
          label={t("updates.enabled")}
          bind:checked={config.enabled}
        /><span>{t("updates.enabled")}</span>
      </div>
      <label for="software-endpoint"
        >{t("updates.endpoint")}<Input
          id="software-endpoint"
          type="url"
          required
          bind:value={config.endpoint}
          placeholder="https://api.example.com/v1"
        /></label
      >
      <label for="software-model"
        >{t("updates.model")}<Input
          id="software-model"
          required
          maxlength={160}
          bind:value={config.model}
        /></label
      >
      <label for="software-key"
        >{t("updates.key")}<Input
          id="software-key"
          type="password"
          autocomplete="new-password"
          bind:value={key}
          placeholder={config.key_configured ? t("updates.keyKept") : ""}
        /></label
      >
      <div>
        <Button type="submit" disabled={busy}
          >{busy ? t("updates.saving") : t("updates.save")}</Button
        >
        {#if saved}<span role="status">{t("updates.saved")}</span>{/if}
      </div>
    </form>
  {:else if error}<button class="btn" onclick={load}
      >{t("updates.retry")}</button
    >{:else}<p role="status">{t("updates.loading")}</p>{/if}
</section>

<style>
  .software-settings {
    padding: var(--s6);
    display: grid;
    gap: var(--s4);
  }
  form {
    display: grid;
    gap: var(--s4);
    max-width: 48rem;
  }
  label {
    display: grid;
    gap: var(--s2);
  }
  .switch-row {
    display: flex;
    align-items: center;
    gap: var(--s3);
  }
</style>
