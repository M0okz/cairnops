<script lang="ts">
  import Icon, { type IconName } from './Icon.svelte';
  import { appearance } from '$lib/appearance.svelte';
  import { solarCities, themeModes } from '$lib/appearance';
  import { localeTag, t } from '$lib/i18n.svelte';

  const cityID = $props.id();
  const icons: Record<string, IconName> = { light: 'sun', dark: 'moon', system: 'devices', solar: 'activity' };
  const time = (date: Date | null) => date ? new Intl.DateTimeFormat(localeTag(), { hour: '2-digit', minute: '2-digit' }).format(date) : '—';
</script>

<div class="appearance-settings">
  <div class="appearance-options" role="group" aria-label={t('rail.theme')}>
    {#each themeModes as mode}
      <button type="button" aria-pressed={appearance.mode === mode} onclick={() => appearance.choose(mode)}>
        <Icon name={icons[mode]} size={22} />
        <span>{t(`appearance.${mode}`)}</span>
      </button>
    {/each}
  </div>
  {#if appearance.mode === 'solar'}
    <label for={cityID}>{t('appearance.city')}</label>
    <select id={cityID} value={appearance.cityName} onchange={(event) => appearance.chooseCity(event.currentTarget.value)}>
      <option value="">{t('appearance.chooseCity')}</option>
      {#each solarCities as city}<option value={city.name}>{city.name}</option>{/each}
    </select>
    {#if appearance.solar}
      <div class="solar-times">
        <span>{t('appearance.sunrise')} <b>{time(appearance.solar.sunrise)}</b></span>
        <span>{t('appearance.sunset')} <b>{time(appearance.solar.sunset)}</b></span>
      </div>
      <p>{t(appearance.solar.alwaysUp ? 'appearance.polarDay' : appearance.solar.alwaysDown ? 'appearance.polarNight' : 'appearance.solarHint')}</p>
      <p>{t('appearance.timeZone', { zone: appearance.timeZone })}</p>
    {:else}<p>{t('appearance.needsCity')}</p>{/if}
  {:else if appearance.mode === 'system'}<p>{t('appearance.systemHint')}</p>{/if}
</div>

<style>
  .appearance-settings { display: grid; gap: var(--s3); min-width: 0; }
  .appearance-options { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--s2); }
  .appearance-options button { min-width: 0; display: grid; justify-items: center; gap: var(--s3); padding: var(--s4) var(--s2); border: 1px solid var(--line); border-radius: var(--r-m); background: var(--bg); font-size: var(--text-sm); cursor: pointer; }
  .appearance-options button:hover { background: var(--surface-2); }
  .appearance-options button[aria-pressed='true'] { border-color: var(--accent); background: var(--surface-3); }
  label { margin-top: var(--s3); font-size: var(--text-sm); font-weight: 600; }
  select { width: 100%; min-width: 0; height: var(--ctl-h-lg); border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--bg); padding: 0 var(--s3); color: var(--ink); font: inherit; }
  p { color: var(--muted); font-size: var(--text-xs); line-height: 1.6; overflow-wrap: anywhere; }
  .solar-times { display: flex; justify-content: space-between; gap: var(--s3); font-size: var(--text-sm); }
  b { font-family: var(--font-num); font-weight: 500; margin-left: var(--s2); }
</style>
