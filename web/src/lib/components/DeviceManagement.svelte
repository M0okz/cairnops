<script lang="ts">
  import { onMount } from 'svelte';
  import DevicePairingDialog from './DevicePairingDialog.svelte';
  import DeviceRevocation from './DeviceRevocation.svelte';
  import { api, type Device } from '$lib/api';
  import { since } from '$lib/format';
  import { t } from '$lib/i18n.svelte';
  import { messageFrom, session } from '$lib/session.svelte';

  let devices = $state<Device[]>([]);
  let loading = $state(true);
  let error = $state('');
  let pairing = $state(false);
  let revoking = $state<Device | null>(null);
  let revokeError = $state('');
  let now = $state(new Date());

  const isAdministrator = $derived(session.user?.role === 'administrator');

  async function load() {
    try {
      devices = (await api<{ devices: Device[] }>('/api/v1/devices')).devices;
      error = '';
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      loading = false;
    }
  }

  async function paired() {
    await load();
  }

  async function revoke() {
    if (!revoking) return;
    revokeError = '';
    try {
      const name = revoking.name;
      await api<void>(`/api/v1/devices/${revoking.id}`, { method: 'DELETE' });
      await load();
      revoking = null;
      session.showNotice(t('devices.revokedNotice', { name }));
    } catch (cause) {
      revokeError = messageFrom(cause);
    }
  }

  function openRevocation(device: Device) {
    revokeError = '';
    revoking = device;
  }

  onMount(() => {
    void load();
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });
</script>

<div class="band-row devices-band">
  <div>
    <h2 class="band">{t('devices.title')}</h2>
    <p>{t('devices.lead')}</p>
  </div>
  <button class="btn primary sm" type="button" onclick={() => (pairing = true)}>
    {t('devices.pairAction')}
  </button>
</div>

<div class="card devices-card">
  {#if loading}
    <div class="empty">{t('devices.loading')}</div>
  {:else if error}
    <div class="empty">
      <strong>{t('devices.unavailable')}</strong>
      <p>{error}</p>
      <button class="btn sm" type="button" onclick={load}>{t('common.retry')}</button>
    </div>
  {:else if devices.length === 0}
    <div class="empty">
      <strong>{t('devices.empty')}</strong>
      <p>{t('devices.emptyHint')}</p>
    </div>
  {:else}
    <div class="thead">
      <span>{t('devices.columnDevice')}</span>
      <span>{t('devices.columnDetails')}</span>
      <span>{t('devices.columnState')}</span>
      <span></span>
    </div>

    {#each devices as device (device.id)}
      <div class="trow device-row" class:revoked={device.revoked_at !== null}>
        <span class="device-name">
          <strong>{device.name}</strong>
          <small>{device.platform === 'ios' ? 'iOS' : 'Android'}{device.app_version ? ` · v${device.app_version}` : ''}</small>
        </span>

        <span class="device-details">
          {#if isAdministrator}<span>{device.user_display_name}</span>{/if}
          <span>{device.last_seen_at ? t('devices.seenAgo', { time: since(device.last_seen_at, now) }) : t('devices.neverConnected')}</span>
          <span>{device.push_enabled ? t('devices.pushActive') : t('devices.pushInactive')}</span>
        </span>

        {#if device.revoked_at}
          <span class="pill crit">{t('devices.revoked')}</span>
        {:else}
          <span class="pill ok">{t('devices.active')}</span>
        {/if}

        <span class="device-actions">
          {#if !device.revoked_at}
            <button class="btn sm" type="button" onclick={() => openRevocation(device)}>{t('devices.revoke')}</button>
          {/if}
        </span>
      </div>
    {/each}

    <p class="device-footnote">{t('devices.identityDoctrine')}</p>
  {/if}
</div>

{#if pairing}
  <DevicePairingDialog
    onclose={() => (pairing = false)}
    onpaired={paired}
  />
{/if}

{#if revoking}
  <DeviceRevocation
    device={revoking}
    error={revokeError}
    onclose={() => (revoking = null)}
    onconfirm={revoke}
  />
{/if}

<style>
  .devices-band {
    display: flex;
    align-items: center;
    gap: var(--s4);
    margin: var(--s6) 0 var(--s4);
  }

  .devices-band > div {
    min-width: 0;
    flex: 1;
  }

  .devices-band .band {
    margin: 0;
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .devices-band p {
    margin-top: var(--s1);
    color: var(--faint);
    font-size: 0.75rem;
  }

  .devices-card {
    --cols: minmax(12rem, 1fr) minmax(16rem, 1.25fr) 7rem 6rem;
  }

  .empty p {
    margin: var(--s3) 0 var(--s4);
  }

  .device-name,
  .device-details {
    min-width: 0;
  }

  .device-name strong,
  .device-name small {
    display: block;
  }

  .device-name strong {
    overflow: hidden;
    font-size: 0.8125rem;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .device-name small {
    margin-top: var(--s1);
    color: var(--faint);
    font-family: var(--font-num);
    font-size: 0.6875rem;
  }

  .device-details {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2) var(--s4);
    color: var(--faint);
    font-size: 0.6875rem;
  }

  .device-details span + span::before {
    content: '·';
    margin-right: var(--s4);
    color: var(--dim);
  }

  .device-actions {
    display: flex;
    justify-content: flex-end;
  }

  .device-row.revoked .device-name,
  .device-row.revoked .device-details {
    opacity: 0.55;
  }

  .device-footnote {
    padding: var(--s4) 1rem;
    border-top: 1px solid var(--line);
    background: var(--bg);
    color: var(--faint);
    font-size: 0.75rem;
  }

  @media (max-width: 48rem) {
    .devices-band {
      align-items: flex-start;
      flex-direction: column;
    }

    .device-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .device-details {
      grid-column: 1 / -1;
      grid-row: 2;
    }

    .device-row > .pill {
      grid-column: 2;
      grid-row: 1;
    }

    .device-actions {
      grid-column: 1 / -1;
      grid-row: 3;
      justify-content: flex-start;
    }
  }
</style>
