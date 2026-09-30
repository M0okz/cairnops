package health

import (
	"context"
	"testing"

	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La règle est éprouvée isolément dans health_test.go. Ce test répond à l'autre
// question : la projection remet-elle à la règle les faits que PostgreSQL
// contient réellement ? Une clause fausse ne se voit que là.
func TestStateProjectionReadsTheFactsPostgresHolds(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := NewStore(pool)

	// Une Ressource sans aucun Contrôle : rien ne permet de conclure.
	bare := createTarget(t, ctx, pool, "Sans Contrôle")

	// Une Ressource dont un Contrôle de disponibilité vient de conclure.
	serving := createTarget(t, ctx, pool, "Service en ligne")
	servingCheck := createCheck(t, ctx, pool, serving, 60, true, true)
	observe(t, ctx, pool, serving, servingCheck, "healthy", 30)

	// Une Ressource indisponible : preuve active, fraîche, de Nature reconnue.
	down := createTarget(t, ctx, pool, "Service coupé")
	downCheck := createCheck(t, ctx, pool, down, 60, true, true)
	observe(t, ctx, pool, down, downCheck, "healthy", 30)
	openIncident(t, ctx, pool, down, downCheck, "availability.unavailable", 60, false)

	// Une Ressource dégradée : condition altérante établie, disponibilité
	// par ailleurs constatée. C'est l'état que rien ne pouvait produire avant.
	slow := createTarget(t, ctx, pool, "Service lent")
	slowCheck := createCheck(t, ctx, pool, slow, 60, true, true)
	observe(t, ctx, pool, slow, slowCheck, "healthy", 30)
	openIncident(t, ctx, pool, slow, slowCheck, "disk.latency.high", 60, false)

	// Une mise à jour disponible ne dégrade pas un service qui fonctionne.
	updatable := createTarget(t, ctx, pool, "Service à mettre à jour")
	updatableCheck := createCheck(t, ctx, pool, updatable, 60, true, true)
	observe(t, ctx, pool, updatable, updatableCheck, "healthy", 30)
	openIncident(t, ctx, pool, updatable, updatableCheck, "software.security_update_available", 60, false)

	// Une preuve invalidée ne soutient plus rien.
	cleared := createTarget(t, ctx, pool, "Faux positif écarté")
	clearedCheck := createCheck(t, ctx, pool, cleared, 60, true, true)
	observe(t, ctx, pool, cleared, clearedCheck, "healthy", 30)
	openIncident(t, ctx, pool, cleared, clearedCheck, "availability.unavailable", 60, true)

	// Un Contrôle suspendu n'établit pas un état disponible.
	suspended := createTarget(t, ctx, pool, "Contrôle suspendu")
	suspendedCheck := createCheck(t, ctx, pool, suspended, 60, false, true)
	observe(t, ctx, pool, suspended, suspendedCheck, "healthy", 30)

	// Une Fenêtre de maintenance ne change pas ce que les preuves établissent :
	// l'Incident reste visible, et c'est au client de neutraliser son impact à
	// partir des dates de la fenêtre. Sans quoi un Incident encore actif à son
	// terme ne pourrait pas « redevenir immédiatement visible ».
	paused := createTarget(t, ctx, pool, "Sous maintenance")
	pausedCheck := createCheck(t, ctx, pool, paused, 60, true, true)
	observe(t, ctx, pool, paused, pausedCheck, "healthy", 30)
	openIncident(t, ctx, pool, paused, pausedCheck, "availability.unavailable", 60, false)
	planMaintenance(t, ctx, pool, paused)

	// Une Ressource archivée sort de la supervision active.
	archived := createTarget(t, ctx, pool, "Retirée")
	if _, err := pool.Exec(ctx, `UPDATE cairnops_targets SET archived_at = now() WHERE id = $1::uuid`, archived); err != nil {
		t.Fatal(err)
	}

	states, err := store.States(ctx)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]State{
		bare: Unknown, serving: Available, down: Unavailable, slow: Degraded,
		updatable: Available, cleared: Available, suspended: Unknown, paused: Unavailable,
	}
	for id, want := range expected {
		if got := states[id]; got != want {
			t.Errorf("Ressource %s : État %q, attendu %q", name(t, ctx, pool, id), got, want)
		}
	}
	if _, present := states[archived]; present {
		t.Error("une Ressource archivée ne doit pas figurer dans l'État global")
	}
	if len(states) != len(expected) {
		t.Errorf("%d Ressources projetées, %d attendues", len(states), len(expected))
	}

	// L'État global se déduit de ces mêmes États, sans seconde règle.
	collected := make([]State, 0, len(states))
	for _, state := range states {
		collected = append(collected, state)
	}
	if overall := Overall(collected); overall != Unavailable {
		t.Errorf("État global %q, attendu %q puisqu'une Ressource est Indisponible", overall, Unavailable)
	}
}

