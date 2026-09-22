<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte';
  let { label, children }: { label: string; children: Snippet } = $props();
  const id = $props.id();
  let trigger: HTMLButtonElement;
  let panel: HTMLDivElement;
  let open = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(timer));
  function close() { clearTimeout(timer); panel?.hidePopover(); }
  function show() {
    clearTimeout(timer);
    if (!panel || !trigger) return;
    panel.showPopover();
    const bounds = trigger.getBoundingClientRect();
    const box = panel.getBoundingClientRect();
    panel.style.left = `${Math.max(12, Math.min(bounds.left, innerWidth - box.width - 12))}px`;
    panel.style.top = `${Math.max(12, bounds.bottom + box.height + 12 > innerHeight ? bounds.top - box.height - 8 : bounds.bottom + 8)}px`;
  }
  function leave() { timer = setTimeout(close, 180); }
</script>
<button bind:this={trigger} class="tooltip-trigger" type="button" aria-expanded={open} aria-controls={id}
  onpointerenter={(event) => { if (event.pointerType === 'mouse') show(); }} onpointerleave={leave}
  onclick={show} onblur={close} onkeydown={(event) => { if (event.key === 'Escape') close(); }}>
  {label}
</button>
<div bind:this={panel} {id} popover="auto" class="resource-tooltip" role="region" aria-label={label}
  ontoggle={(event) => (open = event.newState === 'open')} onpointerenter={() => clearTimeout(timer)} onpointerleave={leave}>
  {@render children()}
</div>
<style>
  .tooltip-trigger { color: var(--ink); background: transparent; border: 1px solid var(--line); border-radius: var(--r-s); padding: var(--s1) var(--s2); min-height: 1.75rem; font: inherit; cursor: pointer; text-align: left; }
  .resource-tooltip { position: fixed; inset: auto; margin: 0; width: min(25rem, calc(100vw - 1.5rem)); max-height: calc(100dvh - 1.5rem); overflow: auto; padding: var(--s4); border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--surface); color: var(--ink); box-shadow: var(--shadow); font-size: var(--text-sm); }
</style>
