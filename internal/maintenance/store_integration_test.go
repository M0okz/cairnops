package maintenance_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/maintenance"
	"github.com/M0okz/cairnops/internal/testsupport"
)

func TestMaintenanceNeutralizesIncidentProjectionWithoutDeletingProof(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	suffix := time.Now().UTC().UnixNano()
	var actorID, targetID, sourceID, incidentID, impactID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Maintenance Operator', 'not-used', 'operator') RETURNING id::text
	`, fmt.Sprintf("maintenance-%d", suffix)).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, fmt.Sprintf("Storage %d", suffix)).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (target_id, kind, name, interval_seconds, timeout_milliseconds, config)
		VALUES ($1::uuid, 'tcp', 'Port stockage', 30, 2000, '{"host":"127.0.0.1","port":443}'::jsonb)
		RETURNING id::text
	`, targetID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			nature_key, nature_label, nature_scope, nature_namespace,
			nature_fingerprint, propagation_eligible, status,
			propagation_status, severity, opened_at, last_impact_at,
			propagation_window_seconds, propagation_ends_at,
			active_impact_count, impact_count, affected_target_count,
			max_affected_targets
		) VALUES (
			'native:tcp', 'Port indisponible', 'canonical', 'cairnops',
			'native:tcp', true, 'active', 'open', 'major', now(), now(),
			60, now() + interval '1 minute', 1, 1, 1, 1
		)
		RETURNING id::text
	`).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incident_impacts (
			incident_id, target_id, status, source_severity,
			effective_severity, opened_at
		) VALUES ($1::uuid, $2::uuid, 'active', 'major', 'major', now())
		RETURNING id::text
	`, incidentID, targetID).Scan(&impactID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incident_evidence (
			incident_id, impact_id, target_id, origin, source_id,
			identity_scope, identity_key, name, active, severity,
			opened_at, last_seen_at
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, 'native', $4::uuid,
			$4, 'availability', 'Connexion refusée', true, 'major', now(), now()
		)
	`, incidentID, impactID, targetID, sourceID); err != nil {
		t.Fatal(err)
	}

	service := maintenance.NewService(maintenance.NewPostgresStore(pool))
	created, err := service.Create(ctx, actorID, maintenance.CreateInput{
		Name: "Remplacement stockage", Reason: "Intervention physique sur le contrôleur",
		TargetIDs: []string{targetID}, EndsAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := incidents.NewPostgresStore(pool).Get(ctx, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Impacts) != 1 || !projected.Impacts[0].MaintenanceActive ||
		projected.Impacts[0].MaintenanceEndsAt == nil || len(projected.Impacts[0].Evidence) != 1 ||
		!projected.Impacts[0].Evidence[0].Active {
		t.Fatalf("maintenance must neutralize the projection and preserve proof: %#v", projected)
	}
	cancelled, err := service.Cancel(ctx, created.ID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != "cancelled" {
		t.Fatalf("unexpected cancellation state: %#v", cancelled)
	}
	projected, err = incidents.NewPostgresStore(pool).Get(ctx, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Impacts) != 1 || projected.Impacts[0].MaintenanceActive ||
		len(projected.Impacts[0].Evidence) != 1 || !projected.Impacts[0].Evidence[0].Active {
		t.Fatalf("cancellation must restore projection and preserve proof: %#v", projected)
	}
}
