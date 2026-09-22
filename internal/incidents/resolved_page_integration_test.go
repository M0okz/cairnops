package incidents

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResolvedPagesKeepTiesAndMultiResourceIncidentsStable(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	api := insertCycleTarget(t, ctx, pool, "API")
	storage := insertCycleTarget(t, ctx, pool, "Storage secondaire")
	resolved := time.Date(2026, 1, 5, 10, 0, 0, 123456000, time.UTC)
	for index := 1; index <= 6; index++ {
		insertListedIncident(t, pool, index, "resolved", SeverityMajor, resolved, api, storage)
	}
	options := ResolvedPageOptions{Limit: 2, TargetID: storage}
	first, err := store.ListResolvedPage(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.NextCursor == "" || len(first.Incidents) != 2 || len(first.Incidents[0].Impacts) != 2 {
		t.Fatalf("page unit must be the complete Incident: %#v", first)
	}
	// A newly imported, backdated Incident must not shift the ongoing snapshot.
	insertListedIncident(t, pool, 7, "resolved", SeverityMajor, resolved.Add(-time.Minute), storage)
	ids := []string{first.Incidents[0].ID, first.Incidents[1].ID}
	options.Cursor = first.NextCursor
	for options.Cursor != "" {
		page, err := store.ListResolvedPage(ctx, options)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Incidents {
			ids = append(ids, item.ID)
		}
		options.Cursor = page.NextCursor
	}
	want := make([]string, 0, 6)
	for index := 6; index >= 1; index-- {
		want = append(want, listedIncidentID(index))
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ties must neither repeat nor skip rows: got %v want %v", ids, want)
	}
}

func TestResolvedPageFiltersDatesBeforeLimitingAndKeepsArchivedResourceOptions(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	api := insertCycleTarget(t, ctx, pool, "API")
	storage := insertCycleTarget(t, ctx, pool, "Storage 100% archive")
	if _, err := pool.Exec(ctx, `UPDATE cairnops_targets SET archived_at = now() WHERE id = $1::uuid`, storage); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	before := from.AddDate(0, 0, 1)
	insertListedIncident(t, pool, 1, "resolved", SeverityMajor, from.Add(-time.Microsecond), api)
	insertListedIncident(t, pool, 2, "resolved", SeverityMajor, from, api, storage)
	insertListedIncident(t, pool, 3, "resolved", SeverityMajor, before.Add(-time.Microsecond), api, storage)
	insertListedIncident(t, pool, 4, "resolved", SeverityMajor, before, api)
	insertListedIncident(t, pool, 5, "resolved", SeverityWarning, from.Add(time.Hour), storage)
	insertListedIncident(t, pool, 6, "active", SeverityMajor, from, storage)
	// An incident opened long before the selected dates still belongs to the
	// history when its resolution falls inside them.
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET opened_at = $2 WHERE id = $1::uuid`, listedIncidentID(2), from.AddDate(0, -2, 0)); err != nil {
		t.Fatal(err)
	}
	options := ResolvedPageOptions{Limit: 1, From: &from, Before: &before, Severity: SeverityMajor, NatureKey: "availability", Query: "100%", TargetID: storage}
	page, err := store.ListResolvedPage(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Incidents) != 1 || page.Incidents[0].ID != listedIncidentID(3) || page.NextCursor == "" {
		t.Fatalf("filters must run before LIMIT: %#v", page)
	}
	if page.Filters == nil || len(page.Filters.Targets) != 2 || len(page.Filters.Natures) != 1 {
		t.Fatalf("options must include retained/archived resources: %#v", page.Filters)
	}
	options.Cursor = page.NextCursor
	page, err = store.ListResolvedPage(ctx, options)
	if err != nil || len(page.Incidents) != 1 || page.Incidents[0].ID != listedIncidentID(2) || page.NextCursor != "" {
		t.Fatalf("date interval must be inclusive/exclusive at its exact instants: %#v %v", page, err)
	}
	options.Cursor, options.Query = "", "Storage 100_"
	page, err = store.ListResolvedPage(ctx, options)
	if err != nil || len(page.Incidents) != 0 {
		t.Fatalf("wildcards must be treated as literal search text: %#v %v", page, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incident_evidence (incident_id, impact_id, target_id, origin, identity_scope,
		    identity_key, name, active, severity, opened_at, resolved_at)
		SELECT incident_id, id, target_id, 'webhook', 'test', 'slow-disk', 'Latency above 200 ms', false,
		    'major', opened_at, resolved_at
		FROM cairnops_incident_impacts WHERE incident_id = $1::uuid AND target_id = $2::uuid
	`, listedIncidentID(2), storage); err != nil {
		t.Fatal(err)
	}
	options.Query = "LATENCY"
	page, err = store.ListResolvedPage(ctx, options)
	if err != nil || len(page.Incidents) != 1 || page.Incidents[0].ID != listedIncidentID(2) {
		t.Fatalf("evidence of a secondary resource must be searchable: %#v %v", page, err)
	}
}

