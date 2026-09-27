<script lang="ts">
  import { onMount } from "svelte";
  import { t } from "$lib/i18n.svelte";
  import { serviceTitle, type SoftwareService } from "$lib/software-updates";
  import Icon from "./Icon.svelte";
  import SoftwareServiceDetail from "./SoftwareServiceDetail.svelte";

  let { service, onclose }: { service: SoftwareService; onclose: () => void } = $props();
  let dialog: HTMLDialogElement;
  let closeButton: HTMLButtonElement;

  onMount(() => {
    dialog.showModal();
    closeButton.focus();
    return () => {
      if (dialog.open) dialog.close();
    };
  });
</script>

<dialog
  bind:this={dialog}
  class="software-drawer"
  aria-labelledby="software-drawer-title"
  oncancel={(event) => { event.preventDefault(); onclose(); }}
  onclick={(event) => event.currentTarget === event.target && onclose()}
>
  <header class="drawer-head">
    <div class="title-copy">
      <span class="eyebrow">{t("updates.title")}</span>
      <h2 id="software-drawer-title">{serviceTitle(service)}</h2>
      {#if service.resource_name && service.resource_name !== service.name}
        <p>{service.resource_name}</p>
      {/if}
      <p class="version-pair mono">
        <span class="visually-hidden">{t("updates.installed")}</span>{service.installed_version || "—"}
        <span aria-hidden="true">→</span>
        <span class="visually-hidden">{t("updates.target")}</span>{service.target_version || "—"}
      </p>
    </div>
    <button bind:this={closeButton} class="close" type="button" aria-label={t("updates.close")} onclick={onclose}>
      <Icon name="close" size={16} />
    </button>
  </header>
  <div class="drawer-body">
    <SoftwareServiceDetail id={service.id} />
  </div>
</dialog>

<style>
  .software-drawer {
    position: fixed;
    inset-block: 0;
    inset-inline-start: auto;
    inset-inline-end: 0;
    margin: 0;
    width: min(100%, var(--incident-drawer-width));
    max-width: none;
    height: 100vh;
    height: 100dvh;
    max-height: none;
    padding: 0;
    border: 0;
    border-inline-start: 1px solid var(--line-strong);
    border-radius: 0;
    background: var(--surface);
    color: var(--ink);
    box-shadow: var(--shadow);
    overflow: hidden;
    overscroll-behavior: contain;
  }
  .software-drawer[open] { display: flex; flex-direction: column; }
  .software-drawer::backdrop { background: var(--drawer-backdrop); }
  .drawer-head {
    flex: none;
    display: flex;
    align-items: flex-start;
    gap: var(--s4);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line);
    background: var(--surface);
  }
  .title-copy { flex: 1; min-width: 0; }
  .eyebrow { color: var(--faint); font-size: var(--text-xs); font-weight: var(--weight-semibold); }
  h2 { margin-block: var(--s1); font-size: var(--text-md); overflow-wrap: anywhere; }
  .title-copy p { color: var(--muted); font-size: var(--text-sm); }
  .version-pair { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--s2); margin-top: var(--s2); font-weight: var(--weight-semibold); }
  .close {
    display: grid;
    flex: none;
    place-items: center;
    width: var(--ctl-h-lg);
    height: var(--ctl-h-lg);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-button);
    background: none;
    color: var(--muted);
    cursor: pointer;
  }
  .close:hover { background: var(--surface-2); color: var(--ink); }
  .drawer-body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--bg);
  }
  .drawer-body :global(.software-detail) { background: var(--bg); }
  @media (max-width: 48rem) {
    .drawer-head { padding: var(--s4); }
  }
</style>
