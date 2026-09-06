import { getTimes } from 'suncalc';

export const themeModes = ['light', 'dark', 'system', 'solar'] as const;
export type ThemeMode = (typeof themeModes)[number];
export const solarCities = [
  { name: 'Paris', lat: 48.8566, lon: 2.3522 },
  { name: 'Lyon', lat: 45.764, lon: 4.8357 },
  { name: 'Marseille', lat: 43.2965, lon: 5.3698 },
  { name: 'Bordeaux', lat: 44.8378, lon: -0.5792 },
  { name: 'Lille', lat: 50.6292, lon: 3.0573 },
  { name: 'Bruxelles', lat: 50.8503, lon: 4.3517 },
  { name: 'Genève', lat: 46.2044, lon: 6.1432 },
  { name: 'Montréal', lat: 45.5019, lon: -73.5674 },
  { name: 'Tromsø', lat: 69.6492, lon: 18.9553 }
];

export function themeMode(value: string | null): ThemeMode {
  return themeModes.find((mode) => mode === value) ?? 'system';
}

/** Un fuseau ne fournit pas de latitude : la ville reste un choix explicite. */
export function solarSchedule(cityName: string, now: Date) {
  const city = solarCities.find((candidate) => candidate.name === cityName);
  return city ? getTimes(now, city.lat, city.lon) : null;
}

export function resolvedTheme(mode: ThemeMode, systemDark: boolean, cityName: string, now: Date): 'light' | 'dark' {
  if (mode === 'light' || mode === 'dark') return mode;
  const sun = mode === 'solar' ? solarSchedule(cityName, now) : null;
  if (sun?.alwaysUp) return 'light';
  if (sun?.alwaysDown) return 'dark';
  if (sun?.sunrise && sun.sunset) {
    return now >= sun.sunrise && now < sun.sunset ? 'light' : 'dark';
  }
  return systemDark ? 'dark' : 'light';
}
