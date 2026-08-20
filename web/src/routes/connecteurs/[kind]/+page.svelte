<script lang="ts">
  import { plural, t } from '$lib/i18n.svelte';
  /* Écran 4e — Connexion guidée d'un Connecteur.
   * Le parcours reste Adresse → Autorisation → Aperçu. Les ateliers existants
   * portent déjà cette séquence et ses vérifications ; l'écran leur donne une
   * route propre et le fil d'Ariane attendu. */

  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import Topbar from '$lib/components/Topbar.svelte';
  import ZabbixConnector from '$lib/components/ZabbixConnector.svelte';
  import UptimeKumaConnector from '$lib/components/UptimeKumaConnector.svelte';
  import PatchMonConnector from '$lib/components/PatchMonConnector.svelte';
  import GenericWebhookConnector from '$lib/components/GenericWebhookConnector.svelte';
  import { session } from '$lib/session.svelte';
  import type { ConnectorImportResult } from '$lib/api';

  const known = {
    zabbix: 'Zabbix',
    'uptime-kuma': 'Uptime Kuma',
    patchmon: 'PatchMon',
    'generic-webhook': t('connector.genericWebhook')
  } as const;

  type Kind = keyof typeof known;

  const kind = $derived(page.params.kind as Kind | undefined);
  const label = $derived(kind && kind in known ? known[kind] : null);

  function leave() {
    void goto('/connecteurs');
  }

  async function imported(result: ConnectorImportResult) {
    await Promise.all([session.loadTargets(), session.loadConnectors()]);
    session.showNotice(
      plural(
        result.connector.kind === 'uptime_kuma'
          ? 'connectorPage.kumaImported'
          : result.connector.kind === 'patchmon'
            ? 'connectorPage.patchmonImported'
            : 'connectorPage.zabbixImported',
        result.targets.length
      )
    );
    leave();
  }
</script>

<svelte:head>
  <title>{label ? t('connectorPage.connect', { name: label }) : t('connectorPage.title')} — {session.instanceLabel}</title>
</svelte:head>

<Topbar
  crumbs={[
    { label: t('nav.connectors'), href: '/connecteurs' },
    { label: label ?? t('state.unknown') }
  ]}
/>

{#if !label}
  <div class="page">
    <div class="card">
      <div class="empty">
        <strong>{t('connectorPage.unknown')}</strong>
        {t('connectorPage.unknownHint')}
        <p class="back"><a href="/connecteurs">Revenir aux Connecteurs</a></p>
      </div>
    </div>
  </div>
{:else if kind === 'zabbix'}
  <ZabbixConnector onclose={leave} onsuccess={imported} />
{:else if kind === 'uptime-kuma'}
  <UptimeKumaConnector onclose={leave} onsuccess={imported} />
{:else if kind === 'patchmon'}
  <PatchMonConnector onclose={leave} onsuccess={imported} />
{:else}
  <GenericWebhookConnector
    onclose={leave}
    onsuccess={async (created) => {
      await session.loadConnectors();
      session.showNotice(
        t('connectorPage.webhookReady', { name: created.connector.name })
      );
      leave();
    }}
  />
{/if}

<style>
  .back {
    margin-top: var(--s4);
  }

  .back a {
    color: var(--accent);
  }
</style>
