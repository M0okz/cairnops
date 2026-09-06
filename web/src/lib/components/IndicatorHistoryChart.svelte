<script lang="ts">
  import { untrack } from 'svelte';
  import { Tween, prefersReducedMotion } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import type { IndicatorPoint, IndicatorUnit } from '$lib/api';
  import { chartCoordinates, chartSegments, monotoneChartPath, nearestChartPoint, type ChartCoordinate } from '$lib/chart-geometry';
  import { CHART_TWEEN_DURATION, indicatorTimeTicks, interpolateChartCoordinates, maximumCoordinates, stepChartPath } from '$lib/indicator-history';
  import { formatIndicator, indicatorBounds } from '$lib/indicator-format';
  import { localeTag, t } from '$lib/i18n.svelte';

  let { points, unit, label, timeBounds, hourly = false }: {
    points: IndicatorPoint[];
    unit: IndicatorUnit;
    label: string;
    timeBounds: [number, number] | null;
    hourly?: boolean;
  } = $props();

  const id = $props.id();
  // SVG geometry uses CSS pixels. No inline styles are needed under the instance CSP.
  let height = $state(280);
  const inset = 8;
  const top = 18;
  const baseline = $derived(height - 30);
  let chart = $state<SVGSVGElement | null>(null);
  let width = $state(0);
  let selectedTime = $state<string | null>(null);
  let pointer = $state<{ x: number; y: number } | null>(null);
  let keyboard = $state(false);
  let dismissed = $state(false);
  const values = new Tween<ChartCoordinate[]>([], { duration: CHART_TWEEN_DURATION, easing: cubicOut, interpolate: interpolateChartCoordinates });
  const maxima = new Tween<ChartCoordinate[]>([], { duration: CHART_TWEEN_DURATION, easing: cubicOut, interpolate: interpolateChartCoordinates });

  $effect(() => {
    if (!chart) return;
    const measure = () => {
      if (!chart) return;
      const rect = chart.getBoundingClientRect();
      width = rect.width;
      height = rect.height;
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(chart);
    return () => observer.disconnect();
  });

  const range = $derived.by((): [number, number] => {
    const [low, high] = indicatorBounds(points, unit);
    return [Math.min(0, low, ...points.map((point) => point.minimum ?? point.value)), Math.max(0, high, ...points.map((point) => point.maximum ?? point.value))];
  });
  const domain = $derived(timeBounds ?? [Date.parse(points[0]?.at ?? ''), Date.parse(points.at(-1)?.at ?? '')] as [number, number]);
  const coordinates = $derived(width ? chartCoordinates(points, { width, height, insetX: inset, insetTop: top, insetBottom: height - baseline, bounds: range, timeBounds: domain }) : []);
  const maximum = $derived(hourly ? maximumCoordinates(points, coordinates, range, top, baseline) : []);
  const ticks = $derived(indicatorTimeTicks(domain, width - inset * 2));
  const gap = $derived(hourly ? 90 * 60_000 : 5 * 60_000);
  const drawnValues = $derived(chartSegments(values.current, gap));
  const drawnMaxima = $derived(chartSegments(maxima.current, gap));
  const hasMaximum = $derived(maximum.length > 0);
  const tooltipWidth = $derived(Math.min(232, Math.max(180, width - 16)));
  const tooltipHeight = $derived(hasMaximum ? 86 : 64);

  let lastWidth = 0;
  let lastDrawing = '';
  $effect(() => {
    const nextValues = coordinates;
    const nextMaxima = maximum;
    const reduced = prefersReducedMotion.current;
    const nextWidth = width;
    untrack(() => {
      const drawing = JSON.stringify([nextValues, nextMaxima]);
      if (drawing === lastDrawing && !reduced) return;
      const resized = lastWidth > 0 && lastWidth !== nextWidth;
      const duration = reduced || resized || keyboard || pointer || unit === 'boolean' ? 0 : CHART_TWEEN_DURATION;
      for (const [tween, next] of [[values, nextValues], [maxima, nextMaxima]] as const) {
        if (!tween.current.length && next.length && duration) void tween.set(next.map((point) => ({ ...point, y: baseline })), { duration: 0 });
        void tween.set(next, { duration: next.length ? duration : 0 });
      }
      lastWidth = nextWidth;
      lastDrawing = drawing;
    });
  });

  $effect(() => () => {
    // Stop pending frames when navigating away, including during an interrupted tween.
    void values.set([], { duration: 0 });
    void maxima.set([], { duration: 0 });
  });

  function settle() {
    void values.set(coordinates, { duration: 0 });
    void maxima.set(maximum, { duration: 0 });
  }

  function pick(event: PointerEvent) {
    if (!chart || !points.length) return;
    const rect = chart.getBoundingClientRect();
    pointer = { x: Math.max(inset, Math.min(width - inset, event.clientX - rect.left)), y: event.clientY - rect.top };
    selectedTime = nearestChartPoint(coordinates, pointer.x)?.at ?? null;
    keyboard = false;
    dismissed = false;
    settle();
  }

  function clear() { selectedTime = null; pointer = null; dismissed = false; }

  function move(event: KeyboardEvent) {
    if (event.key === 'Escape') { clear(); dismissed = true; event.preventDefault(); return; }
    if (!coordinates.length) return;
    let index = Math.max(0, coordinates.findIndex((point) => point.at === selectedTime));
    if (event.key === 'Home') index = 0;
    else if (event.key === 'End') index = coordinates.length - 1;
    else if (event.key === 'ArrowLeft') index = Math.max(0, index - 1);
    else if (event.key === 'ArrowRight') index = Math.min(coordinates.length - 1, index + 1);
    else return;
    event.preventDefault();
    selectedTime = coordinates[index].at;
    pointer = null;
    keyboard = true;
    dismissed = false;
    settle();
  }

  const picked = $derived(dismissed ? null : coordinates.find((point) => point.at === selectedTime) ?? null);
  const pickedMaximum = $derived(picked ? maximum.find((point) => point.index === picked.index) : undefined);
  const tooltipX = $derived(Math.max(4, Math.min(width - tooltipWidth - 4,
    (pointer?.x ?? picked?.x ?? 0) + 12 + tooltipWidth < width ? (pointer?.x ?? picked?.x ?? 0) + 12 : (pointer?.x ?? picked?.x ?? 0) - tooltipWidth - 12)));
  const tooltipY = $derived(Math.max(4, Math.min(baseline - tooltipHeight, (pointer?.y ?? picked?.y ?? 0) - tooltipHeight / 2)));
  const valueLabel = $derived(t(hourly ? 'chart.hourlyLatest' : 'chart.value'));
  const pickedLabel = $derived(picked ? `${timestamp(picked.at)}. ${valueLabel} : ${formatIndicator(picked.value, unit)}${hasMaximum ? `. ${t('chart.hourlyMaximum')} : ${formatIndicator(pickedMaximum?.value, unit)}` : ''}` : '');

  function timestamp(at: string | number, axis = false) {
    const multiDay = domain[1] - domain[0] > 36 * 3_600_000;
    return new Intl.DateTimeFormat(localeTag(), axis
      ? multiDay ? { day: 'numeric', month: 'short' } : { hour: '2-digit', minute: '2-digit' }
      : { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(at));
  }

  function line(segment: ChartCoordinate[]) { return unit === 'boolean' ? stepChartPath(segment) : monotoneChartPath(segment); }
  function area(segment: ChartCoordinate[]) { return `${line(segment)}L${segment.at(-1)!.x},${baseline}L${segment[0].x},${baseline}Z`; }
</script>

<!-- The SVG is one keyboard stop; arrows inspect real samples, Escape dismisses. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<svg bind:this={chart} class="area-chart history-chart" viewBox="0 0 {width || 640} {height}" role="img" tabindex="0"
  aria-label={`${label}${pickedLabel ? `. ${pickedLabel}` : ''}`} aria-describedby={`chart-help-${id}`}
  onpointermove={pick} onpointerdown={pick} onpointerleave={() => { if (!keyboard) clear(); }}
  onfocus={() => { keyboard = true; selectedTime ??= coordinates.at(-1)?.at ?? null; dismissed = false; settle(); }}
  onblur={() => { keyboard = false; clear(); }} onkeydown={move}>
  <title>{label}</title>
  <defs>
    <linearGradient id={`history-value-${id}`} x1="0" x2="0" y1={top} y2={baseline} gradientUnits="userSpaceOnUse">
      <stop class="fill-start" offset="0%" /><stop class="fill-end" offset="100%" />
    </linearGradient>
    <linearGradient id={`history-maximum-${id}`} x1="0" x2="0" y1={top} y2={baseline} gradientUnits="userSpaceOnUse">
      <stop class="maximum-start" offset="0%" /><stop class="maximum-end" offset="100%" />
    </linearGradient>
    <clipPath id={`history-clip-${id}`}><rect x={inset - 2} y={top - 2} width={Math.max(0, width - inset * 2 + 4)} height={baseline - top + 4} /></clipPath>
    <clipPath id={`history-label-${id}`}><rect x="21" y="28" width={tooltipWidth - 112} height="50" /></clipPath>
  </defs>

  <g class="grid" aria-hidden="true">
    {#each [0, 0.25, 0.5, 0.75] as ratio}
      <line x1={inset} x2={Math.max(inset, width - inset)} y1={top + (baseline - top) * ratio} y2={top + (baseline - top) * ratio} />
    {/each}
  </g>
  <g class="axis" aria-hidden="true">
    {#each ticks as at (at)}
      {@const x = inset + (at - domain[0]) / (domain[1] - domain[0]) * (width - inset * 2)}
      <text x={x} y={height - 7} text-anchor={x < 32 ? 'start' : x > width - 32 ? 'end' : 'middle'}>{timestamp(at, true)}</text>
    {/each}
  </g>

  <g class="curves" clip-path={`url(#history-clip-${id})`} aria-hidden="true">
    {#each [{ key: 'maximum', segments: drawnMaxima }, { key: 'value', segments: drawnValues }] as series (series.key)}
      <g class={series.key}>
        {#each series.segments as segment, index (index)}
          {#if segment.length > 1}
            <path class="fill" d={area(segment)} fill={`url(#history-${series.key}-${id})`} />
            <path class="line" d={line(segment)} vector-effect="non-scaling-stroke" />
          {:else if segment.length}
            <circle class="single-point" cx={segment[0].x} cy={segment[0].y} r="2.5" />
          {/if}
        {/each}
      </g>
    {/each}
  </g>

  {#if picked}
    <g class="selection" role="tooltip" aria-label={pickedLabel}>
      <line class="cursor-guide" x1={picked.x} x2={picked.x} y1={top} y2={baseline} />
      {#if pickedMaximum}<circle class="selected-point maximum" cx={pickedMaximum.x} cy={pickedMaximum.y} r="4" />{/if}
      <circle class="selected-point" cx={picked.x} cy={picked.y} r="4" />
      <g class="tooltip" transform={`translate(${tooltipX},${tooltipY})`}>
        <rect class="tooltip-surface" width={tooltipWidth} height={tooltipHeight} rx="8" />
        <text class="tooltip-time" x="12" y="21">{timestamp(picked.at)}</text>
        <rect class="swatch" x="12" y="34" width="3" height="13" rx="1.5" />
        <text class="tooltip-name" x="22" y="45" clip-path={`url(#history-label-${id})`}>{valueLabel}</text>
        <text class="tooltip-value" x={tooltipWidth - 12} y="45" text-anchor="end">{formatIndicator(picked.value, unit)}</text>
        {#if hasMaximum}
          <rect class="swatch maximum" x="12" y="56" width="3" height="13" rx="1.5" />
          <text class="tooltip-name" x="22" y="67" clip-path={`url(#history-label-${id})`}>{t('chart.maximum')}</text>
          <text class="tooltip-value" x={tooltipWidth - 12} y="67" text-anchor="end">{formatIndicator(pickedMaximum?.value, unit)}</text>
        {/if}
      </g>
    </g>
  {/if}
</svg>
<span class="visually-hidden" id={`chart-help-${id}`}>{t('chart.keyboardHelp')}</span>
<span class="visually-hidden" role="status">{keyboard ? pickedLabel : ''}</span>

<style>
  .history-chart { display: block; flex: 1; width: 100%; height: var(--chart-history-height); min-height: var(--chart-history-height); color: var(--chart-series); overflow: visible; touch-action: pan-y; cursor: crosshair; }
  .history-chart:focus-visible { outline-offset: var(--s2); border-radius: var(--r-m); }
  .grid line { stroke: var(--line); stroke-width: 1; }
  .axis text { fill: var(--faint); font-family: var(--font); font-size: var(--chart-text-size); font-variant-numeric: tabular-nums; }
  .fill-start { stop-color: var(--chart-series); stop-opacity: 0.38; }
  .fill-end { stop-color: var(--chart-series); stop-opacity: 0.025; }
  .maximum-start { stop-color: var(--chart-maximum); stop-opacity: 0.26; }
  .maximum-end { stop-color: var(--chart-maximum); stop-opacity: 0.025; }
  .line { fill: none; stroke: currentColor; stroke-width: 1; stroke-linecap: round; stroke-linejoin: round; }
  .maximum { color: var(--chart-maximum); }
  .single-point { fill: currentColor; }
  .selection { pointer-events: none; }
  .cursor-guide { stroke: var(--line-strong); stroke-width: 1; }
  .selected-point { fill: currentColor; stroke: var(--surface); stroke-width: 1.5; }
  .tooltip-surface { fill: var(--chart-popover); stroke: var(--line); stroke-width: 1; filter: drop-shadow(var(--chart-tooltip-shadow)); }
  .tooltip text { font-size: var(--chart-text-size); font-family: var(--font); font-variant-numeric: tabular-nums; }
  .tooltip-time { fill: var(--ink); font-weight: var(--weight-medium); }
  .tooltip-name { fill: var(--faint); }
  .tooltip .tooltip-value { fill: var(--ink); font-family: var(--font-num); font-weight: var(--weight-medium); }
  .swatch { fill: currentColor; }
  @media (prefers-reduced-motion: no-preference) {
    .selection { animation: tooltip-in var(--d1) ease-out; }
    @keyframes tooltip-in { from { opacity: 0; } to { opacity: 1; } }
  }
  @media (forced-colors: active) {
    .history-chart, .maximum { color: CanvasText; }
    .line { stroke: CanvasText; }
    .maximum .line { stroke-dasharray: 4 3; }
    .fill { display: none; }
    .grid line, .cursor-guide { stroke: GrayText; }
    .tooltip-surface { fill: Canvas; stroke: CanvasText; }
    .tooltip text, .axis text { fill: CanvasText; }
    .selected-point { fill: CanvasText; stroke: Canvas; }
  }
</style>
