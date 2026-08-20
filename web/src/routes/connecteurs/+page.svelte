<script lang="ts">
  /* Écran 4d — Connecteurs & Sources.
   * Chaque Connecteur annonce ce qu'il lit et ce qu'il écrit. Aucune écriture
   * n'est implicite : elle est nommée sur la fiche. */

  import Topbar from '$lib/components/Topbar.svelte';
  import ConnectorChooser from '$lib/components/ConnectorChooser.svelte';
  import Icon, { type IconName } from '$lib/components/Icon.svelte';
  import BrandMark, { type BrandName } from '$lib/components/BrandMark.svelte';
  import WebhookQuarantine from '$lib/components/WebhookQuarantine.svelte';
  import ConnectorRemoval from '$lib/components/ConnectorRemoval.svelte';
  import ConnectorSuspension from '$lib/components/ConnectorSuspension.svelte';
  import MattermostConnector from '$lib/components/MattermostConnector.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import { goto } from '$app/navigation';
  import { session } from '$lib/session.svelte';
  import { since } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';
  /* Le sas porte le même nom que son atelier : on renomme le type, pas le
   composant, pour que la fiche garde le vocabulaire du domaine. */
  import {
    api,
    type Connector,
    type NotificationChannel,
    type WebhookQuarantine as QuarantineEntry
  } from '$lib/api';

  let chooserOpen = $state(false);
  let quarantineFor = $state<Connector | null>(null);
  let removalFor = $state<Connector | null>(null);
  let suspensionFor = $state<Connector | null>(null);
  let mattermostOpen = $state(false);
  let now = $state(new Date());

  /* Suspendre et supprimer engagent l'Espace entier : seul l'administrateur les
   * voit, comme pour l'ajout d'un Connecteur. */
  const isAdministrator = $derived(session.user?.role === 'administrator');

  let suspending = $state('');

  /* Suspendre retire toutes les Sources externes du calcul : on l'annonce avant.
   * Reprendre ne retire rien et part sans confirmation. */
  function askSuspension(connector: Connector) {
    if (connector.status === 'disabled') {
      void applySuspension(connector);
      return;
    }
    suspensionFor = connector;
  }

  async function applySuspension(connector: Connector) {
    suspending = connector.id;
    await session.setConnectorSuspension(connector);
    suspending = '';
    suspensionFor = null;
  }

  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });

  const contracts = $derived<Record<Connector['kind'], string>>({
    zabbix: t('connectors.contract.zabbix'),
    uptime_kuma: t('connectors.contract.uptimeKuma'),
    patchmon: t('connectors.contract.patchmon'),
    generic_webhook: t('connectors.contract.genericWebhook')
  });

  /* Zabbix et Uptime Kuma portent leur propre marque ; le webhook générique
   * n'est le produit de personne et garde le trait maison. */
  const kindBrands: Partial<Record<Connector['kind'], BrandName>> = {
    zabbix: 'zabbix',
    uptime_kuma: 'uptime_kuma',
    patchmon: 'patchmon'
  };

  const statusLabels = $derived<Record<Connector['status'], { label: string; tone: string }>>({
    connected: { label: t('connector.status.connected'), tone: 'ok' },
    degraded: { label: t('connector.status.degraded'), tone: 'warn' },
    disabled: { label: t('target.suspended'), tone: 'idle' }
  });

  const boundTargets = $derived(
    session.connectors.reduce((total, connector) => total + connector.binding_count, 0)
  );

  const quarantined = $derived(
    session.connectors.reduce((total, connector) => total + connector.quarantine_count, 0)
  );

  /* Les voies de notification sont des Connecteurs comme les autres : elles
   * s'annoncent sur la même grille, avec le même contrat. Ce qui les distingue
   * est le sens du trafic, et il est écrit sur la fiche. */
  const channelStatus = (channel: NotificationChannel) =>
    !channel.enabled
      ? { label: t('target.suspended'), tone: 'idle' }
      : channel.status === 'connected'
        ? { label: t('connector.status.connected'), tone: 'ok' }
        : channel.status === 'degraded'
          ? { label: t('connector.status.degraded'), tone: 'warn' }
          : { label: t('target.suspended'), tone: 'idle' };

  /* Un Canal n'est pas un Connecteur : il porte la parole vers l'extérieur au
   * lieu de rapporter des preuves, et le Canal intégré est ouvert par
   * construction. Les compter ensemble annoncerait un connecté de plus alors
   * qu'aucune Intégration ne rapporte : le chapeau les tient séparés. */
  const connectedCount = $derived(
    session.connectors.filter((connector) => connector.status === 'connected').length
  );

  const openChannels = $derived(
    session.channels.filter((channel) => channel.enabled && channel.status === 'connected').length
  );

  const hasMattermost = $derived(session.channels.some((channel) => channel.kind === 'mattermost'));

  /* La quarantaine se lit sur l'écran plutôt que derrière un bouton : une
   * identité retenue est une preuve qui n'entre pas, et cela doit se voir sans
   * qu'on aille la chercher. La décision, elle, reste dans son atelier. */
  let quarantineRows = $state<Record<string, QuarantineEntry[]>>({});

  const withQuarantine = $derived(
    session.connectors.filter(
      (connector) => connector.kind === 'generic_webhook' && connector.quarantine_count > 0
    )
  );

  $effect(() => {
    if (!isAdministrator) return;
    for (const connector of withQuarantine) {
      const expected = connector.quarantine_count;
      if ((quarantineRows[connector.id] ?? []).length === expected) continue;
      void api<{ quarantine: QuarantineEntry[] }>(`/api/v1/connectors/${connector.id}/quarantine`)
        .then((response) => {
          quarantineRows = { ...quarantineRows, [connector.id]: response.quarantine };
        })
        .catch(() => {
          /* Une quarantaine illisible ne vide pas l'écran : le compte reste
             affiché sur la fiche du Connecteur. */
        });
    }
  });
