<script lang="ts">
  import Icon from './Icon.svelte';
  import ReconciliationSummary from './ReconciliationSummary.svelte';
  import TargetDecision from './TargetDecision.svelte';
  import { onMount } from 'svelte';
  import { api, type ZabbixHostPreview, type ZabbixImportResult, type ZabbixPreview } from '$lib/api';
  import { clock } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';
  import {
    prepareTargetAssignments,
    reconciliationCounts,
    resolvedTargetAssignments
  } from '$lib/reconciliation';

  let {
    onclose,
    onsuccess,
    connectorId = '',
    initialName = '',
    initialAddress = ''
  }: {
    onclose: () => void;
    onsuccess: (result: ZabbixImportResult) => Promise<void> | void;
    connectorId?: string;
    initialName?: string;
    initialAddress?: string;
  } = $props();

  let addressInput = $state<HTMLInputElement>();
  let name = $state('Zabbix');
  let address = $state('');
  let apiToken = $state('');
  let preview = $state<ZabbixPreview | null>(null);
  let imported = $state<ZabbixImportResult | null>(null);
  let selected = $state<string[]>([]);
  let targetAssignments = $state<Record<string, string>>({});
  let query = $state('');
  let busy = $state(false);
  let error = $state('');
  let reconciliation = $derived(reconciliationCounts(selected, targetAssignments));
  let initialized = $state(false);

  $effect(() => {
    if (initialized) return;
    initialized = true;
    if (initialName) name = initialName;
    if (initialAddress) address = initialAddress;
  });

  onMount(() => connectorId ? void inspectExisting() : addressInput?.focus());

  function adoptPreview(value: ZabbixPreview) {
    preview = value;
    selected = value.hosts.filter((host) => !host.already_imported_to).map((host) => host.external_id);
    targetAssignments = prepareTargetAssignments(value.hosts);
  }

  async function inspectExisting() {
    busy = true; error = '';
    try {
      adoptPreview(await api<ZabbixPreview>(`/api/v1/connectors/${connectorId}/preview`, { method: 'POST' }));
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('zabbix.verifyFailed');
    } finally { busy = false; }
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  function stage() {
    if (imported) return 3;
    if (preview) return 2;
    return 1;
  }

  function filteredHosts() {
    const needle = query.trim().toLocaleLowerCase('fr');
    if (!needle || !preview) return preview?.hosts ?? [];
    return preview.hosts.filter((host) =>
      [host.name, host.technical_name, ...host.interfaces.map((item) => item.address)].some((value) => value.toLocaleLowerCase('fr').includes(needle))
    );
  }

  function primaryInterface(host: ZabbixHostPreview) {
    return host.interfaces.find((item) => item.main) ?? host.interfaces[0];
  }

  function importableHosts() {
    return preview?.hosts.filter((host) => !host.already_imported_to) ?? [];
  }

  function visibleImportableHosts() {
    return filteredHosts().filter((host) => !host.already_imported_to);
  }

  function toggleHost(id: string) {
    selected = selected.includes(id) ? selected.filter((item) => item !== id) : [...selected, id];
  }

  function toggleAllVisible() {
    const visible = visibleImportableHosts().map((host) => host.external_id);
    const allSelected = visible.length > 0 && visible.every((id) => selected.includes(id));
    selected = allSelected ? selected.filter((id) => !visible.includes(id)) : [...new Set([...selected, ...visible])];
  }

  function assignTarget(externalID: string, targetID: string) {
    targetAssignments = { ...targetAssignments, [externalID]: targetID };
  }

  async function inspect(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      adoptPreview(await api<ZabbixPreview>('/api/v1/connectors/zabbix/preview', {
        method: 'POST',
        body: JSON.stringify({ name, address, api_token: apiToken })
      }));
      apiToken = '';
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('zabbix.verifyFailed');
    } finally {
      busy = false;
    }
  }

  async function importHosts() {
    if (!preview || selected.length === 0) return;
    if (reconciliation.review > 0) {
      error = plural('wizard.resolveBeforeImport', reconciliation.review);
      return;
    }
    busy = true;
    error = '';
    try {
      imported = await api<ZabbixImportResult>('/api/v1/connectors/zabbix/import', {
        method: 'POST',
        body: JSON.stringify({
          receipt: preview.receipt,
          host_ids: selected,
          target_assignments: resolvedTargetAssignments(selected, targetAssignments)
        })
      });
      await onsuccess(imported);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('zabbix.importFailed');
    } finally {
      busy = false;
    }
  }

  function resetPreview() {
    preview = null;
    selected = [];
    targetAssignments = {};
    query = '';
    error = '';
    setTimeout(() => addressInput?.focus());
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && !busy && onclose()}>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="connector-title">
    <header>
      <div>
        <h2 id="connector-title">
          {imported ? t('wizard.linked') : preview ? t('wizard.chooseWhatEnters') : t('zabbix.connect')}
        </h2>
        <p>{t('zabbix.lead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label="Fermer">
        <Icon name="close" size={14} />
      </button>
    </header>

    <ol class="stepper">
      <li class:done={stage() > 1} class:on={stage() === 1}>
        <i class="mark">{stage() > 1 ? '✓' : '1'}</i>
        <span class="label">
          <strong>{t('wizard.address')}</strong>
          <small class="faint">{stage() > 1 ? t('wizard.serverVerified') : t('wizard.entryPoint')}</small>
        </span>
      </li>
      <li class:done={stage() > 1} class:on={stage() === 1}>
        <i class="mark">{stage() > 1 ? '✓' : '2'}</i>
        <span class="label">
          <strong>{t('wizard.authorisation')}</strong>
          <small class="faint">{stage() > 1 ? t('wizard.readAccess') : t('zabbix.apiToken')}</small>
        </span>
      </li>
      <li class:done={stage() > 2} class:on={stage() === 2}>
        <i class="mark">{stage() > 2 ? '✓' : '3'}</i>
        <span class="label">
          <strong>{t('wizard.preview')}</strong>
          <small class="faint">{t('wizard.explicitImport')}</small>
        </span>
      </li>
    </ol>

    {#if imported}
      <div class="modal-body">
        <div class="banner ok">
          <i class="dot ok"></i>
          <div>
            <strong>
              {plural('wizard.linkedTargets', imported.targets.length)}
            </strong>
            <p class="muted">
              {t('zabbix.tokenSealed')}
            </p>
          </div>
        </div>

        <div class="figures tally">
          <div class="fig">
            <b>{imported.targets.filter((target) => target.disposition === 'created').length}</b>
            <span>{t('wizard.created')}</span>
          </div>
          <div class="fig">
            <b>{imported.targets.filter((target) => target.disposition === 'reused').length}</b>
            <span>{t('wizard.reused')}</span>
          </div>
          <div class="fig">
            <b>{imported.targets.filter((target) => target.disposition === 'already_imported').length}</b>
            <span>{t('wizard.alreadyLinked')}</span>
          </div>
        </div>
      </div>
      <footer>
        <button class="btn primary" type="button" onclick={onclose}>Revenir aux Cibles</button>
      </footer>
    {:else if preview}
      <div class="modal-body">
        <div class="checks">
          <div class="check">
            <span class="faint">Version du serveur</span>
            <strong>Zabbix {preview.version}</strong>
            <small class={preview.compatibility === 'supported' ? 'ok' : 'warn'}>{preview.compatibility_label}</small>
          </div>
          <div class="check">
            <span class="faint">Transport</span>
            <strong>{preview.encrypted_transport ? t('mattermost.tlsVerified') : t('wizard.plainHttp')}</strong>
            <small class={preview.encrypted_transport ? 'ok' : 'warn'}>
              {preview.encrypted_transport ? t('wizard.certificateValid') : t('wizard.trustedNetworkOnly')}
            </small>
          </div>
          <div class="check">
            <span class="faint">{t('wizard.scope')}</span>
            <strong>{plural('zabbix.hostsVisible', preview.hosts.length)}</strong>
            <small class="faint">{t('wizard.readOnlyPreview')}</small>
          </div>
          <button class="btn sm" type="button" onclick={resetPreview}>{t('wizard.changeAccess')}</button>
        </div>

        <ReconciliationSummary counts={reconciliation} />

        <div class="listbar">
          <div class="field search">
            <label class="sr-only" for="host-filter">{t('zabbix.filterHosts')}</label>
            <input id="host-filter" bind:value={query} placeholder="Filtrer par nom ou adresse" />
          </div>
          <button class="btn sm" type="button" onclick={toggleAllVisible} disabled={visibleImportableHosts().length === 0}>
            {visibleImportableHosts().length === 0
              ? t('wizard.nothingToSelect')
              : visibleImportableHosts().every((host) => selected.includes(host.external_id))
                ? 'Tout retirer'
                : t('wizard.selectAll')}
          </button>
          <span class="faint num">
            {selected.length} / {importableHosts().length} importables ·
            {t('wizard.validUntil', { time: clock(preview.expires_at) })}
          </span>
        </div>

        <ul class="rack">
          {#each filteredHosts() as host (host.external_id)}
            {@const connection = primaryInterface(host)}
            {@const locked = Boolean(host.already_imported_to)}
            <li class:picked={selected.includes(host.external_id)} class:locked>
              <label class="rack-select">
                <input type="checkbox" checked={selected.includes(host.external_id)}
                  onchange={() => toggleHost(host.external_id)} disabled={locked} />
                <span class="rack-name">
                  <strong>{host.name}</strong>
                  <small class="faint mono">{connection?.address || t('zabbix.noInterface')}</small>
                </span>
              </label>
              <div class="rack-decision">
                {#if locked}
                  <span class="pill">{t('wizard.alreadyBound')}</span>
                  <small class="faint">{host.already_imported_to?.name}</small>
                {:else}
                  <TargetDecision
                    name={host.name}
                    value={targetAssignments[host.external_id] ?? ''}
                    candidates={host.candidate_targets}
                    availableTargets={preview.available_targets}
                    disabled={!selected.includes(host.external_id)}
                    onselect={(targetID) => assignTarget(host.external_id, targetID)}
                  />
                {/if}
              </div>
            </li>
          {:else}
            <li class="none faint">{t('zabbix.noHostMatches')}</li>
          {/each}
        </ul>

        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>

      <footer>
        <span class="faint note">{t('wizard.matchesNote')}</span>
        <button class="btn primary" type="button" onclick={importHosts} disabled={busy || selected.length === 0 || reconciliation.review > 0}>
          {busy
            ? t('wizard.importing')
            : selected.length === 0
              ? t('zabbix.noHostChosen')
              : reconciliation.review > 0
                ? plural('wizard.confirmChoices', reconciliation.review)
                : plural('zabbix.importHosts', selected.length)}
        </button>
      </footer>
    {:else}
      <form onsubmit={inspect}>
        <div class="modal-body">
          <section>
            <h3>{t('wizard.connection')}</h3>
            <p class="faint lead">{t('zabbix.addressHint')}</p>
            <div class="grid">
              <div class="field">
                <label for="connector-name">{t('wizard.nameInCairnOps')}</label>
                <input id="connector-name" bind:value={name} required maxlength="160" placeholder="Zabbix production" />
              </div>
              <div class="field">
                <label for="zabbix-address">{t('zabbix.frontendAddress')}</label>
                <input id="zabbix-address" bind:this={addressInput} bind:value={address} required
                  inputmode="url" placeholder="https://zabbix.example.net" />
              </div>
            </div>
          </section>

          <section>
            <h3>{t('wizard.authorisation')}</h3>
            <p class="faint lead">{t('zabbix.tokenHint')}</p>
            <div class="field">
              <label for="zabbix-token">{t('zabbix.apiToken')}</label>
              <input id="zabbix-token" type="password" bind:value={apiToken} required maxlength="4096"
                autocomplete="off" spellcheck="false" placeholder="••••••••••••••••••••••••" />
              <small>{t('zabbix.tokenSmall')}</small>
            </div>
          </section>

          <section class="last">
            <h3>{t('wizard.checksTitle')}</h3>
            <ol class="steps">
              <li><strong>{t('wizard.tlsIdentity')}</strong><small class="faint">{t('wizard.tlsIdentityNote')}</small></li>
              <li><strong>{t('wizard.apiCompatibility')}</strong><small class="faint">{t('wizard.apiCompatibilityNote')}</small></li>
              <li><strong>{t('wizard.tokenScope')}</strong><small class="faint">{t('wizard.tokenScopeNote')}</small></li>
              <li><strong>{t('wizard.exactDuplicates')}</strong><small class="faint">{t('wizard.exactDuplicatesNote')}</small></li>
            </ol>
          </section>

          {#if error}<p class="error" role="alert">{error}</p>{/if}
        </div>

        <footer>
          <span class="faint note">{t('zabbix.noChangeNote')}</span>
          <button class="btn" type="button" onclick={onclose} disabled={busy}>{t('common.cancel')}</button>
          <button class="btn primary" type="submit" disabled={busy}>
            {busy ? t('gate.verifying') : t('wizard.verifyAndPreview')}
          </button>
        </footer>
      </form>
    {/if}
  </div>
</div>

<style>
  .modal {
    max-width: 46rem;
  }

  /* Le fil des étapes : trois jalons répartis à parts égales sur la largeur,
     reliés par une trace pointillée qui se colore dès qu'une étape est
     franchie. Aligné à gauche, ce bandeau laissait un vide qui ne disait rien ;
     réparti, il montre le chemin et où l'on se trouve dessus. */
  .stepper {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    margin: 0;
    padding: var(--s5) var(--s5) var(--s4);
    border-bottom: 1px solid var(--line);
    background: var(--bg);
    list-style: none;
  }

  .stepper li {
    position: relative;
    display: grid;
    justify-items: center;
    gap: 0.4375rem;
    padding: 0 var(--s3);
    text-align: center;
    min-width: 0;
  }

  /* La trace part du bord du marqueur et s'arrête avant le suivant, à
     l'aplomb de leur centre commun. */
  .stepper li:not(:last-child)::after {
    content: '';
    position: absolute;
    top: 0.75rem;
    left: calc(50% + 1.25rem);
    right: calc(-50% + 1.25rem);
    border-top: 2px dotted var(--line-strong);
  }

  .stepper li.done::after {
    border-top-color: var(--ok);
  }

  .mark {
    position: relative;
    z-index: 1;
    width: 1.5rem;
    height: 1.5rem;
    flex: none;
    display: grid;
    place-items: center;
    border: 1px solid var(--line-strong);
    border-radius: 50%;
    background: var(--surface);
    color: var(--faint);
    font-family: var(--font-num);
    font-size: 0.6875rem;
    font-style: normal;
    transition: border-color var(--d2) var(--ease), color var(--d2) var(--ease),
      background var(--d2) var(--ease);
  }

  .stepper li.on .mark {
    border-color: var(--accent);
    color: var(--accent);
    box-shadow: 0 0 0 0.1875rem color-mix(in srgb, var(--accent) 14%, transparent);
  }

  .stepper li.done .mark {
    border-color: var(--ok);
    background: var(--ok-bg);
    color: var(--ok);
  }

  .stepper strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--muted);
  }

  .stepper li.on strong,
  .stepper li.done strong {
    color: var(--ink);
  }

  .stepper small {
    display: block;
    font-size: 0.6875rem;
  }

  /* Sous 34 rem, les trois jalons se serrent : le sous-titre s'efface, le
     titre et le marqueur suffisent à situer l'étape. */
  @media (max-width: 34rem) {
    .stepper {
      padding: var(--s4) var(--s4);
    }

    .stepper li {
      padding: 0 var(--s2);
    }

    .stepper small {
      display: none;
    }
  }

  section {
    padding-bottom: var(--s5);
    margin-bottom: var(--s5);
    border-bottom: 1px solid var(--line);
  }

  section.last {
    padding-bottom: 0;
    margin-bottom: 0;
    border-bottom: 0;
  }

  h3 {
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .lead {
    margin: 0.25rem 0 var(--s4);
    font-size: 0.75rem;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
    gap: var(--s4);
  }

  .grid .field {
    margin: 0;
  }

  .checks {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s5);
    padding: var(--s4);
    margin-bottom: var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
  }

  .check span {
    display: block;
    font-size: 0.6875rem;
  }

  .check strong {
    display: block;
    margin-top: 2px;
    font-size: 0.75rem;
    font-weight: 600;
  }

  .check small {
    display: block;
    font-size: 0.6875rem;
  }

  .checks > .btn {
    margin-left: auto;
  }

  .listbar {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin-bottom: var(--s3);
    flex-wrap: wrap;
  }

  .search {
    flex: 1;
    min-width: 10rem;
    margin: 0;
  }

  .listbar .num {
    font-size: 0.6875rem;
  }

  /* Étape d'aperçu : le bandeau de vérification et la barre de filtre restent
     en place, seule la liste défile. Les actions demeurent ainsi visibles dès
     l'ouverture, quel que soit le nombre de moniteurs découverts. */
  .modal-body {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .checks,
  .listbar {
    flex: none;
  }

  .rack {
    flex: 1 1 auto;
    min-height: 8rem;
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    /* `hidden` en abscisse conserve la découpe des coins arrondis, `auto` en
       ordonnée donne son ascenseur à la liste seule. */
    overflow-x: hidden;
    overflow-y: auto;
    overscroll-behavior: contain;
  }

  .rack li {
    display: grid;
    grid-template-columns: minmax(13rem, 17rem) minmax(0, 1fr);
    align-items: center;
    gap: var(--s3);
    padding-right: var(--s4);
    border-bottom: 1px solid var(--line-row);
  }

  .rack li:last-child {
    border-bottom: 0;
  }

  .rack label {
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: var(--s3) var(--s4);
    cursor: pointer;
    flex: 1;
    min-width: 0;
  }

  .rack li.picked {
    background: var(--surface-2);
  }

  .rack li.locked label {
    cursor: default;
    opacity: 0.55;
  }

  .rack-name {
    flex: 1;
    min-width: 0;
  }

  .rack-name strong {
    display: block;
    overflow: hidden;
    font-size: 0.75rem;
    font-weight: 500;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .rack-name small {
    display: block;
    overflow: hidden;
    font-size: 0.6875rem;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .rack-select {
    min-width: 0;
  }

  .rack-decision {
    flex: 1 1 auto;
    min-width: 0;
  }

  @media (max-width: 40rem) {
    .rack li {
      grid-template-columns: minmax(0, 1fr);
      gap: 0;
      padding-right: 0;
    }

    .rack-decision {
      min-width: 0;
      padding: 0 var(--s4) var(--s3) calc(var(--s4) + 2rem);
      text-align: left;
    }
  }

  .none {
    padding: var(--s5);
    text-align: center;
    font-size: 0.75rem;
  }

  .tally {
    margin-top: var(--s5);
    padding-top: var(--s4);
    border-top: 1px solid var(--line);
  }

  .banner strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .banner p {
    margin-top: 0.25rem;
    font-size: 0.75rem;
  }

  .steps {
    margin: var(--s4) 0 0;
    padding: 0;
    list-style: none;
    display: grid;
    /* Quatre points : deux rangées de deux, ou une seule de quatre.
       auto-fit tombait sur trois colonnes et laissait le dernier seul. */
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--s5) var(--s4);
    counter-reset: step;
  }

  .steps li {
    counter-increment: step;
  }

  .steps li::before {
    content: counter(step, decimal-leading-zero);
    display: block;
    margin-bottom: 0.1875rem;
    color: var(--accent);
    font-family: var(--font-num);
    font-size: 0.625rem;
  }

  .steps strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
  }

  .steps small {
    display: block;
    font-size: 0.6875rem;
  }

  @media (max-width: 44rem) {
    .steps {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }

</style>
