<script lang="ts" module>
  /* Attributs à étaler sur la boîte de la modale : rôle, focus, Échap. */
  export type ModalProps = Record<string, unknown>;

  /* Verrou de défilement partagé par les modales empilées. Celui de Bits UI
   * restaure le style du body par `setAttribute('style')`, que la politique
   * CSP refuse : la page restait figée après la fermeture. Ce verrou ne passe
   * que par le CSSOM, que la politique autorise. */
  let locks = 0;

  function lockScroll() {
    if (locks++ === 0) document.body.style.setProperty('overflow', 'hidden');
    return () => {
      if (--locks === 0) document.body.style.removeProperty('overflow');
    };
  }
</script>

<script lang="ts">
  import { Dialog as DialogPrimitive } from 'bits-ui';
  import type { Snippet } from 'svelte';

  /* Modale commune, fondée sur le Dialog de Bits UI : le focus reste dans la
   * fenêtre, Échap ferme la couche la plus haute seulement, le défilement de la
   * page est bloqué et le focus revient au déclencheur quand le parent retire
   * la modale.
   *
   * Le composant fournit le voile `.scrim` ; l'appelant dessine sa boîte et y
   * étale les attributs reçus, pour que ses styles scopés continuent de
   * l'atteindre :
   *
   *   <Modal {onclose}>
   *     {#snippet children(dialog)}
   *       <div {...dialog} class="modal" aria-labelledby="titre">…</div>
   *     {/snippet}
   *   </Modal>
   *
   * Le parent garde la main sur l'ouverture : il monte la modale, et `onclose`
   * lui demande de la retirer. `dismissible` à faux suspend Échap et le clic
   * sur le voile, le temps qu'une écriture en cours se termine. `side` ancre
   * la boîte à droite, pour les volets de détail. */

  let {
    side = false,
    dismissible = true,
    onclose,
    onInteractOutside,
    children
  }: {
    side?: boolean;
    dismissible?: boolean;
    onclose: () => void;
    /** Appelé avant la fermeture par un clic hors de la boîte ; `preventDefault` la retient. */
    onInteractOutside?: (event: PointerEvent) => void;
    children: Snippet<[ModalProps]>;
  } = $props();

  /* La modale reste ouverte tant que le parent la monte. Une demande de
   * fermeture peut être refusée (écriture en cours, modifications non
   * enregistrées) : Bits ne doit pas se croire fermé pour autant, sinon Échap
   * et le maintien du focus cesseraient de fonctionner. */
  let open = $state(true);

  function close() {
    open = true;
    onclose();
  }

  $effect(lockScroll);
</script>

<DialogPrimitive.Root bind:open onOpenChange={(next) => { if (!next) close(); }}>
  <DialogPrimitive.Content
    escapeKeydownBehavior={dismissible ? 'close' : 'ignore'}
    interactOutsideBehavior={dismissible ? 'close' : 'ignore'}
    preventScroll={false}
    {onInteractOutside}
  >
    {#snippet child({ props })}
      <!-- Bits laisse ces deux réglages dans les attributs : ils n'ont rien à
           faire dans le DOM. -->
      {@const { escapeKeydownBehavior: _escape, interactOutsideBehavior: _outside, ...dialog } = props}
      <div class="scrim" class:side>
        {@render children(dialog)}
      </div>
    {/snippet}
  </DialogPrimitive.Content>
</DialogPrimitive.Root>
