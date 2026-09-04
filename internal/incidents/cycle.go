package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// EvidenceFact est le langage commun remis au cycle par les adapters produit.
// IdentityScope et IdentityKey rendent une Preuve idempotente sans exposer au
// cycle la manière dont Zabbix, Kuma ou un Contrôle natif nomment leurs objets.
type EvidenceFact struct {
	Origin               string
	ConnectorID          string
	BindingID            string
	SourceID             string
	IdentityScope        string
	IdentityKey          string
	TargetID             string
	ExternalEventID      string
	ExternalObjectID     string
	Nature               NatureIdentity
	Name                 string
	Severity             Severity
	OpenedAt             time.Time
	UpstreamAcknowledged bool
	Metadata             map[string]any
}

type EvidenceSnapshot struct {
	Origin            string
	ConnectorID       string
	ObservedAt        time.Time
	CompleteConnector bool
	ObservedScopes    []string
	Facts             []EvidenceFact
}

func (store *PostgresStore) ReconcileZabbix(ctx context.Context, input ReconcileZabbixInput) error {
	facts := make([]EvidenceFact, 0, len(input.Signals))
	for _, signal := range input.Signals {
		fingerprint := strings.TrimSpace(signal.NatureFingerprint)
		if fingerprint == "" {
			fingerprint = strings.TrimSpace(signal.ExternalObjectID)
		}
		nature := ConnectorNature(input.ConnectorID,
			"zabbix:"+input.ConnectorID+":"+fingerprint, signal.Name, fingerprint)
		if signal.CanonicalNature == NatureAvailability {
			nature = CanonicalNature(NatureAvailability, NatureAvailabilityLabel)
		}
		facts = append(facts, EvidenceFact{
			Origin: "zabbix", ConnectorID: input.ConnectorID,
			BindingID: signal.BindingID, IdentityScope: signal.BindingID,
			IdentityKey: signal.ExternalEventID, TargetID: signal.TargetID,
			ExternalEventID:  signal.ExternalEventID,
			ExternalObjectID: signal.ExternalObjectID, Nature: nature,
			Name: signal.Name, Severity: signal.Severity,
			OpenedAt:             signal.OpenedAt,
			UpstreamAcknowledged: signal.UpstreamAcknowledged,
			Metadata:             map[string]any{"suppressed": signal.Suppressed},
		})
	}
	return store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "zabbix", ConnectorID: input.ConnectorID,
		ObservedAt: input.ObservedAt, CompleteConnector: true, Facts: facts,
	})
}

func (store *PostgresStore) ReconcileUptimeKuma(ctx context.Context, input ReconcileUptimeKumaInput) error {
	facts := make([]EvidenceFact, 0, len(input.Signals))
	for _, signal := range input.Signals {
		facts = append(facts, EvidenceFact{
			Origin: "uptime_kuma", ConnectorID: input.ConnectorID,
			BindingID: signal.BindingID, IdentityScope: signal.BindingID,
			IdentityKey: "availability", TargetID: signal.TargetID,
			ExternalEventID:  signal.ExternalMonitor,
			ExternalObjectID: signal.ExternalMonitor,
			Nature:           CanonicalNature(NatureAvailability, NatureAvailabilityLabel),
			Name:             signal.Name, Severity: signal.Severity, OpenedAt: input.ObservedAt,
		})
	}
	return store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "uptime_kuma", ConnectorID: input.ConnectorID,
		ObservedAt: input.ObservedAt, ObservedScopes: input.ObservedBindings, Facts: facts,
	})
}

func (store *PostgresStore) ReconcilePatchMon(ctx context.Context, input ReconcilePatchMonInput) error {
	facts := make([]EvidenceFact, 0, len(input.Signals))
	for _, signal := range input.Signals {
		facts = append(facts, EvidenceFact{
			Origin: "patchmon", ConnectorID: input.ConnectorID,
			BindingID: signal.BindingID, IdentityScope: signal.BindingID,
			IdentityKey: signal.ConditionKey, TargetID: signal.TargetID,
			ExternalEventID:  signal.ConditionKey,
			ExternalObjectID: signal.ConditionKey,
			Nature: ConnectorNature(input.ConnectorID, signal.NatureKey,
				signal.NatureLabel, signal.NatureKey),
			Name: signal.Name, Severity: signal.Severity, OpenedAt: input.ObservedAt,
			Metadata: cloneMetadata(signal.Details),
		})
	}
	return store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "patchmon", ConnectorID: input.ConnectorID,
		ObservedAt: input.ObservedAt, ObservedScopes: input.ObservedBindings, Facts: facts,
	})
}

func (store *PostgresStore) ReconcileArgus(ctx context.Context, input ReconcileArgusInput) error {
	facts := make([]EvidenceFact, 0, len(input.Signals))
	for _, signal := range input.Signals {
		metadata := cloneMetadata(signal.Details)
		metadata["deployed_version"] = signal.DeployedVersion
		metadata["latest_version"] = signal.LatestVersion
		facts = append(facts, EvidenceFact{
			Origin: "argus", ConnectorID: input.ConnectorID,
			BindingID: signal.BindingID, IdentityScope: signal.BindingID,
			IdentityKey: "software_update", TargetID: signal.TargetID,
			ExternalEventID:  signal.LatestVersion,
			ExternalObjectID: "software_update",
			Nature: ConnectorNature(input.ConnectorID, signal.NatureKey,
				signal.NatureLabel, signal.NatureKey),
			Name: signal.Name, Severity: signal.Severity, OpenedAt: input.ObservedAt,
			Metadata: metadata,
		})
	}
	return store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
		Origin: "argus", ConnectorID: input.ConnectorID,
		ObservedAt: input.ObservedAt, ObservedScopes: input.ObservedBindings, Facts: facts,
	})
}

