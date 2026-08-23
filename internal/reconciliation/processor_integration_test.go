package reconciliation

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/controlplane"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTargetMergeConsolidatesHistoryAndKeepsRedirect(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	actorID := insertReconciliationUser(t, ctx, pool)
	primaryID := insertReconciliationTarget(t, ctx, pool, "trust-auth-01")
	secondaryID := insertReconciliationTarget(t, ctx, pool, "Authentik")
	primarySourceID := insertNativeSource(t, ctx, pool, primaryID, "Zabbix host")
	secondarySourceID := insertNativeSource(t, ctx, pool, secondaryID, "Authentik HTTP")

	observedAt := time.Now().UTC().Add(-time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_observations (source_id, target_id, observed_at, outcome, latency_milliseconds)
		VALUES ($1::uuid, $2::uuid, $3, 'healthy', 42)
	`, secondarySourceID, secondaryID, observedAt); err != nil {
		t.Fatalf("insert secondary observation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_observation_hours (source_id, target_id, hour, healthy, expected, latency_sum_milliseconds, latency_count, latency_maximum_milliseconds)
		VALUES ($1::uuid, $2::uuid, date_trunc('hour', $3::timestamptz), 1, 1, 42, 1, 42)
	`, secondarySourceID, secondaryID, observedAt); err != nil {
		t.Fatalf("insert secondary rollup: %v", err)
	}
	olderIncidentID := insertActiveIncident(t, ctx, pool, primaryID, primarySourceID, "major", observedAt.Add(-time.Hour), false, actorID)
	newerIncidentID := insertActiveIncident(t, ctx, pool, secondaryID, secondarySourceID, "critical", observedAt, true, actorID)

	var inAppChannelID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM cairnops_notification_channels WHERE kind = 'in_app'`).Scan(&inAppChannelID); err != nil {
		t.Fatalf("find in-app channel: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_notification_outbox (
			incident_id, channel_id, event_kind, status, target_name, nature_label,
			severity, opened_at, delivered_at
		) VALUES ($1::uuid, $2::uuid, 'firing', 'delivered', 'Authentik', 'Indisponibilité', 'critical', $3, now())
	`, newerIncidentID, inAppChannelID, observedAt); err != nil {
		t.Fatalf("insert delivered opening: %v", err)
	}

	processor := NewProcessor(pool, "integration-test", slog.Default())
	result, err := processor.mergeTargets(ctx, operationWork{
		ID: "10000000-0000-4000-8000-000000000001", Kind: "target_merge",
		PrimaryTargetID: primaryID, SecondaryTargetID: secondaryID,
		Reason: "Identité vérifiée pendant le test", ActorID: actorID,
	})
	if err != nil {
		t.Fatalf("merge targets: %v", err)
	}
	if result["target_id"] != primaryID {
		t.Fatalf("unexpected merge result: %#v", result)
	}

	var archived bool
	var redirectedTo string
	if err := pool.QueryRow(ctx, `
		SELECT archived_at IS NOT NULL, reconciled_into_target_id::text
		FROM cairnops_targets WHERE id = $1::uuid
	`, secondaryID).Scan(&archived, &redirectedTo); err != nil {
		t.Fatalf("read absorbed target: %v", err)
	}
	if !archived || redirectedTo != primaryID {
		t.Fatalf("absorbed target is not a redirect: archived=%v target=%s", archived, redirectedTo)
	}

	var sourceTarget, observationTarget, rollupTarget string
	if err := pool.QueryRow(ctx, `SELECT target_id::text FROM cairnops_signal_sources WHERE id = $1::uuid`, secondarySourceID).Scan(&sourceTarget); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT target_id::text FROM cairnops_observations WHERE source_id = $1::uuid`, secondarySourceID).Scan(&observationTarget); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT target_id::text FROM cairnops_observation_hours WHERE source_id = $1::uuid`, secondarySourceID).Scan(&rollupTarget); err != nil {
		t.Fatal(err)
	}
	if sourceTarget != primaryID || observationTarget != primaryID || rollupTarget != primaryID {
		t.Fatalf("history was not consolidated: source=%s observation=%s rollup=%s", sourceTarget, observationTarget, rollupTarget)
	}

	var status, severity string
	var acknowledgedAt *time.Time
	var signalCount int
	if err := pool.QueryRow(ctx, `
		SELECT status, effective_severity, acknowledged_at,
		       (SELECT count(*)::integer FROM cairnops_incident_signals WHERE incident_id = incident.id)
		FROM cairnops_incidents incident WHERE id = $1::uuid
	`, olderIncidentID).Scan(&status, &severity, &acknowledgedAt, &signalCount); err != nil {
		t.Fatalf("read surviving incident: %v", err)
	}
	if status != "active" || severity != "critical" || acknowledgedAt != nil || signalCount != 2 {
		t.Fatalf("active incidents were not merged conservatively: status=%s severity=%s ack=%v signals=%d", status, severity, acknowledgedAt, signalCount)
	}
	var resolutionStatus string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM cairnops_notification_outbox
		WHERE incident_id = $1::uuid AND channel_id = $2::uuid AND event_kind = 'resolved'
	`, newerIncidentID, inAppChannelID).Scan(&resolutionStatus); err != nil {
		t.Fatalf("read suppressed technical resolution: %v", err)
	}
	if resolutionStatus != "cancelled" {
		t.Fatalf("technical merge emitted a resolution notification: %s", resolutionStatus)
	}

	targets, err := controlplane.NewStore(pool).ListTargets(ctx)
	if err != nil {
		t.Fatalf("list targets after merge: %v", err)
	}
	if len(targets) != 1 || len(targets[0].Aliases) != 1 || targets[0].Aliases[0] != "Authentik" {
		t.Fatalf("absorbed name is not searchable: %#v", targets)
	}

	finalID := insertReconciliationTarget(t, ctx, pool, "Identité finale")
	if _, err := processor.mergeTargets(ctx, operationWork{
		ID: "10000000-0000-4000-8000-000000000003", Kind: "target_merge",
		PrimaryTargetID: finalID, SecondaryTargetID: primaryID,
		Reason: "Deuxième rapprochement pour vérifier la chaîne", ActorID: actorID,
	}); err != nil {
		t.Fatalf("merge a second identity: %v", err)
	}
	targets, err = controlplane.NewStore(pool).ListTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || len(targets[0].Aliases) != 2 || targets[0].Aliases[0] != "Authentik" || targets[0].Aliases[1] != "trust-auth-01" {
		t.Fatalf("alias chain was not consolidated: %#v", targets)
	}
	resolved, err := NewStore(pool).ResolveTarget(ctx, secondaryID)
	if err != nil || resolved != finalID {
		t.Fatalf("oldest target ID did not resolve through the chain: resolved=%s err=%v", resolved, err)
	}
}

func TestSourceMoveSplitsSharedIncidentAndMovesAttributableHistory(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	actorID := insertReconciliationUser(t, ctx, pool)
	destinationID := insertReconciliationTarget(t, ctx, pool, "Application Authentik")
	originID := insertReconciliationTarget(t, ctx, pool, "Serveur partagé")
	destinationSourceID := insertNativeSource(t, ctx, pool, destinationID, "Disponibilité application")
	movingSourceID := insertNativeSource(t, ctx, pool, originID, "Authentik HTTP")
	remainingSourceID := insertNativeSource(t, ctx, pool, originID, "Charge machine")
	now := time.Now().UTC().Add(-time.Minute)
	destinationIncidentID := insertActiveIncident(t, ctx, pool, destinationID, destinationSourceID, "warning", now.Add(-time.Hour), false, actorID)
	originIncidentID := insertActiveIncident(t, ctx, pool, originID, movingSourceID, "major", now, false, actorID)
	insertNativeSignal(t, ctx, pool, originIncidentID, originID, remainingSourceID, "warning", now)
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_observations (source_id, target_id, observed_at, outcome, latency_milliseconds)
		VALUES ($1::uuid, $2::uuid, $3, 'unhealthy', 100)
	`, movingSourceID, originID, now); err != nil {
		t.Fatalf("insert movable observation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_observation_hours (source_id, target_id, hour, unhealthy, expected, latency_sum_milliseconds, latency_count, latency_maximum_milliseconds)
		VALUES ($1::uuid, $2::uuid, date_trunc('hour', $3::timestamptz), 1, 1, 100, 1, 100)
	`, movingSourceID, originID, now); err != nil {
		t.Fatalf("insert movable rollup: %v", err)
	}

	processor := NewProcessor(pool, "integration-test", slog.Default())
	_, err := processor.moveSource(ctx, operationWork{
		ID: "10000000-0000-4000-8000-000000000002", Kind: "source_move",
		PrimaryTargetID: destinationID, SecondaryTargetID: originID, SourceID: movingSourceID,
		Reason: "La Source décrit l’application", ActorID: actorID,
	})
	if err != nil {
		t.Fatalf("move source: %v", err)
	}

	var sourceTarget, observationTarget, rollupTarget, movedIncident string
	if err := pool.QueryRow(ctx, `SELECT target_id::text FROM cairnops_signal_sources WHERE id = $1::uuid`, movingSourceID).Scan(&sourceTarget); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT target_id::text FROM cairnops_observations WHERE source_id = $1::uuid`, movingSourceID).Scan(&observationTarget); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT target_id::text FROM cairnops_observation_hours WHERE source_id = $1::uuid`, movingSourceID).Scan(&rollupTarget); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT incident_id::text FROM cairnops_incident_signals WHERE source_id = $1::uuid`, movingSourceID).Scan(&movedIncident); err != nil {
		t.Fatal(err)
	}
	if sourceTarget != destinationID || observationTarget != destinationID || rollupTarget != destinationID || movedIncident != destinationIncidentID {
		t.Fatalf("Source history was not reconstructed: source=%s observation=%s rollup=%s incident=%s", sourceTarget, observationTarget, rollupTarget, movedIncident)
	}
	var destinationSeverity string
	var destinationSignals int
	if err := pool.QueryRow(ctx, `
		SELECT effective_severity,
		       (SELECT count(*)::integer FROM cairnops_incident_signals WHERE incident_id = incident.id)
		FROM cairnops_incidents incident WHERE id = $1::uuid
	`, destinationIncidentID).Scan(&destinationSeverity, &destinationSignals); err != nil {
		t.Fatal(err)
	}
	if destinationSeverity != "major" || destinationSignals != 2 {
		t.Fatalf("destination Incident did not absorb active evidence: severity=%s signals=%d", destinationSeverity, destinationSignals)
	}
	var originStatus string
	var originSignals int
	if err := pool.QueryRow(ctx, `
		SELECT status, (SELECT count(*)::integer FROM cairnops_incident_signals WHERE incident_id = incident.id)
		FROM cairnops_incidents incident WHERE id = $1::uuid
	`, originIncidentID).Scan(&originStatus, &originSignals); err != nil {
		t.Fatal(err)
	}
	if originStatus != "active" || originSignals != 1 {
		t.Fatalf("unrelated origin evidence was rewritten: status=%s signals=%d", originStatus, originSignals)
	}
	var originArchived bool
	if err := pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM cairnops_targets WHERE id = $1::uuid`, originID).Scan(&originArchived); err != nil {
		t.Fatal(err)
	}
	if originArchived {
		t.Fatal("origin target was archived without explicit request")
	}
}

func TestDurableReconciliationOperationRunsToCompletion(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	actorID := insertReconciliationUser(t, ctx, pool)
	primaryID := insertReconciliationTarget(t, ctx, pool, "Cible administrée")
	secondaryID := insertReconciliationTarget(t, ctx, pool, "Ancienne identité")

	store := NewStore(pool)
	operation, err := store.Enqueue(ctx, actorID, EnqueueInput{
		Kind: "target_merge", PrimaryTargetID: primaryID, SecondaryTargetID: secondaryID,
		Reason: "Décision durable vérifiée par le test", Confirmation: "Cible administrée",
	})
	if err != nil {
		t.Fatalf("enqueue durable reconciliation: %v", err)
	}
	if operation.Status != "queued" || operation.Stage != "preparing" {
		t.Fatalf("unexpected initial operation state: %#v", operation)
	}

	processor := NewProcessor(pool, "durable-integration-test", slog.Default())
	if err := processor.processOne(ctx); err != nil {
		t.Fatalf("process durable reconciliation: %v", err)
	}
	operations, err := store.ListOperations(ctx, 10)
	if err != nil {
		t.Fatalf("list durable operations: %v", err)
	}
	if len(operations) != 1 || operations[0].ID != operation.ID || operations[0].Status != "succeeded" || operations[0].Stage != "completed" {
		t.Fatalf("operation did not reach completion: %#v", operations)
	}
}

func TestDetectorKeepsStableIdentityConflictAsExplicitWeakLead(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	leftID := insertReconciliationTarget(t, ctx, pool, "Authentik")
	rightID := insertReconciliationTarget(t, ctx, pool, "Authentik")
	leftSourceID := insertNativeSource(t, ctx, pool, leftID, "Authentik")
	rightSourceID := insertNativeSource(t, ctx, pool, rightID, "Authentik")
	if _, err := pool.Exec(ctx, `UPDATE cairnops_signal_sources SET config = jsonb_build_object('machine_id', $2::text) WHERE id = $1::uuid`, leftSourceID, "machine-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_signal_sources SET config = jsonb_build_object('machine_id', $2::text) WHERE id = $1::uuid`, rightSourceID, "machine-b"); err != nil {
		t.Fatal(err)
	}

	detector := NewDetector(pool, slog.Default())
	if err := detector.Refresh(ctx); err != nil {
		t.Fatalf("refresh detector: %v", err)
	}
	suggestions, err := NewStore(pool).ListSuggestions(ctx, "pending")
	if err != nil {
		t.Fatalf("list detected suggestions: %v", err)
	}
	if len(suggestions) != 1 || suggestions[0].Confidence != "low" || len(suggestions[0].Contradictions) != 1 || suggestions[0].Contradictions[0].Kind != "different_machine_id" {
		t.Fatalf("detector did not preserve explicit abstention: %#v", suggestions)
	}
}

func insertReconciliationUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ('reconciliation-admin', 'Reconciliation Admin', 'test-hash', 'administrator')
		RETURNING id::text
	`).Scan(&id); err != nil {
		t.Fatalf("insert reconciliation user: %v", err)
	}
	return id
}

func insertReconciliationTarget(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, name).Scan(&id); err != nil {
		t.Fatalf("insert target %s: %v", name, err)
	}
	return id
}

func insertNativeSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (target_id, name, kind, interval_seconds, timeout_milliseconds, config)
		VALUES ($1::uuid, $2, 'http', 60, 5000, '{"url":"https://example.test"}'::jsonb)
		RETURNING id::text
	`, targetID, name).Scan(&id); err != nil {
		t.Fatalf("insert native source %s: %v", name, err)
	}
	return id
}

func insertActiveIncident(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetID, sourceID, severity string, openedAt time.Time, acknowledged bool, actorID string) string {
	t.Helper()
	var incidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status, source_severity,
			effective_severity, opened_at, acknowledged_at, acknowledged_by, acknowledgement_origin
		) VALUES ($1::uuid, 'availability', 'Indisponibilité', 'active', $2, $2, $3,
		          CASE WHEN $4 THEN now() END,
		          CASE WHEN $4 THEN $5::uuid END,
		          CASE WHEN $4 THEN 'user' END)
		RETURNING id::text
	`, targetID, severity, openedAt, acknowledged, actorID).Scan(&incidentID); err != nil {
		t.Fatalf("insert active incident: %v", err)
	}
	insertNativeSignal(t, ctx, pool, incidentID, targetID, sourceID, severity, openedAt)
	return incidentID
}

func insertNativeSignal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, incidentID, targetID, sourceID, severity string, openedAt time.Time) {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incident_signals (
			incident_id, target_id, origin, source_id, name, active, severity, opened_at
		) VALUES ($1::uuid, $2::uuid, 'native', $3::uuid, 'Preuve native', true, $4, $5)
		RETURNING id::text
	`, incidentID, targetID, sourceID, severity, openedAt).Scan(&id); err != nil {
		t.Fatalf("insert native incident signal: %v", err)
	}
}
