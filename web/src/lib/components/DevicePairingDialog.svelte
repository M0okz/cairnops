<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { api, type DevicePairing, type DevicePairingInvitation } from '$lib/api';
  import {
    pairingCanCancel,
    pairingQRCode,
    pairingSecondsRemaining,
    pairingShouldPoll,
    pairingStep
  } from '$lib/device-pairing';
  import { duration } from '$lib/format';
  import { t } from '$lib/i18n.svelte';
  import { messageFrom } from '$lib/session.svelte';

  let {
    onclose,
    onpaired
  }: {
    onclose: () => void;
    onpaired: () => Promise<void> | void;
  } = $props();

  let invitation = $state<DevicePairingInvitation | null>(null);
  let pairing = $state<DevicePairing | null>(null);
  let qrDataURL = $state('');
  let error = $state('');
  let loading = $state(true);
  let busy = $state(false);
  let polling = false;
  let requestVersion = 0;
  let reported = false;
  let closing = false;
  let copied = $state(false);
  let now = $state(new Date());

  const remaining = $derived(pairing ? pairingSecondsRemaining(pairing.expires_at, now) : 0);
  const currentStep = $derived(pairing ? pairingStep(pairing.status) : 1);

  function statusLabel(current: DevicePairing): string {
    switch (current.status) {
      case 'awaiting_scan':
        return t('devices.pairingAwaitingScan');
      case 'awaiting_confirmation':
        return t('devices.pairingAwaitingConfirmation', {
          name: current.claimed_name ?? t('devices.unknownDevice')
        });
      case 'confirmed':
        return t('devices.pairingConfirmed');
      case 'credential_consumed':
        return t('devices.pairingComplete');
      case 'expired':
        return t('devices.pairingExpired');
      case 'cancelled':
        return t('devices.pairingCancelled');
    }
  }

  async function start() {
    requestVersion += 1;
    loading = true;
    error = '';
    copied = false;
    qrDataURL = '';
    invitation = null;
    pairing = null;
    reported = false;
    try {
      const created = await api<DevicePairingInvitation>('/api/v1/device-pairings', {
        method: 'POST'
      });
      invitation = created;
      pairing = created.pairing;
      qrDataURL = await pairingQRCode(created.qr_payload);
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      loading = false;
    }
  }

  async function poll() {
    if (!pairing || !pairingShouldPoll(pairing.status) || polling || busy || closing) return;
    const version = requestVersion;
    polling = true;
    try {
      const latest = await api<DevicePairing>(`/api/v1/device-pairings/${pairing.id}`);
      if (version !== requestVersion || closing) return;
      pairing = latest;
      error = '';
      if ((latest.status === 'confirmed' || latest.status === 'credential_consumed') && !reported) {
        reported = true;
        await onpaired();
      }
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      polling = false;
    }
  }

  async function confirm() {
    if (!pairing || pairing.status !== 'awaiting_confirmation') return;
    requestVersion += 1;
    busy = true;
    error = '';
    try {
      pairing = await api<DevicePairing>(
        `/api/v1/device-pairings/${pairing.id}/confirmation`,
        { method: 'POST' }
      );
      if (!reported) {
        reported = true;
        await onpaired();
      }
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      busy = false;
    }
  }

  async function copyLink() {
    if (!invitation) return;
    try {
      await navigator.clipboard.writeText(invitation.qr_payload);
      copied = true;
      error = '';
    } catch {
      error = t('devices.copyFailed');
    }
  }

  async function cancelCurrent() {
    if (!pairing || !pairingCanCancel(pairing.status)) return;
    try {
      await api<void>(`/api/v1/device-pairings/${pairing.id}`, { method: 'DELETE' });
      pairing = { ...pairing, status: 'cancelled' };
    } catch {
      // L'invitation s'éteint d'elle-même après dix minutes si le réseau coupe ici.
    }
  }

  async function close() {
    if (busy || closing) return;
    closing = true;
    requestVersion += 1;
    await cancelCurrent();
    onclose();
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') void close();
  }

  onMount(() => {
    void start();
    const clock = setInterval(() => (now = new Date()), 1000);
    const refresh = setInterval(() => void poll(), 1500);
    return () => {
      clearInterval(clock);
      clearInterval(refresh);
    };
  });

  onDestroy(() => {
    if (!closing) void cancelCurrent();
  });
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="scrim"
  role="presentation"
  onclick={(event) => event.currentTarget === event.target && void close()}
