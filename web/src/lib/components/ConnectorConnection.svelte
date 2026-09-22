<script lang="ts">
  import { onMount } from 'svelte';
  import { api, APIError, type Connector, type ConnectorConnectionTest, type ProxmoxCertificate } from '$lib/api';
  import { session, messageFrom } from '$lib/session.svelte';
  import { t } from '$lib/i18n.svelte';
  import Icon from './Icon.svelte';

  let { connector, onclose, onsuccess }: { connector: Connector; onclose: () => void; onsuccess: () => Promise<void> | void } = $props();
  let dialog: HTMLDialogElement;
  let name = $state('');
  let address = $state('');
  let token = $state('');
  let identifier = $state('');
  let password = $state('');
  let busy = $state<'test' | 'save' | 'certificate' | ''>('');
  let error = $state('');
  let tested = $state<ConnectorConnectionTest | null>(null);
  let certificate = $state<ProxmoxCertificate | null>(null);
  let approvedCertificate = $state(false);

  onMount(() => {
    name = connector.name; address = connector.endpoint;
    dialog.showModal();
    return () => dialog.close();
  });

  function close() { if (!busy) onclose(); }
  function invalidate() { tested = null; error = ''; }
  function addressChanged() { invalidate(); certificate = null; approvedCertificate = false; }

  function input() {
    const common = { name, address };
    if (connector.kind === 'zabbix') return { ...common, api_token: token };
    if (connector.kind === 'uptime_kuma') return { ...common, api_key: token };
    if (connector.kind === 'patchmon') return { ...common, token_key: identifier, token_secret: password };
    if (connector.kind === 'argus') return { ...common, username: identifier, password };
    return { ...common, credentials: { token_id: identifier, secret: password, ...(certificate && approvedCertificate ? { fingerprint: certificate.fingerprint } : {}) } };
  }

  async function test(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;
    tested = null; error = ''; busy = 'test';
    try {
      tested = await api<ConnectorConnectionTest>(`/api/v1/connectors/${connector.id}/connection/test`, { method: 'POST', body: JSON.stringify(input()) });
    } catch (cause) { error = messageFrom(cause); }
    finally { busy = ''; }
  }

  async function inspectCertificate() {
    if (busy) return;
    invalidate(); certificate = null; approvedCertificate = false; busy = 'certificate';
    try { certificate = await api<ProxmoxCertificate>('/api/v1/connectors/proxmox/certificate', { method: 'POST', body: JSON.stringify({ address }) }); }
    catch (cause) { error = messageFrom(cause); }
    finally { busy = ''; }
  }

  async function save() {
    if (!tested || busy) return;
    if (Date.parse(tested.expires_at) <= Date.now()) { tested = null; error = t('connection.expired'); return; }
    busy = 'save'; error = '';
    try {
      await api<Connector>(`/api/v1/connectors/${connector.id}/connection`, { method: 'PUT', body: JSON.stringify({ receipt: tested.receipt }) });
      token = ''; identifier = ''; password = ''; tested = null;
      try { await onsuccess(); } catch { session.showNotice(t('connection.savedRefresh')); onclose(); return; }
      session.showNotice(t('connection.saved', { name }));
      onclose();
    } catch (cause) {
      error = messageFrom(cause);
      if (!(cause instanceof APIError && cause.status === 409 && cause.code === 'connector_sync_in_progress')) tested = null;
    }
    finally { busy = ''; }
  }
</script>

