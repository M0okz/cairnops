<script lang="ts">
  /* Suspendre n'efface rien, mais retire d'un coup toutes les Sources externes
   * du calcul de l'État de santé : c'est assez pour mériter d'être annoncé
   * avant, dans la même forme que la suppression. La reprise, elle, ne détruit
   * rien et ne demande donc pas de confirmation. */

  import Icon from './Icon.svelte';
  import type { Connector } from '$lib/api';
  import { plural, t } from '$lib/i18n.svelte';
  import Modal from './ui/Modal.svelte';
  import Button from './ui/Button.svelte';

  let {
    connector,
    onclose,
    onconfirm
  }: {
    connector: Connector;
    onclose: () => void;
    onconfirm: () => Promise<void> | void;
  } = $props();

  let busy = $state(false);

  const origins = $derived<Record<Connector['kind'], string>>({
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    argus: 'Argus',
    proxmox: 'Proxmox VE',
    generic_webhook: t('suspension.webhookSender')
  });


  async function suspend() {
    busy = true;
    await onconfirm();
  }
</script>

<Modal dismissible={!busy} {onclose}>
  {#snippet children(dialog)}
    <div {...dialog} class="modal" aria-labelledby="suspension-title">
      <header>
        <div>
          <h2 id="suspension-title">{t('suspension.title', { name: connector.name })}</h2>
          <p>{t('suspension.lead', { origin: origins[connector.kind] })}</p>
        </div>
        <Button variant="close" onclick={onclose} disabled={busy} aria-label={t('common.close')}>
          <Icon name="close" size={14} />
        </Button>
      </header>

      <div class="modal-body">
        <div class="ledger">
          <section class="stops">
            <h3>{t('suspension.stops')}</h3>
            <ul>
              <li>{plural('suspension.bindings', connector.binding_count)}</li>
              {#if connector.kind === 'generic_webhook'}
                <li>{t('suspension.incomingRefused')}</li>
              {:else}
                <li>{t('suspension.noNewEvidence')}</li>
              {/if}
            </ul>
          </section>

          <section class="kept">
            <h3>{t('suspension.stays')}</h3>
            <ul>
              <li>{t('suspension.keepBindings')}</li>
              <li>{t('suspension.keepIncidents')}</li>
              <li>{t('suspension.keepNativeChecks')}</li>
            </ul>
          </section>
        </div>
      </div>

      <footer>
        <span class="faint note">
          {t('suspension.note')}
        </span>
        <Button onclick={onclose} disabled={busy}>{t('common.cancel')}</Button>
        <Button variant="primary" onclick={suspend} disabled={busy}>
          {busy ? t('suspension.busy') : t('suspension.confirm')}
        </Button>
      </footer>
    </div>
  {/snippet}
</Modal>

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