func (store *PostgresStore) ApplyWebhook(ctx context.Context, signal WebhookSignal) error {
	observedAt := normalizedTime(signal.ObservedAt)
	fact := EvidenceFact{
		Origin: "webhook", ConnectorID: signal.ConnectorID,
		BindingID: signal.BindingID, IdentityScope: signal.BindingID,
		IdentityKey: signal.ExternalEventKey, TargetID: signal.TargetID,
		ExternalEventID:  signal.ExternalEventKey,
		ExternalObjectID: signal.ExternalEventKey,
		Nature: ConnectorNature(signal.ConnectorID, signal.NatureKey,
			signal.NatureLabel, signal.NatureKey),
		Name: signal.Summary, Severity: signal.Severity, OpenedAt: observedAt,
		Metadata: cloneMetadata(signal.Details),
	}
	if signal.Status == "firing" {
		return store.ApplyEvidenceSnapshot(ctx, EvidenceSnapshot{
			Origin: "webhook", ConnectorID: signal.ConnectorID,
			ObservedAt: observedAt, Facts: []EvidenceFact{fact},
		})
	}
	if signal.Status != "resolved" {
		return fmt.Errorf("%w: webhook status must be firing or resolved", ErrInvalidInput)
	}
	return store.ResolveEvidence(ctx, "webhook", signal.BindingID,
		signal.ExternalEventKey, observedAt)
}

func cloneMetadata(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func normalizedTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

// ApplyEvidenceSnapshot est l'interface d'écriture principale du module. Une
// lecture partielle ne résout que les Preuves appartenant aux scopes réellement
// observés ; une lecture complète peut résoudre toute Preuve absente du
// Connecteur.
func (store *PostgresStore) ApplyEvidenceSnapshot(ctx context.Context, snapshot EvidenceSnapshot) error {
	observedAt := normalizedTime(snapshot.ObservedAt)
	facts := append([]EvidenceFact(nil), snapshot.Facts...)
	for index := range facts {
		facts[index].Origin = snapshot.Origin
		facts[index].ConnectorID = snapshot.ConnectorID
		if facts[index].OpenedAt.IsZero() {
			facts[index].OpenedAt = observedAt
		} else {
			facts[index].OpenedAt = facts[index].OpenedAt.UTC()
		}
	}
	sort.SliceStable(facts, func(left, right int) bool {
		if facts[left].OpenedAt.Equal(facts[right].OpenedAt) {
			return facts[left].IdentityScope+"\x00"+facts[left].IdentityKey <
				facts[right].IdentityScope+"\x00"+facts[right].IdentityKey
		}
		return facts[left].OpenedAt.Before(facts[right].OpenedAt)
	})
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin evidence cycle: %w", err)
	}
	defer tx.Rollback(ctx)

	seen := make(map[string]struct{}, len(facts))
	impacts := make(map[string]string)
	incidents := make(map[string]struct{})
	for _, fact := range facts {
		key := fact.IdentityScope + "\x00" + fact.IdentityKey
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		incidentID, impactID, err := applyEvidenceFact(ctx, tx, fact, observedAt)
		if errors.Is(err, ErrTargetArchived) {
			continue
		}
		if err != nil {
			return err
		}
		if incidentID != "" {
			incidents[incidentID] = struct{}{}
			impacts[impactID] = incidentID
		}
	}

	active, err := activeEvidenceForSnapshot(ctx, tx, snapshot)
	if err != nil {
		return err
	}
	for _, item := range active {
		if _, present := seen[item.scope+"\x00"+item.key]; present {
			continue
		}
		if err := resolveEvidenceRow(ctx, tx, item.id, item.incidentID,
			item.impactID, item.name, snapshot.Origin, observedAt); err != nil {
			return err
		}
		incidents[item.incidentID] = struct{}{}
		impacts[item.impactID] = item.incidentID
	}
	if err := rearmRecoveredEvidence(ctx, tx, snapshot, seen, observedAt); err != nil {
		return err
	}

	for impactID := range impacts {
		if err := recomputeImpact(ctx, tx, impactID, observedAt); err != nil {
			return err
		}
	}
	for incidentID := range incidents {
		if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
			return err
		}
	}
	if err := advanceDueIncidents(ctx, tx, observedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit evidence cycle: %w", err)
	}
	return nil
}

type activeEvidence struct{ id, incidentID, impactID, scope, key, name string }

