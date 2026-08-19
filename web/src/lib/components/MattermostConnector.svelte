<script lang="ts">
  import { t } from '$lib/i18n.svelte';
  import Icon from './Icon.svelte';
  import { api, type IncidentSeverity, type NotificationChannel } from '$lib/api';

  let {
    onclose,
    onsuccess
  }: {
    onclose: () => void;
    onsuccess: (channel: NotificationChannel) => Promise<void> | void;
  } = $props();

  const choices: Array<{ value: IncidentSeverity; code: string; label: string; note: string }> = [
    { value: 'information', code: 'I', label: 'Information', note: 'Contexte sans urgence' },
    { value: 'warning', code: 'W', label: t('severity.warning'), note: t('mattermost.warningNote') },
    { value: 'major', code: 'M', label: t('severity.major'), note: t('mattermost.majorNote') },
    { value: 'critical', code: 'C', label: t('severity.critical'), note: t('mattermost.criticalNote') }
  ];

  let name = $state('Exploitation');
  let webhookURL = $state('');
  let severities = $state<IncidentSeverity[]>(['warning', 'major', 'critical']);
  let created = $state<NotificationChannel | null>(null);
  let busy = $state(false);
  let error = $state('');

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  function toggle(severity: IncidentSeverity) {
    severities = severities.includes(severity)
      ? severities.filter((item) => item !== severity)
      : [...severities, severity];
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    if (severities.length === 0) {
      error = t('mattermost.pickSeverity');
      return;
    }
    busy = true;
    try {
      const channel = await api<NotificationChannel>('/api/v1/notification-channels/mattermost', {
        method: 'POST',
        body: JSON.stringify({ name, webhook_url: webhookURL, severities })
      });
      created = channel;
      await onsuccess(channel);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('mattermost.failed');
    } finally {
      busy = false;
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="scrim" role="presentation" onclick={(event) => event.target === event.currentTarget && !busy && onclose()}>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="mattermost-title">
    <header>
      <span class="key"><Icon name="bell" size={16} /></span>
      <div>
        <h2 id="mattermost-title">{created ? t('mattermost.confirmed') : t('mattermost.connect')}</h2>
        <p>
          {created
            ? t('mattermost.confirmedSay')
            : t('mattermost.connectSay')}
        </p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label="Fermer">
        <Icon name="close" size={14} />
      </button>
    </header>

    {#if created}
      <div class="modal-body">
        <div class="banner ok">
          <i class="dot ok"></i>
          <div>
            <strong>La voie est ouverte.</strong>
            <p class="muted">{t('mattermost.routingSay')}</p>
          </div>
        </div>

        <dl class="receipt">
          <div><dt>Canal</dt><dd>{created.name}</dd></div>
          <div><dt>Destination</dt><dd class="mono">{created.endpoint}</dd></div>
          <div>
            <dt>{t('mattermost.routedSeverities')}</dt>
            <dd>{created.severities.map((severity) => choices.find((choice) => choice.value === severity)?.label).join(' · ')}</dd>
          </div>
          <div>
            <dt>{t('mattermost.transport')}</dt>
            <dd>{created.encrypted_transport ? t('mattermost.tlsVerified') : t('mattermost.plain')}</dd>
          </div>
        </dl>
      </div>
      <footer>
        <button class="btn primary" type="button" onclick={onclose}>{t('mattermost.backToWatch')}</button>
      </footer>
    {:else}
      <form onsubmit={submit}>
        <div class="modal-body">
          <div class="field">
            <label for="mm-name">Nom du canal</label>
            <input id="mm-name" bind:value={name} maxlength="160" required placeholder="Exploitation" />
          </div>

          <div class="field">
            <label for="mm-url">URL du webhook entrant</label>
            <input id="mm-url" bind:value={webhookURL} type="url" inputmode="url" required
              placeholder="https://mattermost.example/hooks/…" />
            <small>{t('mattermost.httpsHint')}</small>
          </div>

          <fieldset class="routing">
            <legend>
              {t('mattermost.routedSeverities')}
              <span class="faint">{t('mattermost.recommended')}</span>
            </legend>
            <div class="chips">
              {#each choices as choice (choice.value)}
                <button
                  class="chip-toggle"
                  type="button"
                  aria-pressed={severities.includes(choice.value)}
                  onclick={() => toggle(choice.value)}
                  title={choice.note}
                >
                  <i class="mark" aria-hidden="true">{severities.includes(choice.value) ? '✓' : '+'}</i>
                  {choice.label}
                </button>
              {/each}
            </div>
          </fieldset>

          <ol class="contract">
            <li><strong>{t('mattermost.opening')}</strong><small class="faint">{t('mattermost.openingNote')}</small></li>
            <li><strong>{t('incidents.column.acknowledgement')}</strong><small class="faint">{t('mattermost.ackNote')}</small></li>
            <li><strong>{t('incidents.column.resolved')}</strong><small class="faint">{t('mattermost.resolvedNote')}</small></li>
          </ol>

          {#if error}<p class="error" role="alert">{error}</p>{/if}
        </div>

        <footer>
          <span class="faint note">{t('mattermost.probeNote')}</span>
          <button class="btn" type="button" onclick={onclose} disabled={busy}>Annuler</button>
          <button class="btn primary" type="submit" disabled={busy}>
            {busy ? t('gate.verifying') : t('mattermost.verifyAndConnect')}
          </button>
        </footer>
      </form>
    {/if}
  </div>
</div>

<style>
  .modal {
    max-width: 34rem;
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

  .receipt {
    margin: var(--s5) 0 0;
    display: grid;
    gap: 0;
  }

  .receipt > div {
    display: flex;
    align-items: baseline;
    gap: var(--s4);
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--line-row);
  }

  .receipt > div:last-child {
    border-bottom: 0;
  }

  dt {
    flex: none;
    width: 9rem;
    color: var(--faint);
    font-size: 0.75rem;
  }

  dd {
    margin: 0;
    min-width: 0;
    font-size: 0.75rem;
    overflow-wrap: anywhere;
  }

  .routing {
    margin: 0;
    padding: 0;
    border: 0;
  }

  legend {
    padding: 0;
    margin-bottom: var(--s3);
    color: var(--muted);
    font-size: 0.75rem;
    font-weight: 500;
  }

  legend span {
    margin-left: 0.3125rem;
    font-weight: 400;
    font-size: 0.6875rem;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .chip-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.3125rem 0.625rem;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-pill);
    background: var(--bg);
    color: var(--muted);
    font-size: 0.75rem;
    transition: border-color var(--d1) var(--ease), color var(--d1) var(--ease);
  }

  .chip-toggle:hover {
    color: var(--ink);
  }

  .chip-toggle[aria-pressed='true'] {
    border-color: var(--accent);
    background: var(--surface-2);
    color: var(--ink);
  }

  .mark {
    font-style: normal;
    color: var(--faint);
  }

  .chip-toggle[aria-pressed='true'] .mark {
    color: var(--accent);
  }

  .contract {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
    gap: var(--s4);
    margin: var(--s5) 0 0;
    padding: var(--s4) 0 0;
    border-top: 1px solid var(--line);
    list-style: none;
    counter-reset: step;
  }

  .contract li {
    counter-increment: step;
  }

  .contract li::before {
    content: counter(step, decimal-leading-zero);
    display: block;
    margin-bottom: 0.1875rem;
    color: var(--accent);
    font-family: var(--font-num);
    font-size: 0.625rem;
  }

  .contract strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
  }

  .contract small {
    display: block;
    font-size: 0.6875rem;
  }

</style>
