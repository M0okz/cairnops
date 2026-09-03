<script lang="ts">
  import type { ConnectorAccessPreview } from '$lib/api';
  import { t, type MessageKey } from '$lib/i18n.svelte';

  type Product = 'zabbix' | 'uptime_kuma' | 'patchmon';
  type RemoteChange = NonNullable<ConnectorAccessPreview['remote_changes']>[number];

  const managedSummary: Record<Product, MessageKey> = {
    zabbix: 'zabbix.managedSummary',
    uptime_kuma: 'kuma.managedSummary',
    patchmon: 'patchmon.managedSummary'
  };
  const providedSummary: Record<Product, MessageKey> = {
    zabbix: 'zabbix.providedSummary',
    uptime_kuma: 'kuma.providedSummary',
    patchmon: 'patchmon.providedSummary'
  };
  const managedExistingSummary: Record<Product, MessageKey> = {
    zabbix: 'zabbix.managedExistingSummary',
    uptime_kuma: 'kuma.managedExistingSummary',
    patchmon: 'patchmon.managedExistingSummary'
  };
  const remoteChangeMessage: Record<RemoteChange, MessageKey> = {
    create_zabbix_api_token: 'zabbix.createTokenChange',
    create_uptime_kuma_api_key: 'kuma.createKeyChange',
    create_patchmon_host_read_token: 'patchmon.createTokenChange'
  };

  let { access, product }: { access: ConnectorAccessPreview; product: Product } = $props();
</script>

<aside class:managed={access.mode === 'managed'} aria-label={t('wizard.remoteAccess')}>
  <div>
    <span>{t(access.mode === 'managed' ? 'wizard.managedAccess' : 'wizard.providedAccess')}</span>
    <strong>{t(access.mode === 'provided'
      ? providedSummary[product]
      : access.will_provision
        ? managedSummary[product]
        : managedExistingSummary[product])}</strong>
  </div>
  {#if access.remote_changes?.length}
    <div class="changes">
      <span>{t('wizard.onConfirmation')}</span>
      <ul>
        {#each access.remote_changes as change}<li>{t(remoteChangeMessage[change])}</li>{/each}
      </ul>
    </div>
  {/if}
</aside>

<style>
  aside {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
    gap: var(--s4);
    padding: var(--s4);
    margin-bottom: var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
  }
  aside.managed { border-color: color-mix(in srgb, var(--accent) 38%, var(--line-strong)); }
  span { display: block; color: var(--muted); font-size: .6875rem; }
  strong { display: block; margin-top: .2rem; font-size: .75rem; font-weight: 600; }
  ul { margin: .25rem 0 0; padding-left: 1rem; font-size: .75rem; }
</style>
