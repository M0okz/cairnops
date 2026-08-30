<script lang="ts">
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

  /* Le catalogue tient en quatre intégrations : trois sources et une voie de
   * notification. Ce qui n'est pas encore relié reste disponible. */
  const catalogue = 4;
  const freeConnectors = $derived.by(() => {
    const engaged = new Set<string>(session.connectors.map((connector) => connector.kind));
    if (session.channels.some((channel) => channel.kind === 'mattermost')) engaged.add('mattermost');
    return Math.max(0, catalogue - engaged.size);
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

  <div class="band-row">
    <h2 class="band">{t('settings.workspace')}</h2>
    <span class="band-note">{t('settings.workspaceNote')}</span>
  </div>

  <div class="card">
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

    <!-- Clair et sombre se choisissent, ils ne se basculent pas : le segment
         montre les deux états et dit lequel tient. -->
    <div class="row">
      <span class="id">
        <strong>{t('rail.theme')}</strong>
        <small class="faint">{t('settings.themeHint')}</small>
      </span>
      <div class="act segments" role="group" aria-label={t('rail.theme')}>
        <button type="button" aria-pressed={session.lightTheme}
          onclick={() => session.lightTheme || session.toggleTheme()}>{t('settings.themeLight')}</button>
        <button type="button" aria-pressed={!session.lightTheme}
          onclick={() => session.lightTheme && session.toggleTheme()}>{t('settings.themeDark')}</button>
      </div>
    </div>

    <!-- La langue se choisit ici comme dans le menu du rail : c'est le même
         réglage, et il n'appartient qu'à cet appareil. -->
    <div class="row">
      <span class="id">
        <strong>{t('rail.language')}</strong>
        <small class="faint">{t('settings.languageHint')}</small>
      </span>
      <div class="act segments" role="group" aria-label={t('rail.language')}>
        {#each locales as choice (choice.value)}
          <button
            type="button"
            lang={choice.value}
            aria-pressed={i18n.locale === choice.value}
            onclick={() => i18n.choose(choice.value)}>{choice.label}</button
          >
        {/each}
      </div>
    </div>

    <div class="row">
      <span class="id">
        <strong>{t('nav.connectors')}</strong>
        <small class="faint">
          {plural('settings.connections', session.connectors.length)} ·
          {t('settings.connectorList')}
        </small>
      </span>
      <span class="means">
        {#each session.connectors.filter((connector) => connector.status === 'connected') as connector (connector.id)}
          <span class="pill ok">{t('settings.connectorLive', { name: connector.name })}</span>
        {/each}
        {#if freeConnectors > 0}
          <span class="pill">{plural('settings.connectorsFree', freeConnectors)}</span>
        {/if}
      </span>
      <a class="act btn sm" href="/connecteurs">{t('common.open')}</a>
    </div>

    <div class="row">
      <span class="id">
        <strong>{t('health.instance')}</strong>
        <small class="faint">{t('settings.instanceHint')}</small>
      </span>
      <span class="means num faint">
        <span class="dot {session.realtime === 'online' ? 'ok' : 'idle'}"></span>
        v{session.version} ·
        {session.realtime === 'online' ? t('settings.realtimeOn') : t('settings.realtimeOff')} ·
        {freshness}
      </span>
      <a class="act btn sm" href="/sante">{t('settings.viewHealth')}</a>
    </div>
  </div>

  <h2 class="band">{t('settings.yourAccount')}</h2>

  <div class="card">
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
    <form class="card-body" onsubmit={changePassword}>
      <h3>{t('settings.changePassword')}</h3>
      <p class="lead faint">{t('settings.changePasswordHint')}</p>

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
    </form>
    {:else}
      <div class="card-body">
        <h3>{t('settings.externalAccount')}</h3>
        <p class="lead faint">{t('settings.externalAccountHint')}</p>
      </div>
    {/if}
  </div>

  <DeviceManagement />

  {#if isAdministrator}
    {#if isLocalAdministrator}
      <OIDCSettings />
    {/if}

    <div class="band-row">
      <h2 class="band">{t('settings.accounts')}</h2>
      {#if isLocalAdministrator}
        <button class="btn primary sm" type="button" onclick={() => { creating = true; createError = ''; }}>
          {t('settings.openAccount')}
        </button>
      {/if}
    </div>

    <div class="card accounts">
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
    </div>

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
  {/if}
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
  .band {
    margin: var(--s6) 0 var(--s4);
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .band-row {
    display: flex;
    align-items: center;
    gap: var(--s4);
    margin: var(--s6) 0 var(--s4);
  }

  /* Seul le premier bandeau se passe de marge haute : le titre de l'écran lui
   * en a déjà donné. Les suivants séparent deux dalles et la gardent. */
  .page-head + .band-row {
    margin-top: 0;
  }

  .band-row .band {
    flex: 1;
    margin: 0;
  }

  .band-note {
    color: var(--faint);
    font-size: 0.75rem;
  }

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
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .id small {
    display: block;
    font-size: 0.6875rem;
  }

  .self {
    font-size: 0.6875rem;
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
    font-size: 0.6875rem;
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
    font-size: 0.75rem;
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
    font-size: 0.75rem;
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
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .note p {
    color: var(--muted);
    font-size: 0.75rem;
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

  .narrow {
    max-width: 32rem;
  }

  input.mono {
    font-family: var(--font-num);
  }

  @media (max-width: 48rem) {
    .band-row {
      align-items: flex-start;
      flex-direction: column;
      gap: var(--s1);
    }

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
  }
</style>
