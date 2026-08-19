/* Les langues de l'interface.
 *
 * Le français est la langue source : c'est en français que les Écrans sont
 * écrits et que les tournures sont décidées, et `fr` sert donc de contrat. Le
 * type MessageKey en découle, si bien qu'une clé oubliée en anglais — ou une
 * clé anglaise sans équivalent français — arrête `npm run check` plutôt que de
 * se découvrir à l'écran.
 *
 * Le texte reste dans des chaînes plates plutôt que dans un arbre : une clé se
 * cherche alors du même geste dans le dictionnaire et dans les composants qui
 * l'emploient. */

import { fr } from './locales/fr';
import { en } from './locales/en';

export type Locale = 'fr' | 'en';
export type MessageKey = keyof typeof fr;

export const locales: Array<{ value: Locale; label: string }> = [
  { value: 'fr', label: 'Français' },
  { value: 'en', label: 'English' }
];

const dictionaries: Record<Locale, Record<MessageKey, string>> = { fr, en };

/* Les langues que sait rendre l'interface, dans l'ordre où on les reconnaît. */
function fromSystem(): Locale {
  if (typeof navigator === 'undefined') return 'fr';
  for (const tag of navigator.languages ?? [navigator.language]) {
    const base = tag.toLowerCase().split('-')[0];
    if (base === 'fr' || base === 'en') return base;
  }
  return 'fr';
}

class I18n {
  /* Le système décide tant que personne n'a choisi. Le choix, lui, tient sur
   * cet appareil : il porte sur l'interface, pas sur le compte, et rien côté
   * serveur ne le connaît en V1. */
  locale = $state<Locale>('fr');

  get dictionary(): Record<MessageKey, string> {
    return dictionaries[this.locale];
  }

  /* La langue de la page se dit aussi au document : c'est elle que lisent les
   * lecteurs d'écran pour choisir leur prononciation, et la césure pour couper
   * les mots. */
  apply() {
    document.documentElement.lang = this.locale;
  }

  boot() {
    const stored = localStorage.getItem('cairnops-locale');
    this.locale = stored === 'fr' || stored === 'en' ? stored : fromSystem();
    this.apply();
  }

  choose(locale: Locale) {
    this.locale = locale;
    localStorage.setItem('cairnops-locale', locale);
    this.apply();
  }
}

export const i18n = new I18n();

/** Le texte d'une clé, dans la langue courante.
 *
 *  Appelée depuis un balisage, elle lit `i18n.locale` et rend donc l'écran
 *  réactif au changement de langue sans qu'aucun écran n'ait à s'en occuper. */
export function t(key: MessageKey, params?: Record<string, string | number>): string {
  const template = i18n.dictionary[key] ?? fr[key] ?? key;
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    name in params ? String(params[name]) : whole
  );
}

/** Le texte d'une clé accordée en nombre.
 *
 *  Le français et l'anglais partagent la même frontière — un, puis le reste —
 *  et deux clés suffisent donc : `…_one` et `…_other`. Le compte est passé au
 *  gabarit sous le nom `count`, sans qu'il faille le répéter à l'appel. */
export function plural(
  key: string,
  count: number,
  params?: Record<string, string | number>
): string {
  const suffix = Math.abs(count) === 1 ? '_one' : '_other';
  return t(`${key}${suffix}` as MessageKey, { count, ...params });
}

/** La langue courante au format BCP 47, pour Intl. */
export const localeTag = () => (i18n.locale === 'fr' ? 'fr-FR' : 'en-GB');
