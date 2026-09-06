<script lang="ts">
  import Icon from './Icon.svelte';
  import AppearanceSettings from './AppearanceSettings.svelte';
  import { appearance } from '$lib/appearance.svelte';
  import { t } from '$lib/i18n.svelte';

  let details = $state<HTMLDetailsElement | null>(null);
  let trigger = $state<HTMLElement | null>(null);
  $effect(() => {
    const dismiss = (event: PointerEvent) => {
      if (details?.open && !details.contains(event.target as Node)) details.open = false;
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && details?.open) {
        details.open = false;
        trigger?.focus();
      }
    };
    document.addEventListener('pointerdown', dismiss);
    document.addEventListener('keydown', escape);
    return () => { document.removeEventListener('pointerdown', dismiss); document.removeEventListener('keydown', escape); };
  });
</script>

<details class="theme-picker" bind:this={details}>
  <summary bind:this={trigger} aria-label={t('appearance.choose')}>
    <Icon name={appearance.theme === 'dark' ? 'moon' : 'sun'} size={20} />
    <span>{t(`appearance.${appearance.mode}`)}</span>
    <i aria-hidden="true">⌄</i>
  </summary>
  <div class="theme-panel">
    <strong>{t('appearance.title')}</strong>
    <AppearanceSettings />
  </div>
</details>

<style>
  .theme-picker { position: relative; }
  summary { list-style: none; display: flex; align-items: center; gap: var(--s3); min-height: var(--ctl-h-lg); padding: 0 var(--s3); border-radius: var(--r-m); color: var(--muted); cursor: pointer; font-size: var(--text-sm); }
  summary::-webkit-details-marker { display: none; }
  summary:hover, details[open] summary { background: var(--surface-2); color: var(--ink); }
  summary i { font-style: normal; }
  .theme-panel { position: absolute; right: 0; top: calc(100% + var(--s3)); width: 22rem; max-width: calc(100vw - 2rem); padding: var(--s4); border: 1px solid var(--line-strong); border-radius: var(--r-l); background: var(--surface); box-shadow: var(--shadow); z-index: 40; max-height: calc(100dvh - 2 * var(--topbar-h) - 2 * var(--s4)); overflow-y: auto; }
  strong { display: block; margin-bottom: var(--s4); font-size: var(--text-md); }
  @media (max-width: 48rem) { summary > span, summary > i { display: none; } .theme-panel { position: fixed; top: auto; margin-top: var(--s3); right: var(--s4); } }
</style>