<dialog bind:this={dialog} class="connection-dialog modal" aria-labelledby="connection-title" oncancel={(event) => { event.preventDefault(); close(); }}>
  <header>
    <div><h2 id="connection-title">{t('connection.title', { name: connector.name })}</h2><p>{t('connection.lead')}</p></div>
    <button class="close" type="button" onclick={close} disabled={!!busy} aria-label={t('common.close')}><Icon name="close" size={16} /></button>
  </header>
  <form onsubmit={test} oninput={invalidate}>
    <div class="modal-body">
      <fieldset disabled={!!busy}>
        <div class="field">
          <label for="connection-name">{t('connection.name')}</label>
          <input id="connection-name" name="connection-name" bind:value={name} required maxlength="160" autocomplete="off" />
        </div>
        <div class="field">
          <label for="connection-address">{t('connection.address')}</label>
          <input id="connection-address" name="connection-address" type="url" bind:value={address} oninput={addressChanged} required maxlength="2048" autocomplete="url" aria-describedby="connection-address-hint" />
          <small id="connection-address-hint">{t('connection.sameInstance')}</small>
        </div>
        <p id="connection-secret-hint" class="muted">{t('connection.keepSecret')}</p>
        {#if connector.kind === 'zabbix' || connector.kind === 'uptime_kuma'}
          <div class="field">
            <label for="connection-token">{t('connection.token')}</label>
            <input id="connection-token" name="connection-token" type="password" bind:value={token} autocomplete="new-password" aria-describedby="connection-secret-hint" />
          </div>
        {:else}
          <div class="field">
            <label for="connection-identifier">{connector.kind === 'argus' ? t('connection.username') : connector.kind === 'patchmon' ? t('connection.tokenKey') : t('connection.tokenId')}</label>
            <input id="connection-identifier" name="connection-identifier" bind:value={identifier} autocomplete="off" aria-describedby="connection-secret-hint" />
          </div>
          <div class="field">
            <label for="connection-password">{connector.kind === 'argus' ? t('connection.password') : t('connection.tokenSecret')}</label>
            <input id="connection-password" name="connection-password" type="password" bind:value={password} autocomplete="new-password" aria-describedby="connection-secret-hint" />
          </div>
        {/if}
        {#if connector.kind === 'proxmox'}
          <section class="certificate" aria-label={t('connection.certificate')}>
            <button class="btn" type="button" onclick={inspectCertificate}>{busy === 'certificate' ? t('connection.checking') : t('connection.checkCertificate')}</button>
            {#if certificate}
              <p>{certificate.subject}</p>
              <p class="fingerprint">SHA-256 · {certificate.fingerprint}</p>
              <label class="approval"><input type="checkbox" bind:checked={approvedCertificate} />{t('connection.approveCertificate')}</label>
            {/if}
          </section>
        {/if}
      </fieldset>
      <div class="test-result" role="status">
        {#if tested}
          <strong>{t('connection.verified')}</strong>
          <p>{tested.endpoint}{tested.version ? ` · v${tested.version}` : ''}</p>
          {#if tested.compatibility === 'warning'}<p>{t('connectors.compatibilityToCheck')}</p>{/if}
          {#if !tested.encrypted_transport}<p>{t('connectors.plainTransport')}</p>{/if}
        {/if}
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>
    <footer>
      <button class="btn" type="button" onclick={close} disabled={!!busy}>{t('common.cancel')}</button>
      {#if tested}
        <button class="btn primary" type="button" onclick={save} disabled={!!busy}>{busy === 'save' ? t('connection.saving') : t('connection.save')}</button>
      {:else}
        <button class="btn primary" type="submit" disabled={!!busy}>{busy === 'test' ? t('connection.testing') : t('connection.test')}</button>
      {/if}
    </footer>
  </form>
</dialog>

<style>
  .connection-dialog { width: min(var(--connector-modal-max), calc(100vw - var(--s5))); margin: auto; padding: 0; color: var(--ink); max-height: calc(100dvh - var(--s5)); overscroll-behavior: contain; }
  .connection-dialog::backdrop { background: var(--drawer-backdrop); }
  fieldset { display: grid; gap: var(--s4); min-width: 0; border: 0; padding: 0; margin: 0; }
  .field { min-width: 0; }
  .muted, .certificate, .test-result { font-size: .8125rem; }
  .certificate { display: grid; gap: var(--s3); }
  .certificate .btn { justify-self: start; }
  .fingerprint, .test-result p { overflow-wrap: anywhere; }
  .approval { display: flex; gap: var(--s2); align-items: center; }
  .approval input { width: auto; }
  .test-result:not(:empty), .error { margin-top: var(--s4); }
</style>
