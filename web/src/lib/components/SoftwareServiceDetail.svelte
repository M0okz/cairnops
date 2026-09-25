<script lang="ts">
  import { api } from "$lib/api";
  import { t, type MessageKey } from "$lib/i18n.svelte";
  import { session, messageFrom } from "$lib/session.svelte";
  import { stamp } from "$lib/format";
  import {
    currentAnalysis,
    rateLimitedHost,
    safeReleaseURL,
    type ReleaseSource,
    type SoftwareService,
    type VersionEvent,
  } from "$lib/software-updates";
  import ReleasePoints from "./ReleasePoints.svelte";
  import SegmentedControl from "./ui/SegmentedControl.svelte";
  import { Input } from "./ui/input";
  import { Button } from "./ui/button";
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
  const analysis = $derived(service ? currentAnalysis(service) : undefined);
  const previous = $derived(
    service?.analyses.filter((a) => a.id !== analysis?.id) ?? [],
  );
  const missing = $derived(
    service?.collection?.notes.filter((n) => n.missing) ?? [],
  );
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
  function eventLabel(event: VersionEvent) {
    const key =
      event.kind === "installed"
        ? `updates.event.${event.direction ?? "changed"}`
        : `updates.event.${event.kind}`;
    return t(key as MessageKey, {
      version: event.version,
      previous: event.previous ?? "—",
      target: event.target ?? "—",
    });
  }
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
      <details class="source-box" open={!effectiveSource}>
        <summary
          >{t("updates.source")}{#if effectiveSource}
            · {effectiveSource.software}{/if}</summary
        >
        {#if effectiveSource}
          <p class="muted">
            {service.source_origin === "argus" || !service.confirmed_at
              ? t("updates.argusSource")
              : t("updates.customSource")}
          </p>
          <a
            href={safeReleaseURL(effectiveSource.url)}
            target="_blank"
            rel="noreferrer noopener">{effectiveSource.url} ↗</a
          >
          {#if session.user?.role === "administrator" && !editingSource}
            <div class="shadcn-control source-actions">
              <Button
                variant="outline"
                onclick={() => {
                  source = { ...effectiveSource };
                  editingSource = true;
                }}>{t("updates.editSource")}</Button
              >
            </div>
          {/if}
        {/if}
        {#if session.user?.role === "administrator" && (editingSource || !effectiveSource)}
          <form onsubmit={confirm} class="shadcn-control source-form">
            <p class="muted">{t("updates.sourceHint")}</p>
            <SegmentedControl
              label={t("updates.sourceKind")}
              value={source.kind}
              items={[
                { value: "github", label: "GitHub" },
                { value: "forgejo", label: "Forgejo / Gitea" },
                { value: "gitlab", label: "GitLab" },
                { value: "changelog", label: "Changelog" },
              ]}
              onValueChange={(value) => (source.kind = value)}
            />
            <label for={`software-name-${id}`}
              >{t("updates.software")}<Input
                id={`software-name-${id}`}
                required
                maxlength={160}
                bind:value={source.software}
              /></label
            >
            <label for={`software-source-${id}`}
              >{t("updates.sourceURL")}<Input
                id={`software-source-${id}`}
                required
                type="url"
                bind:value={source.url}
              /></label
            >
            <div>
              <Button type="submit" disabled={busy}
                >{busy ? t("updates.saving") : t("updates.confirm")}</Button
              >
            </div>
          </form>
        {/if}
      </details>
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
    {#if service.collection && service.collection_revision !== service.revision}<p
        class="muted"
      >
        {t("updates.oldNotes")} · {service.collection.installed_version} → {service
          .collection.target_version}
      </p>{/if}
    {#if missing.length || service.collection?.incomplete}
      <aside class="partial">
        <strong>{t("updates.partial")}</strong>{#if missing.length}<p>
            {t("updates.missing")} : {missing.map((n) => n.version).join(", ")}
          </p>{/if}{#if service.collection?.incomplete}<p>
            {t("updates.catalogueIncomplete")}
          </p>{/if}
      </aside>
    {/if}
    {#if analysis || comparable || service.situation === "current"}<section>
      <div class="section-title">
        <h3>{t("updates.overview")}</h3>
        {#if analysis}<small class="muted"
            >{t("updates.summaryFrench")} · {stamp(analysis.created_at)}</small
          >{/if}
      </div>
      {#if analysis}<ReleasePoints
          points={analysis.result.overview}
          notes={service.collection?.notes}
          source={analysis.source}
        />{#if !analysis.result.overview.length}<p class="muted">
            {t("updates.noChanges")}
          </p>{/if}
      {:else if service.situation === "current"}<p>
          {t("updates.current")}
        </p>{:else if comparable}<p class="muted">{t("updates.noSummary")}</p>{/if}
    </section>{/if}
    {#if service.collection?.notes.length}
      <section>
        <h3>{t("updates.byVersion")}</h3>
        {#each service.collection.notes as note (note.version)}
          <details class="release">
            <summary
              ><strong class="mono">{note.version}</strong
              >{#if note.missing}<span class="muted"
                  >{t("updates.missing")}</span
                >{/if}</summary
            >
            {#if analysis}<ReleasePoints
                points={analysis.result.details.filter(
                  (p) => p.version === note.version,
                )}
                notes={[note]}
                source={analysis.source}
              />{/if}
            <a
              href={safeReleaseURL(note.url)}
              target="_blank"
              rel="noreferrer noopener">{t("updates.original")} ↗</a
            >
            {#if note.body}<pre class="release-body">{note.body}</pre>{/if}
          </details>
        {/each}
      </section>
    {/if}
    {#if previous.length}
      <details>
        <summary>{t("updates.previous")} · {previous.length}</summary>
        {#each previous as old (old.id)}<details class="release">
            <summary
              >{old.installed_version} → {old.target_version} · {stamp(
                old.created_at,
              )}</summary
            >
            <p class="muted">{t("updates.previousHint")}</p>
            <ReleasePoints
              points={old.result.overview}
              source={old.source}
              notes={old.notes}
            /><ReleasePoints
              points={old.result.details}
              source={old.source}
              notes={old.notes}
            />
          </details>{/each}
      </details>
    {/if}
    <details>
      <summary>{t("updates.history")} · {service.events.length}</summary>
      <p class="muted">{t("updates.historyHint")}</p>
      <ol class="history">
        {#each service.events as event, i (i)}<li class:secondary={event.kind === "target"}>
            <time datetime={event.observed_at}>{stamp(event.observed_at)}</time
            ><span>{eventLabel(event)}</span>
          </li>{/each}
      </ol>
    </details>
  {/if}
</div>

<style>
  .software-detail {
    display: grid;
    gap: var(--s6);
    padding: var(--s6);
    border-top: 1px solid var(--line);
  }
  .detail-meta,
  .section-title {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--s3);
  }
  .detail-meta {
    font-size: var(--text-xs);
    color: var(--muted);
  }
  .source-actions {
    margin-top: var(--s3);
  }
  .source-box {
    overflow-wrap: anywhere;
  }
  .source-form {
    display: grid;
    gap: var(--s4);
    padding-block: var(--s4);
    max-width: 48rem;
  }
  label {
    display: grid;
    gap: var(--s2);
  }
  summary {
    cursor: pointer;
    padding-block: var(--s3);
  }
  summary:focus-visible {
    outline: 2px solid var(--ink);
    outline-offset: 2px;
  }
  .release {
    border-bottom: 1px solid var(--line);
    padding-block: var(--s2);
  }
  .release summary span {
    margin-inline-start: var(--s3);
    font-size: var(--text-sm);
  }
  .release-body {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-family: inherit;
    font-size: var(--text-sm);
    color: var(--muted);
    max-height: 30rem;
    overflow: auto;
    padding-block: var(--s4);
  }
  .partial {
    border-left: 2px solid var(--line-strong);
    padding: var(--s4);
    background: var(--surface-2);
  }
  .partial p {
    margin-top: var(--s2);
  }
  .history {
    list-style: none;
    padding: 0;
  }
  .history li {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s4);
    padding-block: var(--s3);
    border-bottom: 1px solid var(--line);
    font-size: var(--text-sm);
  }
  .history time {
    color: var(--muted);
  }
  .history li.secondary span {
    color: var(--muted);
  }
  .detail-note {
    margin: 0;
    border-left: 2px solid var(--line-strong);
    padding: var(--s3) var(--s4);
    background: var(--surface-2);
  }
  @media (max-width: 48rem) {
    .software-detail {
      padding: var(--s4);
    }
    .history li {
      display: grid;
      gap: var(--s2);
    }
  }
</style>
