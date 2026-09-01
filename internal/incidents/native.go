package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/jackc/pgx/v5"
)

// NatureAvailability regroupe les preuves des Contrôles natifs qui établissent
// qu'une Cible ne répond plus. Une même Cible n'a qu'un Incident actif par Nature.
const (
	NatureAvailability      = "availability"
	NatureAvailabilityLabel = "Indisponibilité"
)

// NativeObservation est le résultat daté d'un Contrôle natif, prêt à être
// confronté à la Politique de déclenchement de sa Source de signal.
type NativeObservation struct {
	SourceID   string
	TargetID   string
	SourceName string
	Outcome    domain.Outcome
	ObservedAt time.Time
	Reason     string
	Message    string
}

// ApplyNativeObservation confronte une Observation à la Politique de
// déclenchement de sa Source, persiste les compteurs correspondants puis, si la
// conclusion est assez certaine, ouvre, alimente ou résout la preuve native de
// l'Incident concerné. Elle s'exécute dans la transaction qui enregistre
// l'Observation afin que preuve et Incident ne divergent jamais.
func ApplyNativeObservation(ctx context.Context, tx pgx.Tx, observation NativeObservation) error {
	observedAt := observation.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	var policy domain.TriggerPolicy
	var streaks domain.TriggerStreaks
	var severity Severity
	if err := tx.QueryRow(ctx, `
		SELECT failure_threshold, recovery_threshold, severity,
		       consecutive_unhealthy, consecutive_healthy
		FROM cairnops_signal_sources
		WHERE id = $1::uuid
		FOR UPDATE
	`, observation.SourceID).Scan(
		&policy.FailureThreshold, &policy.RecoveryThreshold, &severity,
		&streaks.Unhealthy, &streaks.Healthy,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: signal source %s", ErrNotFound, observation.SourceID)
		}
		return fmt.Errorf("load trigger policy: %w", err)
	}

	decision := policy.Evaluate(streaks, observation.Outcome)
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_signal_sources
		SET consecutive_unhealthy = $2, consecutive_healthy = $3, updated_at = now()
		WHERE id = $1::uuid
	`, observation.SourceID, decision.Streaks.Unhealthy, decision.Streaks.Healthy); err != nil {
		return fmt.Errorf("persist trigger streaks: %w", err)
	}

	switch {
	case decision.Triggered:
		return applyNativeTrigger(ctx, tx, observation, severity, observedAt)
	case decision.Recovered:
		return applyNativeRecovery(ctx, tx, observation, observedAt)
	default:
		return nil
	}
}

func applyNativeTrigger(ctx context.Context, tx pgx.Tx, observation NativeObservation, severity Severity, observedAt time.Time) error {
	metadata, err := json.Marshal(nativeMetadata(observation))
	if err != nil {
		return fmt.Errorf("encode native signal metadata: %w", err)
	}

	var signalID, incidentID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, incident_id::text
		FROM cairnops_incident_signals
		WHERE origin = 'native' AND source_id = $1::uuid AND active
		FOR UPDATE
	`, observation.SourceID).Scan(&signalID, &incidentID)
	if err == nil {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET name = $2, severity = $3, last_seen_at = $4,
			    metadata = $5::jsonb, updated_at = now()
			WHERE id = $1::uuid
		`, signalID, observation.SourceName, severity, observedAt, metadata); err != nil {
			return fmt.Errorf("refresh native incident signal: %w", err)
		}
		return recomputeIncident(ctx, tx, incidentID, observedAt)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find active native signal: %w", err)
	}

	// Une Source Invalidée continue ses Observations sans réalimenter l'Incident
	// tant qu'un cycle sain ne l'a pas réarmée.
	var invalidatedSignalID string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM cairnops_incident_signals
		WHERE origin = 'native' AND source_id = $1::uuid
		  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
		ORDER BY invalidated_at DESC
		LIMIT 1
		FOR UPDATE
	`, observation.SourceID).Scan(&invalidatedSignalID)
	if err == nil {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals SET last_seen_at = $2, updated_at = now()
			WHERE id = $1::uuid
		`, invalidatedSignalID, observedAt); err != nil {
			return fmt.Errorf("refresh invalidated native signal: %w", err)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find invalidated native signal: %w", err)
	}

	incidentID, created, err := ensureActiveIncident(
		ctx, tx, observation.TargetID,
		CanonicalNature(NatureAvailability, NatureAvailabilityLabel), severity, observedAt,
	)
	if errors.Is(err, ErrTargetArchived) {
		return nil
	}
	if err != nil {
		return err
	}
	if created {
		if err := insertActivity(ctx, tx, incidentID, "opened", "native", "",
			"Incident ouvert par un Contrôle natif", nativeMetadata(observation)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_signals (
			incident_id, target_id, origin, source_id, name, active, severity,
			opened_at, upstream_acknowledged, last_seen_at, metadata
		) VALUES ($1::uuid, $2::uuid, 'native', $3::uuid, $4, true, $5,
		          $6, false, $6, $7::jsonb)
	`, incidentID, observation.TargetID, observation.SourceID,
		observation.SourceName, severity, observedAt, metadata); err != nil {
		return fmt.Errorf("insert native incident signal: %w", err)
	}
	if err := insertActivity(ctx, tx, incidentID, "signal_added", "native", "",
		observation.SourceName, nativeMetadata(observation)); err != nil {
		return err
	}
	return recomputeIncident(ctx, tx, incidentID, observedAt)
}

func applyNativeRecovery(ctx context.Context, tx pgx.Tx, observation NativeObservation, observedAt time.Time) error {
	var signalID, incidentID string
	err := tx.QueryRow(ctx, `
		SELECT id::text, incident_id::text
		FROM cairnops_incident_signals
		WHERE origin = 'native' AND source_id = $1::uuid AND active
		FOR UPDATE
	`, observation.SourceID).Scan(&signalID, &incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Le cycle sain met fin à une éventuelle Invalidation : un déclenchement
		// ultérieur pourra de nouveau alimenter un Incident.
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_signals
			SET rearmed_at = greatest($2, invalidated_at), last_seen_at = $2, updated_at = now()
			WHERE origin = 'native' AND source_id = $1::uuid
			  AND invalidated_at IS NOT NULL AND rearmed_at IS NULL
		`, observation.SourceID, observedAt); err != nil {
			return fmt.Errorf("rearm invalidated native signal: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("find active native signal: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_signals
		SET active = false, resolved_at = $2, last_seen_at = $2, updated_at = now()
		WHERE id = $1::uuid
	`, signalID, observedAt); err != nil {
		return fmt.Errorf("resolve native incident signal: %w", err)
	}
	if err := insertActivity(ctx, tx, incidentID, "signal_resolved", "native", "",
		observation.SourceName, map[string]any{"source_id": observation.SourceID}); err != nil {
		return err
	}
	return recomputeIncident(ctx, tx, incidentID, observedAt)
}

func nativeMetadata(observation NativeObservation) map[string]any {
	metadata := map[string]any{"source_id": observation.SourceID}
	if observation.Reason != "" {
		metadata["reason"] = observation.Reason
	}
	if observation.Message != "" {
		metadata["message"] = observation.Message
	}
	return metadata
}
