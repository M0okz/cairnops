package bursts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5"
)

func TestProjectFormsPersistentBurstOnlyForSameReliableNature(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)

	targetA := insertTarget(t, pool, "Rafale A")
	targetB := insertTarget(t, pool, "Rafale B")
	first := insertEligibleIncident(t, pool, targetA, "connector", "zabbix-a", "prototype:disk", now, "major")

	project(t, ctx, pool, now.Add(time.Second))
	assertMembershipCount(t, ctx, pool, first, 0)

	second := insertEligibleIncident(t, pool, targetB, "connector", "zabbix-a", "prototype:disk", now.Add(20*time.Second), "warning")
	project(t, ctx, pool, now.Add(21*time.Second))

	var burstID, status, severity string
	var incidents, targets int
	if err := pool.QueryRow(ctx, `
		SELECT burst.id::text, burst.status, burst.severity,
		       burst.incident_count, burst.target_count
		FROM cairnops_incident_bursts burst
	`).Scan(&burstID, &status, &severity, &incidents, &targets); err != nil {
		t.Fatal(err)
	}
	if status != "propagating" || severity != "major" || incidents != 2 || targets != 2 {
		t.Fatalf("unexpected burst projection: status=%s severity=%s incidents=%d targets=%d", status, severity, incidents, targets)
	}
	assertMembershipCount(t, ctx, pool, first, 1)
	assertMembershipCount(t, ctx, pool, second, 1)
	projected, err := NewPostgresStore(pool).Get(ctx, burstID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Members) != 2 || len(projected.Activity) < 3 || projected.Explanation == "" {
		t.Fatalf("persistent burst detail is incomplete: %#v", projected)
	}

	thirdTarget := insertTarget(t, pool, "Rafale C")
	third := insertEligibleIncident(t, pool, thirdTarget, "connector", "zabbix-b", "prototype:disk", now.Add(30*time.Second), "critical")
	project(t, ctx, pool, now.Add(31*time.Second))
	assertMembershipCount(t, ctx, pool, third, 0)

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_incident_bursts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("two connector-local identities must not form another burst, got %d", count)
	}
}

func TestProjectKeepsBurstOpenUntilPropagationIsSealedAndMembersResolve(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)
	targetA := insertTarget(t, pool, "Cycle A")
	targetB := insertTarget(t, pool, "Cycle B")
	first := insertEligibleIncident(t, pool, targetA, "canonical", "cairnops", "availability", now, "major")
	second := insertEligibleIncident(t, pool, targetB, "canonical", "cairnops", "availability", now.Add(10*time.Second), "major")
	project(t, ctx, pool, now.Add(11*time.Second))

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET status = 'resolved', resolved_at = $2, updated_at = now()
		WHERE id = ANY($1::uuid[])
	`, []string{first, second}, now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	project(t, ctx, pool, now.Add(30*time.Second))
	assertBurstStatus(t, ctx, pool, "propagating")

	// Sans cadence déclarée, la fenêtre vaut le minimum de soixante secondes
	// depuis la dernière adhésion.
	project(t, ctx, pool, now.Add(71*time.Second))
	assertBurstStatus(t, ctx, pool, "resolved")

	var sealedAt, resolvedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT sealed_at, resolved_at FROM cairnops_incident_bursts
	`).Scan(&sealedAt, &resolvedAt); err != nil {
		t.Fatal(err)
	}
	if sealedAt == nil || resolvedAt == nil {
		t.Fatalf("sealed and resolved timestamps must survive restart: sealed=%v resolved=%v", sealedAt, resolvedAt)
	}
}

func TestProjectCanonicalNatureCanCrossConnectorTypes(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)
	targetA := insertTarget(t, pool, "Native availability")
	targetB := insertTarget(t, pool, "Uptime availability")
	first := insertEligibleIncident(t, pool, targetA, "canonical", "cairnops", "availability", now, "warning")
	second := insertEligibleIncident(t, pool, targetB, "canonical", "cairnops", "availability", now.Add(5*time.Second), "major")
	insertConnectorSignal(t, pool, first, targetA, "zabbix", now)
	insertConnectorSignal(t, pool, second, targetB, "uptime_kuma", now.Add(5*time.Second))
	project(t, ctx, pool, now.Add(6*time.Second))

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_incident_bursts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("canonical availability must form one cross-source burst, got %d", count)
	}
}

func TestProjectCanonicalNatureDoesNotCrossInstancesOfSameConnectorKind(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)
	targetA := insertTarget(t, pool, "Zabbix availability A")
	targetB := insertTarget(t, pool, "Zabbix availability B")
	first := insertEligibleIncident(t, pool, targetA, "canonical", "cairnops", "availability", now, "major")
	second := insertEligibleIncident(t, pool, targetB, "canonical", "cairnops", "availability", now.Add(5*time.Second), "major")
	insertConnectorSignal(t, pool, first, targetA, "zabbix", now)
	insertConnectorSignal(t, pool, second, targetB, "zabbix", now.Add(5*time.Second))

	project(t, ctx, pool, now.Add(6*time.Second))

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_incident_bursts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("two distinct Zabbix connectors must not form a canonical burst, got %d", count)
	}
}

