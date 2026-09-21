package connectors

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/M0okz/cairnops/internal/connectors/zabbix"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func discoveryConnector(t *testing.T, pool *pgxpool.Pool, kind string, initialized bool) RuntimeConnector {
	t.Helper()
	c := RuntimeConnector{Endpoint: "https://" + kind + ".example.test"}
	err := pool.QueryRow(context.Background(), `INSERT INTO cairnops_connectors (kind,name,endpoint,credential_sealed,status,encrypted_transport,discovery_initialized,lease_owner,lease_until) VALUES ($1,$1,$2,repeat('x',40),'connected',true,$3,'worker',now()+interval '1 minute') RETURNING id::text`, kind, c.Endpoint, initialized).Scan(&c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func discovered(id, name string) DiscoveryObject {
	return DiscoveryObject{ExternalID: id, Name: name, Eligible: true, Identity: DiscoveredIdentity{Names: []string{name}}, Metadata: map[string]any{}}
}

func TestDiscoveryCreatesSourcesOnceForEveryFamily(t *testing.T) {
	for _, kind := range []string{"zabbix", "uptime_kuma", "patchmon", "argus"} {
		t.Run(kind, func(t *testing.T) {
			pool := testsupport.Pool(t)
			ctx := context.Background()
			store := NewPostgresStore(pool)
			c := discoveryConnector(t, pool, kind, true)
			objects := []DiscoveryObject{discovered("one", "Unique machine")}
			first, err := store.RefreshDiscovery(ctx, c, "worker", kind, objects)
			if err != nil {
				t.Fatal(err)
			}
			second, err := store.RefreshDiscovery(ctx, c, "worker", kind, objects)
			if err != nil {
				t.Fatal(err)
			}
			if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID {
				t.Fatalf("not idempotent: %v %v", first, second)
			}
			var sources int
			var availability bool
			err = pool.QueryRow(ctx, `SELECT count(*),bool_and(measures_availability) FROM cairnops_signal_sources WHERE connector_binding_id=$1::uuid`, first[0].ID).Scan(&sources, &availability)
			if err != nil {
				t.Fatal(err)
			}
			if sources != 1 || availability != (kind == "zabbix" || kind == "uptime_kuma") {
				t.Fatalf("wrong source semantics %d %v", sources, availability)
			}
		})
	}
}

func TestDiscoveryPreservesBaselineExclusionsAndEligibility(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	store := NewPostgresStore(pool)
	c := discoveryConnector(t, pool, "argus", false)
	ignored := discovered("ignored", "Excluded object")
	inactive := discovered("inactive", "Later eligible")
	inactive.Eligible = false
	bindings, err := store.RefreshDiscovery(ctx, c, "worker", "argus", []DiscoveryObject{ignored, inactive})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 0 {
		t.Fatal("baseline imported existing inventory")
	}
	inactive.Eligible = true
	fresh := discovered("fresh", "Fresh application")
	bindings, err = store.RefreshDiscovery(ctx, c, "worker", "argus", []DiscoveryObject{ignored, inactive, fresh})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected new and newly eligible objects: %v", bindings)
	}
	if _, err = store.RefreshDiscovery(ctx, c, "worker", "argus", nil); err != nil {
		t.Fatal(err)
	}
	bindings, err = store.RefreshDiscovery(ctx, c, "worker", "argus", []DiscoveryObject{ignored, inactive, fresh})
	if err != nil || len(bindings) != 2 {
		t.Fatalf("disappearance lost exclusions: %v %v", bindings, err)
	}
}