func activeEvidenceForSnapshot(ctx context.Context, tx pgx.Tx, snapshot EvidenceSnapshot) ([]activeEvidence, error) {
	if !snapshot.CompleteConnector && len(snapshot.ObservedScopes) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, incident_id::text, impact_id::text,
		       identity_scope, identity_key, name
		FROM cairnops_incident_evidence
		WHERE origin = $1 AND connector_id = $2::uuid AND active
		  AND ($3 OR identity_scope = ANY($4::text[]))
		FOR UPDATE
	`, snapshot.Origin, snapshot.ConnectorID,
		snapshot.CompleteConnector, snapshot.ObservedScopes)
	if err != nil {
		return nil, fmt.Errorf("list active evidence: %w", err)
	}
	defer rows.Close()
	result := make([]activeEvidence, 0)
	for rows.Next() {
		var item activeEvidence
		if err := rows.Scan(&item.id, &item.incidentID, &item.impactID,
			&item.scope, &item.key, &item.name); err != nil {
			return nil, fmt.Errorf("scan active evidence: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func applyEvidenceFact(ctx context.Context, tx pgx.Tx, fact EvidenceFact, observedAt time.Time) (string, string, error) {
	if strings.TrimSpace(fact.IdentityScope) == "" || strings.TrimSpace(fact.IdentityKey) == "" ||
		strings.TrimSpace(fact.TargetID) == "" || strings.TrimSpace(fact.Nature.Key) == "" ||
		strings.TrimSpace(fact.Name) == "" {
		return "", "", fmt.Errorf("%w: incomplete incident evidence", ErrInvalidInput)
	}
	if !validSeverity(fact.Severity) {
		return "", "", fmt.Errorf("%w: invalid evidence severity", ErrInvalidInput)
	}
	if err := lockIncidentCycle(ctx, tx, fact); err != nil {
		return "", "", err
	}

	var existingID, incidentID, impactID, targetID, natureKey string
	var invalidated bool
	err := tx.QueryRow(ctx, `
		SELECT evidence.id::text, evidence.incident_id::text,
		       evidence.impact_id::text, evidence.target_id::text,
		       incident.nature_key, evidence.invalidated_at IS NOT NULL
		FROM cairnops_incident_evidence evidence
		JOIN cairnops_incidents incident ON incident.id = evidence.incident_id
		WHERE evidence.origin = $1 AND evidence.identity_scope = $2
		  AND evidence.identity_key = $3 AND evidence.active
		FOR UPDATE OF evidence
	`, fact.Origin, fact.IdentityScope, fact.IdentityKey).Scan(
		&existingID, &incidentID, &impactID, &targetID, &natureKey, &invalidated,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("find active evidence: %w", err)
	}
	if err == nil && invalidated {
		_, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET last_seen_at = $2, updated_at = now() WHERE id = $1::uuid
		`, existingID, observedAt)
		return incidentID, impactID, err
	}
	if err == nil && targetID == fact.TargetID && natureKey == fact.Nature.Key {
		metadata, marshalErr := jsonBytes(fact.Metadata)
		if marshalErr != nil {
			return "", "", marshalErr
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET name = $2, severity = $3, external_event_id = $4,
			    external_object_id = $5, upstream_acknowledged = $6,
			    acknowledgement_sync_status = CASE
			        WHEN $6 THEN 'synchronized'
			        WHEN EXISTS (
			            SELECT 1 FROM cairnops_incidents incident
			            WHERE incident.id = cairnops_incident_evidence.incident_id
			              AND incident.acknowledged_at IS NOT NULL
			        ) THEN 'pending'
			        ELSE acknowledgement_sync_status
			    END,
			    acknowledgement_sync_error = CASE
			        WHEN $6 OR EXISTS (
			            SELECT 1 FROM cairnops_incidents incident
			            WHERE incident.id = cairnops_incident_evidence.incident_id
			              AND incident.acknowledged_at IS NOT NULL
			        ) THEN '' ELSE acknowledgement_sync_error
			    END,
			    acknowledgement_synced_at = CASE
			        WHEN $6 THEN $7
			        WHEN EXISTS (
			            SELECT 1 FROM cairnops_incidents incident
			            WHERE incident.id = cairnops_incident_evidence.incident_id
			              AND incident.acknowledged_at IS NOT NULL
			        ) THEN NULL
			        ELSE acknowledgement_synced_at
			    END,
			    last_seen_at = $7, metadata = $8::jsonb, updated_at = now()
			WHERE id = $1::uuid
		`, existingID, fact.Name, fact.Severity, fact.ExternalEventID,
			fact.ExternalObjectID, fact.UpstreamAcknowledged, observedAt, metadata); err != nil {
			return "", "", fmt.Errorf("refresh incident evidence: %w", err)
		}
		if fact.UpstreamAcknowledged {
			if err := acknowledgeFromEvidence(ctx, tx, incidentID, existingID, observedAt); err != nil {
				return "", "", err
			}
		}
		return incidentID, impactID, nil
	}
	if err == nil {
		if err := resolveEvidenceRow(ctx, tx, existingID, incidentID, impactID,
			fact.Name, fact.Origin, observedAt); err != nil {
			return "", "", err
		}
		if err := recomputeImpact(ctx, tx, impactID, observedAt); err != nil {
			return "", "", err
		}
		if err := recomputeIncident(ctx, tx, incidentID, observedAt); err != nil {
			return "", "", err
		}
	}

	var invalidatedID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM cairnops_incident_evidence
		WHERE origin = $1 AND identity_scope = $2 AND identity_key = $3
		  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
		ORDER BY invalidated_at DESC LIMIT 1 FOR UPDATE
	`, fact.Origin, fact.IdentityScope, fact.IdentityKey).Scan(&invalidatedID)
	if err == nil {
		_, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET last_seen_at = $2, updated_at = now() WHERE id = $1::uuid
		`, invalidatedID, observedAt)
		return "", "", err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("find invalidated evidence: %w", err)
	}

	incidentID, impactID, createdIncident, createdImpact, err := ensureImpact(ctx, tx, fact, observedAt)
	if err != nil {
		return "", "", err
	}
	metadata, err := jsonBytes(fact.Metadata)
	if err != nil {
		return "", "", err
	}
	var evidenceID string
	err = tx.QueryRow(ctx, `
		INSERT INTO cairnops_incident_evidence (
			incident_id, impact_id, target_id, origin, connector_id,
			connector_binding_id, source_id, identity_scope, identity_key,
			external_event_id, external_object_id, name, active, severity,
			opened_at, upstream_acknowledged, acknowledgement_sync_status,
			acknowledgement_synced_at, last_seen_at, metadata
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5::uuid, $6::uuid, $7::uuid,
			$8, $9, $10, $11, $12, true, $13, $14, $15, $16, $17,
			$18, $19::jsonb
		) RETURNING id::text
	`, incidentID, impactID, fact.TargetID, fact.Origin, nullableUUID(fact.ConnectorID),
		nullableUUID(fact.BindingID), nullableUUID(fact.SourceID),
		fact.IdentityScope, fact.IdentityKey, fact.ExternalEventID,
		fact.ExternalObjectID, fact.Name, fact.Severity, fact.OpenedAt,
		fact.UpstreamAcknowledged, acknowledgementStatus(fact.UpstreamAcknowledged),
		acknowledgementTime(fact.UpstreamAcknowledged, observedAt), observedAt, metadata).Scan(&evidenceID)
	if err != nil {
		return "", "", fmt.Errorf("insert incident evidence: %w", err)
	}
	if createdIncident {
		if err := appendActivity(ctx, tx, incidentID, impactID, evidenceID,
			"opened", fact.Origin, "", "Incident ouvert", fact.Metadata); err != nil {
			return "", "", err
		}
	} else if createdImpact {
		if err := appendActivity(ctx, tx, incidentID, impactID, evidenceID,
			"impact_joined", fact.Origin, "", fact.Name,
			map[string]any{"target_id": fact.TargetID}); err != nil {
			return "", "", err
		}
	}
	if err := appendActivity(ctx, tx, incidentID, impactID, evidenceID,
		"evidence_added", fact.Origin, "", fact.Name, fact.Metadata); err != nil {
		return "", "", err
	}
	if fact.UpstreamAcknowledged {
		if err := acknowledgeFromEvidence(ctx, tx, incidentID, evidenceID, observedAt); err != nil {
			return "", "", err
		}
	} else if fact.Origin == "zabbix" {
		// L'acquittement couvre aussi les Preuves qui rejoignent un Incident
		// encore en propagation. Le worker reprendra ce statut pending sans
		// demander un nouveau geste utilisateur.
		command, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence evidence
			SET acknowledgement_sync_status = 'pending',
			    acknowledgement_sync_error = '', acknowledgement_synced_at = NULL,
			    updated_at = now()
			FROM cairnops_incidents incident
			WHERE evidence.id = $1::uuid AND incident.id = evidence.incident_id
			  AND incident.acknowledged_at IS NOT NULL
		`, evidenceID)
		if err != nil {
			return "", "", fmt.Errorf("queue inherited evidence acknowledgement: %w", err)
		}
		if command.RowsAffected() > 0 {
			if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incidents
			SET acknowledgement_sync_status = 'pending',
			    acknowledgement_sync_error = '', revision = revision + 1,
			    updated_at = now()
			WHERE id = $1::uuid AND acknowledged_at IS NOT NULL
			`, incidentID); err != nil {
				return "", "", fmt.Errorf("queue inherited acknowledgement: %w", err)
			}
		}
	}
	return incidentID, impactID, nil
}

// Une seule transaction à la fois décide de l'Incident d'une Nature fiable.
// Le verrou couvre aussi la lecture d'une Preuve existante : deux runners qui
// observent le même fait au même instant ne peuvent donc ni ouvrir deux cycles,
// ni se heurter sur l'identité active de la Preuve.
func lockIncidentCycle(ctx context.Context, tx pgx.Tx, fact EvidenceFact) error {
	key := "target:" + fact.TargetID + ":" + fact.Nature.Key
	if fact.Nature.Eligible {
		key = "nature:" + fact.Nature.Scope + ":" + fact.Nature.Namespace + ":" + fact.Nature.Fingerprint
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("lock incident cycle: %w", err)
	}
	return nil
}

func ensureImpact(ctx context.Context, tx pgx.Tx, fact EvidenceFact, observedAt time.Time) (string, string, bool, bool, error) {
	var archived bool
	if err := tx.QueryRow(ctx, `
		SELECT archived_at IS NOT NULL FROM cairnops_targets WHERE id = $1::uuid
	`, fact.TargetID).Scan(&archived); errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, false, ErrNotFound
	} else if err != nil {
		return "", "", false, false, fmt.Errorf("load evidence target: %w", err)
	}
	if archived {
		return "", "", false, false, ErrTargetArchived
	}

	var incidentID, impactID string
	err := tx.QueryRow(ctx, `
		SELECT incident.id::text, impact.id::text
		FROM cairnops_incident_impacts impact
		JOIN cairnops_incidents incident ON incident.id = impact.incident_id
		WHERE impact.target_id = $1::uuid AND impact.status = 'active'
		  AND incident.status = 'active' AND incident.nature_key = $2
		ORDER BY impact.opened_at DESC LIMIT 1
		FOR UPDATE OF impact, incident
	`, fact.TargetID, fact.Nature.Key).Scan(&incidentID, &impactID)
	if err == nil {
		return incidentID, impactID, false, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, false, fmt.Errorf("find active target impact: %w", err)
	}

	window, err := evidenceWindow(ctx, tx, fact)
	if err != nil {
		return "", "", false, false, err
	}
	if fact.Nature.Eligible {
		err = tx.QueryRow(ctx, `
			SELECT id::text FROM cairnops_incidents
			WHERE status = 'active' AND propagation_status = 'open'
			  AND propagation_eligible
			  AND nature_scope = $1 AND nature_namespace = $2
			  AND nature_fingerprint = $3
			  AND opened_at <= $4 AND propagation_ends_at >= $4
			ORDER BY opened_at LIMIT 1 FOR UPDATE
		`, fact.Nature.Scope, fact.Nature.Namespace,
			fact.Nature.Fingerprint, fact.OpenedAt).Scan(&incidentID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, false, fmt.Errorf("find propagating incident: %w", err)
		}
		if err == nil {
			err = tx.QueryRow(ctx, `
				SELECT id::text FROM cairnops_incident_impacts
				WHERE incident_id = $1::uuid AND target_id = $2::uuid
				FOR UPDATE
			`, incidentID, fact.TargetID).Scan(&impactID)
			if err == nil {
				if _, err := tx.Exec(ctx, `
					UPDATE cairnops_incident_impacts
					SET status = 'active', resolved_at = NULL,
					    source_severity = $2, effective_severity = $2,
					    updated_at = now() WHERE id = $1::uuid
				`, impactID, fact.Severity); err != nil {
					return "", "", false, false, fmt.Errorf("reopen target impact: %w", err)
				}
				if err := appendActivity(ctx, tx, incidentID, impactID, "",
					"impact_reopened", fact.Origin, "", fact.Name, nil); err != nil {
					return "", "", false, false, err
				}
				return incidentID, impactID, false, false, extendPropagation(ctx, tx, incidentID, fact.OpenedAt, window)
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return "", "", false, false, fmt.Errorf("find previous target impact: %w", err)
			}
			if err := tx.QueryRow(ctx, `
				INSERT INTO cairnops_incident_impacts (
					incident_id, target_id, status, source_severity,
					effective_severity, opened_at
				) VALUES ($1::uuid, $2::uuid, 'active', $3, $3, $4)
				RETURNING id::text
			`, incidentID, fact.TargetID, fact.Severity, fact.OpenedAt).Scan(&impactID); err != nil {
				return "", "", false, false, fmt.Errorf("insert target impact: %w", err)
			}
			if err := extendPropagation(ctx, tx, incidentID, fact.OpenedAt, window); err != nil {
				return "", "", false, false, err
			}
			return incidentID, impactID, false, true, nil
		}
	}

	propagationStatus := "open"
	var closedAt any
	if !fact.Nature.Eligible {
		propagationStatus = "closed"
		closedAt = observedAt
	}
	endsAt := fact.OpenedAt.Add(time.Duration(window) * time.Second)
	err = tx.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			nature_key, nature_label, nature_scope, nature_namespace,
			nature_fingerprint, propagation_eligible, status,
			propagation_status, severity, opened_at, last_impact_at,
			propagation_window_seconds, propagation_ends_at,
			propagation_closed_at, active_impact_count, impact_count,
			affected_target_count, max_affected_targets
		) VALUES (
			$1, $2, $3, $4, $5, $6, 'active', $7, $8, $9, $10,
			$11, $12, $13, 1, 1, 1, 1
		) RETURNING id::text
	`, fact.Nature.Key, fact.Nature.Label, fact.Nature.Scope,
		fact.Nature.Namespace, fact.Nature.Fingerprint, fact.Nature.Eligible,
		propagationStatus, fact.Severity, fact.OpenedAt, fact.OpenedAt,
		window, endsAt, closedAt).Scan(&incidentID)
	if err != nil {
		return "", "", false, false, fmt.Errorf("insert incident: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO cairnops_incident_impacts (
			incident_id, target_id, status, source_severity,
			effective_severity, opened_at
		) VALUES ($1::uuid, $2::uuid, 'active', $3, $3, $4)
		RETURNING id::text
	`, incidentID, fact.TargetID, fact.Severity, fact.OpenedAt).Scan(&impactID)
	if err != nil {
		return "", "", false, false, fmt.Errorf("insert first target impact: %w", err)
	}
	return incidentID, impactID, true, true, nil
}

func extendPropagation(ctx context.Context, tx pgx.Tx, incidentID string, observedAt time.Time, window int) error {
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents
		SET last_impact_at = greatest(last_impact_at, $2),
		    propagation_window_seconds = greatest(propagation_window_seconds, $3),
		    propagation_ends_at = greatest(propagation_ends_at, $2 + make_interval(secs => $3)),
		    revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid AND propagation_status = 'open'
	`, incidentID, observedAt, window); err != nil {
		return fmt.Errorf("extend incident propagation: %w", err)
	}
	return nil
}

