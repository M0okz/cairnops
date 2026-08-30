<script lang="ts">
  import { api, type OIDCConfiguration, type OIDCConfigurationSet } from '$lib/api';
  import { messageFrom, session } from '$lib/session.svelte';
  import { t } from '$lib/i18n.svelte';

  let configurations = $state<OIDCConfigurationSet>({ active: null, draft: null });
  let label = $state('');
  let issuer = $state('');
  let clientID = $state('');
  let clientSecret = $state('');
  let groupsClaim = $state('groups');
  let administratorGroups = $state('');
  let operatorGroups = $state('');
  let observerGroups = $state('');
  let loaded = $state(false);
  let dirty = $state(false);
  let saving = $state(false);
  let activating = $state(false);
  let error = $state('');

  const secretConfigured = $derived(
    configurations.draft?.client_secret_configured ??
      configurations.active?.client_secret_configured ??
      false
  );
  const testable = $derived(configurations.draft !== null && !dirty && !saving);
  const activatable = $derived(
    configurations.draft?.tested_at !== null &&
      configurations.draft?.tested_at !== undefined &&
      !dirty &&
      !saving &&
      !activating
  );

  $effect(() => {
    if (loaded) return;
    loaded = true;
    void load();
  });

  async function load() {
    try {
      configurations = await api<OIDCConfigurationSet>('/api/v1/oidc/configuration');
      fill(configurations.draft ?? configurations.active);
      const testResult = new URLSearchParams(location.search).get('oidc_test');
      if (testResult === 'success') {
        history.replaceState({}, '', location.pathname);
        session.showNotice(t('oidc.testPassed'));
      } else if (testResult === 'failed') {
        history.replaceState({}, '', location.pathname);
        error = t('oidc.testFailed');
      }
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  function fill(configuration: OIDCConfiguration | null) {
    if (!configuration) return;
    label = configuration.label;
    issuer = configuration.issuer;
    clientID = configuration.client_id;
    clientSecret = '';
    groupsClaim = configuration.groups_claim;
    administratorGroups = configuration.groups.administrator.join('\n');
    operatorGroups = configuration.groups.operator.join('\n');
    observerGroups = configuration.groups.observer.join('\n');
    dirty = false;
  }

  function lines(value: string) {
    return value
      .split('\n')
      .map((entry) => entry.trim())
      .filter(Boolean);
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    saving = true;
    error = '';
    try {
      const response = await api<{ draft: OIDCConfiguration }>('/api/v1/oidc/configuration', {
        method: 'PUT',
        body: JSON.stringify({
          label,
          issuer,
          client_id: clientID,
          client_secret: clientSecret,
          groups_claim: groupsClaim,
          groups: {
            administrator: lines(administratorGroups),
            operator: lines(operatorGroups),
            observer: lines(observerGroups)
          }
        })
      });
      configurations.draft = response.draft;
      clientSecret = '';
      dirty = false;
      session.showNotice(t('oidc.draftSaved'));
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      saving = false;
    }
  }

  async function activate() {
    if (!activatable) return;
    activating = true;
    error = '';
    try {
      const response = await api<{ active: OIDCConfiguration }>(
        '/api/v1/oidc/configuration/activation',
        { method: 'POST' }
      );
      configurations = { active: response.active, draft: null };
      fill(response.active);
      session.oidcEnabled = true;
      session.oidcLabel = response.active.label;
      session.showNotice(t('oidc.activated', { provider: response.active.label }));
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      activating = false;
    }
  }
</script>

<div class="band-row">
  <h2 class="band">{t('oidc.title')}</h2>
  <span class="band-note">{t('oidc.note')}</span>
</div>

<div class="card oidc-card">
  <div class="row">
    <span class="id">
      <strong>{t('oidc.activeConfiguration')}</strong>
      <small class="faint">{t('oidc.activeHint')}</small>
    </span>
    {#if configurations.active}
      <span class="means">
        <span class="pill ok">{configurations.active.label}</span>
        <span class="faint">{configurations.active.issuer}</span>
      </span>
    {:else}
      <span class="means faint">{t('oidc.notConfigured')}</span>
    {/if}
  </div>

  <form class="card-body" onsubmit={save} oninput={() => (dirty = true)}>
    <div class="oidc-heading">
      <div>
        <h3>{t('oidc.draft')}</h3>
        <p class="lead faint">{t('oidc.draftHint')}</p>
      </div>
      {#if configurations.draft?.tested_at}
        <span class="pill ok">{t('oidc.tested')}</span>
      {:else if configurations.draft}
        <span class="pill">{t('oidc.testRequired')}</span>
      {/if}
    </div>

    <div class="grid oidc-grid">
      <div class="field">
        <label for="oidc-label">{t('oidc.label')}</label>
        <input id="oidc-label" bind:value={label} required maxlength="80" autocomplete="off" placeholder={t('oidc.labelPlaceholder')} />
      </div>
      <div class="field span-two">
        <label for="oidc-issuer">{t('oidc.issuer')}</label>
        <input id="oidc-issuer" bind:value={issuer} required type="url" autocomplete="url" placeholder="https://auth.example.net/application/o/cairnops/" />
        <small>{t('oidc.issuerHint')}</small>
      </div>
      <div class="field">
        <label for="oidc-client-id">{t('oidc.clientID')}</label>
        <input id="oidc-client-id" bind:value={clientID} required maxlength="255" autocomplete="off" spellcheck="false" />
      </div>
      <div class="field">
        <label for="oidc-client-secret">{t('oidc.clientSecret')}</label>
        <input id="oidc-client-secret" bind:value={clientSecret} required={!secretConfigured} type="password" autocomplete="new-password" maxlength="4096" />
        <small>{secretConfigured ? t('oidc.secretKept') : t('oidc.secretRequired')}</small>
      </div>
      <div class="field">
        <label for="oidc-groups-claim">{t('oidc.groupsClaim')}</label>
        <input id="oidc-groups-claim" bind:value={groupsClaim} required maxlength="128" autocomplete="off" spellcheck="false" />
        <small>{t('oidc.groupsClaimHint')}</small>
      </div>
    </div>

    <fieldset>
      <legend>{t('oidc.groupMappings')}</legend>
      <p class="lead faint">{t('oidc.groupMappingsHint')}</p>
      <div class="grid">
        <div class="field">
          <label for="oidc-admin-groups">{t('role.administrator')}</label>
          <textarea id="oidc-admin-groups" bind:value={administratorGroups} rows="4" spellcheck="false"></textarea>
        </div>
        <div class="field">
          <label for="oidc-operator-groups">{t('role.operator')}</label>
          <textarea id="oidc-operator-groups" bind:value={operatorGroups} rows="4" spellcheck="false"></textarea>
        </div>
        <div class="field">
          <label for="oidc-observer-groups">{t('role.observer')}</label>
          <textarea id="oidc-observer-groups" bind:value={observerGroups} rows="4" spellcheck="false"></textarea>
        </div>
      </div>
      <small>{t('oidc.oneGroupPerLine')}</small>
    </fieldset>

    {#if error}<p class="error" role="alert">{error}</p>{/if}

    <div class="oidc-actions">
      <button class="btn primary" type="submit" disabled={saving}>
        {saving ? t('oidc.saving') : t('oidc.saveDraft')}
      </button>
      {#if testable}
        <a class="btn" href="/api/v1/oidc/configuration/test">{t('oidc.testDraft')}</a>
      {:else}
        <button class="btn" type="button" disabled>{t('oidc.testDraft')}</button>
      {/if}
      <button class="btn" type="button" disabled={!activatable} onclick={activate}>
        {activating ? t('oidc.activating') : t('oidc.activate')}
      </button>
      {#if dirty}<span class="faint">{t('oidc.saveBeforeTest')}</span>{/if}
    </div>
  </form>
</div>

<style>
  .oidc-card .means {
    overflow-wrap: anywhere;
  }

  .oidc-heading,
  .oidc-actions {
    display: flex;
    align-items: center;
    gap: var(--s3);
  }

  .oidc-heading {
    justify-content: space-between;
  }

  .oidc-heading h3,
  .oidc-heading p {
    margin: 0;
  }

  .oidc-grid .span-two {
    grid-column: span 2;
  }

  fieldset {
    min-width: 0;
    margin: var(--s6) 0 0;
    padding: 0;
    border: 0;
  }

  legend {
    padding: 0;
    color: var(--ink);
    font-size: 0.8125rem;
    font-weight: 600;
  }

  textarea {
    min-height: 6rem;
    resize: vertical;
  }

  .oidc-actions {
    flex-wrap: wrap;
    margin-top: var(--s6);
  }

  @media (max-width: 48rem) {
    .oidc-grid .span-two {
      grid-column: auto;
    }

    .oidc-actions {
      align-items: stretch;
      flex-direction: column;
    }

    .oidc-actions .btn {
      justify-content: center;
    }
  }
</style>
