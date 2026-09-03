<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type PatchMonHostPreview, type PatchMonImportResult, type PatchMonPreview } from '$lib/api';
  import { clock } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';
  import {
    prepareTargetAssignments,
    reconciliationCounts,
    resolvedTargetAssignments
  } from '$lib/reconciliation';
  import Icon from './Icon.svelte';
  import ConnectorAccessPlan from './ConnectorAccessPlan.svelte';
  import ReconciliationSummary from './ReconciliationSummary.svelte';
  import TargetDecision from './TargetDecision.svelte';
  import Checkbox from './ui/Checkbox.svelte';

  let {
    onclose,
    onsuccess,
    connectorId = '',
    initialName = '',
    initialAddress = ''
  }: {
    onclose: () => void;
    onsuccess: (result: PatchMonImportResult) => Promise<void> | void;
    connectorId?: string;
    initialName?: string;
    initialAddress?: string;
  } = $props();

  let addressInput = $state<HTMLInputElement>();
  let name = $state('PatchMon');
  let address = $state('');
  let tokenKey = $state('');
  let tokenSecret = $state('');
  let username = $state('');
  let password = $state('');
  let secondFactor = $state('');
  let manualAccess = $state(false);
  let preview = $state<PatchMonPreview | null>(null);
  let imported = $state<PatchMonImportResult | null>(null);
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

  function adoptPreview(value: PatchMonPreview) {
    preview = value;
    selected = value.hosts.filter((host) => !host.already_imported_to).map((host) => host.external_id);
    targetAssignments = prepareTargetAssignments(value.hosts);
  }

  async function inspectExisting() {
    busy = true; error = '';
    try {
      adoptPreview(await api<PatchMonPreview>(`/api/v1/connectors/${connectorId}/preview`, { method: 'POST' }));
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('patchmon.verifyFailed');
    } finally { busy = false; }
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  function filteredHosts() {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle || !preview) return preview?.hosts ?? [];
    return preview.hosts.filter((host) =>
      [host.name, host.hostname, host.ip ?? '', host.os_type ?? '', host.os_version ?? ''].some((value) =>
        value.toLocaleLowerCase().includes(needle)
      )
    );
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
    selected = allSelected
      ? selected.filter((id) => !visible.includes(id))
      : [...new Set([...selected, ...visible])];
  }

  function assignTarget(externalID: string, targetID: string) {
    targetAssignments = { ...targetAssignments, [externalID]: targetID };
  }

  function posture(host: PatchMonHostPreview) {
    if (host.security_updates_count > 0) return t('patchmon.securityRequired', { count: host.security_updates_count });
    if (host.needs_reboot) return t('patchmon.rebootRequired');
    if (host.updates_count > 0) return t('patchmon.updatesRequired', { count: host.updates_count });
    return t('patchmon.upToDate');
  }

  async function inspect(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      adoptPreview(await api<PatchMonPreview>('/api/v1/connectors/patchmon/preview', {
        method: 'POST',
        body: JSON.stringify(manualAccess
          ? { name, address, token_key: tokenKey, token_secret: tokenSecret }
          : { name, address, username, password, second_factor: secondFactor })
      }));
      tokenKey = '';
      tokenSecret = '';
      password = '';
      secondFactor = '';
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('patchmon.verifyFailed');
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
      imported = await api<PatchMonImportResult>('/api/v1/connectors/patchmon/import', {
        method: 'POST',
        body: JSON.stringify({
          receipt: preview.receipt,
          host_ids: selected,
          target_assignments: resolvedTargetAssignments(selected, targetAssignments)
        })
      });
      await onsuccess(imported);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('patchmon.importFailed');
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
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="patchmon-title">
    <header>
      <div>
        <h2 id="patchmon-title">{imported ? t('wizard.linked') : preview ? t('wizard.chooseWhatEnters') : t('patchmon.connect')}</h2>
        <p>{t('patchmon.lead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    {#if imported}
      <div class="modal-body">
        <div class="banner ok">
          <i class="dot ok"></i>
          <div>
            <strong>{plural('wizard.linkedTargets', imported.targets.length)}</strong>
            <p class="muted">{t('patchmon.credentialsSealed')}</p>
          </div>
        </div>
        <div class="figures tally">
          <div class="fig"><b>{imported.targets.filter((target) => target.disposition === 'created').length}</b><span>{t('wizard.created')}</span></div>
          <div class="fig"><b>{imported.targets.filter((target) => target.disposition === 'reused').length}</b><span>{t('wizard.reused')}</span></div>
          <div class="fig"><b>{imported.targets.filter((target) => target.disposition === 'already_imported').length}</b><span>{t('wizard.alreadyLinked')}</span></div>
        </div>
      </div>
      <footer><button class="btn primary" type="button" onclick={onclose}>{t('patchmon.backToTargets')}</button></footer>
    {:else if preview}
      <div class="modal-body">
        <div class="checks">
          <div><span class="faint">API</span><strong>PatchMon</strong><small class="ok">{preview.compatibility_label}</small></div>
          <div><span class="faint">Transport</span><strong>{preview.encrypted_transport ? 'TLS' : 'HTTP'}</strong><small class={preview.encrypted_transport ? 'ok' : 'warn'}>{preview.encrypted_transport ? t('wizard.certificateValid') : t('wizard.trustedNetworkOnly')}</small></div>
          <div><span class="faint">{t('wizard.scope')}</span><strong>{plural('patchmon.hostsVisible', preview.hosts.length)}</strong><small class="faint">{t('patchmon.postureOnly')}</small></div>
          <button class="btn sm" type="button" onclick={resetPreview}>{t('wizard.changeAccess')}</button>
        </div>

        <ConnectorAccessPlan access={preview.access} product="patchmon" />
        <ReconciliationSummary counts={reconciliation} />

        <div class="listbar">
          <div class="field search"><label class="sr-only" for="patchmon-filter">{t('patchmon.filter')}</label><input id="patchmon-filter" bind:value={query} placeholder={t('patchmon.filter')} /></div>
          <button class="btn sm" type="button" onclick={toggleAllVisible} disabled={visibleImportableHosts().length === 0}>
            {visibleImportableHosts().length > 0 && visibleImportableHosts().every((host) => selected.includes(host.external_id)) ? t('patchmon.removeAll') : t('wizard.selectAll')}
          </button>
          <span class="faint num">{selected.length} / {importableHosts().length} · {t('wizard.validUntil', { time: clock(preview.expires_at) })}</span>
        </div>

        <ul class="rack">
          {#each filteredHosts() as host (host.external_id)}
            {@const locked = Boolean(host.already_imported_to)}
            <li class:picked={selected.includes(host.external_id)} class:locked>
              <Checkbox class="patchmon-host-choice" variant="row" checked={selected.includes(host.external_id)} onCheckedChange={() => toggleHost(host.external_id)} disabled={locked}>
                <span class="host">
                  <strong>{host.name}</strong>
                  <small class="faint mono">{host.hostname}{host.ip ? ` · ${host.ip}` : ''}</small>
                  <small class:warn={host.security_updates_count > 0 || host.needs_reboot} class="posture">{posture(host)}</small>
                </span>
              </Checkbox>
              <div>
                {#if locked}
                  <span class="pill">{t('wizard.alreadyBound')}</span> <small class="faint">{host.already_imported_to?.name}</small>
                {:else}
                  <TargetDecision name={host.name} value={targetAssignments[host.external_id] ?? ''} candidates={host.candidate_targets} availableTargets={preview.available_targets} disabled={!selected.includes(host.external_id)} onselect={(targetID) => assignTarget(host.external_id, targetID)} />
                {/if}
              </div>
            </li>
          {:else}
            <li class="none faint">{t('patchmon.noHostMatches')}</li>
          {/each}
        </ul>
        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>
      <footer>
        <span class="faint note">{t('patchmon.noActionNote')}</span>
        <button class="btn primary" type="button" onclick={importHosts} disabled={busy || selected.length === 0 || reconciliation.review > 0}>
          {busy ? t('wizard.importing') : reconciliation.review > 0 ? plural('wizard.confirmChoices', reconciliation.review) : plural('patchmon.importHosts', selected.length)}
        </button>
      </footer>
    {:else}
      <form onsubmit={inspect}>
        <div class="modal-body">
          <section>
            <h3>{t('wizard.connection')}</h3>
            <p class="faint lead">{t('patchmon.addressHint')}</p>
            <div class="grid">
              <div class="field"><label for="patchmon-name">{t('wizard.nameInCairnOps')}</label><input id="patchmon-name" bind:value={name} required maxlength="160" /></div>
              <div class="field"><label for="patchmon-address">{t('patchmon.instanceAddress')}</label><input id="patchmon-address" bind:this={addressInput} bind:value={address} required inputmode="url" placeholder="https://patchmon.example.net" /></div>
            </div>
          </section>
          <section class="last">
            <h3>{t('wizard.authorisation')}</h3>
            <p class="faint lead">{t('patchmon.setupHint')}</p>
            {#if manualAccess}
              <div class="grid">
                <div class="field"><label for="patchmon-key">{t('patchmon.tokenKey')}</label><input id="patchmon-key" type="password" bind:value={tokenKey} required autocomplete="off" spellcheck="false" /></div>
                <div class="field"><label for="patchmon-secret">{t('patchmon.tokenSecret')}</label><input id="patchmon-secret" type="password" bind:value={tokenSecret} required autocomplete="off" spellcheck="false" /></div>
              </div>
            {:else}
              <div class="grid">
                <div class="field"><label for="patchmon-user">{t('wizard.installerAccount')}</label><input id="patchmon-user" bind:value={username} required maxlength="4096" autocomplete="username" /></div>
                <div class="field"><label for="patchmon-password">{t('wizard.temporaryPassword')}</label><input id="patchmon-password" type="password" bind:value={password} required maxlength="4096" autocomplete="current-password" /></div>
                <div class="field"><label for="patchmon-2fa">{t('wizard.secondFactor')}</label><input id="patchmon-2fa" bind:value={secondFactor} maxlength="32" inputmode="numeric" autocomplete="one-time-code" /></div>
              </div>
            {/if}
            <button class="mode" type="button" onclick={() => manualAccess = !manualAccess}>
              {t(manualAccess ? 'patchmon.useManagedAccess' : 'patchmon.useExistingToken')}
            </button>
          </section>
          {#if error}<p class="error" role="alert">{error}</p>{/if}
        </div>
        <footer>
          <span class="faint note">{t('patchmon.noActionNote')}</span>
          <button class="btn" type="button" onclick={onclose} disabled={busy}>{t('common.cancel')}</button>
          <button class="btn primary" type="submit" disabled={busy}>{busy ? t('gate.verifying') : t('wizard.verifyAndPreview')}</button>
        </footer>
      </form>
    {/if}
  </div>
</div>

<style>
  .modal { max-width: 48rem; }
  section { padding-bottom: var(--s5); margin-bottom: var(--s5); border-bottom: 1px solid var(--line); }
  section.last { padding-bottom: 0; margin-bottom: 0; border-bottom: 0; }
  h3 { font-size: .8125rem; font-weight: 600; }
  .lead { margin: .25rem 0 var(--s4); font-size: .75rem; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr)); gap: var(--s4); }
  .grid .field { margin: 0; }
  .mode { margin-top: var(--s3); padding: 0; border: 0; color: var(--accent); background: none; font: inherit; font-size: .75rem; cursor: pointer; }
  .checks { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s5); padding: var(--s4); margin-bottom: var(--s4); border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--bg); }
  .checks span, .checks small { display: block; font-size: .6875rem; }
  .checks strong { display: block; margin-top: 2px; font-size: .75rem; }
  .checks > .btn { margin-left: auto; }
  .listbar { display: flex; align-items: center; gap: var(--s3); margin-bottom: var(--s3); flex-wrap: wrap; }
  .search { flex: 1; min-width: 10rem; margin: 0; }
  .rack { margin: 0; padding: 0; max-height: 25rem; overflow: auto; list-style: none; border: 1px solid var(--line-strong); border-radius: var(--r-m); }
  .rack li { display: grid; grid-template-columns: minmax(15rem, 20rem) minmax(0, 1fr); align-items: center; gap: var(--s3); padding-right: var(--s4); border-bottom: 1px solid var(--line-row); }
  .rack li:last-child { border-bottom: 0; }
  .rack li.picked { background: var(--surface-2); }
  .host { min-width: 0; }
  .host strong, .host small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .host strong { font-size: .75rem; }
  .host small { font-size: .6875rem; }
  .posture { color: var(--ok); }
  .posture.warn { color: var(--warn); }
  .none { display: block !important; padding: var(--s5) !important; text-align: center; }
  @media (max-width: 40rem) { .rack li { grid-template-columns: 1fr; padding: 0 var(--s3) var(--s3); } :global(.patchmon-host-choice) { padding-left: 0; padding-right: 0; } }
</style>
