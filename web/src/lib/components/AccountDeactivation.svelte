<script lang="ts">
  /* Désactiver un compte.
   *
   * Le geste est réversible et n'efface rien, mais il coupe quelqu'un de
   * l'instance séance tenante : cela mérite d'être annoncé avant, dans la même
   * forme que la suspension d'un Connecteur. La réactivation, elle, ne retire
   * rien et ne demande donc pas de confirmation. */

  import Icon from './Icon.svelte';
  import type { Account } from '$lib/api';
  import { t } from '$lib/i18n.svelte';

  let {
    account,
    onclose,
    onconfirm,
    error = ''
  }: {
    account: Account;
    onclose: () => void;
    onconfirm: () => Promise<void> | void;
    error?: string;
  } = $props();

  let busy = $state(false);

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  async function deactivate() {
    busy = true;
    try {
      await onconfirm();
    } finally {
      busy = false;
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="scrim"
  role="presentation"
  onclick={(event) => event.currentTarget === event.target && !busy && onclose()}
>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="deactivation-title">
    <header>
      <div>
        <h2 id="deactivation-title">
          {t('account.deactivateHeading', { name: account.display_name })}
        </h2>
        <p>{t('account.deactivateLead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    <div class="modal-body">
      <div class="ledger">
        <section class="stops">
          <h3>{t('suspension.stops')}</h3>
          <ul>
            <li>{t('account.stopSessions')}</li>
            <li>{t('account.stopPassword')}</li>
          </ul>
        </section>

        <section class="kept">
          <h3>{t('suspension.stays')}</h3>
          <ul>
            <li>{t('account.keepDecisions')}</li>
            <li>{t('account.keepUsername')}</li>
            <li>{t('account.keepPassword')}</li>
          </ul>
        </section>
      </div>

      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>

    <footer>
      <span class="faint note">{t('account.reactivationNote')}</span>
      <button class="btn" type="button" onclick={onclose} disabled={busy}>{t('common.cancel')}</button>
      <button class="btn primary" type="button" onclick={deactivate} disabled={busy}>
        {busy ? t('account.deactivating') : t('account.deactivateConfirm')}
      </button>
    </footer>
  </div>
</div>

<style>
  .modal {
    max-width: 38rem;
  }

  footer .note {
    flex-basis: 11rem;
  }

  .ledger {
    display: grid;
    gap: var(--s4);
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  }

  section {
    padding: var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
  }

  h3 {
    margin-bottom: var(--s3);
    font-size: 0.75rem;
    font-weight: 600;
  }

  .stops h3 {
    color: var(--warn);
  }

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--s3);
    color: var(--muted);
    font-size: 0.75rem;
    line-height: 1.45;
  }
</style>
