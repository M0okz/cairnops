<script lang="ts">
  import type { IndicatorPoint, IndicatorUnit } from '$lib/api';
  import {
    chartAreaPath,
    chartCoordinates,
    chartSegments,
    monotoneChartPath,
    nearestChartPoint
  } from '$lib/chart-geometry';
  import {
    formatIndicator,
    incidentMarkerIndex,
    incidentMarkerLabelY,
    indicatorBounds
  } from '$lib/indicator-format';
  import { localeTag } from '$lib/i18n.svelte';

  let {
    points,
    unit,
    interactive = false,
    focusable = interactive,
    compact = false,
    label = 'Courbe de l’Indicateur',
    marker = null,
    tone = 'source',
    bounds = null,
    gapThresholdMilliseconds = null
  }: {
    points: IndicatorPoint[];
    unit: IndicatorUnit;
    interactive?: boolean;
    focusable?: boolean;
    compact?: boolean;
    label?: string;
    marker?: { at: string; label: string; tone: 'info' | 'warn' | 'crit' } | null;
    tone?: 'source' | 'ok';
    bounds?: [number, number] | null;
    gapThresholdMilliseconds?: number | null;
  } = $props();

  const chartID = $props.id();
  const fillID = `chart-fill-${chartID}`;
  const clipID = `chart-clip-${chartID}`;
  const height = $derived(compact ? 64 : 132);
  const insetX = $derived(compact ? 3 : 8);
  const insetTop = $derived(compact ? 4 : 10);
  const insetBottom = $derived(compact ? 4 : 20);
  const tooltipWidth = $derived(compact ? 132 : 156);
  const tooltipHeight = $derived(compact ? 31 : 36);
  let selected = $state<number | null>(null);
  let pointerX = $state<number | null>(null);
  let chart = $state<SVGSVGElement | null>(null);
  let renderedWidth = $state(640);

  $effect(() => {
    if (!chart) return;
    const measure = () => {
      const width = chart?.getBoundingClientRect().width;
      if (width) renderedWidth = Math.max(compact ? 120 : 240, width);
    };
    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(measure);
    observer.observe(chart);
    return () => observer.disconnect();
  });

  const geometry = $derived.by(() => {
    const range = bounds ?? indicatorBounds(points, unit);
    const coordinates = chartCoordinates(points, {
      width: renderedWidth,
      height,
      insetX,
      insetTop,
      insetBottom,
      bounds: range
    });
    const baseline = height - insetBottom;
    const segments = chartSegments(coordinates, gapThresholdMilliseconds);
    return {
      width: renderedWidth,
      coordinates,
      segments: segments.map((segment) => ({
        coordinates: segment,
        line: monotoneChartPath(segment),
        area: chartAreaPath(segment, baseline)
      }))
    };
  });

  function clearSelection() {
    selected = null;
    pointerX = null;
  }

  function pick(event: PointerEvent) {
    if (!interactive || points.length === 0) return;
    const svg = event.currentTarget as SVGSVGElement;
    const rect = svg.getBoundingClientRect();
    const localX = Math.min(
      geometry.width - insetX,
      Math.max(insetX, ((event.clientX - rect.left) / rect.width) * geometry.width)
    );
    const closest = nearestChartPoint(geometry.coordinates, localX);
    selected = closest?.index ?? null;
    pointerX = localX;
  }

  function moveSelection(event: KeyboardEvent) {
    if (!focusable || points.length === 0) return;
    const current = selected ?? points.length - 1;
    let next = current;
    if (event.key === 'ArrowLeft') next = Math.max(0, current - 1);
    else if (event.key === 'ArrowRight') next = Math.min(points.length - 1, current + 1);
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = points.length - 1;
    else return;
    event.preventDefault();
    selected = next;
    pointerX = geometry.coordinates[next]?.x ?? null;
  }

  function timestamp(at: string, axis = false): string {
    const date = new Date(at);
    if (!Number.isFinite(date.getTime())) return '';
    if (axis) {
      return new Intl.DateTimeFormat(localeTag(), {
        hour: '2-digit',
        minute: '2-digit'
      }).format(date);
    }
    return new Intl.DateTimeFormat(localeTag(), {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit'
    }).format(date);
  }

  const picked = $derived(selected === null ? null : points[selected]);
  const pickedCoordinate = $derived(
    selected === null
      ? null
      : geometry.coordinates.find((coordinate) => coordinate.index === selected) ?? null
  );
  const pickedLabel = $derived(
    picked ? `${timestamp(picked.at)} · ${formatIndicator(picked.value, unit)}` : ''
  );
  const markerIndex = $derived(marker ? incidentMarkerIndex(points, marker.at) : null);
  const markerCoordinate = $derived(
    markerIndex === null
      ? null
      : geometry.coordinates.find((coordinate) => coordinate.index === markerIndex) ?? null
  );
  const tooltipX = $derived(
    Math.min(
      geometry.width - tooltipWidth - 3,
      Math.max(3, (pointerX ?? pickedCoordinate?.x ?? 0) - tooltipWidth / 2)
    )
  );
  const tooltipY = $derived(
    pickedCoordinate && pickedCoordinate.y > tooltipHeight + 10
      ? pickedCoordinate.y - tooltipHeight - 7
      : Math.min(height - insetBottom - tooltipHeight, (pickedCoordinate?.y ?? 0) + 7)
  );
  const axisPoints = $derived.by(() => {
    if (compact || geometry.coordinates.length < 2) return [];
    const first = geometry.coordinates[0];
    const middle = geometry.coordinates[Math.floor((geometry.coordinates.length - 1) / 2)];
    const last = geometry.coordinates.at(-1)!;
    return [first, middle, last].filter(
      (point, index, candidates) =>
        candidates.findIndex((candidate) => candidate.index === point.index) === index
    );
  });
  const accessibleLabel = $derived(
    `${label}${marker && markerCoordinate ? `. Repère : ${marker.label}.` : ''}${pickedLabel ? ` ${pickedLabel}.` : ''}`
  );
