<script lang="ts">
  import { t } from '$lib/i18n.svelte';
  import Icon from './Icon.svelte';
  import { onMount } from 'svelte';
  import { api, type GenericWebhookCreated } from '$lib/api';

  let {
    onclose,
    onsuccess
  }: {
    onclose: () => void;
    onsuccess: (created: GenericWebhookCreated) => Promise<void> | void;
  } = $props();

  let nameInput = $state<HTMLInputElement>();
  let name = $state('Automations');
  let created = $state<GenericWebhookCreated | null>(null);
  let busy = $state(false);
  let error = $state('');
  let copied = $state('');

  onMount(() => nameInput?.focus());

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  async function create(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      const result = await api<GenericWebhookCreated>('/api/v1/connectors/generic-webhook', {
        method: 'POST',
        body: JSON.stringify({ name })
      });
      created = result;
      await onsuccess(result);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('webhook.failed');
    } finally {
      busy = false;
    }
  }

  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      copied = label;
      setTimeout(() => (copied = ''), 1800);
    } catch {
      error = t('webhook.copyUnavailable');
    }
  }

  function copyCredential(field: 'endpoint' | 'token') {
    if (!created) return;
    void copy(created[field], field);
  }

  function payloadExample() {
    return JSON.stringify({
      identity: 'worker/api-production',
      target_name: 'API production',
      event_key: 'availability',
      nature_key: 'availability',
      nature: t('webhook.sampleNature'),
      status: 'firing',
      severity: 'major',
      summary: t('webhook.sampleSummary'),
      details: { region: 'home' }
    }, null, 2);
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && !busy && onclose()}>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="webhook-connector-title">
    <header>
      <span class="key"><Icon name="webhook" size={16} /></span>
      <div>
        <h2 id="webhook-connector-title">
          {created ? t('webhook.airlockOpen') : t('webhook.createEntry')}
        </h2>
        <p>
          {created
            ? t('webhook.copySecretSay')
            : t('webhook.createSay')}
        </p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label="Fermer">
        <Icon name="close" size={14} />
      </button>
    </header>

    {#if created}
      <div class="modal-body">
        <div class="banner warn">
          <i class="dot warn"></i>
          <div>
            <strong>Copiez le secret maintenant.</strong>
            <p class="muted">{t('webhook.secretOnce')}</p>
          </div>
        </div>

        <div class="credential">
          <span class="label faint">{t('webhook.endpointLabel')}</span>
          <code>{created.endpoint}</code>
          <button class="btn sm" type="button" onclick={() => copyCredential('endpoint')}>
            {copied === 'endpoint' ? t('webhook.copied') : t('workshop.copy')}
          </button>
        </div>

        <div class="credential secret">
          <span class="label faint">Jeton Bearer — affichage unique</span>
          <code>{created.token}</code>
          <button class="btn sm" type="button" onclick={() => copyCredential('token')}>
            {copied === 'token' ? t('webhook.copied') : t('workshop.copy')}
          </button>
        </div>

        <div class="contract">
          <strong>{t('webhook.contract')}</strong>
          <p class="muted">
            {t('webhook.contractBefore')} <code class="inline">Authorization: Bearer …</code>.
            {t('webhook.contractMiddle')} <code class="inline">event_key</code>
            {t('webhook.contractAfter')}
          </p>
          <pre>{payloadExample()}</pre>
        </div>

        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>

      <footer>
        <span class="faint note">Aucun signal inconnu ne devient un Incident sans votre autorisation.</span>
        <button class="btn primary" type="button" onclick={onclose}>{t('webhook.secretKept')}</button>
      </footer>
    {:else}
      <form onsubmit={create}>
        <div class="modal-body">
          <div class="field">
            <label for="webhook-name">Nom du raccordement</label>
            <input id="webhook-name" bind:this={nameInput} bind:value={name} maxlength="160" required
              placeholder="Automations production" />
          </div>

          <ol class="steps">
            <li><strong>{t('webhook.point1')}</strong><small class="faint">{t('webhook.point1Note')}</small></li>
            <li><strong>{t('webhook.point2')}</strong><small class="faint">{t('webhook.point2Note')}</small></li>
            <li><strong>{t('webhook.point3')}</strong><small class="faint">{t('webhook.point3Note')}</small></li>
            <li><strong>{t('webhook.point4')}</strong><small class="faint">{t('webhook.point4Note')}</small></li>
          </ol>

          {#if error}<p class="error" role="alert">{error}</p>{/if}
        </div>

        <footer>
          <button class="btn" type="button" onclick={onclose} disabled={busy}>Annuler</button>
          <button class="btn primary" type="submit" disabled={busy}>
            {busy ? t('webhook.generating') : t('webhook.createEntry')}
          </button>
        </footer>
      </form>
    {/if}
  </div>
</div>

<style>
  .modal {
    max-width: 36rem;
  }

  .key {
    width: 1.875rem;
    height: 1.875rem;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--r-m);
    background: var(--surface-2);
    color: var(--muted);
    font-size: 0.6875rem;
    font-weight: 600;
  }

  .banner strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .banner p {
    margin-top: 0.25rem;
    font-size: 0.75rem;
  }

  .credential {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: center;
    gap: var(--s3);
    margin-top: var(--s4);
    padding: var(--s3) var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
  }

  .credential.secret {
    border-color: var(--warn-line);
  }

  .label {
    grid-column: 1 / -1;
    font-size: 0.625rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  code {
    font-family: var(--font-num);
    font-size: 0.75rem;
    overflow-wrap: anywhere;
  }

  code.inline {
    color: var(--muted);
  }

  .contract {
    margin-top: var(--s5);
    padding-top: var(--s4);
    border-top: 1px solid var(--line);
  }

  .contract strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .contract p {
    margin-top: 0.25rem;
    font-size: 0.75rem;
  }

  pre {
    margin: var(--s4) 0 0;
    padding: var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
    color: var(--muted);
    font-family: var(--font-num);
    font-size: 0.6875rem;
    line-height: 1.6;
    overflow-x: auto;
  }

  .steps {
    margin: var(--s5) 0 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--s3);
    counter-reset: step;
  }

  .steps li {
    counter-increment: step;
    display: grid;
    grid-template-columns: 1.6rem 1fr;
    align-items: baseline;
    gap: var(--s3);
  }

  .steps li::before {
    content: counter(step, decimal-leading-zero);
    color: var(--accent);
    font-family: var(--font-num);
    font-size: 0.625rem;
  }

  .steps strong {
    font-size: 0.75rem;
    font-weight: 600;
  }

  .steps small {
    grid-column: 2;
    font-size: 0.6875rem;
  }

</style>
