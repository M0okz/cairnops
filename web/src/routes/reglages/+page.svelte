<script lang="ts">
  import { appearance } from '$lib/appearance.svelte';
  import { solarCities, themeModes } from '$lib/appearance';
  import type { IconName } from '$lib/components/Icon.svelte';
  import { localeTag } from '$lib/i18n.svelte';
  import { onMount } from 'svelte';
  /* Réglages.
   * Les Écrans placent « Réglages » dans la navigation et font passer les
   * Connecteurs par lui. S'y ajoutent les gestes qui portent sur les comptes,
   * pour qu'aucun ne réclame d'aller dans le conteneur. */

  import Topbar from '$lib/components/Topbar.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import AccountCreation from '$lib/components/AccountCreation.svelte';
  import AccountDeactivation from '$lib/components/AccountDeactivation.svelte';
  import DeviceManagement from '$lib/components/DeviceManagement.svelte';
  import SoftwareAISettings from '$lib/components/SoftwareAISettings.svelte';
  import OIDCSettings from '$lib/components/OIDCSettings.svelte';
  import { session, messageFrom } from '$lib/session.svelte';
  import { api, type Account, type Role } from '$lib/api';
  import { i18n, locales, plural, t } from '$lib/i18n.svelte';
  import { since } from '$lib/format';

  const roleLabels = $derived<Record<string, string>>({
    administrator: t('role.administrator'),
    operator: t('role.operator'),
    observer: t('role.observer')
  });

  const roleOrder: Role[] = ['observer', 'operator', 'administrator'];

  const initials = (name: string) => name.slice(0, 2).toLocaleUpperCase(i18n.locale);
  const themeIcons: Record<string, IconName> = { light: 'sun', dark: 'moon', system: 'devices', solar: 'activity' };
  const solarTime = (date: Date | null) => date
    ? new Intl.DateTimeFormat(localeTag(), { hour: '2-digit', minute: '2-digit' }).format(date)
    : '—';
  let passwordOpen = $state(false);
  let activeSection = $state('general');
  onMount(() => {
    const update = () => (activeSection = location.hash.slice(1) || 'general');
    update();
    window.addEventListener('hashchange', update);
    return () => window.removeEventListener('hashchange', update);
  });

  function exportConfiguration() {
    const configuration = {
      instance: { name: session.instanceName, version: session.version },
      appearance: { mode: appearance.mode, city: appearance.cityName, language: i18n.locale },
      connectors: session.connectors.map(({ kind, name, status }) => ({ kind, name, status }))
    };
    const url = URL.createObjectURL(new Blob([JSON.stringify(configuration, null, 2)], { type: 'application/json' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = 'cairnops-configuration.json';
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  /* La fraîcheur s'égrène comme dans le rail : « 8 s » ne vaut que s'il compte. */
  let now = $state(new Date());
  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 1000);
    return () => clearInterval(timer);
  });

  const freshness = $derived.by(() => {
    if (session.realtime !== 'online' || !session.lastEventAt) return t('common.none');
    return since(session.lastEventAt, now);
  });

  /* ── Le nom de l'instance ─────────────────────────────────────────────── */

  /* Le champ ne suit pas l'état partagé en continu : on le remplit à l'ouverture
   * et on ne le renvoie qu'une fois validé, sinon une frappe en cours serait
   * réécrite par un renommage venu d'ailleurs. */
  let instanceName = $state(session.instanceName);
  let renaming = $state(false);
  let renameError = $state('');

  const renameable = $derived(
    instanceName.trim().length > 0 &&
      instanceName.trim().length <= 80 &&
      instanceName.trim() !== session.instanceName
  );

  async function rename(event: SubmitEvent) {
    event.preventDefault();
    if (!renameable) return;
    renaming = true;
    renameError = '';
    try {
      const name = await session.renameInstance(instanceName.trim());
      instanceName = name;
      session.showNotice(t('settings.instanceRenamed', { name }));
    } catch (cause) {
      renameError = messageFrom(cause);
    } finally {
      renaming = false;
    }
  }

  /* ── Changer son propre mot de passe ──────────────────────────────────── */

  let current = $state('');
  let next = $state('');
  let confirmation = $state('');
  let changing = $state(false);
  let changeError = $state('');

  const canSubmitChange = $derived(
    current.length > 0 && next.length >= 12 && next === confirmation && next !== current
  );

  async function changePassword(event: SubmitEvent) {
    event.preventDefault();
    changeError = '';
    if (next !== confirmation) {
      changeError = t('settings.mismatch');
      return;
    }
    changing = true;
    try {
      await api<{ status: string; session: string }>('/api/v1/session/password', {
        method: 'PUT',
        body: JSON.stringify({ current_password: current, new_password: next })
      });
      current = next = confirmation = '';
      session.showNotice(t('settings.passwordChanged'));
    } catch (cause) {
      changeError = messageFrom(cause);
    } finally {
      changing = false;
    }
  }

  /* ── Les comptes de l'instance ─────────────────────────────────────────── */

  let users = $state<Account[]>([]);
  let usersError = $state('');
  let loaded = $state(false);

  const isAdministrator = $derived(session.user?.role === 'administrator');
  const isLocalAdministrator = $derived(
    isAdministrator && session.user?.authorization_regime === 'local'
  );

  $effect(() => {
    if (!isAdministrator || loaded) return;
    loaded = true;
    void reload();
  });

  async function reload() {
    try {
      users = (await api<{ users: Account[] }>('/api/v1/users')).users;
      usersError = '';
    } catch (cause) {
      usersError = messageFrom(cause);
    }
  }

  /* La règle se compose ici : deux fragments séparés dans le balisage voient
   * leur espace avalé au rendu, et les phrases se recollent. */
  const rule = $derived(
    users.length === 1
      ? `${t('settings.soleAccount')} ${t('settings.adminRule')}`
      : t('settings.adminRule')
  );

  /* Un compte revenu du serveur remplace le sien dans la liste : c'est la
   * réponse qui fait foi, pas ce que l'écran croyait avoir demandé. */
  function replace(account: Account) {
    users = users.map((user) => (user.id === account.id ? account : user));
  }

  /* ── Ouvrir un compte ──────────────────────────────────────────────────── */

  let creating = $state(false);
  let createError = $state('');

  async function createAccount(input: {
    username: string;
    display_name: string;
    role: Role;
    password: string;
  }) {
    createError = '';
    try {
      const { user } = await api<{ user: Account }>('/api/v1/users', {
        method: 'POST',
        body: JSON.stringify(input)
      });
      users = [...users, user];
      creating = false;
      session.showNotice(t('settings.accountOpened', { username: user.username }));
    } catch (cause) {
      createError = messageFrom(cause);
    }
  }

  /* ── Changer un rôle ───────────────────────────────────────────────────── */

  let pendingRole = $state('');

  async function changeRole(account: Account, role: Role) {
    if (role === account.role) return;
    pendingRole = account.id;
    try {
      const { user } = await api<{ user: Account }>(`/api/v1/users/${account.id}`, {
        method: 'PATCH',
        body: JSON.stringify({ role })
      });
      replace(user);
      session.showNotice(
        t('settings.roleChanged', { name: user.display_name, role: roleLabels[user.role] })
      );
    } catch (cause) {
      /* Le <select> affiche déjà le rôle refusé : la liste relue le remet
       * d'accord avec l'instance. */
      await reload();
      session.showNotice(messageFrom(cause));
    } finally {
      pendingRole = '';
    }
  }

  /* ── Désactiver, réactiver ─────────────────────────────────────────────── */

  let deactivating = $state<Account | null>(null);
  let deactivateError = $state('');

  async function deactivate() {
    if (!deactivating) return;
    deactivateError = '';
    try {
      const { user } = await api<{ user: Account }>(
        `/api/v1/users/${deactivating.id}/deactivation`,
        { method: 'POST' }
      );
      replace(user);
      deactivating = null;
      session.showNotice(t('settings.deactivated', { name: user.display_name }));
    } catch (cause) {
      deactivateError = messageFrom(cause);
    }
  }

  async function reactivate(account: Account) {
    try {
      const { user } = await api<{ user: Account }>(`/api/v1/users/${account.id}/deactivation`, {
        method: 'DELETE'
      });
      replace(user);
      session.showNotice(t('settings.reactivated', { name: user.display_name }));
    } catch (cause) {
      session.showNotice(messageFrom(cause));
    }
  }

  /* ── Réinitialiser le compte d'un tiers ───────────────────────────────── */

  let resetFor = $state<Account | null>(null);
  let resetPassword = $state('');
  let resetting = $state(false);
  let resetError = $state('');

  async function submitReset(event: SubmitEvent) {
    event.preventDefault();
    if (!resetFor) return;
    resetError = '';
    resetting = true;
    try {
      await api<{ user: Account }>(`/api/v1/users/${resetFor.id}/password`, {
        method: 'POST',
        body: JSON.stringify({ new_password: resetPassword })
      });
      session.showNotice(t('settings.passwordReset', { name: resetFor.display_name }));
      resetFor = null;
      resetPassword = '';
    } catch (cause) {
      resetError = messageFrom(cause);
    } finally {
      resetting = false;
    }
  }

  /* Une suggestion, pas une obligation : la personne reste libre de sa saisie. */
  function suggest() {
    const bytes = crypto.getRandomValues(new Uint8Array(18));
    resetPassword = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 20);
  }
