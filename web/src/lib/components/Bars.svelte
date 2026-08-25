<script lang="ts">
  /* Micro-graphe des cellules chiffrées : une barre par pas de temps, sous le
   * chiffre qu'elle explique. Trois états, parce qu'il y a trois façons de
   * n'avoir rien à montrer :
   *   — `bars`  : la série existe, chaque barre porte sa hauteur ;
   *   — `slots` : la mesure est un état, pas une histoire (rien n'est stocké
   *               heure par heure) — les emplacements restent vides ;
   *   — `rule`  : il n'y a rien eu à compter, et un trait pointillé le dit.
   *
   * Rendu en SVG plutôt qu'en CSS : la politique de sécurité de l'instance
   * ferme `style-src-attr`, donc aucune hauteur ne peut passer par un attribut
   * style. Les attributs de géométrie SVG, eux, ne sont pas du CSS. */

  const componentId = $props.id();

  let {
    values = [],
    mode = 'bars',
    slots = 24,
    width = 240,
    height = 14,
    tooltips = [],
    label = ''
  }: {
    values?: number[];
    mode?: 'bars' | 'slots' | 'rule';
    slots?: number;
    width?: number;
    height?: number;
    tooltips?: string[];
    label?: string;
  } = $props();

  let hovered = $state<number | null>(null);
  let focused = $state<number | null>(null);

  const round = (value: number) => Math.round(value * 100) / 100;

  /* Une barre ne descend jamais sous un filet : une heure mesurée à presque
     zéro doit rester visible, sinon elle se confond avec une heure absente. */
  const floorHeight = 2;

  const drawn = $derived.by(() => {
    const count = mode === 'slots' ? slots : values.length;
    if (count === 0) return [];

    const gap = count > 16 ? 2 : 3;
    const slot = width / count;
    const bar = Math.max(2, slot - gap);
    const highest = mode === 'slots' ? 1 : Math.max(...values, Number.EPSILON);

    return Array.from({ length: count }, (_, index) => {
      const ratio = mode === 'slots' ? 1 : Math.max(0, values[index]) / highest;
      const drawnHeight = Math.max(floorHeight, round(ratio * height));
      return { x: round(index * slot), y: round(height - drawnHeight), w: round(bar), h: drawnHeight };
    });
  });

  const interactive = $derived(
    mode === 'bars' && values.length > 0 && tooltips.length === values.length
  );
  const active = $derived(hovered ?? focused);

  function dismiss(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    hovered = null;
    focused = null;
    if (event.currentTarget instanceof HTMLElement) event.currentTarget.blur();
    event.stopPropagation();
  }
</script>

<span class="bars-shell">
  <svg
    class="bars {mode}"
    viewBox="0 0 {width} {height}"
    {width}
    {height}
    preserveAspectRatio="none"
    aria-hidden="true"
  >
    {#if mode === 'rule'}
      <line x1="0" y1={height - 1} x2={width} y2={height - 1} vector-effect="non-scaling-stroke" />
    {:else}
      {#each drawn as bar, index (index)}
        <rect x={bar.x} y={bar.y} width={bar.w} height={bar.h} rx="1" />
      {/each}
    {/if}
  </svg>

  {#if interactive}
    <span class="hitboxes" role="group" aria-label={label}>
      {#each tooltips as tooltip, index (`${componentId}-${index}`)}
        <button
          type="button"
          class:active={active === index}
          aria-label={tooltip}
          aria-controls={`${componentId}-${index}`}
          aria-expanded={active === index}
          onpointerenter={() => (hovered = index)}
          onpointerleave={() => hovered === index && (hovered = null)}
          onfocus={() => (focused = index)}
          onblur={() => focused === index && (focused = null)}
          onkeydown={dismiss}
        >
          <span
            class:visible={active === index}
            class="tooltip"
            id={`${componentId}-${index}`}
            role="tooltip"
          >{tooltip}</span>
        </button>
      {/each}
    </span>
  {/if}
</span>

<style>
  .bars-shell {
    position: relative;
    display: block;
    width: 100%;
    height: 0.875rem;
    overflow: visible;
  }

  .bars {
    display: block;
    width: 100%;
    height: 100%;
  }

  rect {
    fill: currentColor;
  }

  /* Des emplacements vides : la forme du graphe est là, la matière non. */
  .slots rect {
    fill: var(--surface-2);
  }

  line {
    stroke: currentColor;
    stroke-width: 1;
    stroke-dasharray: 3 3;
    opacity: 0.4;
  }

  .hitboxes {
    position: absolute;
    inset: calc(-1 * var(--s2)) 0;
    z-index: 1;
    display: flex;
  }

  button {
    position: relative;
    flex: 1 1 0;
    min-width: 0;
    padding: 0;
    border: 0;
    border-radius: var(--r-s);
    background: transparent;
    color: inherit;
    cursor: help;
  }

  button::before {
    position: absolute;
    inset: 0;
    border-radius: var(--r-s);
    background: currentColor;
    content: '';
    opacity: 0;
    transition: opacity var(--d1) var(--ease);
  }

  button:hover::before,
  button.active::before {
    opacity: 0.12;
  }

  button:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: var(--s1);
  }

  .tooltip {
    position: absolute;
    bottom: calc(100% + var(--s2));
    left: 50%;
    z-index: 20;
    width: max-content;
    max-width: min(18rem, calc(100vw - var(--s7)));
    padding: var(--s2) var(--s3);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--surface-3);
    box-shadow: var(--shadow);
    color: var(--ink);
    font-family: var(--font-num);
    font-size: 0.6875rem;
    font-weight: 400;
    line-height: 1.4;
    opacity: 0;
    pointer-events: none;
    transform: translate(-50%, var(--s2));
    transition:
      opacity var(--d1) var(--ease),
      transform var(--d1) var(--ease);
    visibility: hidden;
    white-space: nowrap;
  }

  button:nth-child(-n + 3) .tooltip {
    left: 0;
    transform: translate(0, var(--s2));
  }

  button:nth-last-child(-n + 3) .tooltip {
    right: 0;
    left: auto;
    transform: translate(0, var(--s2));
  }

  .tooltip.visible {
    opacity: 1;
    transform: translate(-50%, 0);
    visibility: visible;
  }

  button:nth-child(-n + 3) .tooltip.visible,
  button:nth-last-child(-n + 3) .tooltip.visible {
    transform: translate(0, 0);
  }

  @media (prefers-reduced-motion: reduce) {
    button::before,
    .tooltip {
      transition: none;
    }
  }
</style>
