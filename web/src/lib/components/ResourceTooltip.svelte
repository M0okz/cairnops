<script lang="ts">
  import { Popover } from 'bits-ui';
  import type { Snippet } from 'svelte';

  /* Détail court d'une ligne de Ressource (problèmes supplémentaires,
   * Contrôles). Il s'ouvre au survol, au clic, au clavier et au toucher ; le
   * Popover de Bits UI le place dans l'écran, laisse la souris rejoindre le
   * panneau, qui peut contenir des liens, et le referme sur Échap ou un clic
   * ailleurs. */

  let { label, children }: { label: string; children: Snippet } = $props();
</script>

<Popover.Root>
  <Popover.Trigger openOnHover openDelay={0} closeDelay={180}>
    {#snippet child({ props })}
      <button {...props} class="tooltip-trigger" type="button">{label}</button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Content sideOffset={8} align="start" collisionPadding={12} trapFocus={false} onOpenAutoFocus={(event) => event.preventDefault()}>
    {#snippet child({ wrapperProps, props, open })}
      {#if open}
        <div {...wrapperProps}>
          <div {...props} class="resource-tooltip" role="region" aria-label={label}>
            {@render children()}
          </div>
        </div>
      {/if}
    {/snippet}
  </Popover.Content>
</Popover.Root>

<style>
  .tooltip-trigger { color: var(--ink); background: transparent; border: 1px solid var(--line); border-radius: var(--r-s); padding: var(--s1) var(--s2); min-height: 1.75rem; font: inherit; font-family: var(--font-num); font-variant-numeric: tabular-nums; cursor: pointer; text-align: left; }
  .resource-tooltip { width: min(25rem, calc(100vw - 1.5rem)); max-height: min(calc(100dvh - 1.5rem), var(--bits-floating-available-height, 100dvh)); overflow: auto; padding: var(--s4); border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--surface); color: var(--ink); box-shadow: var(--shadow); font-size: var(--text-sm); }
</style>
