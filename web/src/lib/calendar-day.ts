/**
 * Rend un jour calendaire défini par l'instance sans le décaler selon le
 * fuseau du navigateur. L'API fournit ces bornes de jour à minuit UTC.
 */
export function formatCalendarDay(value: string | Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    timeZone: 'UTC'
  }).format(new Date(value));
}
