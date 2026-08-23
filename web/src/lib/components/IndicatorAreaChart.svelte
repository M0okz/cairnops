<script lang="ts">
  import type { IndicatorPoint, IndicatorUnit } from '$lib/api';
  import { formatIndicator, indicatorBounds } from '$lib/indicator-format';

  let { points, unit, interactive = false, compact = false, label = 'Courbe de l’Indicateur' }: {
    points: IndicatorPoint[];
    unit: IndicatorUnit;
    interactive?: boolean;
    compact?: boolean;
    label?: string;
  } = $props();

  const width = 640;
  const height = $derived(compact ? 54 : 126);
  const insetX = 4;
  const insetY = $derived(compact ? 5 : 12);
  let selected = $state<number | null>(null);

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
</script>

<svg class="area-chart" class:compact viewBox="0 0 {width} {height}" role="img" aria-label={label} preserveAspectRatio="none" onpointermove={pick} onpointerleave={() => (selected = null)}>
  <title>{label}</title>
  {#if geometry.line}
    <path class="fill" d={geometry.area} />
    <path class="line" d={geometry.line} vector-effect="non-scaling-stroke" />
  {:else}
    <path class="empty" d="M4,{height / 2}H636" vector-effect="non-scaling-stroke" />
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
  .tooltip rect { fill: var(--surface-3); stroke: var(--line-strong); }
  .tooltip text { fill: var(--ink); font-family: var(--font-num); font-size: 9px; }
</style>
