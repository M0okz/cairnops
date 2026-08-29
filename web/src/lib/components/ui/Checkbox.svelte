<script lang="ts">
  import { Checkbox as CheckboxPrimitive } from 'bits-ui';
  import type { Snippet } from 'svelte';

  type CommonProps = {
    checked?: boolean;
    indeterminate?: boolean;
    disabled?: boolean;
    readonly?: boolean;
    required?: boolean;
    name?: string;
    value?: string;
    id?: string;
    variant?: 'label' | 'control' | 'row' | 'selection';
    class?: string;
    onCheckedChange?: (checked: boolean) => void;
  };

  type Props = CommonProps &
    (
      | { children: Snippet; ariaLabel?: string }
      | { children?: never; ariaLabel: string }
    );

  let {
    checked = $bindable(false),
    indeterminate = $bindable(false),
    disabled = false,
    readonly = false,
    required = false,
    name,
    value,
    id,
    variant = 'label',
    class: className = '',
    ariaLabel,
    onCheckedChange,
    children
  }: Props = $props();
</script>

<CheckboxPrimitive.Root
  bind:checked
  bind:indeterminate
  {disabled}
  {readonly}
  {required}
  {name}
  {value}
  {id}
  aria-label={ariaLabel}
  {onCheckedChange}
>
  {#snippet child({ props })}
    <button
      {...props}
      class:control={variant === 'control'}
      class:row={variant === 'row'}
      class:selection={variant === 'selection'}
      class={`checkbox ${className}`.trim()}
    >
      <span class="mark" aria-hidden="true">
        <svg class="check" viewBox="0 0 16 16">
          <path d="m3.25 8.25 3 3 6.5-6.5" />
        </svg>
        <svg class="minus" viewBox="0 0 16 16">
          <path d="M3.5 8h9" />
        </svg>
      </span>
      {@render children?.()}
    </button>
  {/snippet}
</CheckboxPrimitive.Root>

<style>
  .checkbox {
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
  }

  .checkbox.control {
    width: var(--choice-hit-area);
    justify-content: center;
  }

  .checkbox.row {
    width: 100%;
    padding: var(--s3) var(--s4);
    gap: var(--s4);
  }

  .checkbox.selection {
    display: grid;
    width: 100%;
    grid-template-columns: var(--choice-selection-columns, var(--choice-hit-area) minmax(0, 1fr));
    align-items: center;
    gap: var(--s3);
    min-height: var(--choice-row-min-height);
    padding: var(--s3) var(--s4);
    border-bottom: var(--line-width) solid var(--line-row);
  }

  .checkbox.selection:last-child {
    border-bottom: 0;
  }

  .checkbox:disabled {
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
    border-radius: var(--r-s);
    background: var(--surface);
    color: var(--bg);
  }

  svg {
    grid-area: 1 / 1;
    width: 100%;
    height: 100%;
    fill: none;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: var(--choice-icon-stroke);
    opacity: 0;
    scale: var(--choice-icon-hidden-scale);
    filter: blur(var(--choice-icon-blur));
    pointer-events: none;
  }

  .checkbox[data-state='checked'] .mark,
  .checkbox[data-state='indeterminate'] .mark {
    border-color: var(--ink);
    background: var(--ink);
  }

  .checkbox[data-state='checked'] .check,
  .checkbox[data-state='indeterminate'] .minus {
    opacity: 1;
    scale: 1;
    filter: blur(0);
  }

  @media (prefers-reduced-motion: no-preference) {
    .mark {
      transition-property: border-color, background-color;
      transition-duration: var(--d1);
      transition-timing-function: var(--ease);
    }

    svg {
      transition-property: opacity, scale, filter;
      transition-duration: var(--d1);
      transition-timing-function: var(--choice-ease);
    }
  }
</style>
