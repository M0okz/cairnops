<script lang="ts">
  import type { IndicatorPoint, IndicatorUnit } from '$lib/api';
  import { formatIndicator, incidentMarkerIndex, incidentMarkerLabelY, indicatorBounds } from '$lib/indicator-format';

  let { points, unit, interactive = false, compact = false, label = 'Courbe de l’Indicateur', marker = null }: {
    points: IndicatorPoint[];
    unit: IndicatorUnit;
    interactive?: boolean;
    compact?: boolean;
    label?: string;
    marker?: { at: string; label: string; tone: 'info' | 'warn' | 'crit' } | null;
  } = $props();

  const width = 640;
  const height = $derived(compact ? 54 : 126);
  const insetX = 4;
  const insetY = $derived(compact ? 5 : 12);
  let selected = $state<number | null>(null);
  let chart = $state<SVGSVGElement | null>(null);
  let renderedWidth = $state(width);

  $effect(() => {
    if (!chart) return;
    const measure = () => (renderedWidth = chart?.getBoundingClientRect().width || width);
    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(measure);
    observer.observe(chart);
    return () => observer.disconnect();
  });

  const geometry = $derived.by(() => {
    if (points.length === 0) return { line: '', area: '', coordinates: [] as Array<{ x: number; y: number }> };
    const [low, high] = indicatorBounds(points, unit);
    const span = Math.max(1e-9, high - low);
    const coordinates = points.map((point, index) => ({
      x: points.length === 1 ? width / 2 : insetX + (index / (points.length - 1)) * (width - insetX * 2),
      y: height - insetY - ((point.value - low) / span) * (height - insetY * 2)
    }));
    const line = coordinates.map((point, index) => `${index === 0 ? 'M' : 'L'}${point.x.toFixed(2)},${point.y.toFixed(2)}`).join('');
    const baseline = height - insetY;
    const area = `${line}L${coordinates.at(-1)!.x.toFixed(2)},${baseline}L${coordinates[0].x.toFixed(2)},${baseline}Z`;
    return { line, area, coordinates };
  });

  function pick(event: PointerEvent) {
    if (!interactive || points.length === 0) return;
    const svg = event.currentTarget as SVGSVGElement;
    const ratio = Math.min(1, Math.max(0, (event.clientX - svg.getBoundingClientRect().left) / svg.getBoundingClientRect().width));
    selected = Math.round(ratio * (points.length - 1));
  }

  const picked = $derived(selected === null ? null : points[selected]);
  const pickedCoordinate = $derived(selected === null ? null : geometry.coordinates[selected]);
  const markerIndex = $derived(marker ? incidentMarkerIndex(points, marker.at) : null);
  const markerCoordinate = $derived(markerIndex === null ? null : geometry.coordinates[markerIndex]);
  const markerHorizontalScale = $derived(renderedWidth > 0 ? width / renderedWidth : 1);
  const accessibleLabel = $derived(marker && markerCoordinate ? `${label}. Repère : ${marker.label}.` : label);
</script>

<svg bind:this={chart} class="area-chart" class:compact viewBox="0 0 {width} {height}" role="img" aria-label={accessibleLabel} preserveAspectRatio="none" onpointermove={pick} onpointerleave={() => (selected = null)}>
  <title>{accessibleLabel}</title>
  {#if geometry.line}
    <path class="fill" d={geometry.area} />
    <path class="line" d={geometry.line} vector-effect="non-scaling-stroke" />
  {:else}
    <path class="empty" d="M4,{height / 2}H636" vector-effect="non-scaling-stroke" />
  {/if}
  {#if marker && markerCoordinate}
    {@const labelWidth = 104}
    {@const labelHeight = 20}
    {@const labelHalfWidth = (labelWidth / 2 + 4) * markerHorizontalScale}
    {@const labelCenterX = Math.min(width - labelHalfWidth, Math.max(labelHalfWidth, markerCoordinate.x))}
    {@const labelY = incidentMarkerLabelY(markerCoordinate.y, height, labelHeight)}
    <g class="marker {marker.tone}" aria-hidden="true">
      <line class="marker-guide" x1={markerCoordinate.x} x2={markerCoordinate.x} y1="25" y2={height - insetY} vector-effect="non-scaling-stroke" />
      <circle class="marker-point" cx="0" cy="0" r="3.25" transform={`translate(${markerCoordinate.x},${markerCoordinate.y}) scale(${markerHorizontalScale},1)`} vector-effect="non-scaling-stroke" />
      <g class="marker-label" transform={`translate(${labelCenterX},${labelY}) scale(${markerHorizontalScale},1) translate(${-labelWidth / 2},0)`}>
        <rect width={labelWidth} height={labelHeight} rx="4" vector-effect="non-scaling-stroke" />
        <text x={labelWidth / 2} y="13.5" text-anchor="middle">{marker.label}</text>
      </g>
    </g>
  {/if}
  {#if picked && pickedCoordinate}
    <line class="guide" x1={pickedCoordinate.x} x2={pickedCoordinate.x} y1={insetY} y2={height - insetY} vector-effect="non-scaling-stroke" />
    <circle cx={pickedCoordinate.x} cy={pickedCoordinate.y} r="3" vector-effect="non-scaling-stroke" />
    <g class="tooltip" transform={`translate(${Math.min(width - 132, Math.max(4, pickedCoordinate.x - 64))},${pickedCoordinate.y < 36 ? pickedCoordinate.y + 9 : pickedCoordinate.y - 31})`}>
      <rect width="128" height="25" rx="4" />
      <text x="64" y="16" text-anchor="middle">{formatIndicator(picked.value, unit)}</text>
    </g>
  {/if}
</svg>

<style>
  .area-chart { width: 100%; height: 7.875rem; display: block; overflow: visible; color: var(--source-cairnops); touch-action: pan-y; }
  .area-chart.compact { height: 3.375rem; }
  .fill { fill: currentColor; opacity: 0.11; }
  .line { fill: none; stroke: currentColor; stroke-width: 1.4; stroke-linecap: round; stroke-linejoin: round; }
  .empty { fill: none; stroke: currentColor; stroke-width: 1; stroke-dasharray: 4 5; opacity: .35; }
  .guide { stroke: var(--line-strong); stroke-width: 1; }
  circle { fill: var(--surface); stroke: currentColor; stroke-width: 1.5; }
  .marker.info { color: var(--info); }
  .marker.warn { color: var(--warn); }
  .marker.crit { color: var(--crit); }
  .marker-guide { stroke: currentColor; stroke-width: 1; stroke-dasharray: 3 3; opacity: .8; }
  .marker-point { fill: var(--surface); stroke: currentColor; stroke-width: 1.75; }
  .marker-label rect { fill: var(--surface); stroke: currentColor; }
  .marker-label text { fill: var(--ink); font-family: var(--font); font-size: 9px; font-weight: var(--weight-semibold); }
  .tooltip rect { fill: var(--surface-3); stroke: var(--line-strong); }
  .tooltip text { fill: var(--ink); font-family: var(--font-num); font-size: 9px; }
  @media (max-width: 48rem) { .marker-label { display: none; } }
</style>
