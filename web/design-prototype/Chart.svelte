<script lang="ts">
  // Intent : lire l'évolution sans confondre contexte et verdict.
  // Hiérarchie : courbe principale, puis valeur au curseur, puis grille.
  // Palette/surfaces : encre et bleu information, tokens partagés, aucun relief.
  // Typographie : chiffres tabulaires 11/12 px. Espacement : grille de 4 px.
  import { chartSeries } from './data';
  let { range = '24h', mini = false, seed = 0, label = 'Temps de réponse de l’API publique', interactive = true, reveal = 1 }: {
    range?: string; mini?: boolean; seed?: number; label?: string; interactive?: boolean; reveal?: number;
  } = $props();
  const uid = $props.id();
  let hover = $state<number | null>(null);
  const values = $derived(chartSeries(range, seed));
  const width = 800;
  const height = $derived(mini ? 54 : 228);
  const top = $derived(mini ? 4 : 18);
  const bottom = $derived(mini ? 50 : 194);
  const left = $derived(mini ? 0 : 36);
  const right = $derived(mini ? width : 780);
  const x = (i: number) => left + i * (right - left) / (values.length - 1);
  const y = (v: number) => bottom - v / 220 * (bottom - top);
  const points = $derived(values.map((v, i) => [x(i), y(v)]));
  // Interpolation monotone en X avec contrôle borné : aucun dépassement inventé.
  const line = $derived(points.map((p, i) => i === 0 ? `M${p[0]},${p[1]}` : `C${(points[i-1][0]+p[0])/2},${points[i-1][1]} ${(points[i-1][0]+p[0])/2},${p[1]} ${p[0]},${p[1]}`).join(' '));
  const area = $derived(`${line} L${right},${bottom} L${left},${bottom} Z`);
  const selected = $derived(hover ?? values.length - 1);
  const ticks = $derived(range === '7j' ? ['lun.', 'mar.', 'mer.', 'jeu.', 'ven.', 'sam.', 'dim.'] : range === '1h' ? ['13:30', '13:40', '13:50', '14:00', '14:10', '14:20', '14:30'] : ['14:30', '18:30', '22:30', '02:30', '06:30', '10:30', '14:30']);
  const selectedLabel = $derived.by(() => {
    const fraction = selected / (values.length - 1);
    if (range === '7j') return ['Lundi', 'Mardi', 'Mercredi', 'Jeudi', 'Vendredi', 'Samedi', 'Dimanche'][Math.min(6, Math.floor(fraction * 7))];
    const minute = range === '1h' ? 13 * 60 + 30 + Math.round(fraction * 60) : 14 * 60 + 30 + Math.round(fraction * 1440);
    return `${String(Math.floor(minute / 60) % 24).padStart(2, '0')}:${String(minute % 60).padStart(2, '0')}`;
  });
  function pointer(event: PointerEvent) {
    if (!interactive || mini) return;
    const rect = event.currentTarget instanceof Element ? event.currentTarget.getBoundingClientRect() : null;
    if (!rect) return;
    hover = Math.max(0, Math.min(values.length - 1, Math.round(((event.clientX - rect.left) / rect.width * width - left) / (right-left) * (values.length-1))));
  }
  function keyboard(event: KeyboardEvent) {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
    event.preventDefault(); event.stopPropagation();
    hover = Math.max(0, Math.min(values.length - 1, selected + (event.key === 'ArrowRight' ? 1 : -1)));
  }
</script>

<div class:mini class="p-chart">
  <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio={mini ? 'none' : 'xMidYMid meet'} role="img" aria-label={label}>
    <defs>
      <linearGradient id={`${uid}-fill`} x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stop-color="var(--p-chart-color)" stop-opacity="0.24"/><stop offset="100%" stop-color="var(--p-chart-color)" stop-opacity="0.015"/></linearGradient>
      <clipPath id={`${uid}-clip`}><rect x="0" y="0" width={width * reveal} height={height}/></clipPath>
    </defs>
    {#if !mini}
      {#each [0, 50, 100, 150, 200] as tick}<line x1={left} x2={right} y1={y(tick)} y2={y(tick)} class="p-gridline"/><text x="0" y={y(tick)+4} class="p-axis">{tick}</text>{/each}
      {#each ticks as tick, i}<text x={left + i*(right-left)/6} y="222" text-anchor={i===0?'start':i===6?'end':'middle'} class="p-axis">{tick}</text>{/each}
    {/if}
    <g clip-path={`url(#${uid}-clip)`}>
      <path d={area} fill={`url(#${uid}-fill)`}/>
      <path d={line} fill="none" stroke="var(--p-chart-color)" stroke-width={mini?1.8:2.2} vector-effect="non-scaling-stroke" stroke-linecap="round"/>
    </g>
    {#if hover !== null && !mini}
      <line x1={x(selected)} x2={x(selected)} y1={top} y2={bottom} class="p-crosshair"/>
      <circle cx={x(selected)} cy={y(values[selected])} r="4" fill="var(--p-chart-color)" stroke="var(--p-surface)" stroke-width="3"/>
      <g transform={`translate(${Math.max(40, Math.min(644, x(selected)-66))},${Math.max(3,y(values[selected])-58)})`}>
        <rect width="130" height="45" rx="6" fill="var(--p-surface-raised)" stroke="var(--p-line-strong)"/>
        <text x="12" y="17" class="p-axis">{selectedLabel}</text>
        <text x="12" y="34" class="p-chart-value">{values[selected]} ms</text>
      </g>
    {/if}
  </svg>
  {#if interactive && !mini}
    <div class="p-chart-hit" role="slider" aria-label="Explorer le temps de réponse" aria-valuemin="0" aria-valuemax={values.length-1} aria-valuenow={selected} aria-valuetext={`${selectedLabel}, ${values[selected]} millisecondes`} tabindex="0" onpointermove={pointer} onpointerleave={()=>hover=null} onkeydown={keyboard}></div>
  {/if}
</div>