func evidenceWindow(ctx context.Context, tx pgx.Tx, fact EvidenceFact) (int, error) {
	interval := 30
	if fact.SourceID != "" {
		if err := tx.QueryRow(ctx, `
			SELECT interval_seconds FROM cairnops_signal_sources WHERE id = $1::uuid
		`, fact.SourceID).Scan(&interval); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("load source cadence: %w", err)
		}
	} else if fact.ConnectorID != "" {
		if err := tx.QueryRow(ctx, `
			SELECT sync_interval_seconds FROM cairnops_connectors WHERE id = $1::uuid
		`, fact.ConnectorID).Scan(&interval); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("load connector cadence: %w", err)
		}
	}
	window := interval * 2
	if window < 60 {
		window = 60
	}
	if window > 300 {
		window = 300
	}
	return window, nil
}

func resolveEvidenceRow(ctx context.Context, tx pgx.Tx, evidenceID, incidentID, impactID, name, origin string, observedAt time.Time) error {
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_evidence
		SET active = false, resolved_at = $2, last_seen_at = $2, updated_at = now()
		WHERE id = $1::uuid AND active
	`, evidenceID, observedAt); err != nil {
		return fmt.Errorf("resolve incident evidence: %w", err)
	}
	return appendActivity(ctx, tx, incidentID, impactID, evidenceID,
		"evidence_resolved", origin, "", name, nil)
}

func (store *PostgresStore) ResolveEvidence(ctx context.Context, origin, identityScope, identityKey string, observedAt time.Time) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var evidenceID, incidentID, impactID, name string
	err = tx.QueryRow(ctx, `
		SELECT id::text, incident_id::text, impact_id::text, name
		FROM cairnops_incident_evidence
		WHERE origin = $1 AND identity_scope = $2 AND identity_key = $3 AND active
		FOR UPDATE
	`, origin, identityScope, identityKey).Scan(&evidenceID, &incidentID, &impactID, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find evidence to resolve: %w", err)
	}
	if err := resolveEvidenceRow(ctx, tx, evidenceID, incidentID, impactID, name, origin, normalizedTime(observedAt)); err != nil {
		return err
	}
	if err := recomputeImpact(ctx, tx, impactID, normalizedTime(observedAt)); err != nil {
		return err
	}
	if err := recomputeIncident(ctx, tx, incidentID, normalizedTime(observedAt)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func rearmRecoveredEvidence(ctx context.Context, tx pgx.Tx, snapshot EvidenceSnapshot, seen map[string]struct{}, observedAt time.Time) error {
	if !snapshot.CompleteConnector && len(snapshot.ObservedScopes) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, identity_scope, identity_key
		FROM cairnops_incident_evidence
		WHERE origin = $1 AND connector_id = $2::uuid
		  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
		  AND ($3 OR identity_scope = ANY($4::text[]))
		FOR UPDATE
	`, snapshot.Origin, snapshot.ConnectorID,
		snapshot.CompleteConnector, snapshot.ObservedScopes)
	if err != nil {
		return fmt.Errorf("list invalidated evidence: %w", err)
	}
	type candidate struct{ id, scope, key string }
	candidates := make([]candidate, 0)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.scope, &item.key); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range candidates {
		if _, stillUnhealthy := seen[item.scope+"\x00"+item.key]; stillUnhealthy {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET rearmed_at = greatest($2, invalidated_at), last_seen_at = $2,
			    updated_at = now() WHERE id = $1::uuid
		`, item.id, observedAt); err != nil {
			return fmt.Errorf("rearm incident evidence: %w", err)
		}
	}
	return nil
}

func recomputeImpact(ctx context.Context, tx pgx.Tx, impactID string, observedAt time.Time) error {
	var incidentID, previousStatus, previousSeverity string
	if err := tx.QueryRow(ctx, `
		SELECT incident_id::text, status, effective_severity
		FROM cairnops_incident_impacts WHERE id = $1::uuid FOR UPDATE
	`, impactID).Scan(&incidentID, &previousStatus, &previousSeverity); err != nil {
		return fmt.Errorf("lock target impact: %w", err)
	}
	var severity string
	err := tx.QueryRow(ctx, `
		SELECT severity FROM cairnops_incident_evidence
		WHERE impact_id = $1::uuid AND active AND invalidated_at IS NULL
		ORDER BY cairnops_severity_rank(severity) DESC LIMIT 1
	`, impactID).Scan(&severity)
	status := "active"
	var resolvedAt any
	if errors.Is(err, pgx.ErrNoRows) {
		status = "resolved"
		severity = previousSeverity
		resolvedAt = observedAt
	} else if err != nil {
		return fmt.Errorf("derive target impact: %w", err)
	}
	if status == previousStatus && severity == previousSeverity {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_impacts
		SET status = $2, source_severity = $3, effective_severity = $3,
		    resolved_at = $4, updated_at = now() WHERE id = $1::uuid
	`, impactID, status, severity, resolvedAt); err != nil {
		return fmt.Errorf("recompute target impact: %w", err)
	}
	if status == "resolved" && previousStatus != "resolved" {
		return appendActivity(ctx, tx, incidentID, impactID, "",
			"impact_resolved", "cairnops", "", "Atteinte rétablie", nil)
	}
	return nil
}

