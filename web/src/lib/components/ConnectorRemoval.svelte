<script lang="ts">
  import { plural, t } from '$lib/i18n.svelte';
  /* La suppression d'un Connecteur est la seule action de cet écran qui détruit
   * quelque chose. Elle annonce donc ce qu'elle emporte et ce qu'elle laisse,
   * avant d'être confirmée, et rappelle la porte de sortie réversible :
   * suspendre suffit quand on veut seulement arrêter la lecture. */

  import Icon from './Icon.svelte';
  import Modal from './ui/Modal.svelte';
  import Button from './ui/Button.svelte';
  import { api, type Connector, type ConnectorRemoval } from '$lib/api';

  let {
    connector,
    onclose,
    onsuccess
  }: {
    connector: Connector;
    onclose: () => void;
    onsuccess: (removal: ConnectorRemoval) => Promise<void> | void;
  } = $props();

  let busy = $state(false);
  let error = $state('');
  let tokenID = $state('');
  let secret = $state('');

  const origins: Record<Connector['kind'], string> = {
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    argus: 'Argus',
    proxmox: 'Proxmox VE',
    generic_webhook: t('suspension.webhookSender')
  };


  async function remove() {
    busy = true;
    error = '';
    try {
      const removal = connector.kind === 'proxmox' ? await api<ConnectorRemoval>(`/api/v1/connectors/${connector.id}/proxmox/remove`, { method: 'POST', body: JSON.stringify({ token_id: tokenID, secret }) }) : await api<ConnectorRemoval>(`/api/v1/connectors/${connector.id}`, { method: 'DELETE' });
      secret = '';
      await onsuccess(removal);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('removal.failed');
      busy = false;
    }
  }
</script>

<Modal dismissible={!busy} {onclose}>
  {#snippet children(dialog)}
    <div {...dialog} class="modal" aria-labelledby="removal-title">
      <header>
        <div>
          <h2 id="removal-title">Supprimer « {connector.name} » ?</h2>
          <p>
            {t('removal.lead', { origin: origins[connector.kind] })}
          </p>
        </div>
        <Button variant="close" onclick={onclose} disabled={busy} aria-label="Fermer">
          <Icon name="close" size={14} />
        </Button>
      </header>

      <div class="modal-body">
        <div class="ledger">
          <section class="gone">
            <h3>{t('removal.goes')}</h3>
            <ul>
              <li>{plural('removal.bindings', connector.binding_count)}</li>
              {#if connector.quarantine_count > 0}
                <li>{plural('removal.quarantined', connector.quarantine_count)}</li>
              {/if}
              <li>{t('removal.evidence')}</li>
            </ul>
          </section>

          <section class="kept">
            <h3>{t('suspension.stays')}</h3>
            <ul>
              <li>{plural('removal.targets', connector.binding_count)}</li>
              <li>{t('removal.keepChecks')}</li>
            </ul>
          </section>
        </div>

        {#if connector.kind === 'proxmox' && connector.credential_management === 'managed'}
          <p>{t('proxmox.removalHint')}</p>
          <p>{t('connection.cleanupAt', { endpoint: connector.managed_cleanup_endpoint || connector.endpoint })}</p>
          <div class="field"><label for="remove-pve-token">{t('proxmox.tokenID')}</label><input id="remove-pve-token" bind:value={tokenID} autocomplete="off" /></div>
          <div class="field"><label for="remove-pve-secret">{t('proxmox.tokenSecret')}</label><input id="remove-pve-secret" type="password" bind:value={secret} autocomplete="off" /></div>
        {/if}
        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>

      <footer>
        <span class="faint note">
          {t('removal.note')}
        </span>
        <Button onclick={onclose} disabled={busy}>{t('common.cancel')}</Button>
        <Button variant="danger" onclick={remove} disabled={busy}>
          {busy ? 'Suppression…' : 'Supprimer le Connecteur'}
        </Button>
      </footer>
    </div>
  {/snippet}
</Modal>

<style>
  .modal {
    max-width: 38rem;
  }

  /* La note du pied est plus courte que celle des assistants : sans base plus
   * étroite, elle réclamait 18rem et repoussait les deux boutons sur deux
   * lignes séparées. */
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

  .gone h3 {
    color: var(--crit);
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
