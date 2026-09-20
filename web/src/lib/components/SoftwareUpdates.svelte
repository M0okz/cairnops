<script lang="ts">
  import { api } from "$lib/api";
  import { t } from "$lib/i18n.svelte";
  import { messageFrom } from "$lib/session.svelte";
  import type { SoftwareService } from "$lib/software-updates";
  import SoftwareServiceDetail from "./SoftwareServiceDetail.svelte";
  import SegmentedControl from "./ui/SegmentedControl.svelte";
  import { Input } from "./ui/input";
  let { targetID = "" }: { targetID?: string } = $props();
  let services = $state<SoftwareService[]>([]);
  let loading = $state(true);
  let error = $state("");
  let selected = $state("");
  let search = $state("");
  let filter = $state("all");
  let reload = $state(0);
  const visible = $derived(
    services.filter(
      (s) =>
        s.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()) &&
        (filter === "all" ||
          (filter === "pending"
            ? s.installed_version !== s.target_version
            : s.installed_version === s.target_version)),
    ),
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
  function stateLabel(s: SoftwareService) {
    if (!s.known) return t("updates.unknown");
    switch (s.state) {
      case "awaiting_source":
        return t("updates.awaiting_source");
      case "awaiting_ai":
        return t("updates.awaiting_ai");
      case "ready":
        return t("updates.ready");
      case "up_to_date":
        return t("updates.upToDate");
      case "retry":
        return t("updates.failed");
      default:
        return t("updates.processing");
    }
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
          { value: "all", label: t("updates.all"), count: services.length },
          { value: "pending", label: t("updates.pending") },
          { value: "current", label: t("updates.upToDate") },
        ]}
        onValueChange={(value) => (filter = value)}
      />
    </div>
    {#if !visible.length}<p>{t("updates.noMatches")}</p>{/if}
    {#each visible as service (service.id)}
      <article class="card service-card">
        <div class="service-row">
          <div class="identity">
            <a href={`/cibles/${service.target_id}`}>{service.name}</a><small
              >{stateLabel(service)}</small
            >
          </div>
          <div class="version-pair">
            <span
              ><small>{t("updates.installed")}</small><strong class="mono"
                >{service.installed_version || "—"}</strong
              ></span
            ><span aria-hidden="true">→</span><span
              ><small>{t("updates.target")}</small><strong class="mono"
                >{service.target_version || "—"}</strong
              ></span
            >
          </div>
          <button
            class="btn"
            aria-expanded={selected === service.id}
            aria-controls={`release-detail-${service.id}`}
            onclick={() =>
              (selected = selected === service.id ? "" : service.id)}
            >{selected === service.id
              ? t("updates.close")
              : t("updates.details")}</button
          >
        </div>
        {#if selected === service.id}<div id={`release-detail-${service.id}`}>
            {#key service.id}<SoftwareServiceDetail id={service.id} />{/key}
          </div>{/if}
      </article>
    {/each}
  {/if}
</div>

<style>
  .software-updates {
    display: grid;
    gap: var(--s4);
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
  .service-card {
    overflow: hidden;
  }
  .service-row {
    display: grid;
    grid-template-columns: minmax(10rem, 1fr) minmax(14rem, 1fr) auto;
    align-items: center;
    gap: var(--s5);
    padding: var(--s5);
  }
  .identity {
    min-width: 0;
  }
  .identity a {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  small {
    display: block;
    color: var(--muted);
    font-size: var(--text-xs);
    margin-top: var(--s2);
  }
  .version-pair {
    display: flex;
    gap: var(--s4);
    align-items: center;
    min-width: 0;
  }
  .version-pair > span {
    min-width: 0;
  }
  .version-pair strong {
    display: block;
    overflow-wrap: anywhere;
  }
  .version-pair small {
    margin-bottom: var(--s2);
  }
  @media (max-width: 68rem) {
    .service-row {
      grid-template-columns: 1fr 1fr;
    }
    .service-row > button {
      grid-column: 1/-1;
      justify-self: start;
    }
  }
  @media (max-width: 40rem) {
    .service-row {
      grid-template-columns: 1fr;
      gap: var(--s3);
      padding: var(--s4);
    }
    .service-row > button {
      width: 100%;
    }
  }
</style>
