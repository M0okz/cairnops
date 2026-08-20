<script lang="ts">
  /* Chiffres roulants pour les valeurs qui vivent : compteurs, durées et
   * mesures. Le texte accessible reste entier ; le découpage en rouleaux est
   * uniquement visuel et n'est refait que lorsque la valeur change. */

  import { untrack } from 'svelte';

  let { value }: { value: string | number } = $props();

  const initial = untrack(() => String(value));
  let previous = $state(initial);
  let current = $state(initial);
  let revision = $state(0);

  type Cell =
    | { kind: 'literal'; value: string; key: string }
    | { kind: 'digit'; frames: number[]; steps: number; delay: number; key: string };

  function scalar(text: string): number | null {
    const match = text.replace(/\s/g, '').match(/-?\d+(?:[,.]\d+)?/);
    if (!match) return null;
    const parsed = Number(match[0].replace(',', '.'));
    return Number.isFinite(parsed) ? parsed : null;
  }

  function framesBetween(from: number, to: number, descending: boolean): number[] {
    const frames = [from];
    while (frames.at(-1) !== to && frames.length <= 10) {
      const last = frames.at(-1) ?? from;
      frames.push((last + (descending ? 9 : 1)) % 10);
    }
    return frames;
  }

  const cells = $derived.by((): Cell[] => {
    const oldDigits = previous.match(/\d/g) ?? [];
    const nextDigits = current.match(/\d/g) ?? [];
    const oldScalar = scalar(previous);
    const nextScalar = scalar(current);
    const descending = oldScalar !== null && nextScalar !== null && nextScalar < oldScalar;
    let digitIndex = 0;

    return Array.from(current).map((character, index) => {
      if (!/\d/.test(character)) {
        return { kind: 'literal', value: character, key: `literal-${index}-${character}` };
      }

      const fromRight = nextDigits.length - digitIndex - 1;
      const oldIndex = oldDigits.length - fromRight - 1;
      const to = Number(character);
      const from = oldIndex >= 0 ? Number(oldDigits[oldIndex]) : 0;
      const frames = framesBetween(from, to, descending);
      digitIndex += 1;

      return {
        kind: 'digit',
        frames,
        steps: frames.length,
        delay: Math.min(fromRight, 4),
        key: `digit-${index}-${revision}`
      };
    });
  });

  $effect(() => {
    const next = String(value);
    if (next === current) return;
    previous = current;
    current = next;
    revision += 1;
  });
</script>

<span class="odometer">
  <span class="visually-hidden">{current}</span>
  <span class="wheels" aria-hidden="true">
    {#each cells as cell (cell.key)}
      {#if cell.kind === 'digit'}
        <span class="wheel steps-{cell.steps} delay-{cell.delay}">
          <span class="track">
            {#each cell.frames as frame, frameIndex (`${cell.key}-${frameIndex}`)}
              <span class="frame">{frame}</span>
            {/each}
          </span>
        </span>
      {:else}
        <span class="literal">{cell.value}</span>
      {/if}
    {/each}
  </span>
</span>

<style>
  .odometer,
  .wheels {
    display: inline-flex;
    align-items: baseline;
    white-space: nowrap;
  }

  .wheel {
    display: inline-block;
    width: 1ch;
    height: 1em;
    overflow: hidden;
    line-height: 1;
    vertical-align: -0.08em;
    font-variant-numeric: tabular-nums;
    contain: paint;
  }

  .track,
  .frame {
    display: block;
  }

  .frame {
    height: 1em;
    text-align: center;
  }

  .literal {
    white-space: pre;
  }

  .steps-2 .track  { animation: roll-1 240ms var(--ease) both }
  .steps-3 .track  { animation: roll-2 270ms var(--ease) both }
  .steps-4 .track  { animation: roll-3 300ms var(--ease) both }
  .steps-5 .track  { animation: roll-4 330ms var(--ease) both }
  .steps-6 .track  { animation: roll-5 350ms var(--ease) both }
  .steps-7 .track  { animation: roll-6 370ms var(--ease) both }
  .steps-8 .track  { animation: roll-7 390ms var(--ease) both }
  .steps-9 .track  { animation: roll-8 410ms var(--ease) both }
  .steps-10 .track { animation: roll-9 430ms var(--ease) both }
  .steps-11 .track { animation: roll-10 440ms var(--ease) both }

  .delay-1 .track { animation-delay: 18ms }
  .delay-2 .track { animation-delay: 36ms }
  .delay-3 .track { animation-delay: 54ms }
  .delay-4 .track { animation-delay: 72ms }

  @keyframes roll-1  { to { transform: translateY(-1em) } }
  @keyframes roll-2  { to { transform: translateY(-2em) } }
  @keyframes roll-3  { to { transform: translateY(-3em) } }
  @keyframes roll-4  { to { transform: translateY(-4em) } }
  @keyframes roll-5  { to { transform: translateY(-5em) } }
  @keyframes roll-6  { to { transform: translateY(-6em) } }
  @keyframes roll-7  { to { transform: translateY(-7em) } }
  @keyframes roll-8  { to { transform: translateY(-8em) } }
  @keyframes roll-9  { to { transform: translateY(-9em) } }
  @keyframes roll-10 { to { transform: translateY(-10em) } }

  @media (prefers-reduced-motion: reduce) {
    .wheel .track {
      animation: none !important;
      transform: none !important;
    }

    .frame:not(:last-child) {
      display: none;
    }
  }
</style>
