<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { beforeNavigate } from '$app/navigation';
  import { api, type Connector, type ConnectorImportResult, type IndicatorConfiguration, type IndicatorProfileEntry } from '$lib/api';
  import { session, messageFrom } from '$lib/session.svelte';
  import { stamp } from '$lib/format';
  import { addSystemIndicators, indicatorSelectionKey } from '$lib/indicator-bulk';
  import { applyImported, createDrafts, draftFingerprint, importRequest, indicatorPayload, type EquipmentDraft, type SourcePreview } from '$lib/connector-configuration';
  import Icon from './Icon.svelte';
  import TargetDecision from './TargetDecision.svelte';
  import Checkbox from './ui/Checkbox.svelte';
  import SwitchControl from './ui/Switch.svelte';

  let { connector, onclose, onsuccess }: { connector: Connector; onclose: () => void; onsuccess: () => Promise<void> | void } = $props();
  let dialog: HTMLDialogElement;
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let indicatorError = $state('');
  let indicatorEditable = $state(false);
  let preview = $state<SourcePreview | null>(null);
  let configuration = $state<IndicatorConfiguration | null>(null);
  let drafts = $state<EquipmentDraft[]>([]);
  let profiles = $state<Array<{ id?: string; name: string; specification: IndicatorProfileEntry[] }>>([]);
  let profileName = $state('');
  let search = $state('');
  let activeId = $state('');
  let selectedIds = $state<string[]>([]);
  let baseline = $state('');
  let indicatorBaseline = $state('');
  let discarding = $state(false);
  let notice = $state('');
  let saved = false;
  let sourcesSaved = false;
  const supportsIndicators = $derived(connector.kind !== 'argus' && connector.kind !== 'generic_webhook');
  const kindPath = $derived(connector.kind === 'uptime_kuma' ? 'uptime-kuma' : connector.kind);
  const dirty = $derived(!!baseline && baseline !== draftFingerprint(drafts, profiles));
  const visible = $derived(drafts.filter(item => `${item.equipment.name} ${item.equipment.already_imported_to?.name ?? ''}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase())));
  const active = $derived(drafts.find(item => item.equipment.external_id === activeId));
  const selected = $derived(drafts.filter(item => selectedIds.includes(item.equipment.external_id)));
  const newSources = $derived(drafts.filter(item => item.supervise && !item.equipment.already_imported_to).length);

  onMount(() => { dialog.showModal(); void load(); });
  beforeNavigate(({ cancel }) => { if (saving || (dirty && !saved)) { cancel(); if (!saving) discarding = true; } });

  function requestClose() { if (saving) return; if (dirty) discarding = true; else onclose(); }
  function indicatorState() { return draftFingerprint(drafts.map(item => ({ ...item, supervise: false, targetId: '', equipment: { ...item.equipment, expectedRunning: false } })), profiles); }
  async function load() {
    loading = true; error = ''; indicatorError = ''; indicatorEditable = false;
    const [sources, indicators] = await Promise.allSettled([
      api<SourcePreview>(`/api/v1/connectors/${connector.id}/preview`, { method: 'POST' }),
      supportsIndicators ? loadIndicators() : Promise.resolve(null)
    ]);
    if (sources.status === 'fulfilled') preview = sources.value;
    else error = `Découverte des sources indisponible : ${messageFrom(sources.reason)}`;
    if (indicators.status === 'fulfilled') configuration = indicators.value;
    else indicatorError = `Indicateurs indisponibles : ${messageFrom(indicators.reason)}`;
    drafts = createDrafts(preview, configuration);
    for (const item of drafts) if (item.indicators) item.indicators.selected = new SvelteSet(item.indicators.selected);
    profiles = configuration?.profiles.map(profile => ({ id: profile.id, name: profile.name, specification: profile.specification })) ?? [];
    activeId = drafts[0]?.equipment.external_id ?? '';
    selectedIds = [];
    baseline = draftFingerprint(drafts, profiles); indicatorBaseline = indicatorState(); loading = false;
  }
  async function loadIndicators() {
    const stored = await api<IndicatorConfiguration>(`/api/v1/connectors/${connector.id}/indicator-configuration`);
    try {
      const fresh = await api<IndicatorConfiguration>(`/api/v1/connectors/${connector.id}/indicator-configuration/preview`, { method: 'POST' });
      indicatorEditable = true; return fresh;
    } catch (cause) {
      indicatorError = `Indicateurs enregistrés en lecture seule : ${messageFrom(cause)}`; return stored;
    }
  }
  function selectVisible() {
    const ids = visible.map(item => item.equipment.external_id);
    selectedIds = ids.every(id => selectedIds.includes(id)) ? selectedIds.filter(id => !ids.includes(id)) : [...new Set([...selectedIds, ...ids])];
  }
  function superviseSelected() {
    let count = 0;
    for (const draft of selected) if (draft.equipment.importable && !draft.equipment.already_imported_to) { draft.supervise = true; count++; }
    notice = `${count} équipement(s) à ajouter à la supervision. Les rapprochements restent à confirmer.`;
  }
  function collectSelected(enabled: boolean) {
    let count = 0;
    for (const item of selected) if (item.indicators && (!enabled || item.indicators.targetId || item.supervise)) { item.indicators.enabled = enabled; count++; }
    notice = `${count} équipement(s) : collecte ${enabled ? 'activée' : 'désactivée'}. Une Cible liée ou un ajout à la supervision est requis pour activer la collecte.`;
  }
  function systemIndicators() {
    const result = addSystemIndicators(selected.flatMap(item => item.indicators ? [item.indicators] : []));
    notice = `${result.added} indicateur(s) CPU, RAM ou disque ajouté(s) aux équipements sélectionnés dont la collecte est active.`;
  }
  function applyProfile(specification: IndicatorProfileEntry[]) {
    for (const item of selected) for (const candidate of item.indicators?.source.candidates ?? []) {
      const entry = specification.find(entry => entry.semantic_key === candidate.semantic_key && (entry.dimension ?? '') === (candidate.dimension ?? ''));
      if (!entry || !item.indicators) continue;
      const key = indicatorSelectionKey(candidate);
      if (entry.enabled && candidate.available) item.indicators.selected.add(key); else item.indicators.selected.delete(key);
    }
    notice = `Profil appliqué à ${selected.length} équipement(s) sélectionné(s).`;
  }
  function addProfile() {
    if (!active?.indicators || !profileName.trim()) return;
    const name = profileName.trim();
    profiles = [...profiles.filter(profile => profile.name.toLocaleLowerCase() !== name.toLocaleLowerCase()), { name, specification: active.indicators.source.candidates.map(candidate => ({ semantic_key: candidate.semantic_key, dimension: candidate.dimension, enabled: active.indicators!.selected.has(indicatorSelectionKey(candidate)) })) }];
    profileName = '';
  }
  async function save() {
    if (saving || !dirty) return;
    saving = true; error = ''; 
    try {
      // Validate every draft before the first write; new targets receive their IDs from import.
      if (indicatorEditable && indicatorState() !== indicatorBaseline) indicatorPayload(drafts.map(item => item.supervise && item.indicators ? { ...item, indicators: { ...item.indicators, targetId: item.targetId || '__pending_import__' } } : item));
      if (newSources && preview) {
        const body = importRequest(preview, drafts);
        if (Date.parse(preview.expires_at) <= Date.now()) throw new Error('La découverte a expiré. Fermez puis rouvrez le panneau pour actualiser les équipements.');
        const result = await api<ConnectorImportResult>(`/api/v1/connectors/${kindPath}/import`, { method: 'POST', body: JSON.stringify(body) });
        applyImported(drafts, result); sourcesSaved = true;
      }
      if (indicatorEditable && configuration && indicatorState() !== indicatorBaseline) {
        configuration = await api<IndicatorConfiguration>(`/api/v1/connectors/${connector.id}/indicator-configuration`, { method: 'PUT', body: JSON.stringify({ bindings: indicatorPayload(drafts), profiles, summary: 'Sources et indicateurs configurés par équipement' }) });
        indicatorBaseline = indicatorState();
      }
      baseline = draftFingerprint(drafts, profiles); saved = true;
      try { await onsuccess(); } catch { session.showNotice("Configuration enregistrée. Actualisez la page pour recharger les listes."); onclose(); return; }
      session.showNotice(`Configuration de « ${connector.name} » enregistrée.`); onclose();
    } catch (cause) {
      error = `${sourcesSaved ? 'Les sources ont été ajoutées. Les indicateurs restent à enregistrer ; vos choix sont conservés. ' : ''}${messageFrom(cause)}`;
    } finally { saving = false; }
  }
</script>

<svelte:window onbeforeunload={(event) => { if (!saved && (dirty || saving)) { event.preventDefault(); event.returnValue = ''; } }} />

<dialog bind:this={dialog} class="connector-config modal" aria-labelledby="connector-config-title" oncancel={(event) => { event.preventDefault(); requestClose(); }} onclick={(event) => { if (event.target === dialog) { const box = dialog.getBoundingClientRect(); if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) requestClose(); } }}>
  <header>
    <div><h2 id="connector-config-title">Configurer · {connector.name}</h2><p>Choisissez un équipement ou un service, puis ses sources et ses indicateurs.</p></div>
    <button class="close" type="button" onclick={requestClose} disabled={saving} aria-label="Fermer"><Icon name="close" size={16} /></button>
  </header>
  {#if discarding}
    <section class="discard" aria-labelledby="discard-title">
      <h3 id="discard-title">Des modifications ne sont pas enregistrées</h3><p>Revenez aux réglages pour les conserver, ou abandonnez les changements en attente.</p>
      <div class="actions"><button class="btn" onclick={() => (discarding = false)}>Continuer les modifications</button><button class="btn danger" onclick={() => { saved = true; onclose(); }}>Abandonner les modifications</button></div>
    </section>
  {:else if loading}
    <div class="empty" role="status"><strong>Lecture des équipements…</strong>Découverte des sources et des indicateurs disponibles.</div>
  {:else}
    {#if error}<p class="error message" role="alert">{error}</p>{/if}
    {#if indicatorError}<p class="message muted" role="status">{indicatorError}</p>{/if}
    {#if !drafts.length}<div class="empty"><strong>Aucun équipement disponible</strong><button class="btn" onclick={load}>Réessayer la découverte</button></div>
    {:else}
      <div class="equipment-workbench">
        <aside aria-label="Équipements et services">
          <label class="filter"><Icon name="search" size={14} /><input bind:value={search} placeholder="Rechercher un équipement" aria-label="Rechercher un équipement" /></label>
          <div class="selection-heading"><strong>{visible.length} équipement(s)</strong><button class="btn sm" onclick={selectVisible} disabled={saving || !visible.length}>{visible.length && visible.every(item => selectedIds.includes(item.equipment.external_id)) ? 'Désélectionner' : 'Tout sélectionner'}</button></div>
          <div class="equipment-list">
            {#each visible as item (item.equipment.external_id)}
              <div class="equipment-row" class:active={activeId === item.equipment.external_id}>
                <Checkbox checked={selectedIds.includes(item.equipment.external_id)} disabled={saving} ariaLabel={`Sélectionner ${item.equipment.name}`} onCheckedChange={(checked) => { selectedIds = checked ? [...selectedIds, item.equipment.external_id] : selectedIds.filter(id => id !== item.equipment.external_id); }} />
                <button class="equipment-choice" aria-current={activeId === item.equipment.external_id ? 'true' : undefined} onclick={() => (activeId = item.equipment.external_id)}><strong>{item.equipment.name}</strong><small>{item.equipment.already_imported_to ? 'Sources liées' : item.supervise ? 'À superviser' : 'Non supervisé'}{item.indicators?.enabled ? ' · Indicateurs actifs' : ''}</small></button>
              </div>
            {:else}<p class="empty">Aucun équipement ne correspond à la recherche.</p>{/each}
          </div>
        </aside>
        <div class="equipment-detail">
          {#if selected.length}
            <section class="bulk" aria-label="Actions groupées"><strong>{selected.length} sélectionné(s)</strong><div class="actions"><button class="btn sm" onclick={superviseSelected} disabled={saving || !preview}>Ajouter à la supervision</button>{#if supportsIndicators}<button class="btn sm" disabled={saving || !indicatorEditable} onclick={() => collectSelected(true)}>Activer la collecte</button><button class="btn sm" disabled={saving || !indicatorEditable} onclick={() => collectSelected(false)}>Désactiver la collecte</button><button class="btn sm" onclick={systemIndicators} disabled={saving || !indicatorEditable}>Ajouter CPU · RAM · disques</button>{#each profiles as profile (profile.id ?? profile.name)}<button class="btn sm" disabled={saving || !indicatorEditable} onclick={() => applyProfile(profile.specification)}>Appliquer « {profile.name} »</button>{/each}{/if}</div></section>
          {/if}
          {#if notice}<p class="muted" role="status">{notice}</p>{/if}
          {#if active}
            <h3 class="equipment-title">{active.equipment.name}</h3>
            <section class="settings-section" aria-labelledby="source-heading">
              <h4 id="source-heading">Sources de supervision</h4>
              <p class="muted">Leurs signaux participent à l’état de santé et aux incidents.</p>
              {#if active.equipment.already_imported_to}
                <p>Sources déjà liées à <strong>{active.equipment.already_imported_to.name}</strong>.</p>
              {:else if active.equipment.importable && preview}
                <div class="setting"><span>Ajouter cet équipement à la supervision</span><SwitchControl bind:checked={active.supervise} disabled={saving} label={`Superviser ${active.equipment.name}`} /></div>
                {#if active.supervise}
                  <TargetDecision name={active.equipment.name} value={active.targetId} candidates={active.equipment.candidate_targets} availableTargets={preview.available_targets} disabled={saving || !!active.indicators?.source.target_id} compact onselect={(id) => { active.targetId = id; if (active.indicators && !active.indicators.source.imported) active.indicators.targetId = id; }} />
                  {#if connector.kind === 'proxmox'}<div class="setting"><span>Attendre un état démarré</span><SwitchControl bind:checked={active.equipment.expectedRunning} disabled={saving} label="Attendre un état démarré" /></div>{/if}
                {/if}
              {:else}<p class="muted">{active.equipment.reason || 'Découverte des sources indisponible.'}</p>{/if}
            </section>
            {#if supportsIndicators}
              <section class="settings-section" aria-labelledby="indicator-heading">
                <h4 id="indicator-heading">Indicateurs de contexte</h4><p class="muted">Ces mesures aident au diagnostic et ne déclenchent pas d’incident.</p>
                {#if active.indicators}
                  <div class="setting"><span>Collecter les indicateurs</span><SwitchControl bind:checked={active.indicators.enabled} disabled={saving || !indicatorEditable} label={`Collecter les indicateurs de ${active.equipment.name}`} /></div>
                  {#if active.indicators.enabled}
                    {#if !active.equipment.already_imported_to && !active.supervise && !active.indicators.source.target_id}
                      <label class="target-label">Cible CairnOps<select bind:value={active.indicators.targetId} disabled={saving || !indicatorEditable}><option value="">Choisir une Cible…</option>{#each session.targets as target (target.id)}<option value={target.id}>{target.name}</option>{/each}</select></label>
                    {/if}
                    <div class="candidate-list">
                      {#each active.indicators.source.candidates as candidate (indicatorSelectionKey(candidate))}
                        {@const key = indicatorSelectionKey(candidate)}
                        <Checkbox variant="selection" checked={active.indicators.selected.has(key)} disabled={saving || !indicatorEditable || (!candidate.available && !active.indicators.selected.has(key))} onCheckedChange={(checked) => { if (checked) active.indicators!.selected.add(key); else active.indicators!.selected.delete(key); }}>
                          <span><strong>{candidate.label}</strong>{#if candidate.dimension || !candidate.available}<small>{candidate.dimension ?? ''}{!candidate.available ? ` ${candidate.reason || 'Indisponible — à retirer'}` : ''}</small>{/if}</span>
                        </Checkbox>
                      {:else}<p class="muted">Aucun indicateur disponible pour cet équipement.</p>{/each}
                    </div>
                    <details><summary>Réutiliser cette sélection</summary><div class="actions"><input bind:value={profileName} aria-label="Nom du profil" placeholder="Nom du profil" maxlength="100" disabled={saving || !indicatorEditable} /><button class="btn sm" disabled={saving || !indicatorEditable || !profileName.trim()} onclick={addProfile}>Préparer le profil</button></div><p class="muted">Le profil sera enregistré avec les autres modifications.</p></details>
                    <p class="muted">Collecte chaque minute · détail 24 h · agrégats 7 j.</p>
                  {/if}
                {:else}<p class="muted">Aucun catalogue d’indicateurs disponible. Pour une nouvelle ressource Proxmox, enregistrez d’abord sa supervision.</p>{/if}
              </section>
            {/if}
          {/if}
          {#if configuration}
            <details><summary>Capacités et historique des indicateurs</summary>
              {#each configuration.capabilities as capability (capability.key)}<p>{capability.key} · {capability.message || capability.status} · {stamp(capability.checked_at)}</p>{/each}
              {#each configuration.activity as entry (entry.id)}<p>{entry.summary}<br /><small class="muted">{entry.actor_name || 'CairnOps'} · {stamp(entry.occurred_at)}</small></p>{:else}<p class="muted">Aucune modification d’indicateurs enregistrée.</p>{/each}
            </details>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
  <footer><span class="note" role="status">{dirty ? `${newSources} équipement(s) à superviser · Modifications non enregistrées` : 'Aucune modification en attente'}</span><button class="btn" onclick={requestClose} disabled={saving}>Fermer</button><button class="btn primary" onclick={save} disabled={saving || loading || !dirty || discarding}>{saving ? 'Enregistrement…' : 'Enregistrer les modifications'}</button></footer>
</dialog>

<style>
  .connector-config { margin: auto; padding: 0; width: min(var(--connector-config-width), calc(100vw - 2 * var(--s5))); max-width: none; height: min(var(--connector-config-height), calc(100dvh - 2 * var(--s5))); max-height: none; color: var(--ink); background: var(--surface); border: var(--line-width) solid var(--line-strong); }
  .connector-config:not([open]) { display: none; }
  .connector-config[open] { display: flex; flex-direction: column; }
  .connector-config::backdrop { background: var(--drawer-backdrop); }
  header, footer { flex: none; }
  header > div { min-width: 0; }
  header h2, .equipment-title { overflow-wrap: anywhere; }
  .message { margin: var(--s3) var(--s5); flex: none; }
  .equipment-workbench { flex: 1; min-height: 0; display: grid; grid-template-columns: var(--connector-equipment-list-width) minmax(0, 1fr); overflow: hidden; }
  aside { min-height: 0; display: flex; flex-direction: column; padding: var(--s4); gap: var(--s3); border-right: var(--line-width) solid var(--line); }
  .filter { display: flex; align-items: center; gap: var(--s3); }
  input, select { min-width: 0; max-width: 100%; height: var(--ctl-h); padding: 0 var(--s3); background: var(--bg); border: var(--line-width) solid var(--line-strong); border-radius: var(--r-m); color: var(--ink); font: inherit; }
  .filter input { width: 100%; }
  .selection-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--s3); font-size: var(--text-xs); }
  .equipment-list { overflow-y: auto; min-height: 0; }
  .equipment-row { display: flex; align-items: center; gap: var(--s2); padding: var(--s2); border-radius: var(--r-m); }
  .equipment-row.active { background: var(--surface-2); box-shadow: inset var(--s1) 0 var(--accent); }
  .equipment-choice { flex: 1; min-width: 0; min-height: var(--choice-row-min-height); padding: var(--s3); text-align: left; border: 0; background: none; color: var(--ink); }
  .equipment-choice:hover { background: var(--surface-2); }
  .equipment-choice strong, .equipment-choice small { display: block; overflow-wrap: anywhere; }
  .equipment-choice strong { font-size: var(--text-sm); }
  .equipment-choice small, .muted, .note { color: var(--muted); font-size: var(--text-xs); }
  .equipment-detail { overflow-y: auto; min-width: 0; padding: var(--s5); background: var(--bg); }
  .equipment-title { font-size: var(--text-base); margin: 0 0 var(--s5); }
  .settings-section { margin-bottom: var(--s5); padding-bottom: var(--s5); border-bottom: var(--line-width) solid var(--line); }
  .settings-section h4 { font-size: var(--text-sm); }
  .settings-section p, details p { margin: var(--s3) 0; }
  .setting { display: flex; align-items: center; justify-content: space-between; gap: var(--s4); margin: var(--s4) 0; font-size: var(--text-sm); }
  .target-label { display: grid; gap: var(--s3); }
  .candidate-list { --choice-selection-columns: var(--choice-hit-area) minmax(0, 1fr); margin: var(--s4) 0; background: var(--surface); border: var(--line-width) solid var(--line); border-radius: var(--r-m); }
  .candidate-list strong, .candidate-list small { display: block; overflow-wrap: anywhere; }
  .candidate-list small { color: var(--muted); }
  .actions { display: flex; align-items: center; gap: var(--s3); flex-wrap: wrap; }
  .bulk { padding: var(--s4); margin-bottom: var(--s5); background: var(--surface-2); border-radius: var(--r-m); }
  .bulk strong { display: block; margin-bottom: var(--s3); }
  summary { cursor: pointer; font-size: var(--text-sm); padding: var(--s3) 0; }
  .discard { flex: 1; padding: var(--s5); }
  .discard p { margin: var(--s4) 0; }
  footer { flex-wrap: wrap; gap: var(--s3); }
  footer .note { flex: 1; }
  @media (max-width: 48rem) {
    .connector-config { width: 100vw; height: 100dvh; border: 0; border-radius: 0; }
    .equipment-workbench { display: flex; flex-direction: column; overflow-y: auto; }
    aside { flex: none; border-right: 0; border-bottom: var(--line-width) solid var(--line); }
    .equipment-list { max-height: var(--connector-equipment-mobile-height); }
    .equipment-detail { overflow: visible; padding: var(--s4); }
    header, footer { padding: var(--s4); }
    footer .note { flex-basis: 100%; }
    footer .primary { flex: 1; white-space: normal; height: auto; min-height: var(--ctl-h); }
  }
</style>
