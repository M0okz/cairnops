package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le heartbeat est le seul Contrôle natif dont l'Observation entre par l'API
// plutôt que par le worker. Il emprunte ensuite la même Politique de
// déclenchement, et c'est cette continuité que ce test vérifie de bout en bout.
func TestPostgresHeartbeatFeedsTheTriggerPolicy(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	store := NewStore(pool)

	target, err := store.CreateTarget(ctx, CreateTargetInput{
		Name:        "Sauvegarde nocturne",
		Description: "heartbeat.example.net",
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.CreateSource(ctx, target.ID, CreateSourceInput{
		Name:                "Sauvegarde nocturne",
		Kind:                domain.SourceHeartbeat,
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		FailureThreshold:    2,
		RecoveryThreshold:   2,
		Severity:            "critical",
		Config:              json.RawMessage(`{"expected_every_seconds":60,"grace_seconds":10}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.HeartbeatToken == "" {
		t.Fatal("expected a heartbeat token on creation")
	}
	token := created.HeartbeatToken

	// Une seule Observation défavorable ne conclut rien : le seuil est à deux.
	if _, err := store.ReceiveHeartbeat(ctx, token, HeartbeatPayload{Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	if count := activeNativeSignals(t, pool, created.Source.ID); count != 0 {
		t.Fatalf("a single unfavourable observation opened %d signal(s); the policy requires two", count)
	}

	// La seconde atteint le seuil et alimente l'Incident.
	if _, err := store.ReceiveHeartbeat(ctx, token, HeartbeatPayload{Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	if count := activeNativeSignals(t, pool, created.Source.ID); count != 1 {
		t.Fatalf("expected exactly one active native signal, found %d", count)
	}

	nature, severity, status := incidentForTarget(t, pool, target.ID)
	if nature != "availability" {
		t.Fatalf("native evidence joined nature %q instead of availability", nature)
	}
	if severity != "critical" {
		t.Fatalf("incident carries severity %q instead of the source's critical", severity)
	}
	if status != "active" {
		t.Fatalf("incident status is %q instead of active", status)
	}

	// Un premier retour sain ne confirme pas encore le rétablissement.
	if _, err := store.ReceiveHeartbeat(ctx, token, HeartbeatPayload{Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if count := activeNativeSignals(t, pool, created.Source.ID); count != 1 {
		t.Fatal("a single healthy observation resolved the signal before the recovery threshold")
	}

	// Le second le confirme.
	if _, err := store.ReceiveHeartbeat(ctx, token, HeartbeatPayload{Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if count := activeNativeSignals(t, pool, created.Source.ID); count != 0 {
		t.Fatalf("the recovery threshold was reached but %d signal(s) remain active", count)
	}
	if _, _, status := incidentForTarget(t, pool, target.ID); status != "resolved" {
		t.Fatalf("incident status is %q instead of resolved", status)
	}
}

// Les compteurs vivent sur la Source : la décision survit à un redémarrage du
// worker, puisqu'elle ne dépend d'aucun état conservé en mémoire.
func TestPostgresTriggerStreaksArePersistedOnTheSource(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	store := NewStore(pool)

	target, err := store.CreateTarget(ctx, CreateTargetInput{Name: "Cible à compteurs"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSource(ctx, target.ID, CreateSourceInput{
		Name:                "Tâche planifiée",
		Kind:                domain.SourceHeartbeat,
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		FailureThreshold:    3,
		RecoveryThreshold:   2,
		Config:              json.RawMessage(`{"expected_every_seconds":60,"grace_seconds":10}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.ReceiveHeartbeat(ctx, created.HeartbeatToken, HeartbeatPayload{Status: "failed"}); err != nil {
		t.Fatal(err)
	}

	var unhealthy, healthy int
	if err := pool.QueryRow(ctx, `
		SELECT consecutive_unhealthy, consecutive_healthy
		FROM cairnops_signal_sources WHERE id = $1::uuid
	`, created.Source.ID).Scan(&unhealthy, &healthy); err != nil {
		t.Fatal(err)
	}
	if unhealthy != 1 || healthy != 0 {
		t.Fatalf("streaks are %d unfavourable / %d healthy; expected 1 / 0", unhealthy, healthy)
	}

	// Une Observation saine remet à zéro la suite défavorable en cours.
	if _, err := store.ReceiveHeartbeat(ctx, created.HeartbeatToken, HeartbeatPayload{Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT consecutive_unhealthy, consecutive_healthy
		FROM cairnops_signal_sources WHERE id = $1::uuid
	`, created.Source.ID).Scan(&unhealthy, &healthy); err != nil {
		t.Fatal(err)
	}
	if unhealthy != 0 || healthy != 1 {
		t.Fatalf("streaks are %d unfavourable / %d healthy; expected 0 / 1", unhealthy, healthy)
	}
}

// La Politique de déclenchement se règle à la création, dans des bornes que le
// stockage impose aussi : la validation refuse avant d'atteindre PostgreSQL.
func TestPostgresSourceTriggerPolicyDefaultsAndBounds(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	store := NewStore(pool)

	target, err := store.CreateTarget(ctx, CreateTargetInput{Name: "Cible à politique"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.CreateSource(ctx, target.ID, CreateSourceInput{
		Name:                "Contrôle par défaut",
		Kind:                domain.SourceHTTP,
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		Config:              json.RawMessage(`{"url":"https://example.net/status","method":"GET","accepted_statuses":[200]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Source.FailureThreshold != domain.DefaultFailureThreshold ||
		created.Source.RecoveryThreshold != domain.DefaultRecoveryThreshold {
		t.Fatalf("defaults are %d / %d; expected %d / %d",
			created.Source.FailureThreshold, created.Source.RecoveryThreshold,
			domain.DefaultFailureThreshold, domain.DefaultRecoveryThreshold)
	}
	if created.Source.Severity != "major" {
		t.Fatalf("default severity is %q instead of major", created.Source.Severity)
	}

	for _, invalid := range []CreateSourceInput{
		{Name: "Seuil trop bas", FailureThreshold: 0, RecoveryThreshold: -1},
		{Name: "Seuil trop haut", FailureThreshold: 11, RecoveryThreshold: 2},
		{Name: "Gravité inconnue", Severity: "catastrophic"},
	} {
		input := invalid
		input.Kind = domain.SourceHTTP
		input.IntervalSeconds = 60
		input.TimeoutMilliseconds = 5000
		input.Config = json.RawMessage(`{"url":"https://example.net/status","method":"GET","accepted_statuses":[200]}`)
		if _, err := store.CreateSource(ctx, target.ID, input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%q was accepted or failed for the wrong reason: %v", invalid.Name, err)
		}
	}
}

// Archiver retire la Cible du service sans effacer son passé : ses Incidents
// actifs se résolvent, ses Contrôles cessent d'être dus, et plus aucun signal
// ne la ressuscite tant qu'elle n'est pas restaurée.
func TestPostgresArchivingATargetClosesItsPresentAndKeepsItsPast(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	store := NewStore(pool)

	target, err := store.CreateTarget(ctx, CreateTargetInput{Name: "Cible à archiver"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSource(ctx, target.ID, CreateSourceInput{
		Name: "Endpoint public", Kind: domain.SourceHTTP, IntervalSeconds: 20, TimeoutMilliseconds: 5000,
		FailureThreshold: 1, RecoveryThreshold: 1,
		Config: json.RawMessage(`{"url":"https://example.net/status"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	seedActiveIncident(t, pool, target.ID, created.Source.ID)
	if activeIncidents(t, pool, target.ID) != 1 {
		t.Fatal("the fixture should have opened an incident")
	}

	if err := store.ArchiveTarget(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	if active := activeIncidents(t, pool, target.ID); active != 0 {
		t.Fatalf("archiving must resolve the active incidents, %d remain", active)
	}
	// Le passé reste lisible : l'Incident existe toujours, résolu et motivé.
	var resolved int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_incidents incident
		JOIN cairnops_incident_activity activity ON activity.incident_id = incident.id
		WHERE incident.target_id = $1::uuid AND incident.status = 'resolved'
		  AND activity.message = 'Incident résolu : la Cible a été archivée'
	`, target.ID).Scan(&resolved); err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Fatalf("the journal must keep the reason, got %d entries", resolved)
	}

	// La Cible quitte les listes et ses Contrôles cessent d'être dus.
	listed, err := store.ListTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range listed {
		if candidate.ID == target.ID {
			t.Fatal("an archived target must leave the operational lists")
		}
	}
	claimed, err := monitoringDue(t, pool, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed != 0 {
		t.Fatalf("an archived target keeps %d controls due", claimed)
	}

	// Modifier une Cible archivée n'a pas de sens tant qu'elle n'est pas revenue.
	if _, err := store.UpdateTarget(ctx, target.ID, UpdateTargetInput{Name: "Autre nom"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected an archived target to be unreachable, got %v", err)
	}
	if _, err := store.RestoreTarget(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.UpdateTarget(ctx, target.ID, UpdateTargetInput{Name: "Cible restaurée", Description: "de retour"})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Cible restaurée" {
		t.Fatalf("unexpected rename: %#v", renamed)
	}
	// La restauration ne rouvre aucun Incident : il faudra une preuve fraîche.
	if active := activeIncidents(t, pool, target.ID); active != 0 {
		t.Fatalf("restoring must not reopen an incident, %d active", active)
	}
}

// Un Contrôle natif se modifie entièrement ; une Source d'Intégration, jamais.
func TestPostgresUpdateSourceKeepsAbsentFieldsAndProtectsIntegrations(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	store := NewStore(pool)

	target, err := store.CreateTarget(ctx, CreateTargetInput{Name: "Cible à corriger"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSource(ctx, target.ID, CreateSourceInput{
		Name: "Endpoint public", Kind: domain.SourceHTTP, IntervalSeconds: 60, TimeoutMilliseconds: 5000,
		Config: json.RawMessage(`{"url":"https://example.net/status"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	suspended := false
	updated, err := store.UpdateSource(ctx, created.Source.ID, UpdateSourceInput{Enabled: &suspended})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.Name != "Endpoint public" || updated.IntervalSeconds != 60 {
		t.Fatalf("an absent field must stay unchanged: %#v", updated)
	}

	interval := 20
	config := json.RawMessage(`{"url":"https://example.net/health"}`)
	if _, err := store.UpdateSource(ctx, created.Source.ID, UpdateSourceInput{
		IntervalSeconds: &interval, Config: config,
	}); err != nil {
		t.Fatal(err)
	}
	var storedURL string
	if err := pool.QueryRow(ctx,
		`SELECT config->>'url' FROM cairnops_signal_sources WHERE id = $1::uuid`, created.Source.ID,
	).Scan(&storedURL); err != nil {
		t.Fatal(err)
	}
	if storedURL != "https://example.net/health" {
		t.Fatalf("the configuration was not corrected, got %q", storedURL)
	}

	// Un délai devenu plus long que l'intervalle est refusé, comme à la création.
	longTimeout := 30000
	if _, err := store.UpdateSource(ctx, created.Source.ID, UpdateSourceInput{TimeoutMilliseconds: &longTimeout}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected a rejected timeout, got %v", err)
	}

	integrationSourceID := seedIntegrationSource(t, pool, target.ID)
	if _, err := store.UpdateSource(ctx, integrationSourceID, UpdateSourceInput{Enabled: &suspended}); !errors.Is(err, ErrIntegrationOwned) {
		t.Fatalf("an integration source must not be editable, got %v", err)
	}
	if err := store.DeleteSource(ctx, integrationSourceID); !errors.Is(err, ErrIntegrationOwned) {
		t.Fatalf("an integration source must not be deletable, got %v", err)
	}
	if err := store.DeleteSource(ctx, created.Source.ID); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT count(*)::integer FROM cairnops_observations WHERE source_id = $1::uuid`, created.Source.ID,
	).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("deleting a control must take its observations, %d remain", remaining)
	}
}

func TestPostgresDeleteSourceResolvesIncidentWhoseLastSignalDisappears(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	store := NewStore(pool)

	target, err := store.CreateTarget(ctx, CreateTargetInput{Name: "Cible avec Incident"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSource(ctx, target.ID, CreateSourceInput{
		Name: "Endpoint public", Kind: domain.SourceHTTP, IntervalSeconds: 60, TimeoutMilliseconds: 5000,
		Config: json.RawMessage(`{"url":"https://example.net/status"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	seedActiveIncident(t, pool, target.ID, created.Source.ID)

	if err := store.DeleteSource(ctx, created.Source.ID); err != nil {
		t.Fatal(err)
	}
	if active := activeIncidents(t, pool, target.ID); active != 0 {
		t.Fatalf("deleting the last Source must not leave an active Incident with 0/0 evidence, got %d", active)
	}
	var explained int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer
		FROM cairnops_incident_activity activity
		JOIN cairnops_incidents incident ON incident.id = activity.incident_id
		WHERE incident.target_id = $1::uuid AND activity.kind = 'resolved'
		  AND activity.data->>'source_id' = $2
	`, target.ID, created.Source.ID).Scan(&explained); err != nil {
		t.Fatal(err)
	}
	if explained != 1 {
		t.Fatalf("source deletion must explain the automatic resolution, got %d Activity Log entries", explained)
	}
}

// seedActiveIncident pose un Incident actif et sa preuve, tels que la Politique
// de déclenchement les produit — ce que ce test archive plutôt qu'il ne le
// rejoue.
func seedActiveIncident(t *testing.T, pool *pgxpool.Pool, targetID, sourceID string) {
	t.Helper()
	ctx := context.Background()
	var incidentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status, source_severity, effective_severity, opened_at
		) VALUES ($1::uuid, 'availability', 'Indisponibilité', 'active', 'major', 'major', now())
		RETURNING id::text
	`, targetID).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_incident_signals (
			incident_id, target_id, origin, source_id, name, active, severity, opened_at, last_seen_at
		) VALUES ($1::uuid, $2::uuid, 'native', $3::uuid, 'Endpoint public', true, 'major', now(), now())
	`, incidentID, targetID, sourceID); err != nil {
		t.Fatal(err)
	}
}

func activeIncidents(t *testing.T, pool *pgxpool.Pool, targetID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*)::integer FROM cairnops_incidents
		WHERE target_id = $1::uuid AND status = 'active'
	`, targetID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func monitoringDue(t *testing.T, pool *pgxpool.Pool, targetID string) (int, error) {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `
		SELECT count(*)::integer
		FROM cairnops_signal_sources source
		JOIN cairnops_targets target ON target.id = source.target_id
		WHERE source.target_id = $1::uuid AND source.enabled
		  AND source.origin = 'native' AND target.archived_at IS NULL
	`, targetID).Scan(&count)
	return count, err
}

func seedIntegrationSource(t *testing.T, pool *pgxpool.Pool, targetID string) string {
	t.Helper()
	ctx := context.Background()
	var connectorID, bindingID, sourceID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connectors (kind, name, endpoint, credential_sealed, status, compatibility, encrypted_transport, sync_interval_seconds)
		VALUES ('uptime_kuma', $1, $2, 'sealed-credential-with-sufficient-length', 'connected', 'supported', true, 30)
		RETURNING id::text
	`, "Homelab Kuma", "https://kuma.example.net/metrics").Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_connector_bindings (connector_id, target_id, external_id, external_name)
		VALUES ($1::uuid, $2::uuid, '12', 'Moniteur importé')
		RETURNING id::text
	`, connectorID, targetID).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_signal_sources (
			target_id, name, kind, origin, connector_binding_id,
			interval_seconds, timeout_milliseconds, config
		) VALUES ($1::uuid, 'Moniteur importé', 'uptime_kuma', 'integration', $2::uuid, 30, 1000, '{}'::jsonb)
		RETURNING id::text
	`, targetID, bindingID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	return sourceID
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testsupport.Pool(t)
}

func activeNativeSignals(t *testing.T, pool *pgxpool.Pool, sourceID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM cairnops_incident_signals
		WHERE origin = 'native' AND source_id = $1::uuid AND active
	`, sourceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func incidentForTarget(t *testing.T, pool *pgxpool.Pool, targetID string) (nature, severity, status string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT nature_key, effective_severity, status
		FROM cairnops_incidents
		WHERE target_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT 1
	`, targetID).Scan(&nature, &severity, &status); err != nil {
		t.Fatal(err)
	}
	return nature, severity, status
}
