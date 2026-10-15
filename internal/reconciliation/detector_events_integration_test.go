package reconciliation

import (
	"context"
	"log/slog"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le détecteur reconduit chaque suggestion à chaque passage. Tant que ses
// preuves et sa confiance ne bougent pas, il n'y a rien à annoncer.
func TestDetectorRefreshWithoutChangeAppendsNoReconciliationEvent(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	actorID := insertReconciliationUser(t, ctx, pool)
	leftID := insertReconciliationTarget(t, ctx, pool, "Authentik")
	rightID := insertReconciliationTarget(t, ctx, pool, "Authentik")
	insertNativeSource(t, ctx, pool, leftID, "Authentik")
	insertNativeSource(t, ctx, pool, rightID, "Authentik")

	detector := NewDetector(pool, slog.Default())
	if err := detector.Refresh(ctx); err != nil {
		t.Fatalf("refresh detector: %v", err)
	}
	suggestions, err := NewStore(pool).ListSuggestions(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected one suggestion, got %#v", suggestions)
	}
	before := countReconciliationEvents(t, ctx, pool)

	for pass := 1; pass <= 10; pass++ {
		if err := detector.Refresh(ctx); err != nil {
			t.Fatalf("refresh detector: %v", err)
		}
	}
	if got := countReconciliationEvents(t, ctx, pool) - before; got != 0 {
		t.Fatalf("10 unchanged detector passes appended %d reconciliation.changed, want 0", got)
	}

	if _, err := NewStore(pool).RejectSuggestion(ctx, actorID, suggestions[0].ID, "Deux instances distinctes"); err != nil {
		t.Fatal(err)
	}
	if got := countReconciliationEvents(t, ctx, pool) - before; got != 1 {
		t.Fatalf("a rejection appended %d reconciliation.changed, want 1", got)
	}
}

func countReconciliationEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_events WHERE kind = 'reconciliation.changed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
