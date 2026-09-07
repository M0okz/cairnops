<script lang="ts" generics="Value extends string">
  import * as ToggleGroup from '$lib/components/ui/toggle-group/index.js';

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

<div class="shadcn-control segmented-container">
  <ToggleGroup.Root
    type="single"
    bind:value={() => value, (next) => {
      const item = items.find((item) => item.value === next);
      if (item) onValueChange(item.value);
    }}
    variant="outline"
    size={size === 'compact' ? 'sm' : 'default'}
    orientation="horizontal"
    aria-label={label}
    class="max-w-full flex-wrap"
  >
    {#each items as item (item.value)}
      <ToggleGroup.Item value={item.value} lang={item.lang}>
        <span>{item.label}</span>
        {#if item.count !== undefined}
          <span class="min-w-[var(--counter-slot-w)] text-right font-mono text-xs tabular-nums text-muted-foreground">{item.count}</span>
        {/if}
      </ToggleGroup.Item>
    {/each}
  </ToggleGroup.Root>
</div>

<style>
  .segmented-container { max-width: 100%; }
</style>
