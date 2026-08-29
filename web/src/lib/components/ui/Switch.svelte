<script lang="ts">
  import { Switch as SwitchPrimitive } from 'bits-ui';

  let {
    checked = $bindable(false),
    disabled = false,
    required = false,
    name,
    value,
    id,
    label,
    onCheckedChange
  }: {
    checked?: boolean;
    disabled?: boolean;
    required?: boolean;
    name?: string;
    value?: string;
    id?: string;
    label: string;
    onCheckedChange?: (checked: boolean) => void;
  } = $props();
</script>

<SwitchPrimitive.Root
  bind:checked
  {disabled}
  {required}
  {name}
  {value}
  {id}
  aria-label={label}
  {onCheckedChange}
>
  {#snippet child({ props })}
    <button {...props} class="switch">
      <span class="track" aria-hidden="true">
        <SwitchPrimitive.Thumb>
          {#snippet child({ props: thumbProps })}
            <span {...thumbProps} class="thumb"></span>
          {/snippet}
        </SwitchPrimitive.Thumb>
      </span>
    </button>
  {/snippet}
</SwitchPrimitive.Root>

<style>
  .switch {
    display: inline-grid;
    width: var(--switch-width);
    min-width: var(--switch-width);
    height: var(--choice-hit-area);
    padding: 0;
    place-items: center;
    border: 0;
    background: transparent;
  }

  .switch:disabled {
    cursor: default;
    opacity: var(--choice-disabled-opacity);
  }

  .track {
    display: block;
    box-sizing: border-box;
    width: var(--switch-width);
    height: var(--switch-height);
    padding: var(--s1);
    border-radius: var(--r-pill);
    background: var(--surface-3);
    box-shadow: inset 0 0 0 var(--line-width) var(--line-strong);
  }

  .thumb {
    display: block;
    width: var(--switch-thumb-size);
    height: var(--switch-thumb-size);
    border-radius: var(--r-pill);
    background: var(--muted);
    transform: translateX(0);
  }

  .switch[data-state='checked'] .track {
    background: var(--ink);
    box-shadow: inset 0 0 0 var(--line-width) var(--ink);
  }

  .switch[data-state='checked'] .thumb {
    background: var(--bg);
    transform: translateX(var(--switch-thumb-travel));
  }

  @media (prefers-reduced-motion: no-preference) {
    .track {
      transition-property: background-color, box-shadow;
      transition-duration: var(--d1);
      transition-timing-function: var(--ease);
    }

    .thumb {
      transition-property: transform, background-color, scale;
      transition-duration: var(--d1);
      transition-timing-function: var(--choice-ease);
    }

    .switch:active:not(:disabled) .thumb {
      scale: var(--choice-pressed-scale);
    }
  }
</style>