</script>

<svelte:head><title>Connecteurs — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.settings'), href: '/reglages' }, { label: t('nav.connectors') }]} />

<div class="page">
  <div class="page-head">
    <div>
      <h1>{t('nav.connectors')}</h1>
      <p>
        {plural('connectors.connected', connectedCount)} ·{#if session.channels.length > 0}
          {plural('connectors.channelsOpen', openChannels)} ·{/if}
        {plural('connectors.activeSources', boundTargets, { targets: session.targets.length })}
      </p>
    </div>
    <div class="page-actions">
      {#if isAdministrator && !hasMattermost}
        <button class="btn" type="button" onclick={() => (mattermostOpen = true)}>Relier Mattermost</button>
      {/if}
      <button class="btn primary" type="button" onclick={() => (chooserOpen = true)}>Ajouter un Connecteur</button>
    </div>
  </div>

  <div class="grid">
    {#each session.connectors as connector (connector.id)}
      {@const status = statusLabels[connector.status]}
      <article class="card connector">
        <div class="card-body">
          <div class="head">
            {#if kindBrands[connector.kind]}
              <BrandMark name={kindBrands[connector.kind]!} size={34} />
            {:else}
              <span class="key"><Icon name="webhook" size={16} /></span>
            {/if}
            <span class="id">
              <strong>{connector.name}</strong>
              <small class="faint">{connector.endpoint}</small>
            </span>
            <span class="pill {status.tone}">{status.label}</span>
          </div>

          <div class="figures">
            <div class="fig"><b><Odometer value={connector.binding_count} /></b><span>Sources</span></div>
            <div class="fig">
              <b class={connector.quarantine_count > 0 ? 'warn' : ''}><Odometer value={connector.quarantine_count} /></b>
              <span>En quarantaine</span>
            </div>
            <div class="fig">
              <b><Odometer value={connector.last_checked_at ? since(connector.last_checked_at, now) : '—'} /></b>
              <span>Signal</span>
            </div>
          </div>

          <p class="contract">{contracts[connector.kind]}</p>

          {#if connector.last_error}
            <p class="error">{connector.last_error}</p>
          {/if}

          <div class="acts">
            {#if connector.compatibility === 'warning'}
              <span class="pill warn">{t('connectors.compatibilityToCheck')}</span>
            {:else if connector.remote_version}
              <span class="faint num">v{connector.remote_version}</span>
            {/if}
            {#if !connector.encrypted_transport}
              <span class="pill crit" title={t('connectors.plainTransportTitle')}>
                {t('connectors.plainTransport')}
              </span>
            {/if}
            {#if isAdministrator}
              <span class="spacer"></span>
              <button
                class="btn sm"
                type="button"
                disabled={suspending === connector.id}
                onclick={() => askSuspension(connector)}
              >
                {connector.status === 'disabled' ? 'Reprendre' : 'Suspendre'}
              </button>
              <button class="btn sm danger" type="button" onclick={() => (removalFor = connector)}>
                Supprimer
              </button>
            {/if}
          </div>
        </div>
      </article>
    {/each}

    <!-- Une voie de notification tient sur la même grille : elle annonce, comme
         les autres, ce qu'elle lit et ce qu'elle écrit. -->
    {#each session.channels as channel (channel.id)}
      {@const status = channelStatus(channel)}
      <article class="card connector">
        <div class="card-body">
          <div class="head">
            <!-- Le Canal intégré est l'instance elle-même : son nom et sa
                 destination sont dits par l'interface, pas lus en base, pour
                 qu'ils suivent la langue comme le reste de l'écran. -->
            <span class="key">{channel.kind === 'in_app' ? 'IN' : 'MM'}</span>
            <span class="id">
              <strong>
                {channel.kind === 'in_app' ? t('notifications.inAppName') : channel.name}
              </strong>
              <small class="faint">
                {channel.kind === 'in_app' ? t('notifications.inAppEndpoint') : channel.endpoint}
              </small>
            </span>
            <span class="pill {status.tone}">{status.label}</span>
          </div>

          <div class="figures">
            <div class="fig">
              <b><Odometer value={channel.severities.length} /></b>
              <span>{t('mattermost.routedSeverities')}</span>
            </div>
            <div class="fig">
              <b class={channel.last_error ? 'warn' : ''}>
                <Odometer value={channel.last_checked_at ? since(channel.last_checked_at, now) : t('common.none')} />
              </b>
              <span>{t('connectors.lastCheck')}</span>
            </div>
          </div>

          <p class="contract">
            {channel.kind === 'in_app'
              ? t('connectors.contract.inApp')
              : t('connectors.contract.mattermost')}
          </p>

          {#if channel.last_error}
            <p class="error">{channel.last_error}</p>
          {/if}

          <div class="acts">
            <span class="faint num">{channel.severities.join(', ') || t('connectors.noSeverity')}</span>
            {#if !channel.encrypted_transport}
              <span class="pill crit" title={t('connectors.plainTransportTitle')}>
                {t('connectors.plainTransport')}
              </span>
            {/if}
          </div>
        </div>
      </article>
    {/each}

    {#if session.connectors.length === 0 && session.channels.length === 0}
      <div class="card">
        <div class="empty">
          <strong>{t('health.noConnectors')}</strong>
          {t('connectors.emptyHint')}
        </div>
      </div>
    {/if}
  </div>

  <!-- Le sas se lit à découvert : une identité retenue est une preuve qui
       n'entre pas, et ce silence doit être visible sans ouvrir un atelier. -->
  {#each withQuarantine as connector (connector.id)}
    {@const rows = quarantineRows[connector.id] ?? []}
    <div class="card sas">
      <header>
        <i class="dot warn"></i>
        <h2>
          {connector.name} · {plural('connectors.quarantined', connector.quarantine_count)}
        </h2>
        <span class="note warn">{t('connectors.quarantineNote')}</span>
      </header>

      <div class="thead">
        <span>{t('connectors.column.identity')}</span>
        <span>{t('connectors.column.signals')}</span>
        <span class="hide-sm">{t('connectors.column.lastSignal')}</span>
        <span class="hide-sm">{t('connectors.column.payload')}</span>
        <span></span>
      </div>

      {#each rows as row (row.id)}
        <div class="trow">
          <span class="mono identity">{row.external_identity}</span>
          <span class="num"><Odometer value={row.occurrences} /></span>
          <span class="num hide-sm"><Odometer value={t('overview.ago', { duration: since(row.last_seen_at, now) })} /></span>
          <span class="faint payload hide-sm" title={row.summary}>{row.summary}</span>
          {#if isAdministrator}
            <button class="btn sm attach" type="button" onclick={() => (quarantineFor = connector)}>
              {t('connectors.attachToTarget')}
            </button>
          {:else}
            <span></span>
          {/if}
        </div>
      {:else}
        <div class="empty">
          <strong>{plural('connectors.held', connector.quarantine_count)}</strong>
          {isAdministrator ? t('connectors.readingAirlock') : t('connectors.adminOnlyAirlock')}
        </div>
      {/each}
    </div>
  {/each}
</div>

{#if chooserOpen}
  <ConnectorChooser
    onclose={() => (chooserOpen = false)}
    onselect={(kind) => {
      chooserOpen = false;
      void goto(`/connecteurs/${kind.replace('_', '-')}`);
    }}
  />
{/if}

{#if quarantineFor}
  <WebhookQuarantine
    connector={quarantineFor}
    targets={session.targets}
    onclose={() => (quarantineFor = null)}
    onsuccess={async (approval) => {
      await Promise.all([session.loadTargets(), session.loadConnectors(), session.loadIncidents()]);
      session.showNotice(
        t('connectors.identityBound', { identity: approval.identity, target: approval.target_name }) +
          ' ' +
          plural('connectors.statesReplayed', approval.replayed)
      );
    }}
  />
{/if}

{#if removalFor}
  <ConnectorRemoval
    connector={removalFor}
    onclose={() => (removalFor = null)}
    onsuccess={async (removal) => {
      removalFor = null;
      await Promise.all([session.loadConnectors(), session.loadTargets(), session.loadIncidents()]);
      const closed = removal.resolved_incidents;
      session.showNotice(
        t('connectors.removed', { name: removal.name }) +
          ' ' +
          plural('connectors.unbound', removal.bindings) +
          ' ' +
          (closed > 0
            ? plural('connectors.closedIncidents', closed)
            : t('connectors.noIncidentDepended'))
      );
    }}
  />
{/if}

{#if suspensionFor}
  <ConnectorSuspension
    connector={suspensionFor}
    onclose={() => (suspensionFor = null)}
    onconfirm={() => applySuspension(suspensionFor!)}
  />
{/if}

{#if mattermostOpen}
  <MattermostConnector
    onclose={() => (mattermostOpen = false)}
    onsuccess={async (channel) => {
      await session.loadNotifications();
      session.showNotice(t('connectors.mattermostLinked', { name: channel.name }));
    }}
  />
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(24rem, 1fr));
    gap: var(--s4);
  }

  .head {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    margin-bottom: var(--s4);
  }

  .key {
    width: 2.125rem;
    height: 2.125rem;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--r-m);
    background: var(--surface-2);
    color: var(--muted);
    font-size: 0.6875rem;
    font-weight: 600;
  }

  .id {
    flex: 1;
    min-width: 0;
  }

  .id strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .id small {
    display: block;
    font-family: var(--font-num);
    font-size: 0.6875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .figures {
    gap: var(--s5);
    padding-bottom: var(--s4);
    border-bottom: 1px solid var(--line);
  }

  .figures .fig b {
    font-size: 0.9375rem;
  }

  .contract {
    margin-top: var(--s4);
    color: var(--muted);
    font-size: 0.75rem;
  }

  /* Deux fiches côte à côte n'ont pas la même hauteur de contenu : sans cela,
   * celle qui ne dit pas sa version remonte ses boutons d'une ligne. Les
   * actions se posent au bas de la carte, à la même hauteur pour toutes. */
  .connector {
    display: flex;
  }

  .connector .card-body {
    display: flex;
    flex: 1;
    flex-direction: column;
  }

  .acts {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin-top: auto;
    padding-top: var(--s4);
    flex-wrap: wrap;
  }

  /* Les actions de gouvernance se tiennent à droite, séparées des états qu'elles
   * ne commentent pas. */
  .spacer {
    flex: 1;
  }

  /* ── Le sas des identités inconnues ─────────────────────────────────────
     La bande porte la teinte de l'avertissement sur toute sa tête : ce n'est
     pas un défaut de la supervision, c'est une décision qui attend. */
  .sas {
    --cols: minmax(0, 1fr) 5rem 8.75rem minmax(0, 1.2fr) auto;
    margin-top: var(--s5);
    border-color: var(--warn-line);
  }

  .sas > header {
    background: var(--warn-bg);
    border-bottom-color: var(--warn-line);
  }

  .sas > header h2 {
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .sas .note {
    color: var(--warn);
  }

  .identity {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .payload {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Rattacher est la seule action qui fasse entrer une preuve : elle porte
     l'accent, sans devenir l'action dominante de l'écran. */
  .attach {
    border-color: var(--accent);
    color: var(--accent);
  }

  @media (max-width: 48rem) {
    .sas {
      --cols: minmax(0, 1fr) 4rem auto;
    }
  }
</style>
