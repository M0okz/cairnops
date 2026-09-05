<script lang="ts">
  // Intent : choisir un rythme visuel, sans permission de localisation implicite.
  // Hiérarchie : 4 choix explicites puis la ville du mode solaire.
  // Palette/depth : surface élevée, Titane de sélection, bordure faible.
  // Typographie : système 13/12 px. Espacement : 4/8/12/16 px.
  import { onMount } from 'svelte';
  import * as SunCalc from 'suncalc';
  import Icon from '../src/lib/components/Icon.svelte';
  import { cities } from './data';
  let { ontheme }: { ontheme: (theme: 'light' | 'dark') => void } = $props();
  let mode = $state('system');
  let cityName = $state('');
  let systemDark = $state(false);
  let now = $state(new Date());
  let details: HTMLDetailsElement;
  const city = $derived(cities.find((entry) => entry.name === cityName));
  const solar = $derived(city ? SunCalc.getTimes(now, city.lat, city.lon) : null);
  const solarDark = $derived(solar ? solar.alwaysDown || (!solar.alwaysUp && solar.sunrise !== null && solar.sunset !== null && (now < solar.sunrise || now >= solar.sunset)) : systemDark);
  const dark = $derived(mode === 'dark' || (mode === 'system' && systemDark) || (mode === 'solar' && solarDark));
  const modes = [{id:'light',label:'Clair',icon:'sun' as const},{id:'dark',label:'Sombre',icon:'moon' as const},{id:'system',label:'Système',icon:'settings' as const},{id:'solar',label:'Solaire',icon:'sun' as const}];
  const time = (date: Date | null | undefined) => date ? new Intl.DateTimeFormat('fr-FR',{hour:'2-digit',minute:'2-digit'}).format(date) : '—';
  $effect(() => { ontheme(dark ? 'dark' : 'light'); });
  onMount(() => {
    const scheme = matchMedia('(prefers-color-scheme: dark)');
    const params = new URLSearchParams(location.search);
    const requested = params.get('theme');
    if (requested && modes.some((item) => item.id === requested)) mode = requested;
    systemDark = scheme.matches;
    const listener = () => { systemDark = scheme.matches; };
    const refresh = () => { now = new Date(); };
    scheme.addEventListener('change', listener);
    const timer = setInterval(refresh, 30_000);
    window.addEventListener('focus', refresh);
    return () => { scheme.removeEventListener('change', listener); clearInterval(timer); window.removeEventListener('focus', refresh); };
  });
</script>

<details class="p-theme" bind:this={details}>
  <summary aria-label="Choisir l’apparence"><Icon name={dark ? 'moon' : 'sun'} size={17}/><span>{modes.find(item=>item.id===mode)?.label}</span><span class="p-chevron">⌄</span></summary>
  <div class="p-theme-panel">
    <strong>Apparence</strong><p>À l’aise, à toute heure.</p>
    <div class="p-theme-options" role="group" aria-label="Thème">
      {#each modes as item}
        <button class:chosen={mode === item.id} aria-pressed={mode === item.id} onclick={()=>{mode=item.id; if(item.id!=='solar')details.open=false;}}><Icon name={item.icon} size={19}/>{item.label}</button>
      {/each}
    </div>
    {#if mode === 'solar'}
      <label class="p-city-label" for="solar-city">Ville de référence</label>
      <select id="solar-city" bind:value={cityName}><option value="">Choisir une ville</option>{#each cities as entry}<option>{entry.name}</option>{/each}</select>
      {#if solar}
        <div class="p-solar-times"><span><Icon name="sun"/>Lever <b>{time(solar.sunrise)}</b></span><span><Icon name="moon"/>Coucher <b>{time(solar.sunset)}</b></span></div>
        <p>{solar.alwaysUp ? 'Le soleil ne se couche pas aujourd’hui.' : solar.alwaysDown ? 'Le soleil ne se lève pas aujourd’hui.' : 'Le thème bascule au lever et au coucher du soleil.'}</p>
        <p class="p-small">Heures dans le fuseau de cet appareil : {Intl.DateTimeFormat().resolvedOptions().timeZone}.</p>
      {:else}<p>Choisis une ville pour activer le rythme solaire. En attendant, le thème suit le système.</p>{/if}
    {:else if mode === 'system'}<p>Suit l’apparence de ton appareil, y compris lorsqu’elle change.</p>{/if}
  </div>
</details>