</script>

<svelte:head><title>{t('nav.settings')} — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.settings') }]} />

<div class="page">
  <div class="page-head">
    <div>
      <h1>{t('nav.settings')}</h1>
      <p>{t('settings.lead')}</p>
    </div>
  </div>
  <nav class="settings-tabs" aria-label={t('settings.sections')}>
    <a href="#general" class:active={activeSection === 'general'} aria-current={activeSection === 'general' ? 'location' : undefined}><Icon name="settings" size={14} />{t('settings.general')}</a>
    <a href="#connectors" class:active={activeSection === 'connectors'} aria-current={activeSection === 'connectors' ? 'location' : undefined}><Icon name="connectors" size={14} />{t('nav.connectors')}</a>
    <a href="#account" class:active={activeSection === 'account'} aria-current={activeSection === 'account' ? 'location' : undefined}><Icon name="user" size={14} />{t('settings.yourAccount')}</a>
    <a href="#devices" class:active={activeSection === 'devices'} aria-current={activeSection === 'devices' ? 'location' : undefined}><Icon name="devices" size={14} />{t('devices.title')}</a>
    {#if isAdministrator}<a href="#ai" class:active={activeSection === 'ai'} aria-current={activeSection === 'ai' ? 'location' : undefined}><Icon name="activity" size={14} />{t('settings.aiTab')}</a>{/if}
    {#if isLocalAdministrator}<a href="#advanced" class:active={activeSection === 'advanced'} aria-current={activeSection === 'advanced' ? 'location' : undefined}><Icon name="health" size={14} />{t('settings.advanced')}</a>{/if}
  </nav>

  <div class="settings-layout">
  <section id="general" class="card general-card" aria-labelledby="general-title">
    <header class="settings-card-head">
      <span class="settings-icon"><Icon name="settings" size={18} /></span>
      <span><h2 id="general-title">{t('settings.general')}</h2><small>{t('settings.generalHint')}</small></span>
    </header>
    <!-- Le nom de l'instance ouvre la section : c'est le seul réglage de cette
         carte qui vaut pour tout le monde, les autres n'engagent que l'appareil
         devant lequel on est assis. -->
    <div class="row">
      <span class="id">
        <strong>{t('settings.instanceName')}</strong>
        <small class="faint">{t('settings.instanceNameHint')}</small>
      </span>
      {#if isAdministrator}
        <!-- `field` pour que le champ soit celui de toute l'application : même
             hauteur, même bordure, même mise au point. -->
        <form class="act field rename" onsubmit={rename}>
          <label class="visually-hidden" for="instance-name">{t('settings.instanceName')}</label>
          <input
            id="instance-name"
            bind:value={instanceName}
            maxlength="80"
            autocomplete="off"
            placeholder={t('settings.instanceNamePlaceholder')}
          />
          <button class="btn sm" type="submit" disabled={!renameable || renaming}>
            {renaming ? t('settings.renaming') : t('common.save')}
          </button>
        </form>
      {:else}
        <span class="means">{session.instanceLabel}</span>
      {/if}
    </div>
    {#if renameError}<p class="error" role="alert">{renameError}</p>{/if}

    <div class="row">
      <span class="id"><strong>{t('appearance.title')}</strong><small class="faint">{t('appearance.deviceHint')}</small></span>
      <div class="setting-value appearance-options" role="group" aria-label={t('rail.theme')}>
        {#each themeModes as mode}
          <button type="button" aria-pressed={appearance.mode === mode} onclick={() => appearance.choose(mode)}>
            <Icon name={themeIcons[mode]} size={16} /><span>{t(`appearance.${mode}`)}</span>
          </button>
        {/each}
      </div>
    </div>

    <!-- La langue se choisit ici comme dans le menu du rail : c'est le même
         réglage, et il n'appartient qu'à cet appareil. -->
    <div class="row">
      <span class="id">
        <strong>{t('rail.language')}</strong>
        <small class="faint">{t('settings.languageShortHint')}</small>
      </span>
      <div class="setting-value field select-field">
        <label class="visually-hidden" for="settings-language">{t('rail.language')}</label>
        <select id="settings-language" value={i18n.locale} onchange={(event) => i18n.choose(event.currentTarget.value as typeof i18n.locale)}>
          {#each locales as choice}<option value={choice.value}>{choice.label}</option>{/each}
        </select>
        <small>{t('settings.translationHint')}</small>
      </div>
    </div>
    <div class="row">
      <span class="id"><strong>{t('appearance.city')}</strong><small class="faint">{t('settings.cityHint')}</small></span>
      <div class="setting-value field select-field">
        <label class="visually-hidden" for="settings-city">{t('appearance.city')}</label>
        <select id="settings-city" value={appearance.cityName} onchange={(event) => appearance.chooseCity(event.currentTarget.value)}>
          <option value="">{t('appearance.chooseCity')}</option>
          {#each solarCities as city}<option value={city.name}>{city.name}</option>{/each}
        </select>
        {#if appearance.solar}
          <small>{t('appearance.sunrise')} {solarTime(appearance.solar.sunrise)} &nbsp; · &nbsp; {t('appearance.sunset')} {solarTime(appearance.solar.sunset)} &nbsp; · &nbsp; {appearance.timeZone}</small>
        {/if}
      </div>
    </div>
  </section>

  <aside class="settings-aside card" aria-label={t('settings.about')}>
    <div class="aside-block"><span class="settings-icon"><Icon name="health" size={16} /></span><div><strong>{t('settings.about')}</strong><p>{t('settings.workspaceNote')}</p></div></div>
    <div class="aside-block"><span class="settings-icon"><Icon name="server" size={16} /></span><div><strong>{t('health.instance')}</strong><p><span class="dot {session.health === 'ready' ? 'ok' : 'idle'}"></span> v{session.version}</p><small>{t('settings.lastSignal')} {freshness}</small></div></div>
    <div class="aside-block"><span class="settings-icon"><Icon name="book" size={16} /></span><div><strong>{t('settings.help')}</strong><p>{t('settings.helpHint')}</p><a class="btn sm" href="https://github.com/M0okz/cairnops#readme" target="_blank" rel="noopener noreferrer">{t('settings.documentation')} ↗</a></div></div>
    <div class="aside-block"><span class="settings-icon"><Icon name="changelog" size={16} /></span><div><strong>{t('settings.export')}</strong><button class="btn sm" type="button" onclick={exportConfiguration}>{t('settings.exportJSON')}</button></div></div>
  </aside>

  <section id="connectors" class="card status-card" aria-labelledby="connectors-title">
    <header class="settings-card-head"><span class="settings-icon"><Icon name="connectors" size={18} /></span><span><h2 id="connectors-title">{t('settings.statusConnectors')}</h2><small>{t('settings.statusConnectorsHint')}</small></span></header>
    <div class="status-grid">
      <div><strong>{t('settings.version')}</strong><span class="num"><span class="dot {session.health === 'ready' ? 'ok' : 'idle'}"></span> v{session.version}</span></div>
      <div><strong>{t('settings.realtime')}</strong><span>{session.realtime === 'online' ? t('settings.realtimeOn') : t('settings.realtimeOff')}</span><small>{t('settings.lastSignal')} {freshness}</small></div>
      <div><strong>{t('settings.activeConnectors')}</strong><span class="num">{session.connectors.filter((connector) => connector.status === 'connected').length} / {session.connectors.length}</span></div>
      <a class="btn sm" href="/connecteurs">{t('settings.manageConnectors')} →</a>
    </div>
  </section>

  <section id="account" class="card account-card" aria-labelledby="account-title">
    <header class="settings-card-head"><span class="settings-icon"><Icon name="user" size={18} /></span><span><h2 id="account-title">{t('settings.yourAccount')}</h2><small>{t('settings.accountHint')}</small></span></header>
    <div class="row">
      <span class="id who">
        <span class="avatar">{initials(session.user?.display_name ?? '')}</span>
        <span>
          <strong>{session.user?.display_name ?? t('common.none')}</strong>
          <small class="faint">
            {session.user?.username ?? ''}
            {#if session.user}· {roleLabels[session.user.role] ?? session.user.role}{/if}
          </small>
        </span>
      </span>
      <span class="means faint">
        {t('settings.sessionHere')}
        {#if session.activeSessions > 1}
          · {plural('settings.sessionsActive', session.activeSessions)}
        {/if}
      </span>
      <button class="act btn sm" type="button" onclick={() => session.logout()}>{t('rail.logout')}</button>
    </div>

    {#if session.user?.authorization_regime === 'local'}
    <div class="password-toggle">
      <span class="settings-icon"><Icon name="health" size={16} /></span>
      <span><strong>{t('settings.changePassword')}</strong><small>{t('settings.changePasswordHint')}</small></span>
      <button class="btn sm" type="button" aria-expanded={passwordOpen} onclick={() => (passwordOpen = !passwordOpen)}>{passwordOpen ? t('common.close') : t('settings.edit')} →</button>
    </div>
    {#if passwordOpen}<form class="card-body password-form" onsubmit={changePassword}>

      <div class="grid">
        <div class="field">
          <label for="current">{t('settings.currentPassword')}</label>
          <input id="current" bind:value={current} type="password" autocomplete="current-password" required />
        </div>
        <div class="field">
          <label for="next">{t('settings.newPassword')}</label>
          <input id="next" bind:value={next} type="password" autocomplete="new-password" required minlength="12" maxlength="128" />
          <small>{t('settings.passwordBounds')}</small>
        </div>
        <div class="field">
          <label for="confirm">{t('settings.confirm')}</label>
          <input id="confirm" bind:value={confirmation} type="password" autocomplete="new-password" required minlength="12" maxlength="128" />
        </div>
      </div>

      {#if changeError}<p class="error" role="alert">{changeError}</p>{/if}

      <!-- Le bouton reste sous la première colonne, et ce qui lui manque se dit
           à côté de lui plutôt qu'à l'autre bout de la dalle. -->
      <div class="submit">
        <button class="btn primary" type="submit" disabled={changing || !canSubmitChange}>
          {changing ? t('settings.replacing') : t('settings.replacePassword')}
        </button>
        {#if !canSubmitChange && !changing}
          <span class="faint">{t('settings.submitHint')}</span>
        {/if}
      </div>
    </form>{/if}
    {:else}
      <div class="card-body">
        <h3>{t('settings.externalAccount')}</h3>
        <p class="lead faint">{t('settings.externalAccountHint')}</p>
      </div>
    {/if}
  </section>

  <div id="devices" class="settings-section"><DeviceManagement /></div>
  {#if isAdministrator}<div id="ai" class="settings-section"><SoftwareAISettings/></div>{/if}

  {#if isAdministrator}
    {#if isLocalAdministrator}
      <div id="advanced" class="settings-section"><OIDCSettings /></div>
    {/if}

    <section id="accounts" class="card accounts" aria-labelledby="accounts-title">
      <header class="settings-card-head"><span class="settings-icon"><Icon name="user" size={18} /></span><span><h2 id="accounts-title">{t('settings.accounts')}</h2><small>{t('settings.accountsHint')}</small></span>{#if isLocalAdministrator}<button class="btn sm account-create" type="button" onclick={() => { creating = true; createError = ''; }}>{t('settings.openAccount')}</button>{/if}</header>
      {#if usersError}
        <div class="empty">
          <strong>{t('settings.accountsUnread')}</strong>
          {usersError}
        </div>
      {:else}
        <div class="thead">
          <span>{t('settings.columnAccount')}</span>
          <span>{t('settings.columnRole')}</span>
          <span>{t('settings.columnLastSeen')}</span>
          <span>{t('settings.columnSessions')}</span>
          <span></span>
        </div>

        {#each users as user (user.id)}
          {@const self = user.id === session.user?.id}
          {@const off = user.deactivated_at !== null}
          {@const suspended = user.external_suspended_at !== null}
          <div class="trow" class:off>
            <span class="id who">
              <span class="avatar">{initials(user.display_name)}</span>
              <span>
                <strong>{user.display_name}</strong>
                <small class="faint">{user.username}</small>
              </span>
            </span>

            <!-- Son propre rôle se lit, il ne se choisit pas : l'instance
                 refuserait le geste, autant ne pas l'offrir. -->
            {#if off}
              <span class="pill">{t('settings.accessWithdrawn')}</span>
            {:else if suspended}
              <span class="pill warn">{t('settings.externalSuspended')}</span>
            {:else if user.authorization_regime === 'external'}
              <span class="pill">{roleLabels[user.role]}</span>
            {:else if !isLocalAdministrator}
              <span class="pill">{roleLabels[user.role]}</span>
            {:else if self}
              <span class="pill">{roleLabels[user.role]}</span>
            {:else}
              <label class="role">
                <span class="visually-hidden">{t('settings.roleOf', { name: user.display_name })}</span>
                <select
                  value={user.role}
                  disabled={pendingRole === user.id}
                  onchange={(event) => changeRole(user, event.currentTarget.value as Role)}
                >
                  {#each roleOrder as role (role)}
                    <option value={role}>{roleLabels[role]}</option>
                  {/each}
                </select>
              </label>
            {/if}

            <span class="num faint">
              {user.last_seen_at ? since(user.last_seen_at, now) : t('settings.neverSeen')}
            </span>
            <span class="num faint"><Odometer value={user.active_sessions} /></span>

            <span class="actions">
              {#if self}
                <span class="faint self">{t('common.you')}</span>
              {:else if isLocalAdministrator}
                {#if user.authorization_regime === 'local'}
                  <button
                    class="btn sm"
                    type="button"
                    onclick={() => { resetFor = user; resetPassword = ''; resetError = ''; }}
                  >
                    {t('settings.reset')}
                  </button>
                {/if}
                {#if off}
                  <button class="btn sm" type="button" onclick={() => reactivate(user)}>
                    {t('settings.reactivate')}
                  </button>
                {:else}
                  <button
                    class="btn sm"
                    type="button"
                    onclick={() => { deactivating = user; deactivateError = ''; }}
                  >
                    {t('settings.deactivate')}
                  </button>
                {/if}
              {/if}
            </span>
          </div>
        {:else}
          <div class="empty"><strong>{t('settings.noAccount')}</strong></div>
        {/each}

        <p class="footnote">
          <span class="dot idle"></span>
          <span>{rule}</span>
        </p>
      {/if}
    <!-- Ce qui n'est pas une commande se range à côté d'elles, pas dessous en
         paragraphe libre : la doctrine se lit, l'avertissement se remarque. -->
    <div class="notes">
      <div class="note">
        <strong>{t('settings.doctrineTitle')}</strong>
        <p>{t('settings.doctrineBody')}</p>
      </div>
      <div class="note warn">
        <strong>{t('settings.recoveryTitle')}</strong>
        <p>{t('settings.recoveryHint')}</p>
      </div>
    </div>
    </section>
  {/if}
  </div>
</div>

{#if creating}
  <AccountCreation
    error={createError}
    onclose={() => (creating = false)}
    onconfirm={createAccount}
  />
{/if}

{#if deactivating}
  <AccountDeactivation
    account={deactivating}
    error={deactivateError}
    onclose={() => (deactivating = null)}
    onconfirm={deactivate}
  />
{/if}

{#if resetFor}
  <div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && !resetting && (resetFor = null)}>
    <div class="modal narrow" role="dialog" aria-modal="true" aria-labelledby="reset-title">
      <header>
        <div>
          <h2 id="reset-title">{t('settings.resetHeading', { name: resetFor.display_name })}</h2>
          <p>{t('settings.resetLead')}</p>
        </div>
        <button class="close" type="button" onclick={() => (resetFor = null)} aria-label={t('common.close')}>
          <Icon name="close" size={14} />
        </button>
      </header>

      <form onsubmit={submitReset}>
        <div class="modal-body">
          <div class="field">
            <label for="reset-password">{t('settings.newPassword')}</label>
            <input id="reset-password" bind:value={resetPassword} type="text" spellcheck="false"
              autocomplete="off" required minlength="12" maxlength="128" class="mono" />
            <small>{t('settings.resetBounds')}</small>
          </div>
          <button class="btn sm" type="button" onclick={suggest}>{t('settings.suggestPassword')}</button>
          {#if resetError}<p class="error" role="alert">{resetError}</p>{/if}
        </div>
        <footer>
          <button class="btn" type="button" onclick={() => (resetFor = null)} disabled={resetting}>
            {t('common.cancel')}
          </button>
          <button class="btn primary" type="submit" disabled={resetting || resetPassword.length < 12}>
            {resetting ? t('settings.resetting') : t('settings.reset')}
          </button>
        </footer>
      </form>
    </div>
  </div>
{/if}

<style>
  .page-head { margin-bottom: var(--s5); }
  .settings-tabs { display: flex; align-items: center; gap: var(--s2); overflow-x: auto; margin-bottom: var(--s4); border-bottom: 1px solid var(--line); scrollbar-width: none; }
  .settings-tabs::-webkit-scrollbar { display: none; }
  .settings-tabs a { display: inline-flex; align-items: center; gap: var(--s2); flex: none; min-height: 2.75rem; padding: 0 var(--s3); border-bottom: 2px solid transparent; color: var(--faint); font-size: var(--text-sm); font-weight: 500; white-space: nowrap; }
  .settings-tabs a:hover, .settings-tabs a:focus-visible { color: var(--ink); }
  .settings-tabs a.active { border-bottom-color: var(--ink); color: var(--ink); }
  .settings-layout { display: grid; grid-template-columns: minmax(0, 1fr) 18rem; align-items: start; gap: var(--s4); }
  .settings-layout > :not(.general-card):not(.settings-aside) { grid-column: 1 / -1; }
  .settings-layout > section, .settings-section { min-width: 0; scroll-margin-top: calc(var(--topbar-h) + var(--s7) + var(--s7)); }
  .settings-card-head { display: flex; align-items: center; gap: var(--s4); min-height: 3.75rem; padding: var(--s3) var(--s4); border-bottom: 1px solid var(--line); }
  .settings-card-head > span:nth-child(2) { min-width: 0; }
  .settings-card-head h2 { margin: 0; font-size: 0.9375rem; font-weight: 600; }
  .settings-card-head small { display: block; margin-top: var(--s1); color: var(--faint); font-size: var(--text-xs); line-height: 1.4; }
  .settings-icon { display: inline-grid; place-items: center; flex: none; width: 2.25rem; height: 2.25rem; border-radius: var(--r-m); background: var(--surface-2); color: var(--ink); }
  .general-card .row { grid-template-columns: minmax(10rem, 12rem) minmax(0, 1fr); gap: var(--s4); padding: var(--s4) var(--s5); }
  .general-card .row > .act { grid-column: 2; justify-self: stretch; }
  .general-card .id strong { color: var(--ink); }
  .general-card .id small { margin-top: var(--s1); color: var(--faint); line-height: 1.4; }
  .general-card .rename input { width: auto; flex: 1; min-width: 0; }
  .setting-value { min-width: 0; }
  .appearance-options { display: flex; flex-wrap: wrap; gap: var(--s2); }
  .appearance-options button { display: grid; justify-items: center; align-content: center; gap: var(--s1); min-width: 3.5rem; min-height: 3.25rem; padding: var(--s2) var(--s2); border: 1px solid var(--line-strong); border-radius: var(--r-m); background: var(--surface); color: var(--ink); font-size: var(--text-xs); cursor: pointer; }
  .appearance-options button:hover { background: var(--surface-2); }
  .appearance-options button[aria-pressed='true'] { border-color: var(--accent); background: var(--surface-2); }
  .appearance-options button:focus-visible, .settings-tabs a:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .select-field { margin: 0; }
  .select-field select { width: 100%; background: var(--surface); }
  .select-field small { line-height: 1.5; }
  .settings-aside { align-self: stretch; }
  .aside-block { display: flex; align-items: start; gap: var(--s3); padding: var(--s4); border-bottom: 1px solid var(--line); }
  .aside-block:last-child { border-bottom: 0; }
  .aside-block .settings-icon { width: 2rem; height: 2rem; }
  .aside-block strong { display: block; font-size: var(--text-sm); font-weight: 600; }
  .aside-block p, .aside-block small { display: block; margin-top: var(--s3); color: var(--faint); font-size: var(--text-xs); line-height: 1.55; }
  .aside-block .btn { margin-top: var(--s3); }
  .aside-block .dot { display: inline-block; }
  .status-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)) auto; align-items: center; gap: 0; padding: var(--s3) var(--s5); }
  .status-grid > div { display: grid; align-content: start; gap: var(--s3); min-height: 4.25rem; padding: var(--s2) var(--s5); border-right: 1px solid var(--line); }
  .status-grid > div:first-child { padding-left: 0; }
  .status-grid > div:nth-child(3) { border-right: 0; }
  .status-grid strong { font-size: var(--text-xs); font-weight: 600; }
  .status-grid span { font-size: var(--text-sm); }
  .status-grid small { color: var(--faint); font-size: var(--text-xs); }
  .status-grid .btn { margin-left: var(--s4); }
  .account-card > .row { grid-template-columns: minmax(0, 1fr) auto auto; }
  .account-card > .row .act { grid-column: 3; }
  .password-toggle { display: flex; align-items: center; gap: var(--s3); padding: var(--s3) var(--s5); }
  .password-toggle > span:nth-child(2) { flex: 1; min-width: 0; }
  .password-toggle strong, .password-toggle small { display: block; }
  .password-toggle strong { font-size: var(--text-sm); }
  .password-toggle small { margin-top: var(--s1); color: var(--faint); font-size: var(--text-xs); }
  .password-toggle .settings-icon { width: 2rem; height: 2rem; }
  .password-form { border-top: 1px solid var(--line); }
  .account-create { margin-left: auto; }
  .accounts .settings-card-head { border-bottom: 1px solid var(--line); }
  .accounts .footnote { margin: 0; }

  /* Le nom de l'instance se corrige sur place : le champ occupe la colonne du
   * geste, avec sa commande à côté, et le refus du serveur paraît sous la
   * ligne plutôt qu'à la place du champ qu'on vient de remplir. */
  .rename {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin-bottom: 0;
  }

  .rename input {
    width: 16rem;
    max-width: 100%;
  }

  .card > .error {
    margin: 0 1rem var(--s4);
  }

  /* Trois colonnes tenues d'une ligne à l'autre : ce qu'on règle, ce que
   * l'instance en dit, et le geste. La colonne du milieu peut rester vide, la
   * commande n'en bouge pas pour autant. */
  .row {
    display: grid;
    grid-template-columns: minmax(0, 21rem) minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--s4);
    padding: var(--s4) 1rem;
    border-bottom: 1px solid var(--line-row);
  }

  .row:last-child {
    border-bottom: 0;
  }

  .row > .act {
    grid-column: 3;
    justify-self: end;
  }

  .means {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s3);
    min-width: 0;
  }

  .id {
    min-width: 0;
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--s3);
  }

  .id strong {
    display: block;
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .id small {
    display: block;
    font-size: var(--text-xs);
  }

  .self {
    font-size: var(--text-xs);
  }

  /* Compte · Rôle · Dernière activité · Sessions · commandes. La colonne des
   * commandes est fixe : laissée en `auto`, elle change de largeur entre
   * l'en-tête vide et les rangées, et décale tout ce qui la précède. */
  .accounts {
    --cols: minmax(0, 1fr) 9rem 9rem 5rem 13rem;
  }

  .actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s3);
  }

  /* Un compte désactivé reste lisible : il s'efface sans disparaître, et ses
   * commandes gardent leur contraste pour que le retour soit à portée. */
  .trow.off .avatar,
  .trow.off .id {
    opacity: 0.55;
  }

  .role select {
    padding: 0.25rem var(--s3);
    font-size: var(--text-xs);
  }

  /* La règle que l'instance tient se dit sous les comptes qu'elle contraint,
   * dans la dalle et non au-dessous d'elle. */
  .footnote {
    display: flex;
    align-items: baseline;
    gap: var(--s3);
    padding: var(--s4) 1rem;
    border-top: 1px solid var(--line);
    background: var(--bg);
    color: var(--faint);
    font-size: var(--text-sm);
  }

  h3 {
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .lead {
    margin: 0.25rem 0 var(--s4);
    font-size: var(--text-sm);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
    gap: var(--s4);
  }

  .grid .field {
    /* Les champs de la grille alignent leurs saisies : sans cela, le champ
     * porteur d'une note glisse ses lignes et désaligne les autres. */
    align-content: start;
    margin: 0;
  }

  .submit {
    display: flex;
    align-items: center;
    gap: var(--s4);
    margin-top: var(--s5);
    font-size: var(--text-sm);
  }

  /* Les deux notes de bas d'écran : la doctrine, et ce qu'on fait quand on
   * s'est fermé la porte. */
  .notes {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(19rem, 1fr));
    gap: var(--s4);
    margin-top: var(--s5);
  }

  .note {
    padding: var(--s4) 1rem;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--surface);
  }

  .note strong {
    display: block;
    margin-bottom: 0.375rem;
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .note p {
    color: var(--muted);
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .note.warn {
    border-color: var(--warn-line);
    background: var(--warn-bg);
  }

  .note.warn strong {
    color: var(--warn);
  }

  .note.warn p {
    color: var(--warn);
  }

  .accounts .notes { gap: 0; margin-top: 0; padding: var(--s3); border-top: 1px solid var(--warn-line); background: var(--warn-bg); }
  .accounts .note, .accounts .note.warn { padding: var(--s2) var(--s4); border: 0; border-radius: 0; background: transparent; }
  .accounts .note + .note { border-left: 1px solid var(--warn-line); }
  .accounts .note strong, .accounts .note p { color: var(--warn); }

  .narrow {
    max-width: 32rem;
  }

  input.mono {
    font-family: var(--font-num);
  }

  @media (max-width: 58rem) {
    .settings-layout { grid-template-columns: minmax(0, 1fr); }
    .settings-aside { grid-column: 1; }
    .settings-aside { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .aside-block:nth-child(2) { border-right: 0; }
    .aside-block { border-right: 1px solid var(--line); }
    .aside-block:nth-child(even) { border-right: 0; }
  }

  @media (max-width: 48rem) {
    .page { padding: var(--s4); }
    .settings-aside { grid-template-columns: minmax(0, 1fr); }
    .aside-block { border-right: 0; }
    .general-card .row { grid-template-columns: minmax(0, 1fr); }
    .general-card .row > .act, .general-card .row > .setting-value { grid-column: 1; }
    .status-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s4); }
    .status-grid > div { padding: var(--s2); }
    .status-grid > div:nth-child(2) { border-right: 0; }
    .status-grid .btn { justify-self: start; margin-left: 0; }
    .account-card > .row { grid-template-columns: minmax(0, 1fr) auto; }
    .account-card > .row .means { grid-row: 2; }
    .account-card > .row .act { grid-column: 2; grid-row: 1; }
    .row {
      grid-template-columns: minmax(0, 1fr);
      align-items: start;
      gap: var(--s3);
    }

    .row > .means,
    .row > .act {
      grid-column: 1;
      justify-self: start;
    }

    .rename {
      width: 100%;
    }

    .rename input {
      width: 0;
      flex: 1;
    }

    .submit {
      align-items: flex-start;
      flex-direction: column;
      gap: var(--s3);
    }

    .notes {
      grid-template-columns: minmax(0, 1fr);
    }
    .accounts .note + .note { border-left: 0; border-top: 1px solid var(--warn-line); }
  }
</style>
