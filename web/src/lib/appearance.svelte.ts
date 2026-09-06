import { resolvedTheme, solarCities, solarSchedule, themeMode, type ThemeMode } from './appearance';

class Appearance {
  mode = $state<ThemeMode>('system');
  cityName = $state('');
  theme = $state<'light' | 'dark'>('dark');
  now = $state(new Date());
  timeZone = $state('');
  #cleanup: (() => void) | undefined;

  get solar() { return solarSchedule(this.cityName, this.now); }

  #read() {
    try {
      this.mode = themeMode(localStorage.getItem('cairnops-theme'));
      const city = localStorage.getItem('cairnops-solar-city');
      this.cityName = solarCities.find((candidate) => candidate.name === city)?.name ?? '';
    } catch { /* Une préférence locale indisponible conserve le choix en mémoire. */ }
  }

  refresh = () => {
    this.now = new Date();
    this.timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    this.theme = resolvedTheme(this.mode, matchMedia('(prefers-color-scheme: dark)').matches, this.cityName, this.now);
    document.documentElement.dataset.theme = this.theme;
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content',
      getComputedStyle(document.documentElement).getPropertyValue('--bg').trim());
  };

  choose(mode: ThemeMode) {
    this.mode = mode;
    this.#save();
  }

  chooseCity(name: string) {
    this.cityName = solarCities.find((city) => city.name === name)?.name ?? '';
    this.#save();
  }

  #save() {
    try {
      localStorage.setItem('cairnops-theme', this.mode);
      localStorage.setItem('cairnops-solar-city', this.cityName);
    } catch { /* Le thème fonctionne aussi quand le stockage est désactivé. */ }
    this.refresh();
  }

  boot() {
    this.teardown();
    this.#read();
    this.refresh();
    const scheme = matchMedia('(prefers-color-scheme: dark)');
    const sync = (event: StorageEvent) => {
      if (event.key === null || event.key === 'cairnops-theme' || event.key === 'cairnops-solar-city') {
        this.#read();
        this.refresh();
      }
    };
    const timer = setInterval(this.refresh, 30_000);
    scheme.addEventListener('change', this.refresh);
    document.addEventListener('visibilitychange', this.refresh);
    window.addEventListener('focus', this.refresh);
    window.addEventListener('storage', sync);
    this.#cleanup = () => {
      clearInterval(timer);
      scheme.removeEventListener('change', this.refresh);
      document.removeEventListener('visibilitychange', this.refresh);
      window.removeEventListener('focus', this.refresh);
      window.removeEventListener('storage', sync);
    };
  }

  teardown() { this.#cleanup?.(); this.#cleanup = undefined; }
}

export const appearance = new Appearance();