func TestProjectRejectsAnOldBatchReceivedAtOnce(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)
	insertEligibleIncident(t, pool, insertTarget(t, pool, "Historical A"), "connector", "zabbix-a", "prototype:disk", now.Add(-10*time.Minute), "major")
	insertEligibleIncident(t, pool, insertTarget(t, pool, "Historical B"), "connector", "zabbix-a", "prototype:disk", now.Add(-9*time.Minute-30*time.Second), "major")

	project(t, ctx, pool, now)

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cairnops_incident_bursts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("an old batch must stay as individual incidents, got %d burst", count)
	}
}

func TestProjectJoinsAnIncidentReleasedDuringAnOpenPropagation(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)
	first := insertEligibleIncident(t, pool, insertTarget(t, pool, "Propagation A"), "connector", "zabbix-a", "prototype:disk", now, "major")
	second := insertEligibleIncident(t, pool, insertTarget(t, pool, "Propagation B"), "connector", "zabbix-a", "prototype:disk", now.Add(5*time.Second), "major")
	maintenanceTarget := insertTarget(t, pool, "Maintenance release")
	third := insertEligibleIncident(t, pool, maintenanceTarget, "connector", "zabbix-a", "prototype:disk", now.Add(-10*time.Minute), "warning")
	var maintenanceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_maintenances (name, reason, starts_at, ends_at)
		VALUES ('Maintenance Rafale', 'Intervention de test planifiée', $1, $2)
		RETURNING id::text
	`, now.Add(-11*time.Minute), now.Add(time.Hour)).Scan(&maintenanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_maintenance_targets (maintenance_id, target_id)
		VALUES ($1::uuid, $2::uuid)
	`, maintenanceID, maintenanceTarget); err != nil {
		t.Fatal(err)
	}

	project(t, ctx, pool, now.Add(6*time.Second))
	assertMembershipCount(t, ctx, pool, first, 1)
	assertMembershipCount(t, ctx, pool, second, 1)
	assertMembershipCount(t, ctx, pool, third, 0)

	releasedAt := now.Add(20 * time.Second)
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_maintenances SET cancelled_at = $2 WHERE id = $1::uuid
	`, maintenanceID, releasedAt); err != nil {
		t.Fatal(err)
	}
	project(t, ctx, pool, releasedAt.Add(time.Second))
	assertMembershipCount(t, ctx, pool, third, 1)

	var targets int
	if err := pool.QueryRow(ctx, `SELECT target_count FROM cairnops_incident_bursts`).Scan(&targets); err != nil {
		t.Fatal(err)
	}
	if targets != 3 {
		t.Fatalf("released member did not update the persistent impact: %d targets", targets)
	}
}

func TestProjectUsesTheLongestParticipatingSourceCadence(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)
	targetA := insertTarget(t, pool, "Slow source")
	targetB := insertTarget(t, pool, "Fast source")
	first := insertEligibleIncident(t, pool, targetA, "canonical", "cairnops", "availability", now, "major")
	second := insertEligibleIncident(t, pool, targetB, "canonical", "cairnops", "availability", now.Add(100*time.Second), "major")
	insertNativeSignal(t, pool, first, targetA, 120, now)
	insertNativeSignal(t, pool, second, targetB, 30, now.Add(100*time.Second))

	project(t, ctx, pool, now.Add(101*time.Second))

	var window int
	if err := pool.QueryRow(ctx, `SELECT propagation_window_seconds FROM cairnops_incident_bursts`).Scan(&window); err != nil {
		t.Fatal(err)
	}
	if window != 240 {
		t.Fatalf("propagation window = %d, want 240 seconds", window)
	}
}

func TestBurstAcknowledgementCoversFutureMembersAndKeepsIncidentAudit(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	now := time.Now().UTC().Truncate(time.Second)
	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Burst Acknowledger', 'not-used', 'operator') RETURNING id::text
	`, fmt.Sprintf("burst-ack-%d", now.UnixNano())).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	first := insertEligibleIncident(t, pool, insertTarget(t, pool, "Ack A"), "canonical", "cairnops", "availability", now, "major")
	second := insertEligibleIncident(t, pool, insertTarget(t, pool, "Ack B"), "canonical", "cairnops", "availability", now.Add(5*time.Second), "major")
	project(t, ctx, pool, now.Add(6*time.Second))
	var burstID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM cairnops_incident_bursts`).Scan(&burstID); err != nil {
		t.Fatal(err)
	}
	incidentService := incidents.NewService(incidents.NewPostgresStore(pool), nil)
	service := NewService(NewPostgresStore(pool), incidentService)
	if _, err := service.Acknowledge(ctx, burstID, actorID, "Burst Acknowledger"); err != nil {
		t.Fatal(err)
	}

	thirdTarget := insertTarget(t, pool, "Ack C")
	third := insertEligibleIncident(t, pool, thirdTarget, "canonical", "cairnops", "availability", now.Add(10*time.Second), "warning")
	insertConnectorSignal(t, pool, third, thirdTarget, "zabbix", now.Add(10*time.Second))
	project(t, ctx, pool, now.Add(11*time.Second))
	var acknowledged bool
	var activity int
	var syncStatus string
	if err := pool.QueryRow(ctx, `
		SELECT incident.acknowledged_at IS NOT NULL,
		       incident.acknowledgement_sync_status,
		       count(activity.id) FILTER (WHERE activity.kind = 'acknowledged')::integer
		FROM cairnops_incidents incident
		LEFT JOIN cairnops_incident_activity activity ON activity.incident_id = incident.id
		WHERE incident.id = $1::uuid GROUP BY incident.id
	`, third).Scan(&acknowledged, &syncStatus, &activity); err != nil {
		t.Fatal(err)
	}
	if !acknowledged || syncStatus != "pending" || activity != 1 {
		t.Fatalf("future member did not inherit an auditable acknowledgement: acknowledged=%v sync=%s activity=%d", acknowledged, syncStatus, activity)
	}
	recorder := &recordingIncidentAcknowledger{}
	if err := NewAcknowledgementSynchronizer(pool, recorder, nil).Process(ctx); err != nil {
		t.Fatal(err)
	}
	if len(recorder.ids) != 1 || recorder.ids[0] != third {
		t.Fatalf("future member acknowledgement was not delegated upstream: %#v", recorder.ids)
	}
	for _, id := range []string{first, second} {
		var current bool
		if err := pool.QueryRow(ctx, `SELECT acknowledged_at IS NOT NULL FROM cairnops_incidents WHERE id = $1::uuid`, id).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if !current {
			t.Fatalf("current member %s was not acknowledged", id)
		}
	}
}

type recordingIncidentAcknowledger struct{ ids []string }

func (recorder *recordingIncidentAcknowledger) Acknowledge(_ context.Context, incidentID, _, _ string) (incidents.Incident, error) {
	recorder.ids = append(recorder.ids, incidentID)
	return incidents.Incident{ID: incidentID}, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertTarget(t *testing.T, pool queryer, prefix string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text
	`, fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertEligibleIncident(t *testing.T, pool queryer, targetID, scope, namespace, fingerprint string, openedAt time.Time, severity string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status,
			source_severity, effective_severity, opened_at,
			nature_scope, nature_namespace, nature_fingerprint, burst_eligible
		) VALUES ($1::uuid, $2, 'Latence disque élevée', 'active', $3, $3, $4, $5, $6, $7, true)
		RETURNING id::text
	`, targetID, namespace+":"+fingerprint, severity, openedAt, scope, namespace, fingerprint).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertConnectorSignal(t *testing.T, pool queryer, incidentID, targetID, kind string, openedAt time.Time) {
	t.Helper()
	suffix := time.Now().UnixNano()
	var connectorID, bindingID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_connectors (
			kind, name, endpoint, credential_sealed, status, encrypted_transport
		) VALUES ($1, $2, $3, 'sealed-credential-with-sufficient-length', 'connected', true)
		RETURNING id::text
	`, kind, fmt.Sprintf("Burst source %d", suffix), fmt.Sprintf("https://burst-source-%d.example.test", suffix)).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, $3, $4) RETURNING id::text
	`, connectorID, targetID, fmt.Sprintf("target-%d", suffix), fmt.Sprintf("Burst target %d", suffix)).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_incident_signals (
			incident_id, target_id, origin, connector_id, connector_binding_id,
			external_event_id, name, active, severity, opened_at
		) VALUES ($1::uuid, $2::uuid, $3, $4::uuid, $5::uuid, $6,
		          'Indisponibilité', true, 'major', $7)
		RETURNING id::text
	`, incidentID, targetID, kind, connectorID, bindingID, fmt.Sprintf("event-%d", suffix), openedAt).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
}

