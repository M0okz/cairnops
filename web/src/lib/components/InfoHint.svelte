<script lang="ts">
  import { Tooltip } from 'bits-ui';

  /* Aide contextuelle d'un champ ou d'un repère. Le Tooltip de Bits UI l'ouvre
   * au survol comme au clavier, la referme sur Échap avant toute couche
   * parente, et la garde dans l'écran. Un toucher la laisse ouverte : sur
   * mobile, le tap est le seul moyen de la lire. */

  let {
    id,
    ariaLabel,
    text
  }: {
    id: string;
    ariaLabel: string;
    text: string;
  } = $props();
</script>

<Tooltip.Provider delayDuration={0} disableCloseOnTriggerClick>
  <Tooltip.Root>
    <span class="info-hint">
      <Tooltip.Trigger>
        {#snippet child({ props })}
          <button {...props} type="button" aria-label={ariaLabel} aria-describedby={id}>i</button>
        {/snippet}
      </Tooltip.Trigger>
    </span>
    <Tooltip.Content side="bottom" align="start" sideOffset={4} collisionPadding={16} forceMount>
      {#snippet child({ wrapperProps, props, open })}
        <div {...wrapperProps}>
          <span {...props} class="tooltip" class:visible={open} {id} role="tooltip">{text}</span>
        </div>
      {/snippet}
    </Tooltip.Content>
  </Tooltip.Root>
</Tooltip.Provider>

<style>
  .info-hint {
    position: relative;
    z-index: 3;
    display: inline-flex;
    flex: 0 0 auto;
  }

  button {
    display: inline-grid;
    place-items: center;
    width: 1.5rem;
    height: 1.5rem;
    padding: 0;
    border: 1px solid var(--line-strong);
    border-radius: 50%;
    background: transparent;
    color: var(--faint);
    font-family: var(--font-num);
    font-size: 0.625rem;
    font-weight: 700;
    line-height: 1;
    cursor: help;
  }

  button:hover {
    border-color: var(--accent);
    color: var(--accent);
  }

  button:focus-visible {
    border-color: var(--accent);
    color: var(--accent);
    outline: 2px solid var(--accent-soft);
    outline-offset: 2px;
  }

  .tooltip {
    display: block;
    width: min(19rem, calc(100vw - 4rem));
    padding: 0.5rem 0.625rem;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--surface-3);
    box-shadow: var(--shadow);
    color: var(--ink);
    font-size: 0.6875rem;
    font-weight: 400;
    line-height: 1.45;
    opacity: 0;
    pointer-events: none;
    transform: translateY(-0.25rem);
    transition:
      opacity 120ms ease,
      transform 120ms ease;
  }

  .tooltip.visible {
    opacity: 1;
    pointer-events: auto;
    transform: translateY(0);
  }

  @media (prefers-reduced-motion: reduce) {
    .tooltip {
      transition: none;
    }
  }
</style>
