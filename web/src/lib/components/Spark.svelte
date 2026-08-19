<script lang="ts">
  import { plural, t } from '$lib/i18n.svelte';
  /* Tendance sur 24 h — la latence moyenne, heure par heure. La Disponibilité
   * d'une installation saine est plate : elle ne se déforme qu'en cas de
   * défaut, et une droite ne dit rien à qui la regarde tous les jours. La
   * latence, elle, respire — c'est elle qui montre une Cible qui ralentit
   * avant qu'elle ne tombe. La couleur reste celle de l'État de la Cible.
   *
   * L'échelle se resserre sur l'amplitude observée : entre 31 et 34 ms, ce
   * sont les trois millisecondes qui intéressent, pas la distance à zéro.
   *
   * Rendue en SVG plutôt qu'en CSS : la politique de sécurité de l'instance
   * ferme `style-src-attr`, donc aucune géométrie ne peut passer par un
   * attribut style. Les attributs de géométrie SVG, eux, ne sont pas du CSS. */

  let {
    values,
    width = 82,
    height = 20
  }: { values: number[]; width?: number; height?: number } = $props();

  /* Amplitude relative en deçà de laquelle une suite est tenue pour constante :
   * la zoomer ferait d'un arrondi de mesure un relief. */
  const flatRatio = 0.02;

  /* En deçà, la fenêtre de 24 h n'est pas assez peuplée pour qu'une ligne
   * pleine soit honnête : l'instance vient d'être installée. */
  const minimumPoints = 3;

  const inset = 2;

  const round = (value: number) => Math.round(value * 100) / 100;

  const enough = $derived(values.length >= minimumPoints);

  const label = $derived(
    values.length === 0
      ? t('spark.empty')
      : plural('spark.label', values.length, {
          low: Math.round(Math.min(...values)),
          high: Math.round(Math.max(...values))
        })
  );

  const path = $derived.by(() => {
    if (values.length === 0) return '';

    const top = inset;
    const bottom = height - inset;
    const low = Math.min(...values);
    const high = Math.max(...values);
    const middle = (low + high) / 2;
    const span = high - low;

    /* Une latence constante reste une droite, à mi-hauteur : sa valeur est
       déjà dans la colonne voisine, la tendance n'a qu'à dire « rien ne
       bouge ». */
    if (middle === 0 || span / middle < flatRatio) return `M0,${round((top + bottom) / 2)}H${width}`;

    const at = (value: number) => round(bottom - ((value - low) / span) * (bottom - top));

    const points = values.map((value, index) => ({
      x: round((index / (values.length - 1)) * width),
      y: at(value)
    }));

    /* Catmull-Rom converti en cubiques : la courbe passe par chaque heure
       mesurée, sans le pic anguleux d'une polyligne. */
    let drawn = `M${points[0].x},${points[0].y}`;
    for (let index = 0; index < points.length - 1; index += 1) {
      const before = points[index - 1] ?? points[index];
      const start = points[index];
      const end = points[index + 1];
      const after = points[index + 2] ?? end;
      const c1 = { x: round(start.x + (end.x - before.x) / 6), y: round(start.y + (end.y - before.y) / 6) };
      const c2 = { x: round(end.x - (after.x - start.x) / 6), y: round(end.y - (after.y - start.y) / 6) };
      drawn += `C${c1.x},${c1.y} ${c2.x},${c2.y} ${end.x},${end.y}`;
    }
    return drawn;
  });
</script>

<!-- Le pointillé couvre les deux cas où une ligne pleine mentirait : aucune
     mesure, et un historique trop court pour remplir la fenêtre. Une droite
     pleine, elle, dit bien ce qu'elle dit — une latence qui ne bouge pas. -->
<svg
  class="spark-svg"
  class:sparse={path === '' || !enough}
  viewBox="0 0 {width} {height}"
  {width}
  {height}
  role="img"
  preserveAspectRatio="none"
>
  <!-- Le titre sert les deux lectures : l'infobulle du navigateur et le nom
       que rend le lecteur d'écran. -->
  <title>{label}</title>
  <path d={path || `M0,${height / 2}H${width}`} vector-effect="non-scaling-stroke" />
</svg>

<style>
  .spark-svg {
    display: block;
    overflow: visible;
  }

  path {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.25;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .sparse path {
    stroke-dasharray: 3 3;
    opacity: 0.5;
  }
</style>
