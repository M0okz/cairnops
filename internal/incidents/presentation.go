package incidents

import (
	"context"
	"fmt"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/jackc/pgx/v5"
)

// Refreshing presentation alone does not increment the operational revision:
// richer wording is not a new incident fact and must not cause an alert.
func refreshIncidentPresentation(ctx context.Context, tx pgx.Tx, incidentID string, noActiveImpacts bool) error {
	rows, err := tx.Query(ctx, `
		SELECT alert_facts FROM cairnops_incident_evidence
		WHERE incident_id = $1::uuid AND invalidated_at IS NULL
		  AND (active OR $2)
	`, incidentID, noActiveImpacts)
	if err != nil {
		return fmt.Errorf("read incident presentation facts: %w", err)
	}
	facts := []alerttext.Fact{}
	for rows.Next() {
		var fact alerttext.Fact
		if err := rows.Scan(&fact); err != nil {
			rows.Close()
			return err
		}
		facts = append(facts, fact)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE cairnops_incidents SET alert_kind = $2 WHERE id = $1::uuid AND alert_kind IS DISTINCT FROM $2`, incidentID, string(alerttext.Common(facts).Kind))
	return err
}

func patchMonPresentation(signal PatchMonSignal) alerttext.Fact {
	switch signal.ConditionKey {
	case "security_updates":
		fact := alerttext.Fact{Kind: alerttext.SecurityUpdates}
		if count, ok := signal.Details["security_updates_count"].(int); ok && count > 0 {
			fact.Count = &count
		}
		return fact
	case "reboot_required":
		return alerttext.Fact{Kind: alerttext.RebootRequired}
	default:
		return alerttext.Fact{}
	}
}
