package incidents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

type evidenceMove struct{ id, incidentID, impactID string }

// ResolveForSourceRemoval referme les Preuves actives d'un Contrôle avant que
// sa définition disparaisse. L'historique reste lisible, sans référence morte.
func ResolveForSourceRemoval(ctx context.Context, tx pgx.Tx, sourceID string) error {
	return resolveOwnedEvidence(ctx, tx, "source_id = $1::uuid", sourceID,
		"Contrôle supprimé")
}

// ResolveForConnectorRemoval applique la même règle à toutes les Preuves d'un
// Connecteur avant la suppression de ses liaisons.
func ResolveForConnectorRemoval(ctx context.Context, tx pgx.Tx, connectorID string) (int, error) {
	return resolveOwnedEvidenceCount(ctx, tx, "connector_id = $1::uuid", connectorID,
		"Connecteur supprimé")
}

func resolveOwnedEvidence(ctx context.Context, tx pgx.Tx, predicate, ownerID, message string) error {
	_, err := resolveOwnedEvidenceCount(ctx, tx, predicate, ownerID, message)
	return err
}

func resolveOwnedEvidenceCount(ctx context.Context, tx pgx.Tx, predicate, ownerID, message string) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, incident_id::text, impact_id::text, name, origin
		FROM cairnops_incident_evidence
		WHERE active AND `+predicate+`
		ORDER BY incident_id, impact_id, id FOR UPDATE
	`, ownerID)
	if err != nil {
		return 0, fmt.Errorf("lock owned incident evidence: %w", err)
	}
	type item struct{ id, incidentID, impactID, name, origin string }
	items := make([]item, 0)
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.id, &value.incidentID, &value.impactID, &value.name, &value.origin); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	now := time.Now().UTC()
	impacts := map[string]struct{}{}
	incidentIDs := map[string]struct{}{}
	for _, value := range items {
		if err := resolveEvidenceRow(ctx, tx, value.id, value.incidentID,
			value.impactID, value.name, value.origin, now); err != nil {
			return 0, err
		}
		if err := appendActivity(ctx, tx, value.incidentID, value.impactID, value.id,
			"evidence_updated", "cairnops", "", message, nil); err != nil {
			return 0, err
		}
		impacts[value.impactID] = struct{}{}
		incidentIDs[value.incidentID] = struct{}{}
	}
	for _, impactID := range sortedKeys(impacts) {
		if err := recomputeImpact(ctx, tx, impactID, now); err != nil {
			return 0, err
		}
	}
	resolved := 0
	for _, incidentID := range sortedKeys(incidentIDs) {
		var before string
		if err := tx.QueryRow(ctx, `SELECT status FROM cairnops_incidents WHERE id = $1::uuid`, incidentID).Scan(&before); err != nil {
			return 0, err
		}
		if err := recomputeIncident(ctx, tx, incidentID, now); err != nil {
			return 0, err
		}
		var after string
		if err := tx.QueryRow(ctx, `SELECT status FROM cairnops_incidents WHERE id = $1::uuid`, incidentID).Scan(&after); err != nil {
			return 0, err
		}
		if before != "resolved" && after == "resolved" {
			resolved++
		}
	}
	return resolved, nil
}

// MergeTargets réattribue toutes les Atteintes à l'identité conservée. Si un
// même Incident contient déjà les deux Cibles, leurs Atteintes sont fusionnées
// avant la réattribution pour préserver l'unicité Incident/Cible.
func MergeTargets(ctx context.Context, tx pgx.Tx, primaryID, secondaryID string) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT secondary.id::text, secondary.incident_id::text,
		       coalesce(primary_impact.id::text, '')
		FROM cairnops_incident_impacts secondary
		LEFT JOIN cairnops_incident_impacts primary_impact
		  ON primary_impact.incident_id = secondary.incident_id
		 AND primary_impact.target_id = $1::uuid
		WHERE secondary.target_id = $2::uuid
		ORDER BY secondary.incident_id, secondary.id
		FOR UPDATE OF secondary
	`, primaryID, secondaryID)
	if err != nil {
		return 0, fmt.Errorf("lock target impacts: %w", err)
	}
	type candidate struct{ secondaryImpact, incidentID, primaryImpact string }
	items := make([]candidate, 0)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.secondaryImpact, &item.incidentID, &item.primaryImpact); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	merged := 0
	incidentIDs := map[string]struct{}{}
	for _, item := range items {
		incidentIDs[item.incidentID] = struct{}{}
		if item.primaryImpact == "" {
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_impacts SET target_id = $2::uuid, updated_at = now()
				WHERE id = $1::uuid
			`, item.secondaryImpact, primaryID); err != nil {
				return 0, fmt.Errorf("move target impact: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_evidence SET target_id = $2::uuid, updated_at = now()
				WHERE impact_id = $1::uuid
			`, item.secondaryImpact, primaryID); err != nil {
				return 0, fmt.Errorf("move target evidence: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_indicator_snapshots SET target_id = $2::uuid
				WHERE impact_id = $1::uuid
			`, item.secondaryImpact, primaryID); err != nil {
				return 0, fmt.Errorf("move target snapshots: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_indicator_snapshots (
				incident_id, impact_id, target_id, indicator_id, semantic_key,
				label, unit, value, observed_at, created_at
			)
			SELECT incident_id, $1::uuid, $2::uuid, indicator_id, semantic_key,
			       label, unit, value, observed_at, created_at
			FROM cairnops_incident_indicator_snapshots WHERE impact_id = $3::uuid
			ON CONFLICT DO NOTHING
		`, item.primaryImpact, primaryID, item.secondaryImpact); err != nil {
			return 0, fmt.Errorf("merge duplicate target snapshots: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM cairnops_incident_indicator_snapshots WHERE impact_id = $1::uuid
		`, item.secondaryImpact); err != nil {
			return 0, fmt.Errorf("remove duplicate target snapshots: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET impact_id = $1::uuid, target_id = $2::uuid, updated_at = now()
			WHERE impact_id = $3::uuid
		`, item.primaryImpact, primaryID, item.secondaryImpact); err != nil {
			return 0, fmt.Errorf("merge duplicate target evidence: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_activity SET impact_id = $1::uuid
			WHERE impact_id = $2::uuid
		`, item.primaryImpact, item.secondaryImpact); err != nil {
			return 0, fmt.Errorf("merge duplicate target activity: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM cairnops_incident_impacts WHERE id = $1::uuid
		`, item.secondaryImpact); err != nil {
			return 0, fmt.Errorf("remove duplicate target impact: %w", err)
		}
		if err := recomputeImpact(ctx, tx, item.primaryImpact, time.Now().UTC()); err != nil {
			return 0, err
		}
		merged++
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_notification_inbox SET target_id = $1::uuid
		WHERE target_id = $2::uuid
	`, primaryID, secondaryID); err != nil {
		return 0, fmt.Errorf("move incident inbox references: %w", err)
	}
	for _, incidentID := range sortedKeys(incidentIDs) {
		if err := recomputeIncident(ctx, tx, incidentID, time.Now().UTC()); err != nil {
			return 0, err
		}
		if err := touchIncidentStructure(ctx, tx, incidentID); err != nil {
			return 0, err
		}
		if err := appendActivity(ctx, tx, incidentID, "", "", "target_reconciled",
			"cairnops", "", "Atteintes réunies après rapprochement de Cibles",
			map[string]any{"absorbed_target_id": secondaryID, "target_id": primaryID}); err != nil {
			return 0, err
		}
	}
	return merged, nil
}

// ReassignSource déplace les Preuves d'une Source vers une autre Cible en
// conservant l'Incident auquel elles appartiennent.
func ReassignSource(ctx context.Context, tx pgx.Tx, destinationID, sourceID, bindingID string) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT evidence.id::text, evidence.incident_id::text,
		       evidence.impact_id::text
		FROM cairnops_incident_evidence evidence
		WHERE evidence.source_id = $1::uuid
		   OR evidence.connector_binding_id = nullif($2, '')::uuid
		ORDER BY evidence.incident_id, evidence.impact_id, evidence.id
		FOR UPDATE
	`, sourceID, bindingID)
	if err != nil {
		return 0, fmt.Errorf("lock Source evidence: %w", err)
	}
	moves := make([]evidenceMove, 0)
	for rows.Next() {
		var move evidenceMove
		if err := rows.Scan(&move.id, &move.incidentID, &move.impactID); err != nil {
			rows.Close()
			return 0, err
		}
		moves = append(moves, move)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	byImpact := map[string][]evidenceMove{}
	for _, move := range moves {
		byImpact[move.impactID] = append(byImpact[move.impactID], move)
	}
	incidentIDs := map[string]struct{}{}
	for originalImpact, selected := range byImpact {
		incidentID := selected[0].incidentID
		incidentIDs[incidentID] = struct{}{}
		var destinationImpact string
		err := tx.QueryRow(ctx, `
			SELECT id::text FROM cairnops_incident_impacts
			WHERE incident_id = $1::uuid AND target_id = $2::uuid FOR UPDATE
		`, incidentID, destinationID).Scan(&destinationImpact)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("find destination impact: %w", err)
		}
		var total int
		if err := tx.QueryRow(ctx, `
			SELECT count(*)::integer FROM cairnops_incident_evidence
			WHERE impact_id = $1::uuid
		`, originalImpact).Scan(&total); err != nil {
			return 0, err
		}
		if destinationImpact == "" && total == len(selected) {
			destinationImpact = originalImpact
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_impacts SET target_id = $2::uuid, updated_at = now()
				WHERE id = $1::uuid
			`, originalImpact, destinationID); err != nil {
				return 0, fmt.Errorf("move Source impact: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE cairnops_incident_indicator_snapshots SET target_id = $2::uuid
				WHERE impact_id = $1::uuid
			`, originalImpact, destinationID); err != nil {
				return 0, fmt.Errorf("move Source snapshots: %w", err)
			}
		} else if destinationImpact == "" {
			ids := evidenceIDs(selected)
			if err := tx.QueryRow(ctx, `
				INSERT INTO cairnops_incident_impacts (
					incident_id, target_id, status, source_severity,
					effective_severity, opened_at, resolved_at
				)
				SELECT $1::uuid, $2::uuid,
				       CASE WHEN bool_or(active AND invalidated_at IS NULL) THEN 'active' ELSE 'resolved' END,
				       (array_agg(severity ORDER BY cairnops_severity_rank(severity) DESC))[1],
				       (array_agg(severity ORDER BY cairnops_severity_rank(severity) DESC))[1],
				       min(opened_at),
				       CASE WHEN bool_or(active AND invalidated_at IS NULL) THEN NULL
				            ELSE coalesce(max(resolved_at), now()) END
				FROM cairnops_incident_evidence WHERE id = ANY($3::uuid[])
				RETURNING id::text
			`, incidentID, destinationID, ids).Scan(&destinationImpact); err != nil {
				return 0, fmt.Errorf("create reassigned Source impact: %w", err)
			}
		}
		ids := evidenceIDs(selected)
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_evidence
			SET impact_id = $1::uuid, target_id = $2::uuid, updated_at = now()
			WHERE id = ANY($3::uuid[])
		`, destinationImpact, destinationID, ids); err != nil {
			return 0, fmt.Errorf("move Source evidence: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_incident_activity SET impact_id = $1::uuid
			WHERE evidence_id = ANY($2::uuid[])
		`, destinationImpact, ids); err != nil {
			return 0, fmt.Errorf("move Source evidence activity: %w", err)
		}
		if destinationImpact != originalImpact {
			if _, err := tx.Exec(ctx, `
				INSERT INTO cairnops_incident_indicator_snapshots (
					incident_id, impact_id, target_id, indicator_id, semantic_key,
					label, unit, value, observed_at, created_at
				)
				SELECT incident_id, $1::uuid, $2::uuid, indicator_id, semantic_key,
				       label, unit, value, observed_at, created_at
				FROM cairnops_incident_indicator_snapshots WHERE impact_id = $3::uuid
				ON CONFLICT DO NOTHING
			`, destinationImpact, destinationID, originalImpact); err != nil {
				return 0, fmt.Errorf("copy reassigned Source snapshots: %w", err)
			}
			var remaining int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM cairnops_incident_evidence WHERE impact_id = $1::uuid`, originalImpact).Scan(&remaining); err != nil {
				return 0, err
			}
			if remaining == 0 {
				if _, err := tx.Exec(ctx, `
					UPDATE cairnops_incident_activity SET impact_id = $1::uuid WHERE impact_id = $2::uuid
				`, destinationImpact, originalImpact); err != nil {
					return 0, err
				}
				if _, err := tx.Exec(ctx, `DELETE FROM cairnops_incident_impacts WHERE id = $1::uuid`, originalImpact); err != nil {
					return 0, err
				}
			} else if err := recomputeImpact(ctx, tx, originalImpact, time.Now().UTC()); err != nil {
				return 0, err
			}
		}
		if err := recomputeImpact(ctx, tx, destinationImpact, time.Now().UTC()); err != nil {
			return 0, err
		}
	}
	for _, incidentID := range sortedKeys(incidentIDs) {
		if err := recomputeIncident(ctx, tx, incidentID, time.Now().UTC()); err != nil {
			return 0, err
		}
		if err := touchIncidentStructure(ctx, tx, incidentID); err != nil {
			return 0, err
		}
		if err := appendActivity(ctx, tx, incidentID, "", "", "source_reassigned",
			"cairnops", "", "Preuves réattribuées après correction d'une Source",
			map[string]any{"source_id": sourceID, "target_id": destinationID}); err != nil {
			return 0, err
		}
	}
	return len(incidentIDs), nil
}

func touchIncidentStructure(ctx context.Context, tx pgx.Tx, incidentID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incidents
		SET revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid
	`, incidentID); err != nil {
		return fmt.Errorf("revise incident structure: %w", err)
	}
	return nil
}

func evidenceIDs(items []evidenceMove) []string {
	ids := make([]string, len(items))
	for index := range items {
		ids[index] = items[index].id
	}
	return ids
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
