package maintenance_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/maintenance"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestRecurringMaintenanceExtensionAndCancellation(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var actor, target string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users (username,display_name,password_hash,role) VALUES ('operator','Operator','unused','operator') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Weekly resource') RETURNING id::text`).Scan(&target); err != nil {
		t.Fatal(err)
	}
	service := maintenance.NewService(maintenance.NewPostgresStore(pool))
	start := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	rule := &maintenance.Recurrence{Frequency: "weekly", Timezone: "Europe/Paris", Until: start.AddDate(0, 0, 21).Format("2006-01-02")}
	created, err := service.Create(ctx, actor, maintenance.CreateInput{Name: "Weekly patch", Reason: "Scheduled patch installation", TargetIDs: []string{target}, StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: rule})
	if err != nil {
		t.Fatal(err)
	}
	if created.SeriesID == "" || created.Recurrence == nil {
		t.Fatalf("series missing: %+v", created)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_maintenances WHERE series_id=$1::uuid`, created.SeriesID).Scan(&count); err != nil || count != 4 {
		t.Fatalf("expected 4 materialized occurrences, got %d, %v", count, err)
	}
	// Two operators extending the same displayed end time produce one extension.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Extend(ctx, created.ID, actor, created.EndsAt)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, maintenance.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("extension race success=%d conflicts=%d", success, conflicts)
	}
	var duration float64
	if err := pool.QueryRow(ctx, `SELECT extract(epoch from ends_at-starts_at) FROM cairnops_maintenances WHERE id=$1::uuid`, created.ID).Scan(&duration); err != nil || duration != 5400 {
		t.Fatalf("expected 90 minutes, %f %v", duration, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_maintenance_extensions WHERE maintenance_id=$1::uuid AND actor_id=$2::uuid AND previous_ends_at=$3`, created.ID, actor, created.EndsAt).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit missing or duplicated: %d %v", count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_maintenances WHERE series_id=$1::uuid AND id<>$2::uuid AND ends_at-starts_at=interval '1 hour'`, created.SeriesID, created.ID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("future occurrences changed: %d %v", count, err)
	}
	if _, err := service.Cancel(ctx, created.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Extend(ctx, created.ID, actor, created.EndsAt.Add(30*time.Minute)); !errors.Is(err, maintenance.ErrConflict) {
		t.Fatalf("cancelled occurrence extended: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_maintenances WHERE series_id=$1::uuid AND cancelled_at IS NULL`, created.SeriesID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("occurrence cancellation affected series: %d %v", count, err)
	}
	if _, err := service.CancelSeries(ctx, created.ID, actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_maintenances WHERE series_id=$1::uuid AND cancelled_at IS NULL`, created.SeriesID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("series cancellation incomplete: %d %v", count, err)
	}
}

func TestRecurringMaintenanceCreationRollsBackMissingTarget(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var actor string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users (username,display_name,password_hash,role) VALUES ('operator','Operator','unused','operator') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC()
	_, err := maintenance.NewService(maintenance.NewPostgresStore(pool)).Create(ctx, actor, maintenance.CreateInput{Name: "Weekly patch", Reason: "Scheduled patch installation", TargetIDs: []string{"10000000-0000-0000-0000-000000000001"}, StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: &maintenance.Recurrence{Frequency: "weekly", Timezone: "UTC", Until: start.AddDate(0, 0, 21).Format("2006-01-02")}})
	if !errors.Is(err, maintenance.ErrInvalidInput) {
		t.Fatalf("expected invalid resource: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM cairnops_maintenance_series)+(SELECT count(*) FROM cairnops_maintenances)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial series persisted: %d %v", count, err)
	}
}

func TestMaintenanceListPrioritizesActiveOverMaterializedFutureOccurrences(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	var actor, target string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_users (username,display_name,password_hash,role) VALUES ('operator','Operator','unused','operator') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ('Recurring resource') RETURNING id::text`).Scan(&target); err != nil {
		t.Fatal(err)
	}
	service := maintenance.NewService(maintenance.NewPostgresStore(pool))
	start := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	for i := 0; i < 4; i++ {
		_, err := service.Create(ctx, actor, maintenance.CreateInput{Name: "Weekly patch", Reason: "Scheduled patch installation", TargetIDs: []string{target}, StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: &maintenance.Recurrence{Frequency: "weekly", Timezone: "UTC", Until: start.AddDate(1, 0, 0).Format("2006-01-02")}})
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := service.List(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 100 {
		t.Fatalf("expected bounded list, got %d", len(items))
	}
	for i := 0; i < 4; i++ {
		if items[i].State != "active" {
			t.Fatalf("active occurrence evicted by future series at %d", i)
		}
	}
}