func recomputeIncident(ctx context.Context, tx pgx.Tx, incidentID string, observedAt time.Time) error {
	var previousStatus, previousPropagation, previousSeverity string
	var propagationEndsAt time.Time
	var previousExtended bool
	var previousActive, previousTotal, previousMax int
	if err := tx.QueryRow(ctx, `
		SELECT status, propagation_status, severity, propagation_ends_at,
		       extended, active_impact_count, impact_count, max_affected_targets
		FROM cairnops_incidents WHERE id = $1::uuid FOR UPDATE
	`, incidentID).Scan(&previousStatus, &previousPropagation, &previousSeverity,
		&propagationEndsAt, &previousExtended, &previousActive, &previousTotal,
		&previousMax); err != nil {
		return fmt.Errorf("lock incident: %w", err)
	}
	var active, total int
	var severity string
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'active')::integer,
		       count(*)::integer,
		       coalesce((array_agg(effective_severity ORDER BY cairnops_severity_rank(effective_severity) DESC)
		           FILTER (WHERE status = 'active'))[1], $2)
		FROM cairnops_incident_impacts WHERE incident_id = $1::uuid
	`, incidentID, previousSeverity).Scan(&active, &total, &severity); err != nil {
		return fmt.Errorf("derive incident impacts: %w", err)
	}
	propagation := previousPropagation
	var closedAt any
	if propagation == "open" && !observedAt.Before(propagationEndsAt) {
		propagation = "closed"
		closedAt = observedAt
	}
	status := "active"
	var resolvedAt any
	if propagation == "closed" && active == 0 {
		status = "resolved"
		resolvedAt = observedAt
	}
	maxAffected := previousMax
	if active > maxAffected {
		maxAffected = active
	}
	var targetCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_targets WHERE archived_at IS NULL
	`).Scan(&targetCount); err != nil {
		return fmt.Errorf("count active targets: %w", err)
	}
	extended := previousExtended || active >= 20 || (active >= 5 && targetCount > 0 && active*5 >= targetCount)
	changed := status != previousStatus || propagation != previousPropagation ||
		severity != previousSeverity || active != previousActive || total != previousTotal ||
		maxAffected != previousMax || extended != previousExtended
	if !changed {
		return recomputeAcknowledgement(ctx, tx, incidentID)
	}
	if propagation == previousPropagation {
		closedAt = nil
		if propagation == "closed" {
			if err := tx.QueryRow(ctx, `
				SELECT propagation_closed_at FROM cairnops_incidents WHERE id = $1::uuid
			`, incidentID).Scan(&closedAt); err != nil {
				return err
			}
		}
	}
	if status == previousStatus {
		resolvedAt = nil
		if status == "resolved" {
			if err := tx.QueryRow(ctx, `
				SELECT resolved_at FROM cairnops_incidents WHERE id = $1::uuid
			`, incidentID).Scan(&resolvedAt); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents
		SET status = $2, propagation_status = $3, severity = $4,
		    propagation_closed_at = $5, resolved_at = $6,
		    active_impact_count = $7, impact_count = $8,
		    affected_target_count = $7, max_affected_targets = $9,
		    extended = $10, revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid
	`, incidentID, status, propagation, severity, closedAt, resolvedAt,
		active, total, maxAffected, extended); err != nil {
		return fmt.Errorf("recompute incident: %w", err)
	}
	if propagation == "closed" && previousPropagation == "open" {
		if err := appendActivity(ctx, tx, incidentID, "", "",
			"propagation_closed", "cairnops", "", "Propagation fermée", nil); err != nil {
			return err
		}
	}
	if severity != previousSeverity {
		if err := appendActivity(ctx, tx, incidentID, "", "",
			"severity_changed", "cairnops", "", "Gravité actualisée",
			map[string]any{"before": previousSeverity, "after": severity}); err != nil {
			return err
		}
	}
	if extended && !previousExtended {
		if err := appendActivity(ctx, tx, incidentID, "", "",
			"extended", "cairnops", "", "Propagation étendue", nil); err != nil {
			return err
		}
	}
	if status == "resolved" && previousStatus != "resolved" {
		if err := appendActivity(ctx, tx, incidentID, "", "",
			"resolved", "cairnops", "", "Incident résolu", nil); err != nil {
			return err
		}
	}
	return recomputeAcknowledgement(ctx, tx, incidentID)
}

func advanceDueIncidents(ctx context.Context, tx pgx.Tx, observedAt time.Time) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text FROM cairnops_incidents
		WHERE status = 'active' AND propagation_status = 'open'
		  AND propagation_ends_at <= $1
		ORDER BY propagation_ends_at FOR UPDATE SKIP LOCKED
	`, observedAt)
	if err != nil {
		return fmt.Errorf("list due incident propagations: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if err := recomputeIncident(ctx, tx, id, observedAt); err != nil {
			return err
		}
	}
	return nil
}

