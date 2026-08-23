<script lang="ts">
  import { t } from '$lib/i18n.svelte';
  import Icon from './Icon.svelte';
  import { onMount } from 'svelte';
  import { api, type CreatedSource, type IncidentSeverity, type SourceKind, type Target } from '$lib/api';

  let {
    onclose,
    onsuccess,
    target: existingTarget = null
  }: {
    onclose: () => void;
    onsuccess: (target: Target, created: CreatedSource) => Promise<void> | void;
    target?: Target | null;
  } = $props();

  let nameInput = $state<HTMLInputElement>();
  let targetName = $state('');
  let description = $state('');
  let sourceName = $state('');
  let kind = $state<SourceKind>('http');
  let intervalSeconds = $state(60);
  let timeoutMilliseconds = $state(5000);
  let address = $state('');
  let httpMethod = $state('GET');
  let acceptedStatus = $state(200);
  let contains = $state('');
  let tls = $state(false);
  let serverName = $state('');
  let dnsType = $state('A');
  let dnsServer = $state('');
  let expected = $state('');
  let family = $state('auto');
  let graceSeconds = $state(60);
  let failureThreshold = $state(3);
  let recoveryThreshold = $state(2);
  let severity = $state<IncidentSeverity>('major');
  let busy = $state(false);
  let error = $state('');
  let createdTarget = $state<Target | null>(null);
  let receipt = $state<CreatedSource | null>(null);

  $effect(() => {
    if (!createdTarget && existingTarget) createdTarget = existingTarget;
  });
  let copied = $state(false);

  const kindLabels: Record<SourceKind, string> = {
    http: 'HTTP',
    tcp: 'TCP',
    dns: 'DNS',
    icmp: 'ICMP',
    heartbeat: 'Heartbeat'
  };

  onMount(() => nameInput?.focus());

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onclose();
  }

  function sourceConfig(): Record<string, unknown> {
    if (kind === 'http') return { url: address, method: httpMethod, accepted_statuses: [acceptedStatus], ...(contains ? { contains } : {}) };
    if (kind === 'tcp') return { address, ...(tls ? { tls: true, server_name: serverName || undefined } : {}) };
    if (kind === 'dns') return { name: address, type: dnsType, ...(dnsServer ? { server: dnsServer } : {}), ...(expected ? { expected: expected.split(',').map((value) => value.trim()).filter(Boolean) } : {}) };
    if (kind === 'icmp') return { host: address, family };
    return { expected_every_seconds: intervalSeconds, grace_seconds: graceSeconds, activated: false };
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      if (!createdTarget) {
        createdTarget = await api<Target>('/api/v1/targets', {
          method: 'POST',
          body: JSON.stringify({ name: targetName, description })
        });
      }
      const created = await api<CreatedSource>(`/api/v1/targets/${createdTarget.id}/sources`, {
        method: 'POST',
        body: JSON.stringify({
          name: sourceName || `${kindLabels[kind]} principal`,
          kind,
          interval_seconds: intervalSeconds,
          timeout_milliseconds: timeoutMilliseconds,
          failure_threshold: failureThreshold,
          recovery_threshold: recoveryThreshold,
          severity,
          config: sourceConfig()
        })
      });
      receipt = created;
      await onsuccess(createdTarget, created);
      if (kind !== 'heartbeat') onclose();
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : t('workshop.createFailed');
      error = createdTarget ? t('workshop.partialCreate', { error: message }) : message;
    } finally {
      busy = false;
    }
  }

  async function copyHeartbeat() {
    if (!receipt?.heartbeat_path) return;
    await navigator.clipboard.writeText(`${location.origin}${receipt.heartbeat_path}`);
    copied = true;
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="scrim" role="presentation" onclick={(event) => event.currentTarget === event.target && !busy && onclose()}>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="workshop-title">
    <header>
      <div>
        <h2 id="workshop-title">{receipt ? t('workshop.keepSecret') : existingTarget ? t('target.addCheck') : t('workshop.createTarget')}</h2>
        <p>
          {receipt
            ? t('workshop.secretSay')
            : existingTarget ? t('workshop.addCheckSay', { name: existingTarget.name }) : t('workshop.createSay')}
        </p>
      </div>
      <button class="close" type="button" onclick={onclose} disabled={busy} aria-label={t('common.close')}>
        <Icon name="close" size={14} />
      </button>
    </header>

    {#if receipt?.heartbeat_path}
      <div class="modal-body">
        <div class="banner warn">
          <i class="dot warn"></i>
          <div>
            <strong>{t('workshop.heartbeatReady')}</strong>
            <p class="muted">{t('workshop.heartbeatOnce')}</p>
          </div>
        </div>
        <div class="credential">
          <span class="label faint">{t('workshop.heartbeatAddress')}</span>
          <code>{location.origin}{receipt.heartbeat_path}</code>
          <button class="btn sm" type="button" onclick={copyHeartbeat}>
            {copied ? t('workshop.copied') : t('workshop.copy')}
          </button>
        </div>
      </div>
      <footer>
        <button class="btn primary" type="button" onclick={onclose}>{t('workshop.addressKept')}</button>
      </footer>
    {:else}
      <form onsubmit={submit}>
        <div class="modal-body">
          {#if !existingTarget}
            <section>
              <h3>{t('targets.column.target')}</h3>
              <div class="grid">
                <div class="field">
                  <label for="target-name">{t('workshop.targetName')}</label>
                  <input id="target-name" bind:this={nameInput} bind:value={targetName} required maxlength="160"
                    placeholder="Nextcloud" disabled={createdTarget !== null} />
                </div>
                <div class="field">
                  <label for="target-description">{t('workshop.optionalHint')}</label>
                  <input id="target-description" bind:value={description} maxlength="2000"
                    placeholder="cloud.homeblack.fr" disabled={createdTarget !== null} />
                </div>
              </div>
            </section>
          {/if}

          <section>
            <h3>{t('workshop.signalSource')}</h3>
            <div class="segments kinds" role="group" aria-label={t('workshop.checkKind')}>
              {#each Object.entries(kindLabels) as [value, label] (value)}
                <button type="button" aria-pressed={kind === value} onclick={() => (kind = value as typeof kind)}>
                  {label}
                </button>
              {/each}
            </div>

            <div class="grid">
              <div class="field">
                <label for="source-name">{t('workshop.sourceName')}</label>
                <input id="source-name" bind:value={sourceName} maxlength="160"
                  placeholder={`${kindLabels[kind]} principal`} />
              </div>

              {#if kind !== 'heartbeat'}
                <div class="field">
                  <label for="address">
                    {kind === 'http'
                      ? t('workshop.absoluteUrl')
                      : kind === 'tcp'
                        ? t('workshop.hostAndPort')
                        : kind === 'dns'
                          ? t('workshop.nameToResolve')
                          : t('workshop.hostOrIp')}
                  </label>
                  <input id="address" bind:value={address} required
                    placeholder={kind === 'http' ? 'https://cloud.example.net/status.php' : kind === 'tcp' ? 'mail.example.net:443' : 'example.net'} />
                </div>
              {/if}

              {#if kind === 'http'}
                <div class="field">
                  <label for="method">{t('workshop.method')}</label>
                  <select id="method" bind:value={httpMethod}><option>GET</option><option>HEAD</option><option>POST</option></select>
                </div>
                <div class="field">
                  <label for="status">{t('workshop.expectedStatus')}</label>
                  <input id="status" type="number" bind:value={acceptedStatus} min="100" max="599" />
                </div>
                <div class="field wide">
                  <label for="contains">{t('workshop.expectedText')}</label>
                  <input id="contains" bind:value={contains} placeholder="status: ok" />
                </div>
              {:else if kind === 'tcp'}
                <div class="field wide">
                  <label class="inline" for="tls">
                    <input id="tls" type="checkbox" bind:checked={tls} />
                    {t('workshop.negotiateTls')}
                  </label>
                </div>
                {#if tls}
                  <div class="field">
                    <label for="server-name">{t('workshop.tlsName')}</label>
                    <input id="server-name" bind:value={serverName} placeholder="mail.example.net" />
                  </div>
                {/if}
              {:else if kind === 'dns'}
                <div class="field">
                  <label for="dns-type">{t('workshop.dnsType')}</label>
                  <select id="dns-type" bind:value={dnsType}>
                    <option>A</option><option>AAAA</option><option>CNAME</option>
                    <option>MX</option><option>TXT</option><option>NS</option>
                  </select>
                </div>
                <div class="field">
                  <label for="dns-server">{t('workshop.resolver')}</label>
                  <input id="dns-server" bind:value={dnsServer} placeholder="1.1.1.1:53" />
                </div>
                <div class="field wide">
                  <label for="expected">{t('workshop.expectedValues')}</label>
                  <input id="expected" bind:value={expected} />
                </div>
              {:else if kind === 'icmp'}
                <div class="field">
                  <label for="family">{t('workshop.ipFamily')}</label>
                  <select id="family" bind:value={family}>
                    <option value="auto">{t('workshop.automatic')}</option><option value="ipv4">IPv4</option><option value="ipv6">IPv6</option>
                  </select>
                </div>
              {:else}
                <div class="field">
                  <label for="grace">{t('workshop.graceSeconds')}</label>
                  <input id="grace" type="number" bind:value={graceSeconds} min="0" max="86400" />
                </div>
              {/if}

              <div class="field">
                <label for="interval">{t('workshop.intervalSeconds')}</label>
                <input id="interval" type="number" bind:value={intervalSeconds} min="20" max="86400" required />
              </div>
              <div class="field">
                <label for="timeout">{t('workshop.timeoutMs')}</label>
                <input id="timeout" type="number" bind:value={timeoutMilliseconds} min="100" max="60000" required />
              </div>
            </div>
          </section>

          <section>
            <h3>{t('workshop.triggerPolicy')}</h3>
            <div class="grid">
              <div class="field">
                <label for="failure-threshold">{t('workshop.failureThreshold')}</label>
                <input id="failure-threshold" type="number" bind:value={failureThreshold} min="1" max="10" required />
              </div>
              <div class="field">
                <label for="recovery-threshold">{t('workshop.recoveryThreshold')}</label>
                <input id="recovery-threshold" type="number" bind:value={recoveryThreshold} min="1" max="10" required />
              </div>
              <div class="field">
                <label for="severity">{t('workshop.reportedSeverity')}</label>
                <select id="severity" bind:value={severity}>
                  <option value="information">Information</option>
                  <option value="warning">Avertissement</option>
                  <option value="major">Majeur</option>
                  <option value="critical">Critique</option>
                </select>
              </div>
            </div>
            <p class="faint note">{t('workshop.unknownConcludes')}</p>
          </section>

          {#if error}<p class="error" role="alert">{error}</p>{/if}
        </div>

        <footer>
          <span class="faint note">{t('workshop.runFromInstance')}</span>
          <button class="btn" type="button" onclick={onclose} disabled={busy}>Annuler</button>
          <button class="btn primary" type="submit" disabled={busy}>
            {busy
              ? t('workshop.creating')
              : createdTarget
                ? t('workshop.retrySource')
                : t('workshop.startSupervising')}
          </button>
        </footer>
      </form>
    {/if}
  </div>
</div>

<style>
  .modal {
    max-width: 42rem;
  }

  section {
    padding-bottom: var(--s5);
    margin-bottom: var(--s5);
    border-bottom: 1px solid var(--line);
  }

  section:last-of-type {
    padding-bottom: 0;
    margin-bottom: 0;
    border-bottom: 0;
  }

  h3 {
    margin-bottom: var(--s4);
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
    gap: var(--s4);
  }

  .grid .field {
    margin: 0;
  }

  .wide {
    grid-column: 1 / -1;
  }

  .kinds {
    width: 100%;
    margin-bottom: var(--s4);
  }

  .kinds button {
    flex: 1;
  }

  label.inline {
    display: flex;
    align-items: center;
    gap: var(--s3);
    color: var(--muted);
  }

  label.inline input {
    width: auto;
    height: auto;
  }

  .note {
    font-size: 0.6875rem;
  }

  .modal-body .note {
    margin-top: var(--s3);
  }


  .credential {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: center;
    gap: var(--s3);
    margin-top: var(--s4);
    padding: var(--s3) var(--s4);
    border: 1px solid var(--warn-line);
    border-radius: var(--r-m);
    background: var(--bg);
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

  .banner strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .banner p {
    margin-top: 0.25rem;
    font-size: 0.75rem;
  }
</style>
