<script lang="ts">
  import { t } from '$lib/i18n.svelte';

  let { values, kind }: { values: number[]; kind: 'availability' | 'latency' } = $props();
  const id = $props.id();
  const width = 480;
  const height = 112;
  const top = 6;
  const bottom = 104;
  const points = $derived.by(() => {
    const finite = values.filter(Number.isFinite);
    if (finite.length === 0) return [];
    const ceiling = kind === 'availability' ? 1 : Math.max(1, ...finite) * 1.1;
    return finite.map((value, index) => ({
      x: finite.length === 1 ? width / 2 : index / (finite.length - 1) * width,
      y: bottom - Math.max(0, Math.min(1, value / ceiling)) * (bottom - top)
    }));
  });
  const line = $derived(points.map((point, index) => `${index ? 'L' : 'M'}${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(''));
  const area = $derived(points.length > 1 ? `${line}L${width},${bottom}L0,${bottom}Z` : '');
</script>

<svg class="health-trend {kind}" viewBox="0 0 {width} {height}" preserveAspectRatio="none" role="img" aria-label={t(kind === 'availability' ? 'dashboard.hourlyAvailability' : 'dashboard.hourlyLatency')}>
  <defs>
    <linearGradient id="health-fill-{id}" x1="0" x2="0" y1="0" y2="1">
      <stop class="fill-start" offset="0%" />
      <stop class="fill-end" offset="100%" />
    </linearGradient>
  </defs>
  <g class="grid" aria-hidden="true">
    {#each [0, 0.25, 0.5, 0.75, 1] as fraction}
      <line x1="0" x2={width} y1={top + fraction * (bottom - top)} y2={top + fraction * (bottom - top)} />
    {/each}
  </g>
  {#if area}<path class="area" d={area} fill="url(#health-fill-{id})" />{/if}
  {#if line}<path class="line" d={line} vector-effect="non-scaling-stroke" />{/if}
</svg>

<style>
  .health-trend { display: block; width: 100%; height: 7rem; overflow: visible; }
  .grid line { stroke: var(--line); stroke-width: 1; }
  .line { fill: none; stroke: var(--chart-series); stroke-width: 1.75; stroke-linecap: round; stroke-linejoin: round; }
  .availability .line { stroke: var(--ok); }
  .fill-start { stop-color: var(--chart-series); stop-opacity: 0.12; }
  .fill-end { stop-color: var(--chart-series); stop-opacity: 0; }
  .availability .fill-start, .availability .fill-end { stop-color: var(--ok); }
  .availability .fill-start { stop-opacity: 0.22; }
  .availability .fill-end { stop-opacity: 0.02; }
</style>
