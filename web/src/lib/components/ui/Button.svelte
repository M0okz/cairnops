<script lang="ts" module>
  import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';

  export type ButtonVariant = 'default' | 'primary' | 'danger' | 'quiet' | 'close';
  export type ButtonSize = 'default' | 'sm';

  /* Les gestionnaires gardent le type d'un bouton : c'est le cas courant, et
   * un lien n'en reçoit pas d'autre que le clic. */
  export type ButtonProps = HTMLButtonAttributes &
    Pick<HTMLAnchorAttributes, 'href' | 'target' | 'rel' | 'download'> & {
      variant?: ButtonVariant;
      size?: ButtonSize;
      ref?: HTMLButtonElement | HTMLAnchorElement | null;
    };

  /* Les variantes reprennent les classes Titane de app.css : `.btn` et ses
   * modificateurs pour les actions, `.close` pour la fermeture d'une fenêtre
   * ou d'un volet. Le dessin reste dans la feuille commune. */
  const variants: Record<ButtonVariant, string> = {
    default: 'btn',
    primary: 'btn primary',
    danger: 'btn danger',
    quiet: 'btn quiet',
    close: 'close'
  };

  export function buttonClass(variant: ButtonVariant = 'default', size: ButtonSize = 'default') {
    return size === 'sm' && variant !== 'close' ? `${variants[variant]} sm` : variants[variant];
  }
</script>

<script lang="ts">
  /* Le bouton de CairnOps. Un lien qui en a l'allure passe `href` ; il reste
   * un lien, désactivable sans perdre sa place dans la page. */

  let {
    variant = 'default',
    size = 'default',
    class: className,
    ref = $bindable(null),
    href = undefined,
    type = 'button',
    disabled,
    children,
    ...rest
  }: ButtonProps = $props();
</script>

{#if href}
  <a
    bind:this={ref}
    class={[buttonClass(variant, size), className]}
    href={disabled ? undefined : href}
    aria-disabled={disabled}
    role={disabled ? 'link' : undefined}
    tabindex={disabled ? -1 : undefined}
    {...rest as HTMLAnchorAttributes}
  >
    {@render children?.()}
  </a>
{:else}
  <button bind:this={ref} class={[buttonClass(variant, size), className]} {type} {disabled} {...rest}>
    {@render children?.()}
  </button>
{/if}