func TestIncidentStorePrioritizesUnacknowledgedSeverityAndAgeBeforeLimit(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	target := insertCycleTarget(t, ctx, pool, "API")
	opened := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	insertListedIncident(t, pool, 1, "active", SeverityCritical, opened.Add(-time.Hour), target)
	insertListedIncident(t, pool, 2, "active", SeverityWarning, opened.Add(-time.Hour), target)
	insertListedIncident(t, pool, 3, "active", SeverityMajor, opened, target)
	insertListedIncident(t, pool, 4, "active", SeverityMajor, opened.Add(-time.Minute), target)
	if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET acknowledged_at = now(), acknowledgement_origin = 'connector' WHERE id = $1::uuid`, listedIncidentID(1)); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, "active", 3)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{items[0].ID, items[1].ID, items[2].ID}
	want := []string{listedIncidentID(4), listedIncidentID(3), listedIncidentID(2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("most actionable incidents must survive the server limit: %v", got)
	}
}

func TestResolvedPageSearchMatchesActualLocalizedPresentationBeforeLimit(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewPostgresStore(pool)
	target := insertCycleTarget(t, ctx, pool, "VM")
	at := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	for index := 1; index <= 4; index++ {
		insertListedIncident(t, pool, index, "resolved", SeverityMajor, at.Add(time.Duration(index)*time.Hour), target)
	}
	for _, fixture := range []struct {
		index            int
		scope, key, kind string
	}{
		{1, "canonical", "availability", "cpu.usage.high"}, // Canonical meaning wins over alert_kind.
		{2, "connector", "provider:42", "cpu.usage.high"},
		{3, "connector", "availability", ""}, // Local identity must not acquire canonical meaning.
		{4, "canonical", "storage.latency", "cpu.usage.high"},
	} {
		if _, err := pool.Exec(ctx, `UPDATE cairnops_incidents SET nature_scope = $2, nature_key = $3,
		    alert_kind = $4, nature_label = 'Provider condition 42' WHERE id = $1::uuid`,
			listedIncidentID(fixture.index), fixture.scope, fixture.key, fixture.kind); err != nil {
			t.Fatal(err)
		}
	}
	for _, check := range []struct {
		query string
		index int
		title string
	}{
		{"utilisation CPU", 2, "Utilisation CPU élevée"},
		{"HIGH CPU UTILIZATION", 2, "Utilisation CPU élevée"},
		{"indisponibilité", 1, "Indisponibilité"},
		{"unavailability", 1, "Indisponibilité"},
		{"latence disque", 4, "Latence disque élevée"},
		{"high disk latency", 4, "Latence disque élevée"},
	} {
		page, err := store.ListResolvedPage(ctx, ResolvedPageOptions{Limit: 1, Query: check.query})
		if err != nil || len(page.Incidents) != 1 || page.Incidents[0].ID != listedIncidentID(check.index) ||
			page.NextCursor != "" || page.Incidents[0].Presentation.FR.Title != check.title {
			t.Fatalf("search %q must match the rendered title before LIMIT: %#v %v", check.query, page, err)
		}
	}
}

func listedIncidentID(index int) string { return fmt.Sprintf("10000000-0000-0000-0000-%012d", index) }

func insertListedIncident(t *testing.T, pool *pgxpool.Pool, index int, status string, severity Severity, at time.Time, targetIDs ...string) {
	t.Helper()
	ctx := context.Background()
	var resolved *time.Time
	if status == "resolved" {
		resolved = &at
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incidents (id, nature_key, nature_label, nature_scope, nature_namespace, severity,
		    status, propagation_status, opened_at, last_impact_at, propagation_window_seconds, propagation_ends_at,
		    propagation_closed_at, resolved_at, impact_count, affected_target_count, active_impact_count)
		VALUES ($1::uuid, 'availability', 'Indisponibilité', 'canonical', 'cairnops', $2, $3, 'closed', $4, $4, 60, $4, $4, $5, $6, $6,
		    CASE WHEN $3 = 'active' THEN $6 ELSE 0 END)
	`, listedIncidentID(index), severity, status, at, resolved, len(targetIDs))
	if err != nil {
		t.Fatal(err)
	}
	for _, targetID := range targetIDs {
		_, err := pool.Exec(ctx, `
			INSERT INTO cairnops_incident_impacts (incident_id, target_id, status, source_severity, effective_severity, opened_at, resolved_at)
			VALUES ($1::uuid, $2::uuid, $3, $4, $4, $5, $6)
		`, listedIncidentID(index), targetID, status, severity, at, resolved)
		if err != nil {
			t.Fatal(err)
		}
	}
}