func insertNativeSignal(t *testing.T, pool queryer, incidentID, targetID string, interval int, openedAt time.Time) {
	t.Helper()
	var sourceID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, interval_seconds, timeout_milliseconds, config
		) VALUES ($1::uuid, $2, 'tcp', $3, 1000, '{}'::jsonb)
		RETURNING id::text
	`, targetID, fmt.Sprintf("Burst source %d", time.Now().UnixNano()), interval).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_incident_signals (
			incident_id, target_id, origin, source_id, name, active, severity, opened_at
		) VALUES ($1::uuid, $2::uuid, 'native', $3::uuid,
		          'Indisponibilité', true, 'major', $4)
		RETURNING id::text
	`, incidentID, targetID, sourceID, openedAt).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
}

func project(t *testing.T, ctx context.Context, pool interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}, at time.Time) {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := Project(ctx, tx, at); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func assertMembershipCount(t *testing.T, ctx context.Context, pool queryer, incidentID string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM cairnops_incident_burst_members WHERE incident_id = $1::uuid
	`, incidentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("membership count for %s = %d, want %d", incidentID, count, want)
	}
}

func assertBurstStatus(t *testing.T, ctx context.Context, pool queryer, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM cairnops_incident_bursts`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("burst status = %s, want %s", got, want)
	}
}