func (store *PostgresStore) Advance(ctx context.Context, observedAt time.Time) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := advanceDueIncidents(ctx, tx, normalizedTime(observedAt)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func acknowledgeFromEvidence(ctx context.Context, tx pgx.Tx, incidentID, evidenceID string, observedAt time.Time) error {
	command, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents
		SET acknowledged_at = coalesce(acknowledged_at, $2),
		    acknowledgement_origin = coalesce(acknowledgement_origin, 'connector'),
		    acknowledgement_sync_status = CASE WHEN EXISTS (
		        SELECT 1 FROM cairnops_incident_evidence evidence
		        WHERE evidence.incident_id = $1::uuid AND evidence.active
		          AND evidence.origin = 'zabbix' AND evidence.id <> $3::uuid
		          AND NOT evidence.upstream_acknowledged
		    ) THEN 'pending' ELSE 'synchronized' END,
		    acknowledgement_sync_error = '',
		    revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid AND acknowledged_at IS NULL
	`, incidentID, observedAt, evidenceID)
	if err != nil {
		return fmt.Errorf("acknowledge incident from evidence: %w", err)
	}
	if command.RowsAffected() == 0 {
		return recomputeAcknowledgement(ctx, tx, incidentID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_evidence
		SET acknowledgement_sync_status = CASE
		        WHEN id = $2::uuid OR upstream_acknowledged THEN 'synchronized'
		        ELSE 'pending'
		    END,
		    acknowledgement_sync_error = '',
		    acknowledgement_synced_at = CASE
		        WHEN id = $2::uuid OR upstream_acknowledged
		        THEN coalesce(acknowledgement_synced_at, $3) ELSE NULL
		    END,
		    updated_at = now()
		WHERE incident_id = $1::uuid AND active AND origin = 'zabbix'
	`, incidentID, evidenceID, observedAt); err != nil {
		return fmt.Errorf("inherit connector acknowledgement: %w", err)
	}
	if err := appendActivity(ctx, tx, incidentID, "", evidenceID,
		"upstream_acknowledged", "zabbix", "", "Acquittement reçu du Connecteur", nil); err != nil {
		return err
	}
	return recomputeAcknowledgement(ctx, tx, incidentID)
}