// Sans Ressource active, la Supervision n'est pas configurée : la projection
// est vide et l'État global ne conclut rien.
func TestStateProjectionConcludesNothingWithoutResources(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	states, err := NewStore(pool).States(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 {
		t.Fatalf("aucune Ressource attendue, %d projetées", len(states))
	}
	if overall := Overall(nil); overall != "" {
		t.Errorf("État global %q, attendu vide", overall)
	}
}

func createTarget(t *testing.T, ctx context.Context, pool *pgxpool.Pool, label string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_targets (name, description) VALUES ($1, '')
		RETURNING id::text
	`, label).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func createCheck(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID string, intervalSeconds int, enabled, measuresAvailability bool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, interval_seconds, timeout_milliseconds, config,
			enabled, measures_availability
		) VALUES ($1::uuid, 'Contrôle', 'http', $2, 5000, '{"url":"https://exemple.test"}'::jsonb, $3, $4)
		RETURNING id::text
	`, targetID, intervalSeconds, enabled, measuresAvailability).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func observe(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID, sourceID, outcome string, secondsAgo int) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_observations (source_id, target_id, observed_at, outcome, latency_milliseconds)
		VALUES ($1::uuid, $2::uuid, now() - make_interval(secs => $3), $4, 12)
	`, sourceID, targetID, secondsAgo, outcome); err != nil {
		t.Fatal(err)
	}
}

// openIncident pose un Incident actif, son Atteinte et une Preuve active dont
// la Nature reconnue est celle qu'on veut éprouver.
func openIncident(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID, sourceID, kind string, secondsAgo int, invalidated bool) {
	t.Helper()
	var incidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			nature_key, nature_label, nature_scope, nature_namespace, nature_fingerprint,
			status, severity, opened_at, last_impact_at,
			propagation_window_seconds, propagation_ends_at,
			active_impact_count, impact_count, affected_target_count, max_affected_targets
		) VALUES ($1, $1, 'canonical', 'cairnops', $1, 'active', 'major',
		          now() - make_interval(secs => $2), now() - make_interval(secs => $2),
		          60, now() - make_interval(secs => $2) + interval '60 seconds',
		          1, 1, 1, 1)
		RETURNING id::text
	`, kind, secondsAgo).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	var impactID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incident_impacts (
			incident_id, target_id, status, source_severity, effective_severity, opened_at
		) VALUES ($1::uuid, $2::uuid, 'active', 'major', 'major', now() - make_interval(secs => $3))
		RETURNING id::text
	`, incidentID, targetID, secondsAgo).Scan(&impactID); err != nil {
		t.Fatal(err)
	}
	// Le schéma exige un motif pour toute Invalidation : CONTEXT.md parle bien
	// d'une « décision motivée et traçable », et la base le fait respecter.
	invalidatedAt, reason := "NULL", ""
	if invalidated {
		invalidatedAt, reason = "now()", "faux positif confirmé par l'opérateur"
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incident_evidence (
			incident_id, impact_id, target_id, origin, source_id, identity_scope, identity_key,
			name, active, severity, opened_at, last_seen_at, alert_facts,
			invalidated_at, invalidation_reason
		) VALUES ($1::uuid, $2::uuid, $3::uuid, 'native', $4::uuid, $3::text, $5, 'Preuve', true, 'major',
		          now() - make_interval(secs => $6), now() - make_interval(secs => $6),
		          jsonb_build_object('kind', $5::text), `+invalidatedAt+`, $7)
	`, incidentID, impactID, targetID, sourceID, kind, secondsAgo, reason); err != nil {
		t.Fatal(err)
	}
}

func planMaintenance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID string) {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_maintenances (name, reason, starts_at, ends_at)
		VALUES ('Fenêtre', 'Intervention planifiée', now() - interval '10 minutes', now() + interval '1 hour')
		RETURNING id::text
	`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_maintenance_targets (maintenance_id, target_id) VALUES ($1::uuid, $2::uuid)
	`, id, targetID); err != nil {
		t.Fatal(err)
	}
}

func name(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var label string
	if err := pool.QueryRow(ctx, `SELECT name FROM cairnops_targets WHERE id = $1::uuid`, id).Scan(&label); err != nil {
		return id
	}
	return label
}
