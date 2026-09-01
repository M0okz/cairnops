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
  const activatable = $derived(
    configurations.draft?.tested_at !== null &&
      configurations.draft?.tested_at !== undefined &&
      !dirty &&
      !saving &&
      !activating
  );
  const needsConfiguration = $derived(
    dirty || (configurations.active === null && configurations.draft === null)
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

  async function saveAndTest(event: SubmitEvent) {
    event.preventDefault();
    if (!needsConfiguration) return;
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
      location.assign('/api/v1/oidc/configuration/test');
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

<section class="oidc-panel" aria-labelledby="oidc-title">
  <header class="panel-heading">
    <div>
      <h2 id="oidc-title">{t('oidc.title')}</h2>
      <p>{t('oidc.note')}</p>
    </div>
  </header>

  <div class="card oidc-card">
    <div class="status-bar">
      <span class:ok={configurations.active} class:idle={!configurations.active} class="status-mark" aria-hidden="true"></span>
      {#if configurations.active}
        <span class="status-copy">
          <strong>{t('oidc.activeProvider', { provider: configurations.active.label })}</strong>
          <small>{configurations.active.issuer}</small>
        </span>
        <span class="pill ok">{t('oidc.active')}</span>
      {:else}
        <span class="status-copy">
          <strong>{t('oidc.inactive')}</strong>
          <small>{t('oidc.inactiveHint')}</small>
        </span>
        <span class="pill">{t('oidc.toConfigure')}</span>
      {/if}
    </div>

    <form onsubmit={saveAndTest} oninput={() => (dirty = true)}>
      <section class="form-section provider-section" aria-labelledby="oidc-provider-title">
        <div class="section-heading">
          <div>
            <h3 id="oidc-provider-title">{t('oidc.provider')}</h3>
            <p>{t('oidc.providerHint')}</p>
          </div>
        </div>

        <div class="provider-grid">
          <div class="field">
            <label for="oidc-label">{t('oidc.label')}</label>
            <input id="oidc-label" bind:value={label} required maxlength="80" autocomplete="off" placeholder={t('oidc.labelPlaceholder')} />
          </div>
          <div class="field">
            <label for="oidc-client-id">{t('oidc.clientID')}</label>
            <input id="oidc-client-id" bind:value={clientID} required maxlength="255" autocomplete="off" spellcheck="false" />
          </div>
          <div class="field full">
            <label for="oidc-issuer">{t('oidc.issuer')}</label>
            <input id="oidc-issuer" bind:value={issuer} required type="url" inputmode="url" autocomplete="url" aria-describedby="oidc-issuer-hint" placeholder="https://auth.example.net/application/o/cairnops/" />
            <small id="oidc-issuer-hint">{t('oidc.issuerHint')}</small>
          </div>
          <div class="field full">
            <label for="oidc-client-secret">{t('oidc.clientSecret')}</label>
            <input id="oidc-client-secret" bind:value={clientSecret} required={!secretConfigured} type="password" autocomplete="new-password" aria-describedby="oidc-secret-hint" maxlength="4096" />
            <small id="oidc-secret-hint">{secretConfigured ? t('oidc.secretKept') : t('oidc.secretRequired')}</small>
          </div>
        </div>

        <details class="advanced-options">
          <summary>{t('oidc.advancedOptions')}</summary>
          <div class="advanced-body">
            <div class="field">
              <label for="oidc-groups-claim">{t('oidc.groupsClaim')}</label>
              <input id="oidc-groups-claim" bind:value={groupsClaim} required maxlength="128" autocomplete="off" spellcheck="false" aria-describedby="oidc-groups-claim-hint" />
              <small id="oidc-groups-claim-hint">{t('oidc.groupsClaimHint')}</small>
            </div>
          </div>
        </details>
      </section>

      <section class="form-section access-section" aria-labelledby="oidc-groups-title">
        <div class="section-heading">
          <div>
            <h3 id="oidc-groups-title">{t('oidc.groupMappings')}</h3>
            <p>{t('oidc.groupMappingsHint')}</p>
          </div>
          <small>{t('oidc.oneGroupPerLine')}</small>
        </div>

        <div class="role-grid">
          <div class="field role-card">
            <label for="oidc-admin-groups">{t('role.administrator')}</label>
            <small>{t('oidc.administratorHint')}</small>
            <textarea id="oidc-admin-groups" bind:value={administratorGroups} rows="2" spellcheck="false" placeholder={t('oidc.administratorPlaceholder')}></textarea>
          </div>
          <div class="field role-card">
            <label for="oidc-operator-groups">{t('role.operator')}</label>
            <small>{t('oidc.operatorHint')}</small>
            <textarea id="oidc-operator-groups" bind:value={operatorGroups} rows="2" spellcheck="false" placeholder={t('oidc.operatorPlaceholder')}></textarea>
          </div>
          <div class="field role-card">
            <label for="oidc-observer-groups">{t('role.observer')}</label>
            <small>{t('oidc.observerHint')}</small>
            <textarea id="oidc-observer-groups" bind:value={observerGroups} rows="2" spellcheck="false" placeholder={t('oidc.observerPlaceholder')}></textarea>
          </div>
        </div>
      </section>

      {#if error}<p class="error" role="alert">{error}</p>{/if}

      <footer class="flow-footer">
        <span class="flow-copy">
          {#if needsConfiguration}
            <strong>{t('oidc.stepConfigure')}</strong>
            <small>{t('oidc.stepConfigureHint')}</small>
          {:else if activatable}
            <strong>{t('oidc.stepActivate')}</strong>
            <small>{t('oidc.stepActivateHint')}</small>
          {:else if configurations.draft}
            <strong>{t('oidc.stepTest')}</strong>
            <small>{t('oidc.stepTestHint')}</small>
          {:else}
            <strong>{t('oidc.upToDate')}</strong>
            <small>{t('oidc.upToDateHint')}</small>
          {/if}
        </span>

        {#if needsConfiguration}
          <button class="btn primary" type="submit" disabled={saving}>
            {saving ? t('oidc.saving') : t('oidc.saveAndTest')}
          </button>
        {:else if activatable}
          <button class="btn primary" type="button" disabled={activating} onclick={activate}>
            {activating ? t('oidc.activating') : configurations.active ? t('oidc.applyChanges') : t('oidc.activate')}
          </button>
        {:else if configurations.draft}
          <a class="btn primary" href="/api/v1/oidc/configuration/test">{t('oidc.testConnection')}</a>
        {/if}
      </footer>
    </form>
  </div>
</section>

<style>
  .oidc-panel {
    margin-top: var(--s6);
  }

  .panel-heading {
    display: flex;
    align-items: end;
    justify-content: space-between;
    gap: var(--s4);
    margin-bottom: var(--s4);
  }

  .panel-heading h2,
  .panel-heading p,
  .section-heading h3,
  .section-heading p {
    margin: 0;
  }

  .panel-heading h2 {
    font-size: 0.9375rem;
    font-weight: var(--weight-semibold);
  }

  .panel-heading p,
  .section-heading p {
    margin-top: var(--s2);
    color: var(--faint);
    font-size: var(--text-sm);
    line-height: 1.5;
  }

  .status-bar {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--s3);
    padding: var(--s4) var(--s5);
    border-bottom: 1px solid var(--line);
    background: var(--bg);
  }

  .status-mark {
    width: var(--s3);
    height: var(--s3);
    border-radius: var(--r-pill);
  }

  .status-mark.ok {
    background: var(--ok);
  }

  .status-mark.idle {
    background: var(--dim);
  }

  .status-copy,
  .flow-copy {
    display: grid;
    min-width: 0;
    gap: var(--s1);
  }

  .status-copy strong,
  .flow-copy strong {
    color: var(--ink);
    font-size: var(--text-md);
    font-weight: var(--weight-semibold);
  }

  .status-copy small,
  .flow-copy small {
    overflow-wrap: anywhere;
    color: var(--faint);
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  .status-copy small {
    font-family: var(--font-num);
  }

  .form-section {
    padding: var(--s5);
  }

  .form-section + .form-section {
    border-top: 1px solid var(--line);
  }

  .section-heading {
    display: flex;
    align-items: start;
    justify-content: space-between;
    gap: var(--s5);
    margin-bottom: var(--s4);
  }

  .section-heading h3 {
    color: var(--ink);
    font-size: var(--text-md);
    font-weight: var(--weight-semibold);
  }

  .section-heading > small {
    max-width: 24rem;
    color: var(--faint);
    font-size: var(--text-xs);
    line-height: 1.5;
    text-align: end;
  }

  .provider-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--s4);
  }

  .provider-grid .field,
  .role-grid .field,
  .advanced-body .field {
    align-content: start;
    margin: 0;
  }

  .provider-grid .full {
    grid-column: 1 / -1;
  }

  .advanced-options {
    margin-top: var(--s4);
    border: 1px solid var(--line);
    border-radius: var(--r-m);
    background: var(--bg);
  }

  .advanced-options summary {
    display: flex;
    align-items: center;
    gap: var(--s3);
    min-height: calc(var(--ctl-h-lg) + var(--s3));
    padding-inline: var(--s4);
    color: var(--muted);
    cursor: pointer;
    font-size: var(--text-sm);
    font-weight: var(--weight-medium);
    list-style: none;
  }

  .advanced-options summary::-webkit-details-marker {
    display: none;
  }

  .advanced-options summary::before {
    content: '›';
    color: var(--faint);
    font-size: var(--text-md);
    transform: rotate(0deg);
    transition: transform var(--d1) var(--ease);
  }

  .advanced-options[open] summary {
    border-bottom: 1px solid var(--line);
    color: var(--ink);
  }

  .advanced-options[open] summary::before {
    transform: rotate(90deg);
  }

  .advanced-body {
    padding: var(--s4);
  }

  .role-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--s4);
  }

  .role-card {
    padding: var(--s4);
    border: 1px solid var(--line);
    border-radius: var(--r-m);
    background: var(--bg);
  }

  .role-card label {
    color: var(--ink);
    font-size: var(--text-md);
    font-weight: var(--weight-semibold);
  }

  .role-card textarea {
    min-height: 4rem;
    margin-top: var(--s2);
    background: var(--surface);
    font-family: var(--font-num);
  }

  .error {
    margin: 0 var(--s5) var(--s5);
  }

  .flow-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s5);
    padding: var(--s4) var(--s5);
    border-top: 1px solid var(--line);
    background: var(--bg);
  }

  .flow-footer .btn {
    flex: none;
    justify-content: center;
  }

  @media (max-width: 48rem) {
    .status-bar {
      grid-template-columns: auto minmax(0, 1fr);
    }

    .status-bar .pill {
      grid-column: 2;
      justify-self: start;
    }

    .provider-grid,
    .role-grid {
      grid-template-columns: minmax(0, 1fr);
    }

    .provider-grid .full {
      grid-column: auto;
    }

    .section-heading,
    .flow-footer {
      align-items: stretch;
      flex-direction: column;
      gap: var(--s3);
    }

    .section-heading > small {
      max-width: none;
      text-align: start;
    }

    .flow-footer .btn {
      width: 100%;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .advanced-options summary::before {
      transition: none;
    }
  }
</style>
