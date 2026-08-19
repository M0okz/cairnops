<script lang="ts">
  /* Micro-graphe des cellules chiffrées : une barre par heure, sous le chiffre
   * qu'elle explique. Trois états, parce qu'il y a trois façons de n'avoir rien
   * à montrer :
   *   — `bars`  : la série existe, chaque barre porte sa hauteur ;
   *   — `slots` : la mesure est un état, pas une histoire (rien n'est stocké
   *               heure par heure) — les emplacements restent vides ;
   *   — `rule`  : il n'y a rien eu à compter, et un trait pointillé le dit.
   *
   * Rendu en SVG plutôt qu'en CSS : la politique de sécurité de l'instance
   * ferme `style-src-attr`, donc aucune hauteur ne peut passer par un attribut
   * style. Les attributs de géométrie SVG, eux, ne sont pas du CSS. */

  let {
    values = [],
    mode = 'bars',
    slots = 24,
    width = 240,
    height = 14
  }: {
    values?: number[];
    mode?: 'bars' | 'slots' | 'rule';
    slots?: number;
    width?: number;
    height?: number;
  } = $props();

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
</script>

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

<style>
  .bars {
    display: block;
    width: 100%;
    height: 0.875rem;
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
</style>
