package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/jackc/pgx/v5"
)

const (
	NatureAvailability      = "availability"
	NatureAvailabilityLabel = "Indisponibilité"
)

type NativeObservation struct {
	SourceID   string
	TargetID   string
	SourceName string
	Outcome    domain.Outcome
	ObservedAt time.Time
	Reason     string
	Message    string
}

// ApplyNativeObservation conserve la Politique de déclenchement et le cycle de
// la Preuve dans la transaction qui enregistre l'Observation. Le Contrôle
// natif traduit son résultat ; le module Incident possède toutes les
// transitions suivantes.
func ApplyNativeObservation(ctx context.Context, tx pgx.Tx, observation NativeObservation) error {
	observedAt := normalizedTime(observation.ObservedAt)
	var policy domain.TriggerPolicy
	var streaks domain.TriggerStreaks
	var severity Severity
	if err := tx.QueryRow(ctx, `
		SELECT failure_threshold, recovery_threshold, severity,
		       consecutive_unhealthy, consecutive_healthy
		FROM cairnops_signal_sources
		WHERE id = $1::uuid FOR UPDATE
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
		incidentID, impactID, err := applyEvidenceFact(ctx, tx, EvidenceFact{
			Origin: "native", SourceID: observation.SourceID,
			IdentityScope: observation.SourceID, IdentityKey: NatureAvailability,
			TargetID: observation.TargetID,
			Nature:   CanonicalNature(NatureAvailability, NatureAvailabilityLabel),
			Name:     observation.SourceName, Severity: severity, OpenedAt: observedAt,
			Metadata: nativeMetadata(observation),
		}, observedAt)
		if err != nil {
			return err
		}
		if incidentID == "" {
			return nil
		}
		if err := recomputeImpact(ctx, tx, impactID, observedAt); err != nil {
			return err
		}
		return recomputeIncident(ctx, tx, incidentID, observedAt)
	case decision.Recovered:
		return recoverNativeEvidence(ctx, tx, observation, observedAt)
	default:
		return advanceDueIncidents(ctx, tx, observedAt)
	}
}

func recoverNativeEvidence(ctx context.Context, tx pgx.Tx, observation NativeObservation, observedAt time.Time) error {
	var evidenceID, incidentID, impactID, name string
	err := tx.QueryRow(ctx, `
		SELECT id::text, incident_id::text, impact_id::text, name
		FROM cairnops_incident_evidence
		WHERE origin = 'native' AND identity_scope = $1
		  AND identity_key = $2 AND active FOR UPDATE
	`, observation.SourceID, NatureAvailability).Scan(
		&evidenceID, &incidentID, &impactID, &name,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET rearmed_at = greatest($2, invalidated_at), last_seen_at = $2,
			    updated_at = now()
			WHERE origin = 'native' AND identity_scope = $1
			  AND identity_key = $3 AND invalidated_at IS NOT NULL
			  AND rearmed_at IS NULL
		`, observation.SourceID, observedAt, NatureAvailability); err != nil {
			return fmt.Errorf("rearm native evidence: %w", err)
		}
		return advanceDueIncidents(ctx, tx, observedAt)
	}
	if err != nil {
		return fmt.Errorf("find native evidence: %w", err)
	}
	if err := resolveEvidenceRow(ctx, tx, evidenceID, incidentID, impactID,
		name, "native", observedAt); err != nil {
		return err
	}
	if err := recomputeImpact(ctx, tx, impactID, observedAt); err != nil {
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
