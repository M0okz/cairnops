<script lang="ts">
  import { plural, t } from '$lib/i18n.svelte';
  /* La Disponibilité heure par heure, en barres. Là où la courbe de latence
   * montre une Cible qui ralentit, cette bande montre quand elle a manqué —
   * et rien d'autre : une heure vaut une barre, sa couleur vaut son verdict.
   *
   * Les heures que la fenêtre couvre sans qu'aucune Observation ne les
   * renseigne restent grises. Une barre verte affirme une preuve ; une barre
   * absente ne doit pas emprunter cette affirmation.
   *
   * Rendue en SVG comme la tendance : la politique de sécurité de l'instance
   * ferme `style-src-attr`, donc aucune géométrie ne peut passer par un
   * attribut style. */

  let { values, slots = 24 }: { values: Array<number | null>; slots?: number } = $props();

  const slot = 12;
  const bar = 8.5;
  const height = 22;

  /* Les mesures se lisent de gauche à droite jusqu'à maintenant : une fenêtre
     à peine peuplée se remplit par la droite, comme elle se remplira. */
  const hours = $derived.by(() => {
    const kept = values.slice(-slots);
    const missing = Math.max(0, slots - kept.length);
    return [
      ...Array.from({ length: missing }, () => null),
      ...kept
    ] as Array<number | null>;
  });

  /* Trois teintes seulement, celles de l'État de santé : une heure pleine,
     une heure entamée, une heure manquée. */
  function tone(value: number | null): string {
    if (value === null) return 'idle';
    if (value >= 0.999) return 'ok';
    if (value >= 0.9) return 'warn';
    return 'crit';
  }

  const measured = $derived(values.filter((value): value is number => value !== null).length);

  const label = $derived(
    measured === 0
      ? t('uptime.empty')
      : plural('uptime.label', measured, { slots })
  );
</script>

<svg
  class="uptime"
  viewBox="0 0 {slots * slot} {height}"
  role="img"
  preserveAspectRatio="none"
>
  <title>{label}</title>
  {#each hours as value, index (index)}
    <rect
      class={tone(value)}
      x={index * slot}
      y="0"
      width={bar}
      height={height}
      rx="1.5"
    />
  {/each}
</svg>

<style>
  .uptime {
    display: block;
    width: 100%;
    height: 1.375rem;
  }

  rect.ok   { fill: var(--ok) }
  rect.warn { fill: var(--warn) }
  rect.crit { fill: var(--crit) }
  rect.idle { fill: var(--surface-3) }
</style>
