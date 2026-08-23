import { api, type ReconciliationOperation, type ReconciliationSuggestion } from '$lib/api';
import { t } from '$lib/i18n.svelte';

class ReconciliationState {
  suggestions = $state<ReconciliationSuggestion[]>([]);
  operations = $state<ReconciliationOperation[]>([]);
  loading = $state(false);
  loaded = $state(false);
  error = $state('');

  get actionable() {
    return this.suggestions.filter((item) => item.confidence !== 'low');
  }

  get weak() {
    return this.suggestions.filter((item) => item.confidence === 'low');
  }

  get activeOperations() {
    return this.operations.filter((item) => item.status === 'queued' || item.status === 'running');
  }

  async load() {
    this.loading = true;
    this.error = '';
    try {
      const [suggestions, operations] = await Promise.all([
        api<{ suggestions: ReconciliationSuggestion[] }>('/api/v1/target-reconciliation/suggestions'),
        api<{ operations: ReconciliationOperation[] }>('/api/v1/target-reconciliation/operations?limit=25')
      ]);
      this.suggestions = suggestions.suggestions;
      this.operations = operations.operations;
      this.loaded = true;
    } catch (cause) {
      this.error = cause instanceof Error ? cause.message : t('reconciliation.unavailable');
    } finally {
      this.loading = false;
    }
  }

  async reject(id: string, reason = '') {
    await api(`/api/v1/target-reconciliation/suggestions/${id}/rejection`, {
      method: 'POST',
      body: JSON.stringify({ reason })
    });
    this.suggestions = this.suggestions.filter((item) => item.id !== id);
  }

  async snooze(id: string, until: Date, reason = '') {
    await api(`/api/v1/target-reconciliation/suggestions/${id}/snooze`, {
      method: 'POST',
      body: JSON.stringify({ until: until.toISOString(), reason })
    });
    this.suggestions = this.suggestions.filter((item) => item.id !== id);
  }
}

export const reconciliationState = new ReconciliationState();
