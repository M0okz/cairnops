<script lang="ts" generics="Value extends string">
  import { RadioGroup as RadioGroupPrimitive } from 'bits-ui';

  type SegmentedItem<Value extends string = string> = {
    value: Value;
    label: string;
    count?: number | string;
    lang?: string;
  };

  let {
    value,
    label,
    items,
    size = 'default',
    onValueChange
  }: {
    value: Value;
    label: string;
    items: readonly SegmentedItem<Value>[];
    size?: 'default' | 'compact';
    onValueChange: (value: Value) => void;
  } = $props();
</script>

<RadioGroupPrimitive.Root
  {value}
  orientation="horizontal"
  aria-label={label}
  onValueChange={(next) => onValueChange(next as Value)}
>
  {#snippet child({ props })}
    <div {...props} class:compact={size === 'compact'} class="segmented-control">
      {#each items as item (item.value)}
        <RadioGroupPrimitive.Item value={item.value}>
          {#snippet child({ props: itemProps })}
            <button {...itemProps} lang={item.lang} class="segment">
              <span class="label">{item.label}</span>
              {#if item.count !== undefined}
                <span class="count">{item.count}</span>
              {/if}
            </button>
          {/snippet}
        </RadioGroupPrimitive.Item>
      {/each}
    </div>
  {/snippet}
</RadioGroupPrimitive.Root>

<style>
  .segmented-control {
    display: inline-flex;
    height: var(--ctl-h);
    flex: none;
    overflow: hidden;
    border: var(--line-width) solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--surface);
  }

  button {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--s2);
    min-width: 0;
    padding: 0 var(--s3);
    border: 0;
    border-inline-start: var(--line-width) solid var(--line);
    background: transparent;
    color: var(--muted);
    font-size: var(--text-sm);
    font-weight: var(--weight-medium);
    white-space: nowrap;
  }

  button:first-child {
    border-inline-start: 0;
  }

  button[data-state='checked'] {
    background: var(--surface-2);
    color: var(--ink);
  }

  button:focus-visible {
    z-index: 1;
    outline-offset: -2px;
  }

  .count {
    min-inline-size: var(--counter-slot-w);
    color: var(--faint);
    font-family: var(--font-num);
    font-size: var(--text-xs);
    font-weight: var(--weight-medium);
    font-variant-numeric: tabular-nums;
    line-height: 1;
    text-align: end;
  }

  .compact {
    height: 1.5rem;
  }

  .compact button {
    padding-inline: var(--s3);
    font-size: var(--text-xs);
  }

  @media (hover: hover) {
    button:hover {
      color: var(--ink);
    }
  }

  @media (prefers-reduced-motion: no-preference) {
    button {
      transition:
        background-color var(--d1) var(--ease),
        color var(--d1) var(--ease);
    }
  }
</style>
