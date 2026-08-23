export const activeReconciliationPollingDelay = 2_000;
export const idleReconciliationPollingDelay = 30_000;

type TimerHandle = ReturnType<typeof setTimeout>;
type Schedule = (callback: () => void, delay: number) => TimerHandle;
type Cancel = (handle: TimerHandle) => void;

// Le prochain relevé n'est planifié qu'après la fin du précédent. La lecture
// de l'état actif se produit hors de l'effet Svelte qui démarre ce cycle : une
// réponse ne peut donc pas recréer immédiatement le minuteur qui l'a produite.
export function startReconciliationPolling(
  load: () => Promise<void>,
  hasActiveOperations: () => boolean,
  schedule: Schedule = (callback, delay) => setTimeout(callback, delay),
  cancel: Cancel = (handle) => clearTimeout(handle)
): () => void {
  let stopped = false;
  let timer: TimerHandle | undefined;

  const poll = async () => {
    await load();
    if (stopped) return;
    timer = schedule(
      () => {
        timer = undefined;
        void poll();
      },
      hasActiveOperations() ? activeReconciliationPollingDelay : idleReconciliationPollingDelay
    );
  };

  void poll();
  return () => {
    stopped = true;
    if (timer !== undefined) cancel(timer);
  };
}
