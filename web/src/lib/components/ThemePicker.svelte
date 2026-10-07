<script lang="ts">
  import { Popover } from 'bits-ui';
  import Icon from './Icon.svelte';
  import AppearanceSettings from './AppearanceSettings.svelte';
  import { appearance } from '$lib/appearance.svelte';
  import { t } from '$lib/i18n.svelte';
</script>

<Popover.Root>
  <Popover.Trigger>
    {#snippet child({ props })}
      <button {...props} class="theme-trigger" type="button" aria-label={t('appearance.choose')}>
        <Icon name={appearance.theme === 'dark' ? 'moon' : 'sun'} size={20} />
        <span>{t(`appearance.${appearance.mode}`)}</span>
      </button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Content align="end" sideOffset={8} collisionPadding={16}>
    {#snippet child({ wrapperProps, props, open })}
      {#if open}
        <div {...wrapperProps}>
          <div {...props} class="theme-panel">
            <strong>{t('appearance.title')}</strong>
            <AppearanceSettings />
          </div>
        </div>
      {/if}
    {/snippet}
  </Popover.Content>
</Popover.Root>

<style>
  .theme-trigger { display: flex; align-items: center; gap: var(--s3); min-height: var(--ctl-h-lg); padding: 0 var(--s3); border: 0; border-radius: var(--r-m); background: none; color: var(--muted); cursor: pointer; font: inherit; font-size: var(--text-sm); }
  .theme-trigger:hover, .theme-trigger[data-state='open'] { background: var(--surface-2); color: var(--ink); }
  .theme-panel { width: 22rem; max-width: calc(100vw - 2rem); padding: var(--s4); border-radius: var(--r-l); background: var(--surface); box-shadow: var(--shadow); max-height: min(calc(100dvh - 2 * var(--topbar-h) - 2 * var(--s4)), var(--bits-floating-available-height, 100dvh)); overflow-y: auto; }
  strong { display: block; margin-bottom: var(--s4); font-size: var(--text-md); }
  @media (max-width: 48rem) { .theme-trigger > span { display: none; } }
</style>