func TestDiscoveryWaitsForMatchingAndNeverReenablesDisabledBinding(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	store := NewPostgresStore(pool)
	c := discoveryConnector(t, pool, "zabbix", true)
	if _, err := pool.Exec(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Existing server')`); err != nil {
		t.Fatal(err)
	}
	match := discovered("ambiguous", "Existing server")
	fresh := discovered("fresh", "New server")
	bindings, err := store.RefreshDiscovery(ctx, c, "worker", "zabbix", []DiscoveryObject{match, fresh})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 {
		t.Fatalf("silently matched existing target: %v", bindings)
	}
	items, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].QuarantineCount != 1 {
		t.Fatalf("pending discovery invisible: %v", items)
	}
	if _, err = pool.Exec(ctx, `UPDATE cairnops_connector_bindings SET integration_enabled=false WHERE id=$1::uuid`, bindings[0].ID); err != nil {
		t.Fatal(err)
	}
	bindings, err = store.RefreshDiscovery(ctx, c, "worker", "zabbix", []DiscoveryObject{match, fresh})
	if err != nil || len(bindings) != 0 {
		t.Fatalf("disabled binding revived: %v %v", bindings, err)
	}
	if _, err = store.RefreshDiscovery(ctx, c, "worker", "zabbix", []DiscoveryObject{fresh}); err != nil {
		t.Fatal(err)
	}
	items, err = store.List(ctx)
	if err != nil || items[0].QuarantineCount != 0 {
		t.Fatalf("absent pending still shown: %v %v", items, err)
	}
}

func TestDiscoveryLeaseAndAtomicity(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	store := NewPostgresStore(pool)
	c := discoveryConnector(t, pool, "zabbix", true)
	a := discovered("a", "Server a")
	bad := discovered("b", "Server b")
	bad.Metadata["invalid"] = make(chan int)
	if _, err := store.RefreshDiscovery(ctx, c, "worker", "zabbix", []DiscoveryObject{a, bad}); err == nil {
		t.Fatal("expected encoding failure")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_targets`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial import committed: %d %v", count, err)
	}
	for _, condition := range []string{"lease_until=now()-interval '1 second'", "lease_until=now()+interval '1 minute',status='disabled'"} {
		if _, err := pool.Exec(ctx, `UPDATE cairnops_connectors SET `+condition+` WHERE id=$1::uuid`, c.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RefreshDiscovery(ctx, c, "worker", "zabbix", []DiscoveryObject{a}); err == nil {
			t.Fatalf("ignored lease/status: %s", condition)
		}
	}
}

func TestConcurrentDiscoveryDoesNotDuplicateTargetAcrossConnectors(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	store := NewPostgresStore(pool)
	cs := []RuntimeConnector{discoveryConnector(t, pool, "zabbix", true), discoveryConnector(t, pool, "patchmon", true)}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, c := range cs {
		wg.Add(1)
		go func(i int, c RuntimeConnector) {
			defer wg.Done()
			kind := []string{"zabbix", "patchmon"}[i]
			_, err := store.RefreshDiscovery(ctx, c, "worker", kind, []DiscoveryObject{discovered(fmt.Sprint(i), "Shared machine")})
			errs <- err
		}(i, c)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_targets`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate target: %d %v", count, err)
	}
}

func TestConfirmedSelectionRemembersUnselectedObjects(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	store := NewPostgresStore(pool)
	var actorID string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users(username,display_name,password_hash,role) VALUES('discovery-admin','Test','unused','administrator') RETURNING id::text`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	selected := discovered("selected", "Chosen server")
	excluded := discovered("excluded", "Unselected server")
	imported, err := store.ImportZabbix(ctx, PersistZabbixInput{ActorID: actorID, Name: "Zabbix", Endpoint: "https://zabbix.example.test", CredentialSealed: "sealed-credential-with-sufficient-length", Version: "7.4.13", Compatibility: "supported", EncryptedTransport: true, Hosts: []zabbix.Host{{ID: selected.ExternalID, Name: selected.Name}}, Discovery: []DiscoveryObject{selected, excluded}})
	if err != nil {
		t.Fatal(err)
	}
	c := RuntimeConnector{ID: imported.Connector.ID, Endpoint: imported.Connector.Endpoint}
	if _, err = pool.Exec(ctx, `UPDATE cairnops_connectors SET lease_owner='worker',lease_until=now()+interval '1 minute' WHERE id=$1::uuid`, c.ID); err != nil {
		t.Fatal(err)
	}
	fresh := discovered("fresh", "Future server")
	bindings, err := store.RefreshDiscovery(ctx, c, "worker", "zabbix", []DiscoveryObject{selected, excluded, fresh})
	if err != nil || len(bindings) != 2 {
		t.Fatalf("initial selection lost: %v %v", bindings, err)
	}
	for _, b := range bindings {
		if b.ExternalID == excluded.ExternalID {
			t.Fatal("unselected host automatically imported")
		}
	}
}
