<script lang="ts">
  /* Ouvrir un compte.
   *
   * CairnOps n'envoie pas de courrier en V1 : l'Administrateur choisit le
   * premier mot de passe et le transmet lui-même, exactement comme une
   * réinitialisation. L'écran le dit plutôt que de laisser croire qu'une
   * invitation partira. */

  import Icon from './Icon.svelte';
  import type { Role } from '$lib/api';
  import { t } from '$lib/i18n.svelte';

  let {
    onclose,
    onconfirm,
    error = ''
  }: {
    onclose: () => void;
    onconfirm: (input: {
      username: string;
      display_name: string;
      role: Role;
      password: string;
    }) => Promise<void> | void;
    error?: string;
  } = $props();

  let username = $state('');
  let displayName = $state('');
  let role = $state<Role>('operator');
  let password = $state('');
  let busy = $state(false);

  /* La même borne que le serveur : trois à soixante-quatre caractères, en
   * minuscules. L'annoncer ici évite un aller-retour pour l'apprendre. */
  const usernamePattern = /^[a-z0-9][a-z0-9._-]{2,63}$/;

  const validUsername = $derived(usernamePattern.test(username));
  const canSubmit = $derived(
    validUsername && displayName.trim().length > 0 && password.length >= 12 && !busy
  );

  const roles = $derived<Array<{ value: Role; label: string; hint: string }>>([
    { value: 'observer', label: t('role.observer'), hint: t('account.observerHint') },
    { value: 'operator', label: t('role.operator'), hint: t('account.operatorHint') },
    { value: 'administrator', label: t('role.administrator'), hint: t('account.administratorHint') }
  ]);

  const chosen = $derived(roles.find((entry) => entry.value === role));

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  /* Une suggestion, pas une obligation : la personne reste libre de sa saisie. */
  function suggest() {
    const bytes = crypto.getRandomValues(new Uint8Array(18));
    password = btoa(String.fromCharCode(...bytes))
      .replace(/[+/=]/g, '')
      .slice(0, 20);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    try {
      await onconfirm({
        username: username.trim().toLowerCase(),
        display_name: displayName.trim(),
        role,
        password
      });
    } finally {
      busy = false;
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="scrim"
  role="presentation"
  onclick={(event) => event.currentTarget === event.target && !busy && onclose()}
>
  <div class="modal narrow" role="dialog" aria-modal="true" aria-labelledby="creation-title">
    <header>
      <div>
        <h2 id="creation-title">{t('settings.openAccount')}</h2>
        <p>{t('settings.resetLead')}</p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    <form onsubmit={submit}>
      <div class="modal-body">
        <div class="grid">
          <div class="field">
            <label for="account-username">{t('gate.username')}</label>
            <input
              id="account-username"
              bind:value={username}
              type="text"
              spellcheck="false"
              autocapitalize="none"
              autocomplete="off"
              required
              class="mono"
            />
            <small class:warn={username.length > 0 && !validUsername}>
              {t('account.usernameHint')}
            </small>
          </div>
          <div class="field">
            <label for="account-name">{t('gate.displayName')}</label>
            <input
              id="account-name"
              bind:value={displayName}
              type="text"
              autocomplete="off"
              required
              maxlength="100"
            />
            <small>{t('account.displayNameHint')}</small>
          </div>
        </div>

        <div class="field">
          <label for="account-role">{t('account.role')}</label>
          <select id="account-role" bind:value={role}>
            {#each roles as entry (entry.value)}
              <option value={entry.value}>{entry.label}</option>
            {/each}
          </select>
          <small>{chosen?.hint}</small>
        </div>

        <div class="field">
          <label for="account-password">{t('account.firstPassword')}</label>
          <input
            id="account-password"
            bind:value={password}
            type="text"
            spellcheck="false"
            autocomplete="off"
            required
            minlength="12"
            maxlength="128"
            class="mono"
          />
          <small>{t('settings.passwordBounds')}</small>
        </div>
        <button class="btn sm" type="button" onclick={suggest}>{t('settings.suggestPassword')}</button>

        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>

      <footer>
        <button class="btn" type="button" onclick={onclose} disabled={busy}>{t('common.cancel')}</button>
        <button class="btn primary" type="submit" disabled={!canSubmit}>
          {busy ? t('account.opening') : t('account.open')}
        </button>
      </footer>
    </form>
  </div>
</div>

<style>
  .narrow {
    max-width: 34rem;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
    gap: var(--s4);
  }

  .grid .field {
    margin: 0;
  }

  input.mono {
    font-family: var(--font-num);
  }

  small.warn {
    color: var(--warn);
  }
</style>
