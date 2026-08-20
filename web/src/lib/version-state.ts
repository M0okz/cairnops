export type VersionState = {
  currentVersion: string;
  currentKnown: boolean;
  availableVersion: string;
};

/* La version courante reste celle que cet écran a chargée. Une version
 * observée différente n'écrase rien : elle se signale séparément jusqu'au
 * rechargement choisi par l'utilisateur. */
export function absorbObservedVersion(
  state: VersionState,
  observedVersion?: string
): VersionState {
  const observed = observedVersion?.trim() ?? '';
  if (!observed) return state;

  if (!state.currentKnown || state.currentVersion.trim().length === 0) {
    return {
      currentVersion: observed,
      currentKnown: true,
      availableVersion: ''
    };
  }

  return {
    ...state,
    availableVersion: observed === state.currentVersion ? '' : observed
  };
}
