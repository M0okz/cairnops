<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type ConnectorImportResult, type ProxmoxCertificate, type ProxmoxPreview } from '$lib/api';
  import { plural, t } from '$lib/i18n.svelte';
  import { prepareTargetAssignments, reconciliationCounts, resolvedTargetAssignments } from '$lib/reconciliation';
  import Icon from './Icon.svelte';
  import Checkbox from './ui/Checkbox.svelte';
  import TargetDecision from './TargetDecision.svelte';
  import ReconciliationSummary from './ReconciliationSummary.svelte';

  let { onclose, onsuccess, connectorId = '', initialName = '', initialAddress = '' }: {
    onclose: () => void;
    onsuccess: (result: ConnectorImportResult) => Promise<void> | void;
    connectorId?: string; initialName?: string; initialAddress?: string;
  } = $props();

  let name = $state('Proxmox VE');
  let address = $state('');
  let stage = $state<'address' | 'authorization'>('address');
  let certificate = $state<ProxmoxCertificate | null>(null);
  let approved = $state(false);
  let mode = $state<'automatic' | 'provided'>('automatic');
  let tokenID = $state('');
  let secret = $state('');
  let preview = $state<ProxmoxPreview | null>(null);
  let selected = $state<string[]>([]);
  let expected = $state<string[]>([]);
  let assignments = $state<Record<string, string>>({});
  let query = $state('');
  let busy = $state(false);
  let error = $state('');
  const counts = $derived(reconciliationCounts(selected, assignments));
  const visible = $derived(preview?.resources.filter(r => `${r.name} ${r.external_id} ${r.node}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())) ?? []);
  const stoppedAlerts = $derived(preview?.resources.filter(r => selected.includes(r.external_id) && expected.includes(r.external_id) && r.status === 'stopped').length ?? 0);

  onMount(() => {
    name = initialName || 'Proxmox VE'; address = initialAddress;
    if (connectorId) void inspectExisting();
  });

  function adopt(value: ProxmoxPreview) {
    preview = value;
    selected = value.resources.filter(r => r.importable && (r.already_imported || (r.type === 'node' || r.status === 'running' || r.status === 'available'))).map(r => r.external_id);
    expected = value.resources.filter(r => r.expected_running).map(r => r.external_id);
    assignments = prepareTargetAssignments(value.resources);
    for (const r of value.resources) if (r.already_imported_to) assignments[r.external_id] = r.already_imported_to.id;
    tokenID = ''; secret = '';
  }

  async function perform(action: () => Promise<void>) {
    busy = true; error = '';
    try { await action(); }
    catch (cause) { error = cause instanceof Error ? cause.message : t('proxmox.failed'); }
    finally { busy = false; }
  }

  async function probe(event: SubmitEvent) {
    event.preventDefault();
    await perform(async () => {
      certificate = await api<ProxmoxCertificate>('/api/v1/connectors/proxmox/certificate', { method: 'POST', body: JSON.stringify({ address }) });
      address = certificate.endpoint; approved = false;
      if (certificate.trusted && !connectorId) stage = 'authorization';
    });
  }

  async function acceptCertificate() {
    if (!certificate || !approved) return;
    if (!connectorId) { stage = 'authorization'; return; }
    await perform(async () => {
      await api(`/api/v1/connectors/${connectorId}/proxmox/certificate`, { method: 'POST', body: JSON.stringify({ fingerprint: certificate!.fingerprint }) });
      adopt(await api<ProxmoxPreview>(`/api/v1/connectors/${connectorId}/preview`, { method: 'POST' }));
    });
  }

  async function inspect(event: SubmitEvent) {
    event.preventDefault();
    if (!certificate || (!certificate.trusted && !approved)) return;
    await perform(async () => adopt(await api<ProxmoxPreview>('/api/v1/connectors/proxmox/preview', {
      method: 'POST', body: JSON.stringify({ name, address: certificate!.endpoint, mode, credentials: { token_id: tokenID, secret, fingerprint: certificate!.trusted ? '' : certificate!.fingerprint } })
    })));
  }

  async function inspectExisting() {
    await perform(async () => adopt(await api<ProxmoxPreview>(`/api/v1/connectors/${connectorId}/preview`, { method: 'POST' })));
  }

  function toggle(id: string) { selected = selected.includes(id) ? selected.filter(value => value !== id) : [...selected, id]; }
  function toggleExpected(id: string) { expected = expected.includes(id) ? expected.filter(value => value !== id) : [...expected, id]; }
  function toggleVisible() {
    const ids = visible.filter(r => r.importable).map(r => r.external_id);
    selected = ids.every(id => selected.includes(id)) ? selected.filter(id => !ids.includes(id)) : [...new Set([...selected, ...ids])];
  }
  async function save() {
    if (!preview || !selected.length || counts.review) return;
    await perform(async () => {
      const result = await api<ConnectorImportResult>('/api/v1/connectors/proxmox/import', { method: 'POST', body: JSON.stringify({
        receipt: preview!.receipt, resource_ids: selected, expected_running_ids: expected.filter(id => selected.includes(id)), target_assignments: resolvedTargetAssignments(selected, assignments)
      }) });
      await onsuccess(result);
    });
  }
</script>

<svelte:window onkeydown={(event) => { if (event.key === 'Escape' && !busy) onclose(); }} />

<div class="scrim" role="presentation" onclick={(event) => { if (event.currentTarget === event.target && !busy) onclose(); }}>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="proxmox-title">
    <header>
      <div><h2 id="proxmox-title">{t(preview ? 'wizard.chooseWhatEnters' : 'proxmox.connect')}</h2><p>{t('proxmox.lead')}</p></div>
      <button class="close" type="button" disabled={busy} onclick={onclose} aria-label={t('common.close')}><Icon name="close" size={14} /></button>
    </header>

    {#if preview}
      <div class="modal-body">
        <div class="checks"><strong>Proxmox VE {preview.version}</strong><span>{t('proxmox.https')}</span><button class="btn sm" disabled={busy} onclick={() => { if (connectorId) void inspectExisting(); else { preview = null; stage = 'authorization'; } }}>{t('proxmox.refresh')}</button></div>
        {#if preview.access.will_provision}
          <aside><strong>{t('proxmox.onConfirm')}</strong><p>{t('proxmox.accessPlan')}</p><p>{t('proxmox.cleanupHint')}</p></aside>
        {:else}<aside>{t('proxmox.existingAccess')}</aside>{/if}
        <p class="lead">{t('proxmox.stopHint')}</p>
        <ReconciliationSummary counts={counts} />
        <div class="listbar">
          <div class="field search"><label class="sr-only" for="pve-search">{t('proxmox.filter')}</label><input id="pve-search" bind:value={query} placeholder={t('proxmox.filter')} /></div>
          <button class="btn sm" type="button" onclick={toggleVisible}>{t('proxmox.toggleVisible')}</button>
          <span class="faint num">{selected.length} / {preview.importable_count}</span>
        </div>
        <ul class="rack">
          {#each visible as resource (resource.external_id)}
            {@const picked = selected.includes(resource.external_id)}
            <li class:picked>
              <Checkbox variant="row" checked={picked} disabled={!resource.importable || busy} onCheckedChange={() => toggle(resource.external_id)}>
                <span class="identity"><strong>{resource.name}</strong><small class="faint mono">{resource.external_id} · {resource.node} · {resource.status}</small></span>
              </Checkbox>
              <div class="decision">
                {#if resource.already_imported_to}<span class="faint">{t('wizard.alreadyBound')} · {resource.already_imported_to.name}</span>
                {:else if !resource.importable}<span class="pill idle">{t('proxmox.template')}</span>
                {:else}<TargetDecision name={resource.name} value={assignments[resource.external_id] ?? ''} candidates={resource.candidate_targets} availableTargets={preview.available_targets} disabled={!picked || busy} onselect={(targetID) => { assignments = { ...assignments, [resource.external_id]: targetID }; }} />{/if}
                {#if resource.importable && (resource.type === 'qemu' || resource.type === 'lxc')}
                  <Checkbox checked={expected.includes(resource.external_id)} disabled={!picked || busy} onCheckedChange={() => toggleExpected(resource.external_id)}>{t('proxmox.alertOnStop')}</Checkbox>
                {/if}
              </div>
            </li>
          {:else}<li class="empty">{t('proxmox.noMatch')}</li>{/each}
        </ul>
        {#if stoppedAlerts}<p class="warn" role="status">{t('proxmox.stoppedWarning', { count: stoppedAlerts })}</p>{/if}
        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>
      <footer><span class="faint note">{t('proxmox.observes')}</span><button class="btn primary" onclick={save} disabled={busy || !selected.length || counts.review > 0}>{busy ? t('wizard.importing') : counts.review ? plural('wizard.confirmChoices', counts.review) : t('proxmox.apply')}</button></footer>
    {:else if stage === 'address'}
      <form onsubmit={probe}>
        <div class="modal-body">
          <div class="fields"><div class="field"><label for="pve-name">{t('wizard.nameInCairnOps')}</label><input id="pve-name" bind:value={name} required maxlength="160" /></div>
          <div class="field"><label for="pve-address">{t('proxmox.address')}</label><input id="pve-address" bind:value={address} oninput={() => { certificate = null; approved = false; }} required maxlength="2048" inputmode="url" placeholder="https://proxmox.example.net:8006" /></div></div>
          {#if certificate && (!certificate.trusted || connectorId)}
            <aside><strong>{t('proxmox.certificateReview')}</strong><p>{certificate.subject}</p><p>{t('proxmox.issuer')}: {certificate.issuer}</p><code class="fingerprint">SHA-256 {certificate.fingerprint}</code>
              <Checkbox checked={approved} onCheckedChange={(value) => { approved = value; }}>{t('proxmox.approveCertificate')}</Checkbox>
            </aside>
          {/if}
          {#if error}<p class="error" role="alert">{error}</p>{/if}
        </div>
        <footer><span class="faint note">{t('proxmox.certificateHint')}</span>
          {#if certificate && (!certificate.trusted || connectorId)}<button class="btn primary" type="button" disabled={busy || !approved} onclick={acceptCertificate}>{t('proxmox.continue')}</button>
          {:else}<button class="btn primary" type="submit" disabled={busy}>{busy ? t('gate.verifying') : t('proxmox.checkAddress')}</button>{/if}
        </footer>
      </form>
    {:else}
      <form onsubmit={inspect}>
        <div class="modal-body">
          <div class="checks"><span>{address}</span><button class="btn sm" type="button" onclick={() => { stage = 'address'; certificate = null; secret = ''; }}>{t('proxmox.changeAddress')}</button></div>
          <div class="field"><label for="pve-mode">{t('wizard.authorisation')}</label><select id="pve-mode" bind:value={mode}><option value="automatic">{t('proxmox.automatic')}</option><option value="provided">{t('proxmox.provided')}</option></select></div>
          <p class="lead">{t(mode === 'automatic' ? 'proxmox.automaticHint' : 'proxmox.providedHint')}</p>
          <div class="fields"><div class="field"><label for="pve-token-id">{t('proxmox.tokenID')}</label><input id="pve-token-id" bind:value={tokenID} placeholder="user@pve!token" required maxlength="256" autocomplete="off" spellcheck="false" /></div>
          <div class="field"><label for="pve-secret">{t('proxmox.tokenSecret')}</label><input id="pve-secret" type="password" bind:value={secret} required maxlength="4096" autocomplete="off" spellcheck="false" /></div></div>
          {#if error}<p class="error" role="alert">{error}</p>{/if}
        </div>
        <footer><span class="faint note">{t('proxmox.previewHint')}</span><button class="btn primary" type="submit" disabled={busy}>{busy ? t('gate.verifying') : t('wizard.verifyAndPreview')}</button></footer>
      </form>
    {/if}
  </div>
</div>

<style>
  .modal { max-width: var(--connector-modal-max); }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--connector-field-min)), 1fr)); gap: var(--s4); }
  .checks, .listbar { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s3); margin-bottom: var(--s4); }
  .checks > button { margin-left: auto; }
  aside { padding: var(--s4); margin: var(--s3) 0; border: var(--line-width) solid var(--line-strong); border-radius: var(--r-m); background: var(--bg); font-size: var(--text-sm); }
  aside p { margin: var(--s2) 0; color: var(--muted); }
  .fingerprint { display: block; overflow-wrap: anywhere; font-size: var(--text-xs); margin: var(--s3) 0; }
  .lead { color: var(--muted); font-size: var(--text-sm); margin: var(--s3) 0 var(--s4); }
  .search { flex: 1; min-width: min(100%, var(--connector-filter-min)); margin: 0; }
  .rack { margin: 0; padding: 0; max-height: var(--connector-rack-max); overflow: auto; list-style: none; border: var(--line-width) solid var(--line-strong); border-radius: var(--r-m); }
  .rack li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: center; gap: var(--s3); padding-right: var(--s4); border-bottom: var(--line-width) solid var(--line-row); }
  .rack li:last-child { border-bottom: 0; }
  .rack li.picked { background: var(--surface-2); }
  .identity { min-width: 0; }
  .identity strong, .identity small { display: block; overflow-wrap: anywhere; }
  .identity strong { font-size: var(--text-sm); }
  .identity small, .decision { font-size: var(--text-xs); }
  .decision { display: grid; gap: var(--s2); padding: var(--s3) 0; }
  .checks span { overflow-wrap: anywhere; min-width: 0; }
  @media (max-width: 640px) { .rack li { grid-template-columns: minmax(0, 1fr); gap: 0; padding-right: 0; } .decision { padding: 0 var(--s4) var(--s4); } }
</style>
