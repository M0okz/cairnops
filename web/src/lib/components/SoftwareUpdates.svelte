<script lang="ts">
  import { api } from "$lib/api";
  import { t, type MessageKey } from "$lib/i18n.svelte";
  import { messageFrom } from "$lib/session.svelte";
  import { stamp } from "$lib/format";
  import {
    compareServices,
    rateLimitedHost,
    serviceTitle,
    updateGroups,
    type SoftwareService,
    type UpdateGroup,
  } from "$lib/software-updates";
  import SoftwareServiceDetail from "./SoftwareServiceDetail.svelte";
  import SegmentedControl from "./ui/SegmentedControl.svelte";
  import { Input } from "./ui/input";
  let { targetID = "" }: { targetID?: string } = $props();
  let services = $state<SoftwareService[]>([]);
  let loading = $state(true);
  let error = $state("");
  let selected = $state("");
  let search = $state("");
  let filter = $state<UpdateGroup | "all">("apply");
  let reload = $state(0);
  // Sur la fiche d'une Ressource, quelques services se lisent sans filtre.
  const compact = $derived(targetID !== "" && services.length <= 3);
  const ordered = $derived([...services].sort(compareServices));
  const counts = $derived(
    Object.fromEntries(
      updateGroups.map((group) => [
        group,
        services.filter((s) => s.group === group).length,
      ]),
    ) as Record<UpdateGroup, number>,
  );
  const matching = $derived(
    ordered.filter((s) => {
      const query = search.trim().toLocaleLowerCase();
      return (
        !query ||
        serviceTitle(s).toLocaleLowerCase().includes(query) ||
        s.name.toLocaleLowerCase().includes(query)
      );
    }),
  );
  const sections = $derived(
    (compact || filter === "all" ? updateGroups : [filter]).map((group) => ({
      group,
      items: matching.filter((s) => s.group === group),
    })),
  );
  $effect(() => {
    const target = targetID;
    reload;
    let alive = true;
    let running = false;
    async function load() {
      if (running) return;
      running = true;
      try {
        const data = await api<{ services: SoftwareService[] }>(
          `/api/v1/software-updates${target ? "?target_id=" + encodeURIComponent(target) : ""}`,
        );
        if (alive) {
          services = data.services;
          error = "";
        }
      } catch (e) {
        if (alive) error = messageFrom(e);
      } finally {
        if (alive) loading = false;
        running = false;
      }
    }
    void load();
    const timer = setInterval(() => {
      if (!document.hidden) void load();
    }, 30000);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  });
  /** Pourquoi le service est dans son groupe, puis où en est l'analyse des notes. */
  function statusLabel(s: SoftwareService): string {
    if (s.group === "review") {
      if (!s.known)
        return t(`updates.issue.${s.verification_issue ?? "unconfirmed"}` as MessageKey);
      return t(`updates.situation.${s.situation}` as MessageKey);
    }
    if (s.group === "current")
      return s.skipped ? t("updates.skipped") : t("updates.current");
    const host = rateLimitedHost(s);
    if (host)
      return t("updates.rateLimitedUntil", {
        host,
        time: s.next_check_at ? stamp(s.next_check_at) : "—",
      });
    switch (s.state) {
      case "awaiting_source":
        return t("updates.awaiting_source");
      case "awaiting_ai":
        return t("updates.awaiting_ai");
      case "ready":
        return t("updates.ready");
      case "notes_unavailable":
        return t("updates.notesUnavailable");
      case "retry":
        return t("updates.failed");
      default:
        return t("updates.processing");
    }
  }
  function levelLabel(s: SoftwareService): string | undefined {
    if (s.situation === "prerelease") return t("updates.level.prerelease");
    if (s.situation === "update" && s.level)
      return t(`updates.level.${s.level}` as MessageKey);
  }
</script>

