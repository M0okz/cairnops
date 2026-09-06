<script lang="ts">
  import { getChartContext } from 'layerchart';
  import { prefersReducedMotion } from 'svelte/motion';
  import * as Chart from './ui/chart';
  import type { ChartCoordinate } from '$lib/chart-geometry';
  import type { IndicatorUnit } from '$lib/api';
  import { formatIndicator } from '$lib/indicator-format';
  import { t } from '$lib/i18n.svelte';

  let { picked, maximum, pointer, valueLabel, unit, timestamp }: {
    picked: ChartCoordinate | null;
    maximum?: ChartCoordinate;
    pointer: { x: number; y: number } | null;
    valueLabel: string;
    unit: IndicatorUnit;
    timestamp: (at: string | number) => string;
  } = $props();

  const chart = getChartContext();
  let tooltipElement = $state<HTMLElement | null>(null);
  let tooltipSize = $state({ width: 0, height: 0 });
  $effect(() => {
    if (!tooltipElement) return;
    const element = tooltipElement;
    const measure = () => { tooltipSize = { width: element.offsetWidth, height: element.offsetHeight }; };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  });
  // A simple side flip cannot contain a wide tooltip in a narrow chart.
  // Use LayerChart's explicit-position API and the real content dimensions.
  const position = $derived.by(() => {
    const x = pointer?.x ?? picked?.x ?? 0;
    const y = pointer?.y ?? picked?.y ?? 0;
    const left = x + 10 + tooltipSize.width <= chart.containerWidth ? x + 10 : x - tooltipSize.width - 10;
    const top = y + 10 + tooltipSize.height <= chart.containerHeight ? y + 10 : y - tooltipSize.height - 10;
    return {
      x: Math.max(0, Math.min(chart.containerWidth - tooltipSize.width, left)),
      y: Math.max(0, Math.min(chart.containerHeight - tooltipSize.height, top))
    };
  });
  // LayerChart's manual mode lets pointer, touch and keyboard inspect the same
  // exact sample as the SVG, including collection gaps and boolean transitions.
  $effect(() => {
    chart.tooltip.data = picked;
    chart.tooltip.x = pointer?.x ?? picked?.x ?? 0;
    chart.tooltip.y = pointer?.y ?? picked?.y ?? 0;
    chart.tooltip.series = picked ? [
      { key: 'value', label: valueLabel, value: picked.value, color: 'var(--chart-series)', visible: true, config: { key: 'value' } },
      ...(maximum ? [{ key: 'maximum', label: t('chart.hourlyMaximum'), value: maximum.value, color: 'var(--chart-maximum)', visible: true, config: { key: 'maximum' } }] : [])
    ] : [];
  });
</script>

<Chart.Tooltip
  bind:ref={tooltipElement}
  indicator="line"
  role="tooltip"
  labelFormatter={(at) => timestamp(at)}
  valueFormatter={(value) => formatIndicator(value, unit)}
  rootProps={{ portal: false, x: position.x, y: position.y, xOffset: 0, yOffset: 0, motion: prefersReducedMotion.current ? 'none' : { type: 'spring', stiffness: 0.5, damping: 1 }, fadeDuration: prefersReducedMotion.current ? 0 : 100 }}
/>