>
  <div
    class="modal pairing-modal"
    role="dialog"
    aria-modal="true"
    aria-labelledby="pairing-title"
    aria-busy={loading || busy}
  >
    <header>
      <div>
        <h2 id="pairing-title">{t('devices.pairingTitle')}</h2>
        <p>{t('devices.pairingLead')}</p>
      </div>
      <button class="close" type="button" onclick={() => void close()} disabled={busy} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    <div class="modal-body">
      {#if loading}
        <div class="pairing-loading" role="status">{t('devices.pairingCreating')}</div>
      {:else if !pairing || !invitation}
        <div class="pairing-failure">
          <strong>{t('devices.pairingUnavailable')}</strong>
          {#if error}<p class="error" role="alert">{error}</p>{/if}
          <button class="btn primary" type="button" onclick={start}>{t('common.retry')}</button>
        </div>
      {:else}
        <ol class="pairing-steps" aria-label={t('devices.pairingSteps')}>
          <li class:current={currentStep === 1} class:done={currentStep > 1}>
            <span class="step-number">1</span>
            <span><strong>{t('devices.stepScan')}</strong><small>{t('devices.stepScanHint')}</small></span>
          </li>
          <li class:current={currentStep === 2} class:done={currentStep > 2}>
            <span class="step-number">2</span>
            <span><strong>{t('devices.stepCheck')}</strong><small>{t('devices.stepCheckHint')}</small></span>
          </li>
          <li class:current={currentStep === 3} class:done={pairing.status === 'credential_consumed'}>
            <span class="step-number">3</span>
            <span><strong>{t('devices.stepConfirm')}</strong><small>{t('devices.stepConfirmHint')}</small></span>
          </li>
        </ol>

        <div class="pairing-grid">
          <section class="qr-tile" aria-labelledby="qr-heading">
            {#if pairing.status === 'awaiting_scan'}
              <div class="qr-heading">
                <h3 id="qr-heading">{t('devices.scanQRCode')}</h3>
                <span class="pill warn">{duration(remaining)}</span>
              </div>
              {#if qrDataURL}
                <img class="qr-code" src={qrDataURL} width="512" height="512" alt={t('devices.qrAlt')} />
              {/if}
              <p>{t('devices.scanInstruction')}</p>
            {:else if pairing.status === 'awaiting_confirmation'}
              <div class="claimed-mark" aria-hidden="true">{pairing.claimed_platform === 'ios' ? 'iOS' : 'Android'}</div>
              <h3 id="qr-heading">{pairing.claimed_name ?? t('devices.unknownDevice')}</h3>
              <p>{t('devices.claimedDevice', { platform: pairing.claimed_platform === 'ios' ? 'iOS' : 'Android' })}</p>
            {:else if pairing.status === 'confirmed'}
              <div class="claimed-mark confirmed" aria-hidden="true">✓</div>
              <h3 id="qr-heading">{t('devices.identityCreated')}</h3>
              <p>{t('devices.waitingForPhone')}</p>
            {:else if pairing.status === 'credential_consumed'}
              <div class="claimed-mark complete" aria-hidden="true">✓</div>
              <h3 id="qr-heading">{t('devices.phonePaired')}</h3>
              <p>{t('devices.phonePairedHint')}</p>
            {:else}
              <div class="claimed-mark" aria-hidden="true">—</div>
              <h3 id="qr-heading">{statusLabel(pairing)}</h3>
              <p>{t('devices.newInvitationHint')}</p>
            {/if}
          </section>

          <section class="pairing-state" aria-labelledby="state-heading">
            <span class="eyebrow">{t('devices.liveState')}</span>
            <h3 id="state-heading" aria-live="polite">{statusLabel(pairing)}</h3>

            {#if pairing.status === 'awaiting_scan'}
              <p>{t('devices.threeConfirmations')}</p>
              <div class="manual-link">
                <label for="pairing-link">{t('devices.manualLink')}</label>
                <div>
                  <input id="pairing-link" value={invitation.qr_payload} readonly onclick={(event) => event.currentTarget.select()} />
                  <button class="btn sm" type="button" onclick={copyLink}>
                    {copied ? t('devices.copied') : t('common.copy')}
                  </button>
                </div>
                <small>{t('devices.manualLinkHint')}</small>
              </div>
            {:else if pairing.status === 'awaiting_confirmation'}
              <dl class="claim-details">
                <div><dt>{t('devices.deviceName')}</dt><dd>{pairing.claimed_name}</dd></div>
                <div><dt>{t('devices.platform')}</dt><dd>{pairing.claimed_platform === 'ios' ? 'iOS' : 'Android'}</dd></div>
              </dl>
              <p class="security-note">{t('devices.confirmOnlyIfExpected')}</p>
            {:else if pairing.status === 'confirmed'}
              <p>{t('devices.keepOpen')}</p>
            {:else if pairing.status === 'credential_consumed'}
              <p>{t('devices.revokeReminder')}</p>
            {:else}
              <p>{t('devices.invitationEnded')}</p>
            {/if}

            {#if error}<p class="error" role="alert">{error}</p>{/if}
          </section>
        </div>
      {/if}
    </div>

    {#if pairing && invitation}
      <footer>
        {#if pairing.status === 'awaiting_scan'}
          <span class="faint note">{t('devices.expiresIn', { time: duration(remaining) })}</span>
          <button class="btn" type="button" onclick={() => void close()}>{t('devices.cancelInvitation')}</button>
        {:else if pairing.status === 'awaiting_confirmation'}
          <span class="faint note">{t('devices.webConfirmationRequired')}</span>
          <button class="btn" type="button" onclick={() => void close()} disabled={busy}>{t('common.cancel')}</button>
          <button class="btn primary" type="button" onclick={confirm} disabled={busy}>
            {busy ? t('devices.confirming') : t('devices.confirmDevice')}
          </button>
        {:else if pairing.status === 'expired' || pairing.status === 'cancelled'}
          <button class="btn primary" type="button" onclick={start}>{t('devices.createAnother')}</button>
        {:else}
          <span class="faint note">{pairing.status === 'confirmed' ? t('devices.phoneCollecting') : t('devices.pairingDone')}</span>
          <button class="btn primary" type="button" onclick={() => void close()}>{t('common.close')}</button>
        {/if}
      </footer>
    {/if}
  </div>
</div>

<style>
  .pairing-modal {
    max-width: 54rem;
  }

  .pairing-loading,
  .pairing-failure {
    min-height: 18rem;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s4);
    color: var(--muted);
    text-align: center;
  }

  .pairing-failure strong {
    color: var(--ink);
  }

  .pairing-steps {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--s3);
    margin: 0 0 var(--s5);
    padding: 0;
    list-style: none;
  }

  .pairing-steps li {
    display: flex;
    align-items: center;
    gap: var(--s3);
    min-width: 0;
    padding: var(--s3);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    color: var(--faint);
  }

  .pairing-steps li.current {
    border-color: var(--accent);
    color: var(--ink);
  }

  .pairing-steps li.done {
    border-color: var(--ok-line);
    background: var(--ok-bg);
    color: var(--ok);
  }

  .step-number {
    width: 1.5rem;
    height: 1.5rem;
    display: grid;
    place-items: center;
    flex: none;
    border: 1px solid currentColor;
    border-radius: var(--r-pill);
    font-family: var(--font-num);
    font-size: 0.6875rem;
  }

  .pairing-steps strong,
  .pairing-steps small {
    display: block;
  }

  .pairing-steps strong {
    font-size: 0.75rem;
    font-weight: 600;
  }

  .pairing-steps small {
    margin-top: var(--s1);
    font-size: 0.625rem;
  }

  .pairing-grid {
    display: grid;
    grid-template-columns: minmax(16rem, 0.9fr) minmax(18rem, 1.1fr);
    gap: var(--s4);
  }

  .qr-tile,
  .pairing-state {
    min-width: 0;
    padding: var(--s5);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-l);
    background: var(--bg);
  }

  .qr-tile {
    display: grid;
    align-content: center;
    justify-items: center;
    min-height: 21rem;
    text-align: center;
  }

  .qr-heading {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
  }

  .qr-tile h3,
  .pairing-state h3 {
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .qr-code {
    width: min(15rem, 100%);
    height: auto;
    margin: var(--s4) auto;
    border-radius: var(--r-m);
  }

  .qr-tile p,
  .pairing-state p {
    margin-top: var(--s3);
    color: var(--muted);
    font-size: 0.75rem;
    line-height: 1.5;
  }

  .claimed-mark {
    min-width: 4.5rem;
    height: 4.5rem;
    display: grid;
    place-items: center;
    margin-bottom: var(--s4);
    padding: 0 var(--s3);
    border: 1px solid var(--accent);
    border-radius: var(--r-l);
    color: var(--accent);
    font-family: var(--font-num);
    font-size: 1rem;
    font-weight: 600;
  }

  .claimed-mark.confirmed,
  .claimed-mark.complete {
    border-color: var(--ok-line);
    background: var(--ok-bg);
    color: var(--ok);
    font-size: 1.5rem;
  }

  .pairing-state {
    display: flex;
    flex-direction: column;
    justify-content: center;
  }

  .eyebrow {
    margin-bottom: var(--s3);
    color: var(--faint);
    font-size: 0.625rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .manual-link {
    margin-top: var(--s5);
  }

  .manual-link label,
  .manual-link small {
    display: block;
    font-size: 0.6875rem;
  }

  .manual-link label {
    margin-bottom: var(--s3);
    color: var(--ink);
    font-weight: 600;
  }

  .manual-link > div {
    display: flex;
    gap: var(--s3);
  }

  .manual-link input {
    width: 100%;
    min-width: 0;
    font-family: var(--font-num);
    font-size: 0.6875rem;
  }

  .manual-link small {
    margin-top: var(--s3);
    color: var(--faint);
    line-height: 1.45;
  }

  .claim-details {
    display: grid;
    gap: var(--s3);
    margin: var(--s5) 0 0;
  }

  .claim-details div {
    display: grid;
    grid-template-columns: 7rem minmax(0, 1fr);
    gap: var(--s3);
    padding: var(--s3) 0;
    border-bottom: 1px solid var(--line-row);
    font-size: 0.75rem;
  }

  .claim-details dt {
    color: var(--faint);
  }

  .claim-details dd {
    min-width: 0;
    color: var(--ink);
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .security-note {
    padding: var(--s3) var(--s4);
    border: 1px solid var(--warn-line);
    border-radius: var(--r-m);
    background: var(--warn-bg);
    color: var(--warn) !important;
  }

  .pairing-state .error {
    margin-top: var(--s4);
  }

  @media (max-width: 48rem) {
    .pairing-steps {
      grid-template-columns: 1fr;
    }

    .pairing-steps small {
      display: none;
    }

    .pairing-grid {
      grid-template-columns: 1fr;
    }

    .qr-tile {
      min-height: 0;
    }
  }
</style>
