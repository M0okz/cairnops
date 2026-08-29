// Ces écrans portent déjà l'accès à la file dans leur propre contexte. La
// barre supérieure le conserve partout ailleurs, ainsi que pendant un travail
// actif, afin de ne pas transformer une notification globale en doublon.
const reconciliationReviewContexts = new Set(['/cibles', '/cibles/rapprochements']);

export function showReconciliationReviewInTopbar(pathname: string): boolean {
  const normalizedPath = pathname.replace(/\/+$/, '') || '/';
  return !reconciliationReviewContexts.has(normalizedPath);
}
