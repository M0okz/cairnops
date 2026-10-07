<script lang="ts">
  import { api } from "$lib/api";
  import { t, type MessageKey } from "$lib/i18n.svelte";
  import { session, messageFrom } from "$lib/session.svelte";
  import { stamp } from "$lib/format";
  import {
    rateLimitedHost,
    safeReleaseURL,
    type ReleaseSource,
    type SoftwareService,
  } from "$lib/software-updates";
  import SoftwareUpdateJournal from "./SoftwareUpdateJournal.svelte";
  import SegmentedControl from "./ui/SegmentedControl.svelte";
  import { Input } from "./ui/input";
  import Button from "./ui/Button.svelte";
  let { id }: { id: string } = $props();
  let service = $state<SoftwareService | null>(null);
  let error = $state("");
  let busy = $state(false);
  let initialized = false;
  let editingSource = $state(false);
  const effectiveSource = $derived(
    service?.confirmed_at ? service.source : service?.suggested_source,
  );
  let source = $state<ReleaseSource>({ kind: "github", url: "", software: "" });
  const comparable = $derived(
    service?.situation === "update" || service?.situation === "prerelease",
  );
  const reviewNotice = $derived.by(() => {
    if (!service) return "";
    if (!service.known)
      return t(
        `updates.issue.${service.verification_issue ?? "unconfirmed"}` as MessageKey,
      );
    if (service.group === "review")
      return t(`updates.situation.${service.situation}` as MessageKey);
    return service.skipped ? t("updates.skipped") : "";
  });
  const limitedHost = $derived(service ? rateLimitedHost(service) : undefined);
  $effect(() => {
    const serviceID = id;
    let alive = true;
    let running = false;
    async function load() {
      if (running) return;
      running = true;
      try {
        const data = await api<SoftwareService>(
          `/api/v1/software-updates/${serviceID}`,
        );
        if (!alive) return;
        service = data;
        error = "";
        if (!initialized) {
          source = data.confirmed_at
            ? { ...data.source }
            : data.suggested_source
              ? { ...data.suggested_source }
              : { kind: "github", url: "", software: "" };
          initialized = true;
        }
      } catch (e) {
        if (alive) error = messageFrom(e);
      } finally {
        running = false;
      }
    }
    void load();
    const timer = setInterval(() => {
      if (!document.hidden) void load();
    }, 15000);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  });
  async function confirm(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;
    busy = true;
    error = "";
    try {
      await api(`/api/v1/software-updates/${id}/source`, {
        method: "PUT",
        body: JSON.stringify(source),
      });
      service = await api<SoftwareService>(`/api/v1/software-updates/${id}`);
      editingSource = false;
    } catch (e) {
      error = messageFrom(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="software-detail">
  {#if error}<p role="alert">{error}</p>{/if}
  {#if !service}<p role="status">{t("updates.loading")}</p>{:else}
    <div class="detail-meta">
      <span
      >{t("updates.observed")} : {service.observed_at
          ? stamp(service.observed_at)
          : "—"}</span
      >
    </div>
    {#if reviewNotice}<p class="detail-note">{reviewNotice}</p>{/if}
    {#if effectiveSource || session.user?.role === "administrator"}
      <section class="source-box" aria-label={t("updates.source")}>
        <div class="source-heading">
          <div>
            <strong>{t("updates.source")}{#if effectiveSource} · {effectiveSource.software}{/if}</strong>
            {#if effectiveSource}
              {#if safeReleaseURL(effectiveSource.url)}<a href={safeReleaseURL(effectiveSource.url)} target="_blank" rel="noreferrer noopener">{t("updates.openSource")} ↗</a>{/if}
            {/if}
          </div>
          {#if effectiveSource && session.user?.role === "administrator" && !editingSource}
            <div><Button onclick={() => { source = { ...effectiveSource }; editingSource = true; }}>{t("updates.editSource")}</Button></div>
          {/if}
        </div>
        {#if session.user?.role === "administrator" && (editingSource || !effectiveSource)}
          <form onsubmit={confirm} class="shadcn-control source-form">
            <p class="muted">{t("updates.sourceHint")}</p>
            <SegmentedControl label={t("updates.sourceKind")} value={source.kind} items={[
              { value: "github", label: "GitHub" },
              { value: "forgejo", label: "Forgejo / Gitea" },
              { value: "gitlab", label: "GitLab" },
              { value: "changelog", label: "Changelog" },
            ]} onValueChange={(value) => (source.kind = value)} />
            <label for={`software-name-${id}`}>{t("updates.software")}<Input id={`software-name-${id}`} required maxlength={160} bind:value={source.software} /></label>
            <label for={`software-source-${id}`}>{t("updates.sourceURL")}<Input id={`software-source-${id}`} required type="url" bind:value={source.url} /></label>
            <div><Button variant="primary" type="submit" disabled={busy}>{busy ? t("updates.saving") : t("updates.confirm")}</Button></div>
          </form>
        {/if}
      </section>
    {/if}
    {#if comparable && service.state === "awaiting_ai"}<p class="detail-note">
        {t("updates.awaiting_ai")}{#if session.user?.role === "administrator"}
          · <a href="/reglages#software-analysis">{t("updates.settings")}</a
          >{/if}
      </p>{/if}
    {#if comparable && service.state === "notes_unavailable"}<p role="status">
        {t("updates.notesUnavailable")}
      </p>{/if}
    {#if comparable && service.state === "retry"}<p role="status">
        {limitedHost
          ? t("updates.rateLimitedUntil", {
              host: limitedHost,
              time: service.next_check_at ? stamp(service.next_check_at) : "—",
            })
          : service.last_error.startsWith("invalid_ai_")
            ? t("updates.invalidAI")
            : t("updates.failed")}
      </p>{/if}
    <div class="current-status">
      <span>{t("updates.journalNow")}</span>
      <strong>{service.situation === "current" ? t("updates.current") : t(`updates.group.${service.group}` as MessageKey)}</strong>
    </div>
    <SoftwareUpdateJournal {service} />
  {/if}
</div>

<style>
  .software-detail { display: grid; gap: var(--s4); padding: var(--s4) var(--s5); border-top: 1px solid var(--line); min-width: 0; }
  .detail-meta { color: var(--muted); font-size: var(--text-sm); }
  .source-box { display: grid; gap: var(--s3); padding-block: var(--s2) var(--s4); border-bottom: 1px solid var(--line); min-width: 0; }
  .source-heading { display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: var(--s3); }
  .source-heading > div:first-child { min-width: 0; overflow-wrap: anywhere; }
  .source-heading strong { font-size: var(--text-sm); }
  .source-heading a { display: inline-block; margin-inline-start: var(--s2); font-size: var(--text-sm); }
  .muted { color: var(--muted); font-size: var(--text-sm); }
  .source-form { display: grid; gap: var(--s4); padding-block: var(--s3); max-width: 48rem; }
  label { display: grid; gap: var(--s2); }
  .current-status { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--s2); padding-block: var(--s1); }
  .current-status span { color: var(--muted); font-size: var(--text-xs); }
  .current-status strong { font-size: var(--text-sm); }
  .detail-note { margin: 0; border-left: 2px solid var(--line-strong); padding: var(--s3) var(--s4); background: var(--surface-2); }
  @media (max-width: 48rem) { .software-detail { padding: var(--s4); } }
</style>
