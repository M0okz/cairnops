// Package testsupport donne à chaque test d'intégration une base isolée.
//
// Les tests écrivent tous dans la même instance PostgreSQL, et Go exécute les
// paquets en parallèle : un test qui interroge une file de travail — les
// Incidents actifs, la boîte d'envoi des notifications, les Sources dues —
// verrait sinon ce que ses voisins viennent d'y déposer. Le remède n'est pas
// d'apprendre à chaque test à reconnaître ce qui lui appartient, mais de ne
// jamais lui montrer autre chose.
package testsupport

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/database"
	"github.com/M0okz/cairnops/internal/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	adminOnce sync.Once
	adminPool *pgxpool.Pool
	adminErr  error
	schemaSeq atomic.Uint64
)

// Pool ouvre une base de test dont ce test est le seul occupant : un schéma
// PostgreSQL créé pour lui, peuplé par les migrations, et supprimé avec tout ce
// qu'il contient à la fin. Le test n'a donc ni à nommer ses fixtures de façon
// unique, ni à les effacer, ni à filtrer ce qu'il lit.
//
// Sans CAIRNOPS_TEST_DATABASE_URL, le test est sauté : une suite verte ne prouve
// rien tant que cette variable n'est pas renseignée.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("CAIRNOPS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CAIRNOPS_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()

	admin, err := openAdmin(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open the administration pool: %v", err)
	}
	schema := schemaName()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create the test schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Errorf("drop the test schema %s: %v", schema, err)
		}
	})

	scoped, err := scopedURL(databaseURL, schema)
	if err != nil {
		t.Fatalf("scope the database URL to %s: %v", schema, err)
	}
	pool, err := database.Open(ctx, scoped)
	if err != nil {
		t.Fatalf("open the test pool on %s: %v", schema, err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply the migrations to %s: %v", schema, err)
	}
	return pool
}

// openAdmin partage une seule connexion d'administration pour tout le paquet :
// elle ne sert qu'à créer et supprimer les schémas.
func openAdmin(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	adminOnce.Do(func() {
		adminPool, adminErr = database.Open(ctx, databaseURL)
	})
	return adminPool, adminErr
}

// schemaName reste unique même quand deux paquets démarrent dans la même
// milliseconde : l'horloge donne la lisibilité, le compteur et le processus
// donnent l'unicité.
func schemaName() string {
	return fmt.Sprintf("cairnops_test_%d_%d_%d", os.Getpid(), time.Now().UnixNano(), schemaSeq.Add(1))
}

// scopedURL enferme la connexion dans le schéma du test. Les migrations créent
// leurs tables sans les qualifier : elles atterrissent donc là, et nulle part
// ailleurs.
func scopedURL(databaseURL, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