<div class="software-updates">
  {#if error}<div role="alert">
      {error}
      <button class="btn" onclick={() => reload++}>{t("updates.retry")}</button>
    </div>{/if}
  {#if loading}<p role="status">{t("updates.loading")}</p>
  {:else if !services.length}<div class="card empty">
      <strong>{t("updates.empty")}</strong>
      <p>{t("updates.emptyHint")}</p>
      <a href="/connecteurs/argus">Argus ↗</a>
    </div>
  {:else}
    {#if !compact}
      <div class="update-tools">
        <div class="shadcn-control">
          <Input
            type="search"
            aria-label={t("updates.search")}
            placeholder={t("updates.search")}
            bind:value={search}
          />
        </div>
        <SegmentedControl
          value={filter}
          label={t("updates.title")}
          items={[
            ...updateGroups.map((group) => ({
              value: group,
              label: t(`updates.group.${group}`),
              count: counts[group],
            })),
            { value: "all" as const, label: t("updates.all"), count: services.length },
          ]}
          onValueChange={(value) => (filter = value)}
        />
      </div>
    {/if}
    {#if !sections.some((section) => section.items.length)}
      <p class="card empty-group">
        {filter !== "all" && !search.trim()
          ? t(`updates.none.${filter}`)
          : t("updates.noMatches")}
      </p>
    {:else}
      {#each sections as section (section.group)}
        {#if section.items.length}
          <section class="update-section" aria-labelledby={`updates-${section.group}`}>
            <h2
              id={`updates-${section.group}`}
              class:visually-hidden={!compact && filter !== "all"}
            >
              {t(`updates.group.${section.group}`)}
              <span class="count num">{section.items.length}</span>
            </h2>
            <ul class="card update-list">
              {#each section.items as service (service.id)}
                {@const level = levelLabel(service)}
                <li class="update-row">
                  <div class="row-main">
                    <div class="identity">
                      <strong class="service-name"
                        >{#each serviceTitle(service).split("/") as part, index}{#if index}/<wbr />{/if}{part}{/each}</strong
                      >
                      {#if service.resource_name && service.resource_name !== service.name}<a class="resource-link" href={`/cibles/${service.target_id}`} title={service.resource_name}>{service.resource_name}</a>{:else}<a class="resource-link" href={`/cibles/${service.target_id}`}>{t("updates.openResource")}</a>{/if}
                    </div>
                    <div class="versions">
                      <span class="version-pair mono" class:unverified={!service.known}
                        ><span class="visually-hidden">{t("updates.installed")}</span><span class="version-value">{service.installed_version || "—"}</span> <span aria-hidden="true" class="arrow">→</span> <span class="visually-hidden">{t("updates.target")}</span><span class="version-value">{service.target_version || "—"}</span></span
                      >
                      {#if level || service.security_mentioned || (service.approved && service.group === "apply")}
                        <span class="tags">
                          {#if level}<span class="pill">{level}</span>{/if}
                          {#if service.security_mentioned}<span class="pill" title={t("updates.securityHint")}>{t("updates.securityMentioned")}</span>{/if}
                          {#if service.approved && service.group === "apply"}<span class="pill">{t("updates.approved")}</span>{/if}
                        </span>
                      {/if}
                    </div>
                    <p class="status" class:attention={service.group === "review"}>{statusLabel(service)}</p>
                    <button
                      class="btn"
                      aria-expanded={selected === service.id}
                      aria-controls={`release-detail-${service.id}`}
                      onclick={() => (selected = selected === service.id ? "" : service.id)}
                      >{selected === service.id ? t("updates.close") : t("updates.details")}</button
                    >
                  </div>
                  {#if selected === service.id}<div id={`release-detail-${service.id}`}>
                      {#key service.id}<SoftwareServiceDetail id={service.id} />{/key}
                    </div>{/if}
                </li>
              {/each}
            </ul>
          </section>
        {/if}
      {/each}
    {/if}
  {/if}
</div>

<style>
  .software-updates,
  .update-section {
    display: grid;
    gap: var(--s4);
  }
  .software-updates {
    gap: var(--s5);
  }
  .update-tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--s4);
  }
  .update-tools > .shadcn-control {
    flex: 1;
    min-width: 12rem;
    max-width: 28rem;
  }
  h2 {
    display: flex;
    align-items: baseline;
    gap: var(--s3);
    font-size: var(--text-md);
    font-weight: 600;
  }
  .count {
    color: var(--muted);
    font-size: var(--text-sm);
    font-weight: 500;
  }
  .update-list {
    list-style: none;
    margin: 0;
    padding: 0;
    overflow: hidden;
    /* Une grille commune garde les colonnes alignées d'une ligne à l'autre,
       y compris lorsqu'une ligne ouverte change le libellé de son bouton. */
    display: grid;
    grid-template-columns: minmax(0, 1.25fr) minmax(0, 1.1fr) minmax(0, 1.3fr) max-content;
  }
  .update-row,
  .row-main {
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: subgrid;
  }
  .update-row > div:not(.row-main) {
    grid-column: 1 / -1;
  }
  .update-row + .update-row {
    border-top: 1px solid var(--line);
  }
  .row-main {
    align-items: center;
    column-gap: var(--s5);
    padding: var(--s4) var(--s5);
  }
  .row-main > button {
    justify-self: end;
  }
  .update-list,
  .update-row {
    column-gap: var(--s5);
  }
  .identity {
    display: grid;
    gap: var(--s1);
    min-width: 0;
  }
  .service-name {
    font-weight: 600;
    overflow-wrap: break-word;
  }
  .resource-link {
    color: var(--muted);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    color: var(--muted);
    overflow-wrap: anywhere;
  }
  .status {
    font-size: var(--text-sm);
    line-height: 1.45;
  }
  .status.attention {
    color: var(--ink);
  }
  .versions {
    display: grid;
    gap: var(--s2);
    min-width: 0;
  }
  .version-pair {
    font-weight: 600;
  }
  /* Une version se lit d'un bloc ; seule une version plus large que sa
     colonne peut encore se couper. */
  .version-value {
    display: inline-block;
    max-width: 100%;
    overflow-wrap: anywhere;
  }
  .version-pair.unverified {
    color: var(--muted);
  }
  .arrow {
    color: var(--muted);
    font-weight: 400;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2);
  }
  .empty-group {
    padding: var(--s5);
    color: var(--muted);
  }
  @media (max-width: 68rem) {
    .update-list,
    .update-row {
      grid-template-columns: minmax(0, 1fr);
    }
    .row-main {
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
      gap: var(--s3) var(--s5);
    }
    .row-main > button {
      justify-self: end;
    }
  }
  @media (max-width: 40rem) {
    .row-main {
      grid-template-columns: minmax(0, 1fr);
      padding: var(--s4);
    }
    .row-main > button {
      justify-self: stretch;
    }
  }
</style>
