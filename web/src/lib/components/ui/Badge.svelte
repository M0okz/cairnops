<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import type { Tone } from '$lib/format';

  /* L'étiquette courte de CairnOps : état, gravité, version, compteur. Sa
   * tonalité reprend le registre des États de santé et des Gravités ; sans
   * tonalité, elle reste neutre. Le dessin vit dans `.pill` (app.css). */

  let {
    tone = '',
    class: className,
    children,
    ...rest
  }: HTMLAttributes<HTMLSpanElement> & {
    tone?: Tone | '';
    children?: Snippet;
  } = $props();
</script>

<span class={['pill', tone, className]} {...rest}>{@render children?.()}</span>
