/* Ouverture de la Palette.
 *
 * L'état vit hors du composant parce que deux endroits le commandent : le
 * déclencheur de la barre supérieure, présent sur chaque écran, et le raccourci
 * ⌘K, écouté une seule fois par la Palette elle-même. */

class Palette {
  open = $state(false);

  /* L'élément qui avait le focus avant l'ouverture. La Palette le lui rend en
   * se fermant, sinon le focus retombe sur le corps du document et la
   * navigation au clavier repart du début de l'écran. */
  #origin: HTMLElement | null = null;

  show() {
    if (this.open) return;
    this.#origin = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    this.open = true;
  }

  hide() {
    if (!this.open) return;
    this.open = false;
    this.#origin?.focus();
    this.#origin = null;
  }

  toggle() {
    if (this.open) this.hide();
    else this.show();
  }
}

export const palette = new Palette();
