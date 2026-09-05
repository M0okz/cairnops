<script module lang="ts">
  import { cairnopsIconPaths, type CairnOpsIconName } from '../brand/cairnops-icon-paths';

  export type IconName = CairnOpsIconName;
</script>

<script lang="ts">
  let { name, size = 15 }: { name: IconName; size?: number } = $props();
</script>

<svg
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  width={size}
  height={size}
  aria-hidden="true"
>
  <!-- Tracés constants du jeu d'icônes, jamais issus de données. -->
  {@html cairnopsIconPaths[name]}
</svg>