</script>

<!-- Une courbe détaillée rejoint l'ordre de tabulation ; l'analyse statique
     ne peut pas relier ce comportement au tabindex conditionnel. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<svg
  bind:this={chart}
  class="area-chart tone-{tone}"
  class:compact
  class:interactive
  viewBox="0 0 {geometry.width} {height}"
  role="img"
  aria-label={accessibleLabel}
  tabindex={focusable ? 0 : undefined}
  preserveAspectRatio="none"
  onpointermove={pick}
  onpointerleave={clearSelection}
  onfocus={() => {
    if (focusable && points.length > 0 && selected === null) {
      selected = points.length - 1;
      pointerX = geometry.coordinates.at(-1)?.x ?? null;
    }
  }}
  onblur={clearSelection}
  onkeydown={moveSelection}
>
  <title>{accessibleLabel}</title>
  <defs>
    <linearGradient id={fillID} x1="0" x2="0" y1={insetTop} y2={height - insetBottom} gradientUnits="userSpaceOnUse">
      <stop class="fill-start" offset="0%" />
      <stop class="fill-middle" offset="58%" />
      <stop class="fill-end" offset="100%" />
    </linearGradient>
    <clipPath id={clipID}>
      <rect x={insetX} y={insetTop} width={geometry.width - insetX * 2} height={height - insetTop - insetBottom} />
    </clipPath>
  </defs>

  <g class="grid" aria-hidden="true">
    {#each compact ? [0.5] : [0.25, 0.5, 0.75] as ratio}
      <line
        x1={insetX}
        x2={geometry.width - insetX}
        y1={insetTop + (height - insetTop - insetBottom) * ratio}
        y2={insetTop + (height - insetTop - insetBottom) * ratio}
        vector-effect="non-scaling-stroke"
      />
    {/each}
  </g>

  {#if geometry.coordinates.length > 0}
    <g clip-path={`url(#${clipID})`} aria-hidden="true">
      {#each geometry.segments as segment, index (`${index}-${segment.line}`)}
        {#if segment.area}
          <path class="fill" d={segment.area} fill={`url(#${fillID})`} />
        {/if}
        {#if segment.coordinates.length > 1}
          <path class="line" d={segment.line} vector-effect="non-scaling-stroke" />
        {:else}
          <circle class="single-point" cx={segment.coordinates[0].x} cy={segment.coordinates[0].y} r="2.5" vector-effect="non-scaling-stroke" />
        {/if}
      {/each}
    </g>
  {:else}
    <path class="empty" d={`M${insetX},${height / 2}H${geometry.width - insetX}`} vector-effect="non-scaling-stroke" />
  {/if}

  {#if axisPoints.length > 0}
    <g class="axis" aria-hidden="true">
      {#each axisPoints as point, index (point.index)}
        <text
          x={point.x}
          y={height - 4}
          text-anchor={index === 0 ? 'start' : index === axisPoints.length - 1 ? 'end' : 'middle'}
        >{timestamp(point.at, true)}</text>
      {/each}
    </g>
  {/if}

  {#if marker && markerCoordinate}
    {@const labelWidth = 104}
    {@const labelHeight = 20}
    {@const labelHalfWidth = labelWidth / 2 + 4}
    {@const labelCenterX = Math.min(geometry.width - labelHalfWidth, Math.max(labelHalfWidth, markerCoordinate.x))}
    {@const labelY = incidentMarkerLabelY(markerCoordinate.y, height, labelHeight)}
    <g class="marker {marker.tone}" aria-hidden="true">
      <line class="marker-guide" x1={markerCoordinate.x} x2={markerCoordinate.x} y1="25" y2={height - insetBottom} vector-effect="non-scaling-stroke" />
      <circle class="marker-point" cx={markerCoordinate.x} cy={markerCoordinate.y} r="3.25" vector-effect="non-scaling-stroke" />
      <g class="marker-label" transform={`translate(${labelCenterX - labelWidth / 2},${labelY})`}>
        <rect width={labelWidth} height={labelHeight} rx="4" vector-effect="non-scaling-stroke" />
        <text x={labelWidth / 2} y="13.5" text-anchor="middle">{marker.label}</text>
      </g>
    </g>
  {/if}

  {#if interactive && picked && pickedCoordinate}
    <g class="selection" role="tooltip" aria-label={pickedLabel}>
      <line class="cursor-guide" x1={pickedCoordinate.x} x2={pickedCoordinate.x} y1={insetTop} y2={height - insetBottom} vector-effect="non-scaling-stroke" />
      <circle class="selected-point" cx={pickedCoordinate.x} cy={pickedCoordinate.y} r="3.25" vector-effect="non-scaling-stroke" />
      <g class="tooltip" transform={`translate(${tooltipX},${tooltipY})`}>
        <rect width={tooltipWidth} height={tooltipHeight} rx="5" vector-effect="non-scaling-stroke" />
        <text class="tooltip-time" x="8" y={compact ? 12 : 14}>{timestamp(picked.at)}</text>
        <text class="tooltip-value" x="8" y={compact ? 25 : 29}>{formatIndicator(picked.value, unit)}</text>
      </g>
    </g>
  {/if}
</svg>

{#if focusable}
  <span class="visually-hidden" role="status">{pickedLabel}</span>
{/if}

<style>
  .area-chart {
    display: block;
    width: 100%;
    height: 8.25rem;
    overflow: visible;
    color: var(--source-cairnops);
    shape-rendering: geometricPrecision;
    touch-action: pan-y;
  }

  .area-chart.compact {
    height: 4rem;
  }

  .area-chart.interactive {
    cursor: crosshair;
  }

  .area-chart:focus-visible {
    outline-offset: 2px;
  }

  .area-chart.tone-ok {
    color: var(--ok);
  }

  .fill-start,
  .fill-middle,
  .fill-end {
    stop-color: currentColor;
  }

  .fill-start { stop-opacity: 0.3; }
  .fill-middle { stop-opacity: 0.12; }
  .fill-end { stop-opacity: 0.015; }

  .grid line {
    stroke: var(--line-row);
    stroke-width: 1;
  }

  .line {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.5;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .compact .line {
    stroke-width: 1.35;
  }

  .single-point,
  .selected-point {
    fill: var(--surface);
    stroke: currentColor;
    stroke-width: 1.75;
  }

  .empty {
    fill: none;
    stroke: currentColor;
    stroke-width: 1;
    stroke-dasharray: 4 5;
    opacity: 0.35;
  }

  .axis text {
    fill: var(--faint);
    font-family: var(--font-num);
    font-size: 8px;
    font-variant-numeric: tabular-nums;
  }

  .cursor-guide {
    stroke: var(--line-strong);
    stroke-width: 1;
    stroke-dasharray: 2 3;
  }

  .marker.info { color: var(--info); }
  .marker.warn { color: var(--warn); }
  .marker.crit { color: var(--crit); }
  .marker-guide { stroke: currentColor; stroke-width: 1; stroke-dasharray: 3 3; opacity: 0.8; }
  .marker-point { fill: var(--surface); stroke: currentColor; stroke-width: 1.75; }
  .marker-label rect { fill: var(--surface); stroke: currentColor; }
  .marker-label text { fill: var(--ink); font-family: var(--font); font-size: 9px; font-weight: var(--weight-semibold); }

  .tooltip rect {
    fill: var(--surface-3);
    stroke: var(--line-strong);
  }

  .tooltip text {
    font-variant-numeric: tabular-nums;
    pointer-events: none;
  }

  .tooltip-time {
    fill: var(--faint);
    font-family: var(--font);
    font-size: 8px;
  }

  .tooltip-value {
    fill: var(--ink);
    font-family: var(--font-num);
    font-size: 9px;
    font-weight: var(--weight-semibold);
  }

  @media (max-width: 48rem) {
    .marker-label { display: none; }
  }
</style>
