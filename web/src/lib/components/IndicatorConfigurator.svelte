<script lang="ts">
  import Icon from './Icon.svelte';
  import BrandMark, { type BrandName } from './BrandMark.svelte';
  import { api, type Connector, type IndicatorBinding, type IndicatorCandidate, type IndicatorConfiguration, type IndicatorProfile, type IndicatorProfileEntry } from '$lib/api';
  import { session, messageFrom } from '$lib/session.svelte';
  import { stamp } from '$lib/format';
  import { addSystemIndicators, indicatorSelectionKey, setBindingsEnabled } from '$lib/indicator-bulk';

  let { connector, onclose, onsuccess }: { connector: Connector; onclose: () => void; onsuccess: () => Promise<void> | void } = $props();
  type Section = 'scope' | 'indicators' | 'capabilities' | 'history';
  type DraftBinding = { source: IndicatorBinding; enabled: boolean; targetId: string; selected: Set<string> };

  let section = $state<Section>('scope');
  let maximized = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let reviewing = $state(false);
  let error = $state('');
  let editable = $state(false);
  let configuration = $state<IndicatorConfiguration | null>(null);
  let bindings = $state<DraftBinding[]>([]);
  let profiles = $state<Array<{ id?: string; name: string; specification: IndicatorProfileEntry[] }>>([]);
  let profileName = $state('');
  let search = $state('');
  let activeExternal = $state('');
  let bulkNotice = $state('');

  const brands: Record<Exclude<Connector['kind'], 'generic_webhook'>, BrandName> = { zabbix: 'zabbix', uptime_kuma: 'uptime_kuma', patchmon: 'patchmon' };
  $effect(() => { void load(); });

  async function load() {
    loading = true; error = ''; editable = false;
    try {
      const saved = await api<IndicatorConfiguration>(`/api/v1/connectors/${connector.id}/indicator-configuration`);
      try {
        configuration = await api<IndicatorConfiguration>(`/api/v1/connectors/${connector.id}/indicator-configuration/preview`, { method: 'POST' });
        editable = true;
      } catch (cause) {
        configuration = saved;
        editable = false;
        error = `Configuration enregistrée affichée en lecture seule. La redécouverte distante a échoué : ${messageFrom(cause)}`;
      }
      bindings = configuration.bindings.map((binding) => {
        const current = new Set(binding.indicators.filter((indicator) => indicator.enabled).map((indicator) => `${indicator.external_id}\u0000${indicator.semantic_key}\u0000${indicator.dimension ?? ''}`));
        const intelligent = current.size === 0 ? new Set(binding.candidates.filter((candidate) => candidate.recommended && candidate.available).map(indicatorSelectionKey)) : current;
        return { source: binding, enabled: binding.imported ? binding.enabled : false, targetId: binding.target_id ?? '', selected: intelligent };
      });
      profiles = configuration.profiles.map((profile) => ({ id: profile.id, name: profile.name, specification: profile.specification }));
      activeExternal = bindings.find((binding) => binding.enabled)?.source.external_id ?? bindings[0]?.source.external_id ?? '';
    } catch (cause) { error = messageFrom(cause); }
    finally { loading = false; }
  }

  const active = $derived(bindings.find((binding) => binding.enabled && binding.source.external_id === activeExternal) ?? bindings.find((binding) => binding.enabled) ?? null);
  const visibleBindings = $derived(bindings.filter((binding) => `${binding.source.external_name} ${binding.source.target_name ?? ''}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase())));
  const enabledCount = $derived(bindings.filter((binding) => binding.enabled).length);
  const selectedCount = $derived(bindings.reduce((total, binding) => total + (binding.enabled ? binding.selected.size : 0), 0));
  const unassigned = $derived(bindings.filter((binding) => binding.enabled && !binding.targetId).length);
  const unavailableSelected = $derived(bindings.reduce((total, binding) => total + [...binding.selected].filter((key) => !binding.source.candidates.some((candidate) => indicatorSelectionKey(candidate) === key && candidate.available)).length, 0));
  const visibleAttached = $derived(visibleBindings.filter((binding) => binding.targetId.trim()));
  const allVisibleEnabled = $derived(visibleAttached.length > 0 && visibleAttached.every((binding) => binding.enabled));

  function toggleCandidate(binding: DraftBinding, candidate: IndicatorCandidate) {
    const key = indicatorSelectionKey(candidate);
    if (binding.selected.has(key)) binding.selected.delete(key); else if (candidate.available) binding.selected.add(key);
  }

  function addProfile() {
    const name = profileName.trim(); if (!name || !active) return;
    const specification = active.source.candidates.map((candidate) => ({ semantic_key: candidate.semantic_key, dimension: candidate.dimension, enabled: active.selected.has(indicatorSelectionKey(candidate)) }));
    profiles = [...profiles.filter((profile) => profile.name.toLocaleLowerCase() !== name.toLocaleLowerCase()), { name, specification }];
    profileName = '';
  }

  function applyProfile(profile: Pick<IndicatorProfile, 'specification'>) {
    for (const binding of bindings) {
      for (const candidate of binding.source.candidates) {
        const entry = profile.specification.find((item) => item.semantic_key === candidate.semantic_key && (item.dimension ?? '') === (candidate.dimension ?? ''));
        if (!entry) continue;
        if (entry.enabled && candidate.available) binding.selected.add(indicatorSelectionKey(candidate)); else binding.selected.delete(indicatorSelectionKey(candidate));
      }
    }
    bulkNotice = `Profil appliqué aux ${bindings.length} périmètre(s).`;
  }

  function toggleVisibleBindings() {
    const enable = !allVisibleEnabled;
    const result = setBindingsEnabled(bindings, new Set(visibleBindings.map((binding) => binding.source.external_id)), enable);
    if (enable) {
      bulkNotice = `${result.changed} périmètre(s) activé(s)${result.skipped ? ` · ${result.skipped} ignoré(s), sans Cible liée` : ''}.`;
      activeExternal = bindings.find((binding) => binding.enabled)?.source.external_id ?? activeExternal;
    } else {
      bulkNotice = `${result.changed} périmètre(s) désactivé(s).`;
      activeExternal = bindings.find((binding) => binding.enabled)?.source.external_id ?? '';
    }
  }

  function applySystemIndicators() {
    const result = addSystemIndicators(bindings);
    bulkNotice = result.added
      ? `${result.added} Indicateur(s) CPU, RAM ou disque ajouté(s) sur ${result.affectedBindings} périmètre(s).`
      : 'Le socle CPU, RAM et disques est déjà appliqué à tous les périmètres actifs.';
  }

  function review() {
    error = '';
    if (unassigned > 0) { section = 'scope'; error = `${unassigned} périmètre(s) activé(s) doivent être rattachés à une Cible.`; return; }
    if (unavailableSelected > 0) { section = 'indicators'; error = `${unavailableSelected} sélection(s) externe(s) doivent être confirmées de nouveau.`; return; }
    reviewing = true;
  }

  async function save() {
    if (!configuration) return;
    saving = true; error = '';
    try {
      await api(`/api/v1/connectors/${connector.id}/indicator-configuration`, {
        method: 'PUT',
        body: JSON.stringify({
          bindings: bindings.map((binding) => ({
            id: binding.source.id,
            target_id: binding.targetId,
            external_id: binding.source.external_id,
            external_name: binding.source.external_name,
            enabled: binding.enabled,
            indicators: binding.source.candidates.filter((candidate) => binding.selected.has(indicatorSelectionKey(candidate))).map((candidate) => ({ semantic_key: candidate.semantic_key, label: candidate.label, external_id: candidate.external_id, dimension: candidate.dimension ?? '', unit: candidate.unit, metadata: candidate.metadata }))
          })),
          profiles: profiles.map((profile) => ({ id: profile.id, name: profile.name, specification: profile.specification })),
          summary: `${enabledCount} périmètre(s) · ${selectedCount} Indicateur(s)`
        })
      });
      await onsuccess();
      session.showNotice(`Indicateurs de « ${connector.name} » enregistrés.`);
      onclose();
    } catch (cause) { error = messageFrom(cause); reviewing = false; }
    finally { saving = false; }
  }

  function onKeydown(event: KeyboardEvent) { if (event.key === 'Escape' && !saving) { if (reviewing) reviewing = false; else onclose(); } }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && !saving && onclose()}>
  <div class="modal indicator-modal" class:maximized role="dialog" aria-modal="true" aria-labelledby="indicator-config-title">
    <header>
      {#if connector.kind !== 'generic_webhook'}<BrandMark name={brands[connector.kind]} size={32} />{/if}
      <div><h2 id="indicator-config-title">Indicateurs · {connector.name}</h2><p>Choisissez le contexte utile au quotidien. Les seuils et alertes restent dans le produit d’origine.</p></div>
      <button class="window-control" type="button" onclick={() => (maximized = !maximized)} aria-label={maximized ? 'Réduire la modale' : 'Agrandir la modale'} title={maximized ? 'Réduire' : 'Agrandir'}>{maximized ? '⊡' : '□'}</button>
      <button class="close" type="button" onclick={onclose} disabled={saving} aria-label="Fermer"><Icon name="close" size={14} /></button>
    </header>

    <nav class="config-nav" aria-label="Sections de configuration">
      <button type="button" class:active={section === 'scope'} onclick={() => { section = 'scope'; reviewing = false; }}>Périmètre <span>{enabledCount}</span></button>
      <button type="button" class:active={section === 'indicators'} onclick={() => { section = 'indicators'; reviewing = false; }}>Indicateurs <span>{selectedCount}</span></button>
      <button type="button" class:active={section === 'capabilities'} onclick={() => { section = 'capabilities'; reviewing = false; }}>Capacités</button>
      <button type="button" class:active={section === 'history'} onclick={() => { section = 'history'; reviewing = false; }}>Historique</button>
    </nav>

    <div class="modal-body">
      {#if loading}
        <div class="empty"><strong>Lecture des capacités…</strong>Le catalogue court est redécouvert dans {connector.name}.</div>
      {:else if !configuration}
        <div class="error" role="alert">{error || 'Configuration indisponible.'}</div>
      {:else if reviewing}
        <section class="review" aria-labelledby="configuration-summary-title">
          <div class="review-mark">{selectedCount}</div>
          <div><h3 id="configuration-summary-title">Confirmer la configuration</h3><p>{enabledCount} périmètre(s) seront collectés toutes les minutes avec {selectedCount} Indicateur(s).</p></div>
          <dl><div><dt>Nouveaux périmètres</dt><dd>{bindings.filter((binding) => binding.enabled && !binding.source.imported).length}</dd></div><div><dt>Profils nommés</dt><dd>{profiles.length}</dd></div><div><dt>Épingles personnelles</dt><dd>Inchangées</dd></div></dl>
          <div class="retention"><strong>Rétention bornée</strong><p>Les points détaillés expirent après 24 h, les agrégats après 7 j. Une désactivation masque et arrête immédiatement la collecte ; les points déjà stockés expirent naturellement, sans suppression brutale.</p></div>
        </section>
      {:else if section === 'scope'}
        <section class="scope-section">
          <div class="section-head"><div><h3>Périmètre</h3><p>Une nouvelle Cible n’entre jamais automatiquement. Chaque activation reste explicite.</p></div><div class="scope-actions"><button class="btn sm" type="button" onclick={toggleVisibleBindings} disabled={!editable || visibleAttached.length === 0}>{allVisibleEnabled ? 'Tout désélectionner' : 'Tout sélectionner'} <span class="num">{visibleAttached.length}</span></button><button class="btn sm" type="button" onclick={applySystemIndicators} disabled={!editable || enabledCount === 0} title="Ajoute les Indicateurs disponibles sans retirer la sélection actuelle">Ajouter CPU · RAM · disques</button><label class="search"><Icon name="search" size={13} /><input bind:value={search} aria-label="Filtrer les périmètres" placeholder="Filtrer les Cibles" /></label></div></div>
          {#if bulkNotice}<p class="bulk-notice" role="status">{bulkNotice}</p>{/if}
          <div class="scope-table">
            {#each visibleBindings as binding (binding.source.external_id)}
              <div class="scope-row" class:pending={!binding.source.imported}>
                <label class="switch"><input type="checkbox" bind:checked={binding.enabled} /><span></span></label>
                <button class="scope-name" type="button" onclick={() => { activeExternal = binding.source.external_id; section = 'indicators'; }}><strong>{binding.source.external_name}</strong><small>{binding.source.imported ? binding.source.target_name ?? 'Cible liée' : 'Nouvelle découverte · confirmation requise'}</small></button>
                <select bind:value={binding.targetId} disabled={!binding.enabled || binding.source.imported} aria-label={`Cible CairnOps pour ${binding.source.external_name}`}>
                  <option value="">Choisir une Cible…</option>
                  {#each session.targets as target (target.id)}<option value={target.id}>{target.name}</option>{/each}
                </select>
                <span class="count num">{binding.selected.size}</span>
              </div>
            {/each}
          </div>
        </section>
      {:else if section === 'indicators'}
        <section class="indicator-workbench">
          <aside>
            <h3>Cibles</h3>
            {#each bindings.filter((binding) => binding.enabled) as binding (binding.source.external_id)}
              <button type="button" class:active={active?.source.external_id === binding.source.external_id} onclick={() => (activeExternal = binding.source.external_id)}><span>{binding.source.external_name}</span><b class="num">{binding.selected.size}</b></button>
            {:else}<p>Activez d’abord un périmètre.</p>{/each}
          </aside>
          <div class="catalog">
            {#if active}
              <div class="section-head"><div><h3>{active.source.external_name}</h3><p>Présélection intelligente · chaque élément reste modifiable avant confirmation.</p></div></div>
              {#if bulkNotice}<p class="bulk-notice" role="status">{bulkNotice}</p>{/if}
              <div class="profile-strip">
                {#each profiles as profile (profile.id ?? profile.name)}<button class="btn sm" type="button" onclick={() => applyProfile(profile)}>Appliquer « {profile.name} »</button>{/each}
                <span></span><input bind:value={profileName} maxlength="100" placeholder="Nom du profil" aria-label="Nom du nouveau profil" /><button class="btn sm" type="button" onclick={addProfile} disabled={!profileName.trim()}>Enregistrer le profil</button>
              </div>
              <div class="candidate-list">
                {#each active.source.candidates as candidate (indicatorSelectionKey(candidate))}
                  <label class="candidate" class:unavailable={!candidate.available}>
                    <input type="checkbox" checked={active.selected.has(indicatorSelectionKey(candidate))} disabled={!candidate.available} onchange={() => toggleCandidate(active, candidate)} />
                    <span><strong>{candidate.label}</strong><small>{candidate.dimension || candidate.semantic_key}</small></span>
                    {#if candidate.recommended}<span class="recommended">Recommandé</span>{/if}
                    <code>{candidate.external_id}</code>
                    {#if !candidate.available}<small class="why">{candidate.reason || 'À vérifier'}</small>{/if}
                  </label>
                {:else}<div class="empty"><strong>Aucun Indicateur permis</strong>Ce produit ne publie rien du catalogue court pour cette Cible.</div>{/each}
              </div>
            {:else}<div class="empty"><strong>Aucun périmètre actif</strong>Activez une Cible dans la section Périmètre.</div>{/if}
          </div>
        </section>
      {:else if section === 'capabilities'}
        <section><div class="section-head"><div><h3>Capacités</h3><p>Les pannes sont isolées : les Incidents peuvent continuer à se synchroniser même si les Indicateurs deviennent indisponibles.</p></div></div><div class="capabilities">{#each configuration.capabilities as capability (capability.key)}<article><i class="dot {capability.status === 'available' ? 'ok' : capability.status === 'degraded' ? 'warn' : 'crit'}"></i><span><strong>{capability.key === 'indicators' ? 'Indicateurs' : capability.key}</strong><small>{capability.message || 'Capacité disponible'}</small></span><time>{stamp(capability.checked_at)}</time></article>{:else}<div class="empty"><strong>Première vérification</strong>La capacité sera datée au premier relevé.</div>{/each}</div></section>
      {:else}
        <section><div class="section-head"><div><h3>Historique de configuration</h3><p>Journal local au Connecteur : auteur, date et résumé de chaque confirmation.</p></div></div><div class="activity">{#each configuration.activity as entry (entry.id)}<article><i></i><span><strong>{entry.summary}</strong><small>{entry.actor_name || 'CairnOps'} · {stamp(entry.occurred_at)}</small></span></article>{:else}<div class="empty"><strong>Aucune modification enregistrée</strong>La première confirmation apparaîtra ici.</div>{/each}</div></section>
      {/if}
      {#if error && configuration}<p class:error={editable} class="readonly-note" role={editable ? 'alert' : 'status'}>{error}</p>{/if}
    </div>

    <footer>
      <span class="note">Collecte 1 min · détail 24 h · agrégats 7 j · contexte uniquement</span>
      {#if reviewing}<button class="btn" type="button" onclick={() => (reviewing = false)} disabled={saving}>Revenir</button><button class="btn primary" type="button" onclick={save} disabled={saving || !editable}>{saving ? 'Enregistrement…' : 'Confirmer et enregistrer'}</button>{:else}<button class="btn" type="button" onclick={onclose}>{editable ? 'Annuler' : 'Fermer'}</button><button class="btn primary" type="button" onclick={review} disabled={loading || !configuration || !editable}>Vérifier la sélection</button>{/if}
    </footer>
  </div>
</div>

<style>
  .indicator-modal { width: 85vw; max-width: 85vw; height: min(85vh, 54rem); }
  .indicator-modal.maximized { width: calc(100vw - 2 * var(--s5)); max-width: none; height: calc(100dvh - 2 * var(--s5)); max-height: none; }
  .window-control { margin-left: auto; width: 1.625rem; height: 1.625rem; display: grid; place-items: center; border: 1px solid var(--line-strong); border-radius: var(--r-m); background: none; color: var(--muted); }
  .modal header .close { margin-left: 0; }
  .config-nav { display: flex; gap: var(--s1); padding: 0 var(--s5); border-bottom: 1px solid var(--line); overflow-x: auto; }
  .config-nav button { min-height: 2.5rem; padding: 0 var(--s3); border: 0; border-bottom: 2px solid transparent; background: none; color: var(--muted); white-space: nowrap; }
  .config-nav button.active { color: var(--ink); border-color: var(--accent); }
  .config-nav span { margin-left: var(--s2); color: var(--faint); font-family: var(--font-num); font-size: .625rem; }
  .modal-body { background: var(--bg); }
  .readonly-note { margin: var(--s4) 0 0; padding: var(--s3); border: 1px solid color-mix(in srgb, var(--warn) 35%, var(--line)); border-radius: var(--r-m); color: var(--warn); background: color-mix(in srgb, var(--warn) 7%, transparent); font-size: .6875rem; }
  .section-head { display: flex; align-items: center; gap: var(--s4); margin-bottom: var(--s4); }
  .section-head > div { min-width: 0; flex: 1; }
  h3 { font-size: .875rem; }
  .section-head p, .review p { margin-top: var(--s1); color: var(--muted); font-size: .75rem; }
  .search { display: flex; align-items: center; gap: var(--s2); width: 16rem; height: var(--ctl-h); padding: 0 var(--s3); border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--surface); color: var(--faint); }
  .scope-actions { display: flex; align-items: center; justify-content: flex-end; gap: var(--s3); flex-wrap: wrap; }
  .scope-actions .btn { white-space: nowrap; }
  .scope-actions .btn span { margin-left: var(--s1); color: var(--faint); }
  .search input, .profile-strip input { min-width: 0; width: 100%; border: 0; outline: 0; background: none; color: var(--ink); font: inherit; }
  .scope-table, .candidate-list, .capabilities, .activity { border: 1px solid var(--line-strong); border-radius: var(--r-l); background: var(--surface); overflow: hidden; }
  .scope-row { display: grid; grid-template-columns: 2.5rem minmax(12rem, 1fr) minmax(12rem, .8fr) 3rem; align-items: center; min-height: 3.5rem; padding: 0 var(--s4); border-bottom: 1px solid var(--line-row); }
  .scope-row:last-child { border-bottom: 0; }
  .scope-row.pending { background: var(--surface-2); }
  .switch input { position: absolute; opacity: 0; pointer-events: none; }
  .switch span { display: block; width: 1.75rem; height: 1rem; padding: 2px; border-radius: var(--r-pill); background: var(--surface-3); transition: background var(--d1) var(--ease); }
  .switch span::after { content: ''; display: block; width: .75rem; height: .75rem; border-radius: 50%; background: var(--muted); transition: transform var(--d1) var(--ease), background var(--d1) var(--ease); }
  .switch input:checked + span { background: var(--accent); }
  .switch input:checked + span::after { transform: translateX(.75rem); background: var(--accent-ink); }
  .scope-name { min-width: 0; border: 0; background: none; text-align: left; }
  .scope-name strong, .scope-name small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .scope-name strong { font-size: .75rem; }
  .scope-name small { color: var(--faint); font-size: .6875rem; }
  select, .profile-strip input { height: var(--ctl-h); border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--bg); color: var(--ink); padding: 0 var(--s3); }
  .count { justify-self: end; color: var(--faint); }
  .bulk-notice { margin: 0 0 var(--s4); padding: var(--s3); border-left: 2px solid var(--accent); background: var(--surface); color: var(--muted); font-size: .6875rem; }
  .indicator-workbench { display: grid; grid-template-columns: 14rem minmax(0, 1fr); min-height: 100%; margin: calc(-1 * var(--s5)); }
  .indicator-workbench aside { padding: var(--s5); border-right: 1px solid var(--line); background: var(--surface); }
  aside h3 { margin-bottom: var(--s3); }
  aside button { width: 100%; display: flex; align-items: center; gap: var(--s3); min-height: 2.25rem; padding: 0 var(--s3); border: 0; border-radius: var(--r-m); background: none; color: var(--muted); text-align: left; }
  aside button.active { background: var(--surface-2); color: var(--ink); }
  aside button span { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  aside p { color: var(--faint); font-size: .6875rem; }
  .catalog { min-width: 0; padding: var(--s5); }
  .profile-strip { display: flex; align-items: center; gap: var(--s3); margin-bottom: var(--s4); flex-wrap: wrap; }
  .profile-strip > span { flex: 1; }
  .profile-strip input { width: 12rem; }
  .candidate { display: grid; grid-template-columns: 1.25rem minmax(10rem, 1fr) 6rem minmax(7rem, .8fr); align-items: center; gap: var(--s3); min-height: 3.25rem; padding: var(--s3) var(--s4); border-bottom: 1px solid var(--line-row); }
  .candidate:last-child { border-bottom: 0; }
  .candidate > span:nth-of-type(1) strong, .candidate > span:nth-of-type(1) small { display: block; }
  .candidate strong { font-size: .75rem; }
  .candidate small { color: var(--faint); font-size: .625rem; }
  .candidate code { color: var(--faint); font-family: var(--font-num); font-size: .625rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .candidate.unavailable { opacity: .65; }
  .recommended { color: var(--accent); font-size: .625rem; }
  .candidate .why { grid-column: 2 / -1; color: var(--warn); }
  .capabilities article { display: flex; align-items: center; gap: var(--s3); min-height: 3.75rem; padding: 0 var(--s4); border-bottom: 1px solid var(--line-row); }
  .capabilities article span { min-width: 0; flex: 1; }
  .capabilities strong, .capabilities small { display: block; }
  .capabilities small, .capabilities time { color: var(--faint); font-size: .6875rem; }
  .activity article { display: flex; gap: var(--s4); padding: var(--s4); border-bottom: 1px solid var(--line-row); }
  .activity article > i { width: .5rem; height: .5rem; margin-top: .25rem; border-radius: 50%; border: 1px solid var(--accent); }
  .activity strong, .activity small { display: block; }
  .activity strong { font-size: .75rem; }
  .activity small { color: var(--faint); font-size: .6875rem; }
  .review { max-width: 44rem; margin: var(--s7) auto; display: grid; grid-template-columns: 4rem minmax(0, 1fr); gap: var(--s5); }
  .review-mark { grid-row: span 2; width: 4rem; height: 4rem; display: grid; place-items: center; border: 1px solid var(--accent); border-radius: 50%; color: var(--accent); font-family: var(--font-num); font-size: 1.25rem; }
  .review dl { grid-column: 2; display: grid; grid-template-columns: repeat(3, 1fr); margin: 0; border: 1px solid var(--line-strong); border-radius: var(--r-l); }
  .review dl div { padding: var(--s4); border-right: 1px solid var(--line-row); }
  .review dt { color: var(--faint); font-size: .6875rem; }
  .review dd { margin: var(--s2) 0 0; font-family: var(--font-num); font-size: 1rem; }
  .retention { grid-column: 2; padding: var(--s4); border-left: 2px solid var(--accent); background: var(--surface); }
  .retention strong { font-size: .75rem; }
  .retention p { font-size: .6875rem; }
  @media (max-width: 48rem) { .scrim { padding: 0; } .indicator-modal, .indicator-modal.maximized { width: 100vw; max-width: none; height: 100dvh; max-height: none; border: 0; border-radius: 0; } .window-control { display: none; } .section-head { align-items: flex-start; flex-direction: column; } .scope-actions { width: 100%; flex-wrap: wrap; } .search { flex: 1; } .indicator-workbench { grid-template-columns: 8rem minmax(0, 1fr); } .scope-row { grid-template-columns: 2.5rem minmax(0, 1fr) 3rem; padding: var(--s3); } .scope-row select { grid-column: 2 / -1; width: 100%; } .candidate { grid-template-columns: 1.25rem minmax(0, 1fr); } .candidate .recommended, .candidate code { grid-column: 2; } .review { grid-template-columns: 1fr; margin: var(--s4) 0; } .review-mark { grid-row: auto; } .review dl, .retention { grid-column: 1; } }
</style>
