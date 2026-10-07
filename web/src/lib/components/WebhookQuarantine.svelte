<script lang="ts">
  import { plural, t } from '$lib/i18n.svelte';
  import Icon from './Icon.svelte';
  import Modal from './ui/Modal.svelte';
  import Button from './ui/Button.svelte';
  import Badge from './ui/Badge.svelte';
  import { onMount } from 'svelte';
  import { api, type Connector, type Target, type WebhookApproval, type WebhookQuarantine } from '$lib/api';

  let {
    connector,
    targets,
    onclose,
    onsuccess
  }: {
    connector: Connector;
    targets: Target[];
    onclose: () => void;
    onsuccess: (approval: WebhookApproval) => Promise<void> | void;
  } = $props();

  let items = $state<WebhookQuarantine[]>([]);
  let choices = $state<Record<string, string>>({});
  let loading = $state(true);
  let busy = $state('');
  let error = $state('');

  onMount(load);


  async function load() {
    loading = true;
    error = '';
    try {
      const response = await api<{ quarantine: WebhookQuarantine[] }>(`/api/v1/connectors/${connector.id}/quarantine`);
      items = response.quarantine;
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'La quarantaine est indisponible.';
    } finally {
      loading = false;
    }
  }

  async function approve(item: WebhookQuarantine) {
    busy = item.id;
    error = '';
    try {
      const targetID = choices[item.id] ?? '';
      const approval = await api<WebhookApproval>(`/api/v1/connectors/${connector.id}/quarantine/${item.id}/approve`, {
        method: 'POST',
        body: JSON.stringify(targetID ? { target_id: targetID } : {})
      });
      items = items.filter((candidate) => candidate.external_identity !== approval.identity);
      await onsuccess(approval);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : t('quarantine.failed');
    } finally {
      busy = '';
    }
  }

  function exactTarget(item: WebhookQuarantine) {
    const expected = item.target_name.trim().toLocaleLowerCase('fr');
    return targets.find((target) => target.name.trim().toLocaleLowerCase('fr') === expected);
  }
</script>

<Modal dismissible={!busy} {onclose}>
  {#snippet children(dialog)}
    <div {...dialog} class="modal" aria-labelledby="quarantine-title">
      <header>
        <div>
          <h2 id="quarantine-title">Qui peut produire une preuve ?</h2>
          <p>
            {t('quarantine.lead', { name: connector.name })}
          </p>
        </div>
        <Button variant="close" onclick={onclose} disabled={Boolean(busy)} aria-label="Fermer">
          <Icon name="close" size={14} />
        </Button>
      </header>

      <div class="modal-body">
        {#if loading}
          <p class="faint">{t('quarantine.loading')}</p>
        {:else if items.length === 0}
          <div class="empty">
            <strong>{t('quarantine.empty')}</strong>
            {t('quarantine.emptyHint')}
          </div>
        {:else}
          <ul class="list">
            {#each items as item (item.id)}
              <li>
                <div class="line">
                  <i class="dot {item.status === 'firing' ? 'crit' : 'ok'}"></i>
                  <span class="identity">
                    <strong class="mono">{item.external_identity}</strong>
                    <small class="faint">
                      {plural('quarantine.messages', item.occurrences)} ·
                      dernier {new Date(item.last_seen_at).toLocaleString('fr-FR', { dateStyle: 'short', timeStyle: 'short' })}
                    </small>
                  </span>
                  <Badge tone={item.status === 'firing' ? 'crit' : 'ok'}>
                    {item.status === 'firing' ? t('quarantine.firing') : t('quarantine.resolved')}
                  </Badge>
                </div>

                <p class="summary">
                  {item.summary}
                  <span class="faint mono">· {item.severity} · {item.event_key}</span>
                </p>

                <div class="decide">
                  <div class="field">
                    <label for={`target-${item.id}`}>Cible de confiance</label>
                    <select
                      id={`target-${item.id}`}
                      value={choices[item.id] ?? ''}
                      onchange={(event) => (choices[item.id] = event.currentTarget.value)}
                    >
                      <option value="">
                        {exactTarget(item)
                          ? t('quarantine.reuse', { name: exactTarget(item)?.name ?? '' })
                          : t('quarantine.create', { name: item.target_name })}
                      </option>
                      {#each targets as target (target.id)}
                        <option value={target.id}>{target.name}</option>
                      {/each}
                    </select>
                  </div>
                  <Button variant="primary" onclick={() => approve(item)} disabled={Boolean(busy)}>
                    {busy === item.id ? 'Autorisation…' : 'Autoriser et rejouer'}
                  </Button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}

        {#if error}<p class="error" role="alert">{error}</p>{/if}
      </div>

      <footer>
        <span class="faint note">
          {t('quarantine.note')}
        </span>
      </footer>
    </div>
  {/snippet}
</Modal>

<style>
  .list {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--s4);
  }

  li {
    padding: var(--s4);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-m);
    background: var(--bg);
  }

  .line {
    display: flex;
    align-items: center;
    gap: 0.5625rem;
  }

  .identity {
    flex: 1;
    min-width: 0;
  }

  .identity strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .identity small {
    display: block;
    margin-top: 2px;
    font-size: 0.6875rem;
  }

  .summary {
    margin: var(--s3) 0 var(--s4);
    color: var(--muted);
    font-size: 0.75rem;
  }

  .decide {
    display: flex;
    align-items: flex-end;
    gap: var(--s3);
  }

  .decide .field {
    flex: 1;
    min-width: 0;
    margin: 0;
  }


  @media (max-width: 34rem) {
    .decide {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
