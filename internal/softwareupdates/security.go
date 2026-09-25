package softwareupdates

import (
	"context"
	"fmt"
)

// SecurityStatus indique ce que l'analyse des notes officielles établit sur une
// comparaison de versions. Seule une faille corrigée justifie un Incident :
// une mise à jour disponible n'est pas, à elle seule, un problème opérationnel.
type SecurityStatus string

const (
	// SecurityPending : la comparaison courante attend encore sa collecte ou
	// son analyse. L'appelant ne doit ni ouvrir ni résoudre de preuve.
	SecurityPending SecurityStatus = "pending"
	// SecurityNotEstablished : l'analyse est terminée sans point de sécurité, ou
	// aucune analyse ne peut être produite (notes absentes, IA désactivée…).
	SecurityNotEstablished SecurityStatus = "not_established"
	// SecurityFixes : l'analyse courante cite au moins un point de sécurité.
	SecurityFixes SecurityStatus = "fixes"
)

// SecurityAssessment rattache le statut aux versions réellement comparées afin
// que l'appelant écarte une analyse qui ne porterait pas sur ses versions.
type SecurityAssessment struct {
	Installed string
	Target    string
	Status    SecurityStatus
}

// SecurityAssessments lit, pour chaque service Argus demandé, le statut de
// sécurité de sa comparaison courante. Un service absent n'est pas renvoyé.
func (s *Store) SecurityAssessments(ctx context.Context, bindingIDs []string) (map[string]SecurityAssessment, error) {
	result := make(map[string]SecurityAssessment, len(bindingIDs))
	if len(bindingIDs) == 0 {
		return result, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT s.binding_id::text, s.installed_version, s.target_version, s.state,
 s.state = 'ready' AND EXISTS(SELECT 1 FROM cairnops_software_analyses a
   CROSS JOIN LATERAL jsonb_array_elements(coalesce(a.result->'overview','[]'::jsonb) || coalesce(a.result->'details','[]'::jsonb)) point
   WHERE a.binding_id=s.binding_id AND a.revision=s.revision AND a.content_hash=s.content_hash AND point->>'category'='security')
 FROM cairnops_software_services s WHERE s.binding_id::text = ANY($1::text[])`, bindingIDs)
	if err != nil {
		return nil, fmt.Errorf("read software security assessments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, state string
		var assessment SecurityAssessment
		var fixes bool
		if err := rows.Scan(&id, &assessment.Installed, &assessment.Target, &state, &fixes); err != nil {
			return nil, fmt.Errorf("scan software security assessment: %w", err)
		}
		assessment.Status = securityStatus(state, fixes)
		result[id] = assessment
	}
	return result, rows.Err()
}

func securityStatus(state string, fixes bool) SecurityStatus {
	switch {
	case fixes:
		return SecurityFixes
	case state == "pending" || state == "retry":
		return SecurityPending
	default:
		return SecurityNotEstablished
	}
}
