<script lang="ts">
  import { aiProviders, providerForEndpoint } from "$lib/ai-providers";
  import { api } from "$lib/api";
  import { localeTag, t } from "$lib/i18n.svelte";
  import { messageFrom } from "$lib/session.svelte";
  import type { SoftwareAIConfig, SoftwareAIUsageHistory } from "$lib/software-updates";
  import { Input } from "./ui/input";
  import { Button } from "./ui/button";
  import Switch from "./ui/Switch.svelte";
  import Icon from './Icon.svelte';
  let config = $state<SoftwareAIConfig | null>(null);
  let key = $state("");
  let providerID = $state("");
  let customModel = $state(false);
  let savedEndpoint = $state("");
  let usage = $state<SoftwareAIUsageHistory | null>(null);
  let usageError = $state("");
  const totals = $derived((usage?.days ?? []).reduce((sum, day) => ({
    requests: sum.requests + day.requests,
    unreported: sum.unreported + day.unreported,
    prompt: sum.prompt + day.prompt_tokens,
    completion: sum.completion + day.completion_tokens,
    tokens: sum.tokens + day.total_tokens,
  }), { requests: 0, unreported: 0, prompt: 0, completion: 0, tokens: 0 }));
  const number = (value: number) => new Intl.NumberFormat(localeTag()).format(value);
  const date = (value: string) => new Intl.DateTimeFormat(localeTag(), { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(`${value}T12:00:00Z`));
  const provider = $derived(
    aiProviders.find((entry) => entry.id === providerID),
  );
  const canKeepKey = $derived(
    !!config?.key_configured && config.endpoint === savedEndpoint,
  );
  function restoreSelection() {
    if (!config) return;
    savedEndpoint = config.endpoint;
    const preset = providerForEndpoint(config.endpoint);
    providerID = preset?.id ?? (config.endpoint ? "custom" : "");
    customModel = !preset?.models.some((model) => model.id === config?.model);
  }
  function chooseProvider(event: Event) {
    if (!config) return;
    providerID = (event.currentTarget as HTMLSelectElement).value;
    const next = aiProviders.find((entry) => entry.id === providerID);
    // Changing destination must never carry a newly entered key to another provider.
    key = "";
    saved = false;
    error = "";
    if (next) {
      config.endpoint = next.endpoint;
      config.model = next.models[0].id;
      customModel = false;
    } else if (providerID !== "custom") {
      config.endpoint = "";
      config.model = "";
    } else {
      customModel = true;
    }
  }
  function chooseModel(event: Event) {
    if (!config) return;
    const value = (event.currentTarget as HTMLSelectElement).value;
    customModel = value === "custom";
    config.model = customModel ? "" : value;
    saved = false;
  }
  let busy = $state(false);
  let error = $state("");
  let saved = $state(false);
  async function load() {
    try {
      config = await api<SoftwareAIConfig>("/api/v1/software-update-settings");
      restoreSelection();
      error = "";
      await loadUsage();
    } catch (e) {
      error = messageFrom(e);
    }
  }
  async function loadUsage() {
    try {
      usage = await api<SoftwareAIUsageHistory>("/api/v1/software-update-settings/usage");
      usageError = "";
    } catch (e) {
      usageError = messageFrom(e);
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
      restoreSelection();
      saved = true;
      await loadUsage();
    } catch (e) {
      error = messageFrom(e);
    } finally {
      busy = false;
    }
  }
</script>

<section class="card software-settings" id="software-analysis">
  <header class="software-heading">
    <span class="software-icon"><Icon name="activity" size={18} /></span>
    <span><h2>{t("updates.aiTitle")}</h2><small>{t("updates.aiHint")}</small></span>
  </header>
  {#if error}<p role="alert">{error}</p>{/if}
  {#if config}
    <form onsubmit={save} class="shadcn-control">
      <div class="switch-row">
        <Switch
          label={t("updates.enabled")}
          bind:checked={config.enabled}
        /><span>{t("updates.enabled")}</span>
      </div>
      <label class="field" for="software-provider"
        >{t("updates.provider")}
        <select
          id="software-provider"
          required
          value={providerID}
          onchange={chooseProvider}
          disabled={busy}
        >
          <option value="" disabled>{t("updates.chooseProvider")}</option>
          {#each aiProviders as entry}<option value={entry.id}
              >{entry.name}</option
            >{/each}
          <option value="custom">{t("updates.customProvider")}</option>
        </select>
      </label>
      {#if providerID === "custom"}
        <label for="software-endpoint"
          >{t("updates.endpoint")}
          <Input
            id="software-endpoint"
            type="url"
            required
            bind:value={config.endpoint}
            oninput={() => {
              key = "";
              saved = false;
            }}
            placeholder="https://api.example.com/v1"
            disabled={busy}
          />
        </label>
      {/if}
      {#if provider}
        <label class="field" for="software-model-choice"
          >{t("updates.model")}
          <select
            id="software-model-choice"
            value={customModel ? "custom" : config.model}
            onchange={chooseModel}
            disabled={busy}
          >
            {#each provider.models as model}<option value={model.id}
                >{model.name}</option
              >{/each}
            <option value="custom">{t("updates.customModel")}</option>
          </select>
        </label>
      {/if}
      {#if providerID && (customModel || providerID === "custom")}
        <label for="software-model"
          >{t("updates.modelID")}
          <Input
            id="software-model"
            required
            maxlength={160}
            bind:value={config.model}
            disabled={busy}
          />
        </label>
      {/if}
      <label for="software-key"
        >{t("updates.key")}<Input
          id="software-key"
          type="password"
          autocomplete="new-password"
          bind:value={key}
          placeholder={canKeepKey ? t("updates.keyKept") : ""}
          required={!canKeepKey}
          disabled={busy}
        /></label
      >
      <div>
        <Button type="submit" disabled={busy || !providerID}
          >{busy ? t("updates.saving") : t("updates.save")}</Button
        >
        {#if saved}<span role="status">{t("updates.saved")}</span>{/if}
      </div>
    </form>
    <div class="usage-section">
      <div class="usage-heading">
        <div><h3>{t("updates.usageTitle")}</h3><p>{t("updates.usageScope")}</p></div>
        {#if providerForEndpoint(savedEndpoint)?.id === 'gemini'}
          <a href="https://aistudio.google.com/billing" target="_blank" rel="noopener noreferrer">{t("updates.geminiBilling")}</a>
        {/if}
      </div>
      {#if usageError}<p role="alert">{usageError} <button class="usage-retry" onclick={loadUsage}>{t("updates.retry")}</button></p>{/if}
      {#if usage}
        {#if totals.requests > 0}
          <div class="usage-totals">
            <div><strong>{number(totals.tokens)}</strong><span>{t("updates.usageTokens")}</span></div>
            <div><strong>{number(totals.requests)}</strong><span>{t("updates.usageRequests")}</span></div>
          </div>
          <p class="usage-note">{t("updates.usageNote")}</p>
          {#if totals.unreported > 0}<p class="usage-note">{t("updates.usageUnreported", { count: number(totals.unreported) })}</p>{/if}
          <details>
            <summary>{t("updates.usageDetails")}</summary>
            <div class="usage-scroll"><table>
              <thead><tr><th scope="col">{t("updates.usageDate")}</th><th scope="col">{t("updates.usageRequests")}</th><th scope="col">{t("updates.usageInput")}</th><th scope="col">{t("updates.usageOutput")}</th><th scope="col">{t("updates.usageTokens")}</th></tr></thead>
              <tbody>{#each usage.days as day}<tr><th scope="row">{date(day.date)}</th><td>{number(day.requests)}</td><td>{number(day.prompt_tokens)}</td><td>{number(day.completion_tokens)}</td><td>{number(day.total_tokens)}</td></tr>{/each}</tbody>
            </table></div>
          </details>
        {:else}<p class="usage-note">{t("updates.usageEmpty")}</p>{/if}
      {:else if !usageError}<p class="usage-note" role="status">{t("updates.loading")}</p>{/if}
    </div>
  {:else if error}<button class="btn" onclick={load}
      >{t("updates.retry")}</button
    >{:else}<p role="status">{t("updates.loading")}</p>{/if}
</section>

<style>
  .software-settings {
    display: block;
  }
  .software-heading { display: flex; align-items: center; gap: var(--s4); min-height: 3.75rem; padding: var(--s3) var(--s4); border-bottom: 1px solid var(--line); }
  .software-heading h2 { margin: 0; font-size: 0.9375rem; font-weight: 600; }
  .software-heading small { display: block; margin-top: var(--s1); color: var(--faint); font-size: var(--text-xs); line-height: 1.4; }
  .software-icon { display: inline-grid; place-items: center; flex: none; width: 2.25rem; height: 2.25rem; border-radius: var(--r-m); background: var(--surface-2); }
  .software-settings > p { margin: var(--s4) var(--s5); }
  form {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr)) auto;
    align-items: end;
    gap: var(--s4);
    max-width: none;
    padding: var(--s4) var(--s5) var(--s5);
  }
  .switch-row { grid-column: 1 / -1; }
  form > div:last-child { display: flex; align-items: center; gap: var(--s3); min-height: var(--ctl-h-lg); }
  form > div:last-child span { color: var(--ok); font-size: var(--text-xs); }
  label {
    display: grid;
    gap: var(--s2);
  }
  label.field {
    margin-bottom: 0;
  }
  select {
    width: 100%;
    min-width: 0;
  }
  .switch-row {
    display: flex;
    align-items: center;
    gap: var(--s3);
  }
  .usage-section { padding: var(--s4) var(--s5) var(--s5); border-top: 1px solid var(--line); }
  .usage-heading { display: flex; align-items: baseline; justify-content: space-between; flex-wrap: wrap; gap: var(--s3); }
  .usage-heading h3 { margin: 0; font-size: var(--text-sm); }
  .usage-heading p, .usage-note { margin: var(--s2) 0; color: var(--faint); font-size: var(--text-xs); line-height: 1.5; }
  .usage-heading a { font-size: var(--text-xs); color: var(--text); text-decoration: underline; }
  .usage-totals { display: flex; gap: var(--s5); flex-wrap: wrap; margin: var(--s4) 0 var(--s2); }
  .usage-totals div { display: grid; gap: var(--s1); }
  .usage-totals strong { font-family: var(--font-num); font-size: 1.5rem; font-variant-numeric: tabular-nums; }
  .usage-totals span { color: var(--faint); font-size: var(--text-xs); }
  details { margin-top: var(--s4); }
  summary { cursor: pointer; font-size: var(--text-sm); }
  .usage-scroll { overflow-x: auto; margin-top: var(--s3); }
  table { width: 100%; border-collapse: collapse; font-size: var(--text-xs); font-variant-numeric: tabular-nums; }
  th, td { padding: var(--s2); text-align: right; border-bottom: 1px solid var(--line); white-space: nowrap; }
  th:first-child { text-align: left; }
  thead th { color: var(--faint); font-weight: 500; }
  .usage-retry { text-decoration: underline; }
  @media (max-width: 58rem) { form { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 48rem) { form { grid-template-columns: minmax(0, 1fr); } .software-heading { align-items: start; } }
</style>
