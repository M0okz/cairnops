<script lang="ts">
  import Icon from './Icon.svelte';
  import type { Device } from '$lib/api';
  import { t } from '$lib/i18n.svelte';

  let {
    device,
    error = '',
    onclose,
    onconfirm
  }: {
    device: Device;
    error?: string;
    onclose: () => void;
    onconfirm: () => Promise<void> | void;
  } = $props();

  let busy = $state(false);

  async function revoke() {
    busy = true;
    try {
      await onconfirm();
    } finally {
      busy = false;
    }
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && !busy && onclose()}>
  <div class="modal revoke-modal" role="dialog" aria-modal="true" aria-labelledby="revoke-device-title">
    <header>
      <div>
        <h2 id="revoke-device-title">{t('devices.revokeTitle', { name: device.name })}</h2>
        <p>{t('devices.revokeLead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    <div class="modal-body">
      <div class="device-summary">
        <strong>{device.name}</strong>
        <span>{device.user_display_name} · {device.platform === 'ios' ? 'iOS' : 'Android'}</span>
      </div>
      <p>{t('devices.revokeExplain')}</p>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>

    <footer>
      <span class="faint note">{t('devices.revokeScope')}</span>
      <button class="btn" type="button" onclick={onclose} disabled={busy}>{t('common.cancel')}</button>
      <button class="btn danger" type="button" onclick={revoke} disabled={busy}>
        {busy ? t('devices.revoking') : t('devices.revokeConfirm')}
      </button>
    </footer>
  </div>
</div>

<style>
  .revoke-modal {
    max-width: 38rem;
  }

  .device-summary {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s4);
    padding: var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
  }

  .device-summary strong {
    font-size: 0.8125rem;
  }

  .device-summary span,
  .modal-body > p {
    color: var(--muted);
    font-size: 0.75rem;
  }

  .modal-body > p {
    margin-top: var(--s4);
    line-height: 1.5;
  }
</style>
