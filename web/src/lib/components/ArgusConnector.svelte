<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type ArgusImportResult, type ArgusPreview, type ArgusServicePreview } from '$lib/api';
  import { clock } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';
  import { prepareTargetAssignments, reconciliationCounts, resolvedTargetAssignments } from '$lib/reconciliation';
  import Icon from './Icon.svelte';
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
    onsuccess: (result: ArgusImportResult) => Promise<void> | void;
    connectorId?: string;
    initialName?: string;
    initialAddress?: string;
  } = $props();

  let addressInput = $state<HTMLInputElement>();
  let name = $state('Argus');
  let address = $state('');
  let username = $state('');
  let password = $state('');
  let preview = $state<ArgusPreview | null>(null);
  let imported = $state<ArgusImportResult | null>(null);
  let selected = $state<string[]>([]);
  let targetAssignments = $state<Record<string, string>>({});
  let query = $state('');
  let busy = $state(false);
  let error = $state('');
  let initialized = $state(false);
  let reconciliation = $derived(reconciliationCounts(selected, targetAssignments));
  let selectedPendingCount = $derived(
    preview?.services.filter((service) =>
      selected.includes(service.external_id) && !service.unknown && !service.skipped && service.deployed_version !== service.latest_version
    ).length ?? 0
  );
  let authIncomplete = $derived(Boolean(username.trim()) !== Boolean(password));

  $effect(() => {
    if (initialized) return;
    initialized = true;
    if (initialName) name = initialName;
    if (initialAddress) address = initialAddress;
  });

  onMount(() => connectorId ? void inspectExisting() : addressInput?.focus());

  function adoptPreview(value: ArgusPreview) {
    preview = value;
    selected = value.services
      .filter((service) => service.importable && !service.already_imported_to)
      .map((service) => service.external_id);
    targetAssignments = prepareTargetAssignments(value.services);
  }

  async function inspectExisting() {
    busy = true; error = '';
    try {
      adoptPreview(await api<ArgusPreview>(`/api/v1/connectors/${connectorId}/preview`, { method: 'POST' }));
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('argus.verifyFailed');
    } finally { busy = false; }
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  function filteredServices() {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle || !preview) return preview?.services ?? [];
    return preview.services.filter((service) =>
      [service.name, service.external_id, service.deployed_version ?? '', service.latest_version ?? '']
        .some((value) => value.toLocaleLowerCase().includes(needle))
    );
  }

  function selectableServices() {
    return preview?.services.filter((service) => service.importable && !service.already_imported_to) ?? [];
  }

  function visibleSelectableServices() {
    return filteredServices().filter((service) => service.importable && !service.already_imported_to);
  }

  function toggleService(id: string) {
    selected = selected.includes(id) ? selected.filter((item) => item !== id) : [...selected, id];
  }

  function toggleAllVisible() {
    const visible = visibleSelectableServices().map((service) => service.external_id);
    const allSelected = visible.length > 0 && visible.every((id) => selected.includes(id));
    selected = allSelected
      ? selected.filter((id) => !visible.includes(id))
      : [...new Set([...selected, ...visible])];
  }

  function assignTarget(externalID: string, targetID: string) {
    targetAssignments = { ...targetAssignments, [externalID]: targetID };
  }

  function stateLabel(service: ArgusServicePreview) {
    if (!service.importable) {
      return service.ineligibility === 'inactive' ? t('argus.inactive') : t('argus.noDeployedVersion');
    }
    if (service.unknown) return t('argus.unknown');
    if (service.skipped) return t('argus.skipped');
    if (service.deployed_version === service.latest_version) return t('argus.deployed');
    if (service.approved) return t('argus.approved');
    return t('argus.available');
  }

  async function inspect(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      adoptPreview(await api<ArgusPreview>('/api/v1/connectors/argus/preview', {
        method: 'POST',
        body: JSON.stringify({ name, address, username, password })
      }));
      username = '';
      password = '';
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('argus.verifyFailed');
    } finally { busy = false; }
  }

  async function importServices() {
    if (!preview || selected.length === 0) return;
    if (reconciliation.review > 0) {
      error = plural('wizard.resolveBeforeImport', reconciliation.review);
      return;
    }
    busy = true;
    error = '';
    try {
      imported = await api<ArgusImportResult>('/api/v1/connectors/argus/import', {
        method: 'POST',
        body: JSON.stringify({
          receipt: preview.receipt,
          service_ids: selected,
          target_assignments: resolvedTargetAssignments(selected, targetAssignments)
        })
      });
      await onsuccess(imported);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('argus.importFailed');
    } finally { busy = false; }
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
  <div class="modal argus-modal" role="dialog" aria-modal="true" aria-labelledby="argus-title">
    <header>
      <div>
        <h2 id="argus-title">{imported ? t('wizard.linked') : preview ? t('wizard.chooseWhatEnters') : t('argus.connect')}</h2>
        <p>{t('argus.lead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label={t('common.close')}><Icon name="close" size={14} /></button>
    </header>

    {#if imported}
      <div class="modal-body">
        <div class="banner ok"><i class="dot ok"></i><div><strong>{plural('wizard.linkedTargets', imported.targets.length)}</strong><p class="muted">{t('argus.credentialsSealed')}</p></div></div>
        <div class="figures tally">
          <div class="fig"><b>{imported.targets.filter((target) => target.disposition === 'created').length}</b><span>{t('wizard.created')}</span></div>
          <div class="fig"><b>{imported.targets.filter((target) => target.disposition === 'reused').length}</b><span>{t('wizard.reused')}</span></div>
          <div class="fig"><b>{imported.targets.filter((target) => target.disposition === 'already_imported').length}</b><span>{t('wizard.alreadyLinked')}</span></div>
        </div>
      </div>
      <footer><button class="btn primary" type="button" onclick={onclose}>{t('argus.backToTargets')}</button></footer>
    {:else if preview}
      <div class="modal-body">
        <div class="checks">
          <div><span class="faint">API</span><strong>Argus {preview.version}</strong><small class="ok">{preview.compatibility_label}</small></div>
          <div><span class="faint">Transport</span><strong>{preview.encrypted_transport ? 'TLS' : 'HTTP'}</strong><small class={preview.encrypted_transport ? 'ok' : 'warn'}>{preview.encrypted_transport ? t('wizard.certificateValid') : t('wizard.trustedNetworkOnly')}</small></div>
          <div><span class="faint">{t('wizard.scope')}</span><strong>{plural('argus.servicesImportable', preview.importable_count)}</strong><small class="faint">{t('argus.postureOnly')}</small></div>
          <button class="btn sm" type="button" onclick={resetPreview}>{t('wizard.changeAccess')}</button>
        </div>

        <ReconciliationSummary counts={reconciliation} />

        <div class="listbar">
          <div class="field search"><label class="sr-only" for="argus-filter">{t('argus.filter')}</label><input id="argus-filter" bind:value={query} placeholder={t('argus.filter')} /></div>
          <button class="btn sm" type="button" onclick={toggleAllVisible} disabled={visibleSelectableServices().length === 0}>
            {visibleSelectableServices().length > 0 && visibleSelectableServices().every((service) => selected.includes(service.external_id)) ? t('argus.removeAll') : t('wizard.selectAll')}
          </button>
          <span class="faint num">{selected.length} / {selectableServices().length} · {t('wizard.validUntil', { time: clock(preview.expires_at) })}</span>
        </div>

        <ul class="rack argus-rack">
          {#each filteredServices() as service (service.external_id)}
            {@const locked = Boolean(service.already_imported_to) || !service.importable}
            <li class:picked={selected.includes(service.external_id)} class:locked>
              <Checkbox class="argus-service-choice" variant="row" checked={selected.includes(service.external_id)} onCheckedChange={() => toggleService(service.external_id)} disabled={locked}>
                <span class="service argus-service-summary">
                  <span class="service-title"><strong>{service.name}</strong><small class="faint mono">{service.external_id}</small></span>
                  <span class="versions argus-versions mono"><span>{service.deployed_version || '—'}</span><span aria-hidden="true">→</span><strong>{service.latest_version || '—'}</strong></span>
                  <small class:warn={service.importable && service.deployed_version !== service.latest_version && !service.skipped} class:unknown={service.unknown} class="state">{stateLabel(service)}</small>
                </span>
              </Checkbox>
              <div class="decision">
                {#if service.version_url}<a class="version-link" href={service.version_url} target="_blank" rel="noreferrer">{t('argus.viewVersion')} <span aria-hidden="true">↗</span></a>{/if}
                {#if service.already_imported_to}
                  <span class="pill">{t('wizard.alreadyBound')}</span> <small class="faint">{service.already_imported_to.name}</small>
                {:else if !service.importable}
                  <span class="pill idle">{stateLabel(service)}</span>
                {:else}
                  <TargetDecision name={service.name} value={targetAssignments[service.external_id] ?? ''} candidates={service.candidate_targets} availableTargets={preview.available_targets} disabled={!selected.includes(service.external_id)} onselect={(targetID) => assignTarget(service.external_id, targetID)} />
                {/if}
              </div>
            </li>
          {:else}
            <li class="none faint">{t('argus.noServiceMatches')}</li>
          {/each}
        </ul>
        {#if selectedPendingCount > 0}
          <div class="banner warn impact" role="status"><i class="dot warn"></i><div><strong>{plural('argus.incidentsMayOpen', selectedPendingCount)}</strong><p class="muted">{t('argus.notificationsApply')}</p></div></div>
        {/if}
        {#if error}<p class="error" role="alert" aria-live="assertive">{error}</p>{/if}
      </div>
      <footer>
        <span class="faint note">{t('argus.noActionNote')}</span>
        <button class="btn primary" type="button" onclick={importServices} disabled={busy || selected.length === 0 || reconciliation.review > 0}>
          {busy ? t('wizard.importing') : reconciliation.review > 0 ? plural('wizard.confirmChoices', reconciliation.review) : plural('argus.importServices', selected.length)}
        </button>
      </footer>
    {:else}
      <form onsubmit={inspect}>
        <div class="modal-body">
          <section>
            <h3>{t('wizard.connection')}</h3>
            <p class="faint lead">{t('argus.addressHint')}</p>
            <div class="grid">
              <div class="field"><label for="argus-name">{t('wizard.nameInCairnOps')}</label><input id="argus-name" bind:value={name} required maxlength="160" /></div>
              <div class="field"><label for="argus-address">{t('argus.instanceAddress')}</label><input id="argus-address" bind:this={addressInput} bind:value={address} required maxlength="2048" inputmode="url" placeholder="https://argus.example.net" /></div>
            </div>
          </section>
          <section class="last">
            <h3>{t('wizard.authorisation')}</h3>
            <p class="faint lead">{t('argus.authHint')}</p>
            <div class="grid">
              <div class="field"><label for="argus-username">{t('argus.username')}</label><input id="argus-username" bind:value={username} maxlength="4096" autocomplete="username" spellcheck="false" aria-invalid={authIncomplete} aria-describedby={authIncomplete ? 'argus-auth-error' : undefined} /></div>
              <div class="field"><label for="argus-password">{t('argus.password')}</label><input id="argus-password" type="password" bind:value={password} maxlength="4096" autocomplete="current-password" spellcheck="false" aria-invalid={authIncomplete} aria-describedby={authIncomplete ? 'argus-auth-error' : undefined} /></div>
            </div>
            {#if authIncomplete}<p id="argus-auth-error" class="error auth-error" role="status">{t('argus.authPairRequired')}</p>{/if}
            <p class="security-note"><strong>{t('argus.basicAuthWarning')}</strong> {t('argus.basicAuthWarningDetail')}</p>
          </section>
          {#if error}<p class="error" role="alert" aria-live="assertive">{error}</p>{/if}
        </div>
        <footer>
          <span class="faint note">{t('argus.noActionNote')}</span>
          <button class="btn" type="button" onclick={onclose} disabled={busy}>{t('common.cancel')}</button>
          <button class="btn primary" type="submit" disabled={busy || authIncomplete}>{busy ? t('gate.verifying') : t('wizard.verifyAndPreview')}</button>
        </footer>
      </form>
    {/if}
  </div>
</div>

<style>
  .modal { max-width: var(--connector-modal-max); }
  section { padding-bottom: var(--s5); margin-bottom: var(--s5); border-bottom: var(--line-width) solid var(--line); }
  section.last { padding-bottom: 0; margin-bottom: 0; border-bottom: 0; }
  h3 { font-size: var(--text-md); font-weight: var(--weight-semibold); }
  .lead { margin: var(--s2) 0 var(--s4); font-size: var(--text-sm); }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(var(--connector-field-min), 1fr)); gap: var(--s4); }
  .grid .field { margin: 0; }
  .security-note { margin: var(--s3) 0 0; padding: var(--s3); border-left: var(--s1) solid var(--warn); background: var(--surface-2); color: var(--muted); font-size: var(--text-xs); }
  .security-note strong { color: var(--warn); }
  .auth-error { margin: var(--s2) 0 0; }
  .checks { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s5); padding: var(--s4); margin-bottom: var(--s4); border: var(--line-width) solid var(--line-strong); border-radius: var(--r-m); background: var(--bg); }
  .checks span, .checks small { display: block; font-size: var(--text-xs); }
  .checks strong { display: block; margin-top: var(--s1); font-size: var(--text-sm); }
  .checks > .btn { margin-left: auto; }
  .listbar { display: flex; align-items: center; gap: var(--s3); margin-bottom: var(--s3); flex-wrap: wrap; }
  .search { flex: 1; min-width: var(--connector-filter-min); margin: 0; }
  .rack { margin: 0; padding: 0; max-height: var(--connector-rack-max); overflow: auto; list-style: none; border: var(--line-width) solid var(--line-strong); border-radius: var(--r-m); }
  .rack li { display: grid; grid-template-columns: minmax(var(--connector-summary-min), var(--connector-summary-max)) minmax(0, 1fr); align-items: center; gap: var(--s3); padding-right: var(--s4); border-bottom: var(--line-width) solid var(--line-row); }
  .rack li:last-child { border-bottom: 0; }
  .rack li.picked { background: var(--surface-2); }
  .rack li.locked { background: var(--choice-locked-surface); }
  .service { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--s1) var(--s3); min-width: 0; }
  .service-title { min-width: 0; }
  .service-title strong, .service-title small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .service-title strong { font-size: var(--text-sm); }
  .service-title small, .state, .version-link { font-size: var(--text-xs); }
  .versions { grid-row: 1; grid-column: 2; display: flex; align-items: center; gap: var(--s2); font-size: var(--text-xs); }
  .versions strong { color: var(--text); }
  .state { color: var(--ok); }
  .state.warn { color: var(--warn); }
  .state.unknown { color: var(--muted); }
  .decision { display: grid; gap: var(--s2); }
  .version-link { justify-self: start; color: var(--accent); text-decoration: none; }
  .version-link:hover { text-decoration: underline; }
  .impact { margin-top: var(--s4); }
  .none { display: block !important; padding: var(--s5) !important; text-align: center; }
</style>
