<script lang="ts">
  // Intent : choisir un rythme visuel, sans permission de localisation implicite.
  // Hiérarchie : 4 choix explicites puis la ville du mode solaire.
  // Palette/depth : surface élevée, Titane de sélection, bordure faible.
  // Typographie : système 13/12 px. Espacement : 4/8/12/16 px.
  import { Button } from '$lib/components/ui/button';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import { onMount } from 'svelte';
  import * as SunCalc from 'suncalc';
  import Icon from '../src/lib/components/Icon.svelte';
  import { cities } from './data';
  let { ontheme }: { ontheme: (theme: 'light' | 'dark') => void } = $props();
  let mode = $state('system');
  let cityName = $state('');
  let systemDark = $state(false);
  let now = $state(new Date());
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

<DropdownMenu.Root>
  <DropdownMenu.Trigger>
    {#snippet child({ props })}
      <Button {...props} variant="ghost" class="p-theme-trigger" aria-label="Choisir l’apparence">
        <Icon name={dark ? 'moon' : 'sun'} size={20}/>
        <span class="hidden sm:inline">{modes.find(item=>item.id===mode)?.label}</span>
      </Button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="end" class="w-64">
    <DropdownMenu.Label>Apparence</DropdownMenu.Label>
    <DropdownMenu.RadioGroup bind:value={mode}>
      {#each modes as item}
        <DropdownMenu.RadioItem value={item.id}><Icon name={item.icon} size={18}/>{item.label}</DropdownMenu.RadioItem>
      {/each}
    </DropdownMenu.RadioGroup>
    {#if mode === 'solar'}
      <DropdownMenu.Separator/>
      <DropdownMenu.Sub>
        <DropdownMenu.SubTrigger>Ville : {cityName || 'à choisir'}</DropdownMenu.SubTrigger>
        <DropdownMenu.SubContent>
          <DropdownMenu.RadioGroup bind:value={cityName}>
            {#each cities as entry}<DropdownMenu.RadioItem value={entry.name}>{entry.name}</DropdownMenu.RadioItem>{/each}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.SubContent>
      </DropdownMenu.Sub>
      <div class="px-2 py-2 text-xs text-muted-foreground leading-relaxed">
        {#if solar}
          <p>Lever {time(solar.sunrise)} · Coucher {time(solar.sunset)}</p>
          <p>{solar.alwaysUp ? 'Le soleil ne se couche pas aujourd’hui.' : solar.alwaysDown ? 'Le soleil ne se lève pas aujourd’hui.' : 'Le thème suit le lever et le coucher du soleil.'}</p>
          <p>Fuseau : {Intl.DateTimeFormat().resolvedOptions().timeZone}.</p>
        {:else}<p>Choisis une ville. En attendant, le thème suit le système.</p>{/if}
      </div>
    {:else if mode === 'system'}
      <DropdownMenu.Separator/>
      <p class="px-2 py-2 text-xs text-muted-foreground leading-relaxed">Suit l’apparence de ton appareil.</p>
    {/if}
  </DropdownMenu.Content>
</DropdownMenu.Root>
