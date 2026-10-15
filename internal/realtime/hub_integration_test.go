package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ces tests ne sont pas parallèles : ils reconnaissent la connexion d'écoute
// à son nom d'application, qu'un voisin du même paquet porterait aussi.

func TestPostgresHubDeliversAppendedEventsOnNotify(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	appendEvent(t, pool, "before-start")
	existing, err := NewStore(pool).LatestVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Le sondage de secours est repoussé hors de portée du test : seule la
	// notification peut réveiller le hub.
	hub := startHub(t, NewStore(pool), NewPostgresListener(pool), HubOptions{FallbackInterval: time.Hour})
	subscription := subscribe(t, hub)
	if subscription.Version() != existing {
		t.Fatalf("expected the subscription to start from %d, got %d", existing, subscription.Version())
	}
	listenerPID(t, pool, 0)

	transaction, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		if _, err := transaction.Exec(ctx, "SELECT cairnops_append_event('target.changed', 'target', $1)", id); err != nil {
			t.Fatal(err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	appendEvent(t, pool, "third")

	for _, want := range []string{"first", "second", "third"} {
		event := receive(t, subscription)
		if event.EntityID != want || event.Kind != "target.changed" || event.EntityType != "target" {
			t.Fatalf("expected the %s event, got %#v", want, event)
		}
	}
	expectNothing(t, subscription)
}

func TestPostgresHubCatchesUpAfterItsListenerIsTerminated(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	hub := startHub(t, NewStore(pool), NewPostgresListener(pool), HubOptions{
		FallbackInterval: time.Hour,
		RetryDelay:       100 * time.Millisecond,
	})
	subscription := subscribe(t, hub)
	terminated := listenerPID(t, pool, 0)

	if _, err := pool.Exec(ctx, "SELECT pg_terminate_backend($1)", terminated); err != nil {
		t.Fatal(err)
	}
	// Enregistré pendant que l'écoute est rompue : sa notification est perdue,
	// seul le rattrapage au rétablissement peut l'apporter.
	appendEvent(t, pool, "during-outage")
	listenerPID(t, pool, terminated)
	appendEvent(t, pool, "after-reconnection")

	for _, want := range []string{"during-outage", "after-reconnection"} {
		if event := receive(t, subscription); event.EntityID != want {
			t.Fatalf("expected the %s event, got %#v", want, event)
		}
	}
}

func appendEvent(t *testing.T, pool *pgxpool.Pool, entityID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "SELECT cairnops_append_event('target.changed', 'target', $1)", entityID); err != nil {
		t.Fatalf("append event %s: %v", entityID, err)
	}
}

// listenerPID attend une connexion d'écoute établie autre que previous et
// renvoie son processus serveur.
func listenerPID(t *testing.T, pool *pgxpool.Pool, previous int32) int32 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var pid int32
		err := pool.QueryRow(context.Background(), `
			SELECT pid FROM pg_stat_activity
			WHERE datname = current_database()
			  AND application_name = $1
			  AND query ILIKE 'LISTEN%'
			  AND pid <> $2
			LIMIT 1
		`, ListenerApplicationName, previous).Scan(&pid)
		if err == nil {
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("no realtime listener connection appeared: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
