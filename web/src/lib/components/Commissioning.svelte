<script lang="ts">
  import { t } from '$lib/i18n.svelte';
  import type { User } from '$lib/api';
  import InfoHint from '$lib/components/InfoHint.svelte';

  /* Les Écrans ne couvrent pas les portes d'entrée. Elles reprennent donc les
   * mêmes jetons, la même densité et les mêmes contrôles que le reste, sans
   * inventer de traitement propre. La logique de validation est inchangée. */

  type Mode = 'loading' | 'setup' | 'login' | 'unavailable';

  let {
    mode,
    instance,
    health,
    version,
    lightTheme,
    error,
    busy,
    oidcEnabled,
    oidcLabel,
    ontoggletheme,
    onsetup,
    onlogin,
    onoidc,
    onrecover
  }: {
    mode: Mode;
    instance: string;
    health: 'checking' | 'ready' | 'unavailable';
    version: string;
    lightTheme: boolean;
    error: string;
    busy: boolean;
    oidcEnabled: boolean;
    oidcLabel: string;
    ontoggletheme: () => void;
    onsetup: (input: {
      bootstrap: string;
      instance_name: string;
      username: string;
      display_name: string;
      password: string;
    }) => Promise<User | void>;
    onlogin: (input: { username: string; password: string }) => Promise<User | void>;
    onoidc: () => void;
    onrecover: (input: { bootstrap: string; username: string; password: string }) => Promise<User | void>;
  } = $props();

  /* La porte de secours ne se substitue pas à la connexion : elle s'ouvre à la
   * demande, pour le cas où plus personne ne peut ouvrir de session. */
  let recovering = $state(false);

  let bootstrap = $state('');
  let instanceName = $state('');
  let displayName = $state('');
  let username = $state('');
  let password = $state('');
  let confirmation = $state('');
  let localError = $state('');

  async function submitSetup(event: SubmitEvent) {
    event.preventDefault();
    localError = '';
    if (password !== confirmation) {
      localError = t('gate.passwordMismatch');
      return;
    }
    await onsetup({
      bootstrap,
      instance_name: instanceName,
      username,
      display_name: displayName,
      password
    });
  }

  async function submitLogin(event: SubmitEvent) {
    event.preventDefault();
    localError = '';
    await onlogin({ username, password });
  }

  async function submitRecovery(event: SubmitEvent) {
    event.preventDefault();
    localError = '';
    if (password !== confirmation) {
      localError = t('gate.passwordMismatch');
      return;
    }
    await onrecover({ bootstrap, username, password });
  }
</script>

<svelte:head>
  <title>{mode === 'setup' ? t('gate.setupTab') : t('gate.loginTab')} — {instance}</title>
</svelte:head>

