<script lang="ts">
  import { RadioGroup as RadioGroupPrimitive } from 'bits-ui';
  import type { Snippet } from 'svelte';

  /* Groupe de choix exclusifs, fondé sur le RadioGroup de Bits UI : flèches
   * pour passer d'une option à l'autre, une seule étape de tabulation pour le
   * groupe. Quand la mise en page impose son propre conteneur (une grille de
   * cartes, par exemple), l'appelant le dessine avec `child` et y étale les
   * attributs reçus. */

  let {
    value = $bindable(''),
    name,
    label,
    orientation = 'vertical',
    class: className = '',
    onValueChange,
    child,
    children
  }: {
    value?: string;
    name?: string;
    label?: string;
    orientation?: 'vertical' | 'horizontal';
    class?: string;
    onValueChange?: (value: string) => void;
    child?: Snippet<[{ props: Record<string, unknown> }]>;
    children?: Snippet;
  } = $props();
</script>

<RadioGroupPrimitive.Root bind:value {name} {orientation} {onValueChange} aria-label={label}>
  {#snippet child({ props })}
    {#if child}
      {@render child({ props })}
    {:else}
      <div {...props} class={`radio-group ${className}`.trim()}>
        {@render children?.()}
      </div>
    {/if}
  {/snippet}
</RadioGroupPrimitive.Root>

<style>
  .radio-group {
    display: grid;
    gap: var(--s1);
  }
</style>
