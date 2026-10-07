<script lang="ts">
  import { RadioGroup as RadioGroupPrimitive } from 'bits-ui';
  import type { Snippet } from 'svelte';

  /* Une option d'un RadioGroup, dessinée comme Checkbox : la pastille garde la
   * taille des cases à cocher, la cible reste celle d'un contrôle de choix.
   * Sans libellé, l'option tire son nom d'un <label> qui l'entoure. */

  let {
    value,
    disabled = false,
    class: className = '',
    children
  }: {
    value: string;
    disabled?: boolean;
    class?: string;
    children?: Snippet;
  } = $props();
</script>

<RadioGroupPrimitive.Item {value} {disabled}>
  {#snippet child({ props })}
    <button {...props} class={`radio ${className}`.trim()}>
      <span class="mark" aria-hidden="true"><span class="dot"></span></span>
      {@render children?.()}
    </button>
  {/snippet}
</RadioGroupPrimitive.Item>

<style>
  .radio {
    display: inline-flex;
    align-items: center;
    justify-content: flex-start;
    gap: var(--s3);
    min-width: 0;
    min-height: var(--choice-hit-area);
    margin: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .radio:disabled {
    cursor: default;
    opacity: var(--choice-disabled-opacity);
  }

  .mark {
    display: grid;
    flex: 0 0 var(--checkbox-size);
    width: var(--checkbox-size);
    height: var(--checkbox-size);
    place-items: center;
    border: var(--line-width) solid var(--line-strong);
    border-radius: var(--r-pill);
    background: var(--surface);
  }

  .dot {
    width: 50%;
    height: 50%;
    border-radius: var(--r-pill);
    background: var(--bg);
    opacity: 0;
    scale: var(--choice-icon-hidden-scale);
  }

  .radio[data-state='checked'] .mark {
    border-color: var(--ink);
    background: var(--ink);
  }

  .radio[data-state='checked'] .dot {
    opacity: 1;
    scale: 1;
  }

  @media (prefers-reduced-motion: no-preference) {
    .mark {
      transition-property: border-color, background-color;
      transition-duration: var(--d1);
      transition-timing-function: var(--ease);
    }

    .dot {
      transition-property: opacity, scale;
      transition-duration: var(--d1);
      transition-timing-function: var(--choice-ease);
    }
  }
</style>