func recomputeAcknowledgement(ctx context.Context, tx pgx.Tx, incidentID string) error {
	_, err := tx.Exec(ctx, `
		WITH evidence_state AS (
			SELECT count(*) FILTER (WHERE active AND origin = 'zabbix') AS total,
			       count(*) FILTER (WHERE active AND origin = 'zabbix'
			           AND acknowledgement_sync_status = 'pending') AS pending,
			       count(*) FILTER (WHERE active AND origin = 'zabbix'
			           AND acknowledgement_sync_status = 'failed') AS failed,
			       coalesce(string_agg(nullif(acknowledgement_sync_error, ''), '; '
			           ORDER BY id) FILTER (WHERE active AND origin = 'zabbix'
			           AND acknowledgement_sync_status = 'failed'), '') AS errors
			FROM cairnops_incident_evidence WHERE incident_id = $1::uuid
		), desired AS (
			SELECT CASE
			           WHEN total = 0 THEN 'not_applicable'
			           WHEN failed > 0 THEN 'failed'
			           WHEN pending > 0 THEN 'pending'
			           ELSE 'synchronized'
			       END AS status,
			       left(errors, 500) AS error
			FROM evidence_state
		)
		UPDATE cairnops_incidents incident
		SET acknowledgement_sync_status = desired.status,
		    acknowledgement_sync_error = desired.error,
		    revision = revision + 1, updated_at = now()
		FROM desired
		WHERE incident.id = $1::uuid AND incident.acknowledged_at IS NOT NULL
		  AND (incident.acknowledgement_sync_status,
		       incident.acknowledgement_sync_error)
		      IS DISTINCT FROM (desired.status, desired.error)
	`, incidentID)
	if err != nil {
		return fmt.Errorf("recompute incident acknowledgement: %w", err)
	}
	return nil
}

