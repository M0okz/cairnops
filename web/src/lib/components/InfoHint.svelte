<script lang="ts">
  let focused = $state(false);
  let hovered = $state(false);
  let dismissed = $state(false);

  let {
    id,
    ariaLabel,
    text
  }: {
    id: string;
    ariaLabel: string;
    text: string;
  } = $props();

  let visible = $derived((focused || hovered) && !dismissed);

  function showFromFocus() {
    focused = true;
    dismissed = false;
  }

  function showFromHover() {
    hovered = true;
    dismissed = false;
  }

  function dismiss(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    dismissed = true;
    event.stopPropagation();
  }
</script>

<span
  class="info-hint"
  role="group"
  onmouseenter={showFromHover}
  onmouseleave={() => (hovered = false)}
>
  <button
    type="button"
    aria-label={ariaLabel}
    aria-describedby={id}
    aria-controls={id}
    aria-expanded={visible}
    onfocus={showFromFocus}
    onblur={() => (focused = false)}
    onkeydown={dismiss}
  >i</button>
  <span class:visible class="tooltip" {id} role="tooltip">{text}</span>
</span>

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
    position: absolute;
    top: 100%;
    left: 0;
    z-index: 20;
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
