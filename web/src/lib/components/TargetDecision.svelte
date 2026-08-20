<script lang="ts">
  import type { MatchEvidence, TargetMatch, TargetReference } from '$lib/api';
  import { REVIEW_TARGET } from '$lib/reconciliation';
  import { plural, t } from '$lib/i18n.svelte';

  let {
    name,
    value,
    candidates = [],
    availableTargets,
    disabled = false,
    onselect
  }: {
    name: string;
    value: string;
    candidates?: TargetMatch[];
    availableTargets: TargetReference[];
    disabled?: boolean;
    onselect: (targetID: string) => void;
  } = $props();

  const candidateIDs = $derived(new Set(candidates.map((match) => match.target.id)));
  const otherTargets = $derived(availableTargets.filter((target) => !candidateIDs.has(target.id)));
  const selectedMatch = $derived(candidates.find((match) => match.target.id === value));

  function evidenceLabel(evidence: MatchEvidence) {
    if (evidence.kind === 'same_ip') return t('wizard.sameIP', { value: evidence.value });
    if (evidence.kind === 'same_hostname') return t('wizard.sameHostname', { value: evidence.value });
    if (evidence.kind === 'similar_name') return t('wizard.similarInfrastructureName', { value: evidence.value });
    return t('wizard.sameName', { value: evidence.value });
  }
</script>

<div class="decision" class:needs-review={value === REVIEW_TARGET}>
  <select
    aria-label={t('wizard.targetChoice', { name })}
    {value}
    {disabled}
    onchange={(event) => onselect(event.currentTarget.value)}
  >
    {#if candidates.length > 0}
      <option value={REVIEW_TARGET} disabled>{t('wizard.chooseTarget')}</option>
      <optgroup label={t('wizard.suggestions')}>
        {#each candidates as match (match.target.id)}
          <option value={match.target.id}>
            {match.target.name} — {match.confidence === 'high'
              ? t('wizard.highConfidence')
              : match.confidence === 'medium'
                ? t('wizard.mediumConfidence')
                : t('wizard.lowConfidence')}
          </option>
        {/each}
      </optgroup>
    {/if}
    {#if otherTargets.length > 0}
      <optgroup label={t('wizard.otherTargets')}>
        {#each otherTargets as target (target.id)}
          <option value={target.id}>{target.name}</option>
        {/each}
      </optgroup>
    {/if}
    <optgroup label={t('wizard.creation')}>
      <option value="">{t('wizard.createNewTarget')}</option>
    </optgroup>
  </select>

  <div class="proof" aria-live="polite">
    {#if value === REVIEW_TARGET}
      <span class="pill warn">{t('wizard.toReview')}</span>
      <small>{plural('wizard.possibleMatches', candidates.length)}</small>
    {:else if selectedMatch}
      <span class:ok={selectedMatch.confidence === 'high'} class:info={selectedMatch.confidence === 'medium'} class:warn={selectedMatch.confidence === 'low'} class="pill">
        {selectedMatch.confidence === 'high'
          ? t('wizard.highConfidence')
          : selectedMatch.confidence === 'medium'
            ? t('wizard.suggestion')
            : t('wizard.lowConfidence')}
      </span>
      <small>{selectedMatch.evidence.map(evidenceLabel).join(' · ')}</small>
    {:else if value}
      <span class="pill">{t('wizard.manual')}</span>
      <small>{t('wizard.manualChoice')}</small>
    {:else}
      <span class="pill">{t('wizard.new')}</span>
      <small>{t('wizard.newTarget')}</small>
    {/if}
  </div>
</div>

<style>
  .decision select {
    width: 100%;
    min-height: 2.75rem;
    padding: 0 var(--s3);
    border: 1px solid var(--line-strong);
    border-radius: var(--r-s);
    background: var(--surface);
    color: var(--ink);
    font: inherit;
    font-size: 0.6875rem;
  }

  .decision.needs-review select {
    border-color: var(--warn);
    box-shadow: 0 0 0 0.125rem color-mix(in srgb, var(--warn) 12%, transparent);
  }

  .decision select:focus-visible {
    border-color: var(--accent);
    outline: 2px solid color-mix(in srgb, var(--accent) 25%, transparent);
    outline-offset: 1px;
  }

  .proof {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s2);
    margin-top: 0.25rem;
    min-width: 0;
  }

  .proof .pill {
    flex: none;
    padding: 0.125rem 0.375rem;
    font-size: 0.5625rem;
  }

  .proof small {
    min-width: 0;
    overflow: hidden;
    color: var(--faint);
    font-size: 0.625rem;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (max-width: 40rem) {
    .proof {
      justify-content: flex-start;
    }
  }
</style>