func acknowledgementStatus(upstreamAcknowledged bool) string {
	if upstreamAcknowledged {
		return "synchronized"
	}
	return "not_applicable"
}

func acknowledgementTime(upstreamAcknowledged bool, observedAt time.Time) any {
	if upstreamAcknowledged {
		return observedAt
	}
	return nil
}

func ResolveForArchivedTarget(ctx context.Context, tx pgx.Tx, targetID string) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text, incident_id::text, impact_id::text, name, origin
		FROM cairnops_incident_evidence
		WHERE target_id = $1::uuid AND active FOR UPDATE
	`, targetID)
	if err != nil {
		return fmt.Errorf("list evidence for archived target: %w", err)
	}
	type pair struct{ evidenceID, incidentID, impactID, name, origin string }
	pairs := make([]pair, 0)
	for rows.Next() {
		var item pair
		if err := rows.Scan(
			&item.evidenceID, &item.incidentID, &item.impactID,
			&item.name, &item.origin,
		); err != nil {
			rows.Close()
			return err
		}
		pairs = append(pairs, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	now := time.Now().UTC()
	seenImpacts := map[string]struct{}{}
	seenIncidents := map[string]struct{}{}
	for _, item := range pairs {
		if err := resolveEvidenceRow(ctx, tx, item.evidenceID, item.incidentID,
			item.impactID, item.name, item.origin, now); err != nil {
			return err
		}
		if err := appendActivity(ctx, tx, item.incidentID, item.impactID,
			item.evidenceID, "evidence_updated", "cairnops", "",
			"Cible archivée", nil); err != nil {
			return err
		}
		seenImpacts[item.impactID] = struct{}{}
		seenIncidents[item.incidentID] = struct{}{}
	}
	for _, impactID := range sortedKeys(seenImpacts) {
		if err := recomputeImpact(ctx, tx, impactID, now); err != nil {
			return err
		}
	}
	for _, incidentID := range sortedKeys(seenIncidents) {
		if err := recomputeIncident(ctx, tx, incidentID, now); err != nil {
			return err
		}
	}
	return nil
}

func validSeverity(value Severity) bool {
	switch value {
	case SeverityInformation, SeverityWarning, SeverityMajor, SeverityCritical:
		return true
	default:
		return false
	}
}

func nullableUUID(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func jsonBytes(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode incident evidence metadata: %w", err)
	}
	return payload, nil
}