<main class="gate">
  <div class="gate-card">
    <div class="gate-head">
      <span class="cairn" aria-hidden="true"><i></i><i></i><i></i></span>
      <!-- Une instance nommée se nomme elle-même ici : c'est la première chose
           que voit quelqu'un qui garde plusieurs instances ouvertes. -->
      <strong>{instance}</strong>
      <span class="version">v{version}</span>
      <button class="btn sm quiet" type="button" onclick={ontoggletheme}>
        {lightTheme ? t('rail.dark') : t('rail.light')}
      </button>
    </div>

    <div class="gate-state">
      <i class="dot" class:ok={health === 'ready'} class:warn={health === 'checking'} class:crit={health === 'unavailable'}></i>
      <span>
        {health === 'ready'
          ? t('gate.reachable')
          : health === 'checking'
            ? t('gate.connecting')
            : t('gate.offline')}
      </span>
    </div>

    {#if mode === 'loading'}
      <h1>{t('gate.loadingTitle')}</h1>
      <p>{t('gate.loadingSay')}</p>
    {:else if mode === 'unavailable'}
      <h1>{t('gate.unavailableTitle')}</h1>
      <p>{t('gate.unavailableSay')}</p>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="button" onclick={() => location.reload()}>
        {t('common.retry')}
      </button>
    {:else if mode === 'setup'}
      <h1>{t('gate.setupTitle')}</h1>
      <p>{t('gate.setupSay')}</p>
      <form onsubmit={submitSetup}>
        <div class="field">
          <div class="field-label">
            <InfoHint id="bootstrap-hint" ariaLabel={t('gate.fieldHelp', { field: t('gate.bootstrapToken') })} text={`${t('gate.bootstrapHint')} CAIRNOPS_BOOTSTRAP_TOKEN.`} />
            <label for="bootstrap">{t('gate.bootstrapToken')}</label>
          </div>
          <input id="bootstrap" bind:value={bootstrap} type="password" autocomplete="off" required minlength="32" spellcheck="false" aria-describedby="bootstrap-hint" />
        </div>
        <!-- Le nom vient avant l'Administrateur : on nomme d'abord ce que l'on
             pose, on dit ensuite qui le tiendra. -->
        <div class="field">
          <div class="field-label">
            <InfoHint id="instance-name-hint" ariaLabel={t('gate.fieldHelp', { field: t('gate.instanceName') })} text={t('gate.instanceNameHint')} />
            <label for="instance-name">{t('gate.instanceName')}</label>
          </div>
          <input id="instance-name" bind:value={instanceName} autocomplete="off" required maxlength="80" placeholder={t('gate.instanceNamePlaceholder')} aria-describedby="instance-name-hint" />
        </div>
        <div class="field">
          <div class="field-label">
            <InfoHint id="display-name-hint" ariaLabel={t('gate.fieldHelp', { field: t('gate.adminName') })} text={t('gate.adminNameHint')} />
            <label for="display-name">{t('gate.adminName')}</label>
          </div>
          <input id="display-name" bind:value={displayName} autocomplete="name" required maxlength="100" placeholder={t('gate.displayNamePlaceholder')} aria-describedby="display-name-hint" />
        </div>
        <div class="field">
          <div class="field-label">
            <InfoHint id="setup-username-hint" ariaLabel={t('gate.fieldHelp', { field: t('gate.username') })} text={t('gate.usernameHint')} />
            <label for="setup-username">{t('gate.username')}</label>
          </div>
          <input id="setup-username" bind:value={username} autocomplete="username" required minlength="3" maxlength="64" pattern={'[a-z0-9][a-z0-9._\\-]{2,63}'} placeholder={t('gate.usernamePlaceholder')} aria-describedby="setup-username-hint" />
        </div>
        <div class="field">
          <div class="field-label">
            <InfoHint id="setup-password-hint" ariaLabel={t('gate.fieldHelp', { field: t('gate.password') })} text={t('gate.passwordHint')} />
            <label for="setup-password">{t('gate.password')}</label>
          </div>
          <input id="setup-password" bind:value={password} type="password" autocomplete="new-password" required minlength="12" maxlength="128" aria-describedby="setup-password-hint" />
        </div>
        <div class="field">
          <div class="field-label">
            <InfoHint id="confirmation-hint" ariaLabel={t('gate.fieldHelp', { field: t('gate.confirmPassword') })} text={t('gate.confirmPasswordHint')} />
            <label for="confirmation">{t('gate.confirmPassword')}</label>
          </div>
          <input id="confirmation" bind:value={confirmation} type="password" autocomplete="new-password" required minlength="12" maxlength="128" aria-describedby="confirmation-hint" />
        </div>
        {#if localError || error}<p class="error" role="alert">{localError || error}</p>{/if}
        <div class="gate-submit">
          <small>{t('gate.sessionNote')}</small>
          <button class="btn primary" type="submit" disabled={busy}>
            {busy ? t('gate.sealing') : t('gate.initialise')}
          </button>
        </div>
      </form>
    {:else}
      {#if recovering}
        <h1>{t('gate.recoveryTitle')}</h1>
        <p>{t('gate.recoverySay')}</p>
        <form onsubmit={submitRecovery}>
          <div class="field">
            <label for="recovery-bootstrap">{t('gate.bootstrapToken')}</label>
            <input id="recovery-bootstrap" bind:value={bootstrap} type="password" autocomplete="off" required minlength="32" spellcheck="false" />
            <small>{t('gate.bootstrapServerHintBefore')} <code>CAIRNOPS_BOOTSTRAP_TOKEN</code>
              {t('gate.bootstrapServerHintAfter')}</small>
          </div>
          <div class="field">
            <label for="recovery-username">{t('gate.accountUsername')}</label>
            <input id="recovery-username" bind:value={username} autocomplete="username" required />
          </div>
          <div class="field">
            <label for="recovery-password">{t('settings.newPassword')}</label>
            <input id="recovery-password" bind:value={password} type="password" autocomplete="new-password" required minlength="12" maxlength="128" />
          </div>
          <div class="field">
            <label for="recovery-confirm">{t('settings.confirm')}</label>
            <input id="recovery-confirm" bind:value={confirmation} type="password" autocomplete="new-password" required minlength="12" maxlength="128" />
          </div>
          {#if localError || error}<p class="error" role="alert">{localError || error}</p>{/if}
          <div class="gate-submit">
            <button class="btn sm quiet" type="button" onclick={() => { recovering = false; localError = ''; }}>
              {t('gate.backToLogin')}
            </button>
            <button class="btn primary" type="submit" disabled={busy}>
              {busy ? t('gate.restoring') : t('gate.restoreAccess')}
            </button>
          </div>
        </form>
      {:else}
      <h1>{t('gate.loginTitle')}</h1>
      <p>{t('gate.loginSay')}</p>
      {#if oidcEnabled}
        <button class="btn primary oidc" type="button" onclick={onoidc}>
          {t('gate.oidcContinue', { provider: oidcLabel })}
        </button>
        <div class="gate-divider"><span>{t('gate.localFallback')}</span></div>
      {/if}
      <form onsubmit={submitLogin}>
        <div class="field">
          <label for="login-username">{t('gate.username')}</label>
          <input id="login-username" bind:value={username} autocomplete="username" required />
        </div>
        <div class="field">
          <label for="login-password">{t('gate.password')}</label>
          <input id="login-password" bind:value={password} type="password" autocomplete="current-password" required />
        </div>
        {#if localError || error}<p class="error" role="alert">{localError || error}</p>{/if}
        <div class="gate-submit">
          <small>{t('gate.credentialsNote')}</small>
          <button class="btn primary" type="submit" disabled={busy}>
            {busy ? t('gate.verifying') : t('gate.signIn')}
          </button>
        </div>
      </form>
      <p class="lost">
        <button type="button" onclick={() => { recovering = true; localError = ''; password = ''; }}>
          {t('gate.lostAccess')}
        </button>
      </p>
      {/if}
    {/if}
  </div>
</main>

<style>
  .gate-head {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    min-width: 0;
    margin-bottom: var(--s4);
  }

  /* Le nom d'une instance peut être long ; il se coupe plutôt que de repousser
     la version et la bascule de thème hors de la carte. */
  .gate-head strong {
    font-size: 0.9375rem;
    font-weight: 600;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .gate-head .btn {
    margin-left: var(--s2);
  }

  .gate-state {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin-bottom: var(--s6);
    color: var(--faint);
    font-size: 0.75rem;
  }

  form {
    margin-top: var(--s6);
  }

  .oidc {
    justify-content: center;
    width: 100%;
    margin-top: var(--s6);
  }

  .gate-divider {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin-top: var(--s5);
    color: var(--faint);
    font-size: 0.75rem;
  }

  .gate-divider::before,
  .gate-divider::after {
    content: '';
    flex: 1;
    border-top: 1px solid var(--line);
  }

  .gate-divider + form {
    margin-top: var(--s5);
  }

  .field-label {
    display: flex;
    align-items: center;
    gap: var(--s2);
    min-height: 1.5rem;
  }

  code {
    font-family: var(--font-num);
    font-size: 0.6875rem;
    color: var(--muted);
  }

  .gate-submit {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s4);
    margin-top: var(--s6);
  }

  .lost {
    margin-top: var(--s4);
    padding-top: var(--s4);
    border-top: 1px solid var(--line);
    text-align: center;
  }

  .lost button {
    border: 0;
    background: none;
    color: var(--faint);
    font-size: 0.75rem;
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .lost button:hover {
    color: var(--ink);
  }

  .gate-submit small {
    color: var(--faint);
    font-size: 0.75rem;
  }

  .gate-submit .btn {
    height: 2.375rem;
    padding: 0 var(--s5);
    font-size: 0.8125rem;
  }

  @media (max-width: 30rem) {
    .gate-submit {
      align-items: stretch;
      flex-direction: column;
    }

    .gate-submit small {
      line-height: 1.5;
    }

    .gate-submit .btn {
      justify-content: center;
      width: 100%;
    }
  }
</style>
