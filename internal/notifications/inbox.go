package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
)

// La boîte de réception d'une personne. Elle ne dit que ce que cette personne a
// reçu : deux comptes ouverts sur la même instance ne lisent pas la même chose,
// puisqu'un compte arrivé après une ouverture n'en a pas eu la nouvelle.

// InboxLimit borne ce qu'un écran demande d'un coup. Au-delà, ce n'est plus une
// boîte de réception mais le Journal d'activité, qui a son propre écran.
const InboxLimit = 50

type InboxEntry struct {
	ID                  int64              `json:"id"`
	IncidentID          string             `json:"incident_id"`
	BurstID             string             `json:"burst_id,omitempty"`
	Revision            int                `json:"revision"`
	TargetID            string             `json:"target_id,omitempty"`
	EventKind           string             `json:"event_kind"`
	TargetName          string             `json:"target_name"`
	NatureKey           string             `json:"nature_key"`
	NatureLabel         string             `json:"nature_label"`
	Severity            incidents.Severity `json:"severity"`
	IncidentCount       int                `json:"incident_count"`
	AffectedTargetCount int                `json:"affected_target_count"`
	MaxAffectedTargets  int                `json:"max_affected_targets"`
	BurstStatus         string             `json:"burst_status,omitempty"`
	BurstExtended       bool               `json:"burst_extended"`
	OccurredAt          time.Time          `json:"occurred_at"`
	ReadAt              *time.Time         `json:"read_at"`
}

type Inbox struct {
	Entries []InboxEntry `json:"entries"`
	Unread  int          `json:"unread"`
}

// Inbox retourne les dernières entrées d'une personne, les plus récentes
// d'abord, et le compte de celles qu'elle n'a pas encore lues. Le compte porte
// sur toute la boîte, pas seulement sur la page rendue : c'est lui que le rail
// affiche, et il mentirait s'il s'arrêtait à la limite.
func (store *PostgresStore) Inbox(ctx context.Context, userID string, limit int) (Inbox, error) {
	if limit <= 0 || limit > InboxLimit {
		limit = InboxLimit
	}

	rows, err := store.pool.Query(ctx, `
		SELECT inbox.id, inbox.incident_id::text, coalesce(inbox.burst_id::text, ''),
		       inbox.revision, coalesce(inbox.target_id::text, ''),
		       inbox.event_kind, inbox.target_name, incident.nature_key,
		       inbox.nature_label, inbox.severity, inbox.incident_count,
		       inbox.affected_target_count, inbox.max_affected_targets,
		       coalesce(inbox.burst_status, ''), inbox.burst_extended,
		       inbox.occurred_at, inbox.read_at
		FROM cairnops_notification_inbox inbox
		JOIN cairnops_incidents incident ON incident.id = inbox.incident_id
		WHERE inbox.user_id = $1::uuid AND inbox.dismissed_at IS NULL
		ORDER BY inbox.occurred_at DESC, inbox.id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return Inbox{}, fmt.Errorf("read notification inbox: %w", err)
	}
	defer rows.Close()

	inbox := Inbox{Entries: make([]InboxEntry, 0, limit)}
	for rows.Next() {
		var entry InboxEntry
		if err := rows.Scan(
			&entry.ID, &entry.IncidentID, &entry.BurstID, &entry.Revision,
			&entry.TargetID, &entry.EventKind,
			&entry.TargetName, &entry.NatureKey, &entry.NatureLabel, &entry.Severity,
			&entry.IncidentCount, &entry.AffectedTargetCount,
			&entry.MaxAffectedTargets, &entry.BurstStatus, &entry.BurstExtended,
			&entry.OccurredAt, &entry.ReadAt,
		); err != nil {
			return Inbox{}, fmt.Errorf("scan notification inbox: %w", err)
		}
		inbox.Entries = append(inbox.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return Inbox{}, fmt.Errorf("iterate notification inbox: %w", err)
	}

	if err := store.pool.QueryRow(ctx, `
		SELECT count(*) FROM cairnops_notification_inbox
		WHERE user_id = $1::uuid AND read_at IS NULL AND dismissed_at IS NULL
	`, userID).Scan(&inbox.Unread); err != nil {
		return Inbox{}, fmt.Errorf("count unread notifications: %w", err)
	}
	return inbox, nil
}

// MarkRead marque des entrées comme lues. Sans identifiant, toute la boîte est
// lue : c'est le geste courant, et il ne concerne jamais que son auteur.
//
// Lire n'efface rien. Une entrée lue reste dans la boîte avec sa date, parce
// que ce qu'on a su et quand on l'a su fait partie de ce qu'on relit.
func (store *PostgresStore) MarkRead(ctx context.Context, userID string, ids []int64) (int, error) {
	query := `
		UPDATE cairnops_notification_inbox
		SET read_at = now()
		WHERE user_id = $1::uuid AND read_at IS NULL AND dismissed_at IS NULL
	`
	args := []any{userID}
	if len(ids) > 0 {
		query += ` AND id = ANY($2::bigint[])`
		args = append(args, ids)
	}
	result, err := store.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("mark notifications read: %w", err)
	}
	return int(result.RowsAffected()), nil
}

// Dismiss vide le volet d'une personne sans effacer la mémoire de livraison.
// Les ouvertures retirées restent donc disponibles au routage d'une future
// Résolution vers les mêmes destinataires, mais ne réapparaissent plus dans la
// boîte et ne participent plus à son compteur.
func (store *PostgresStore) Dismiss(ctx context.Context, userID string) (int, error) {
	result, err := store.pool.Exec(ctx, `
		UPDATE cairnops_notification_inbox
		SET dismissed_at = now(), read_at = coalesce(read_at, now())
		WHERE user_id = $1::uuid AND dismissed_at IS NULL
	`, userID)
	if err != nil {
		return 0, fmt.Errorf("dismiss notification inbox: %w", err)
	}
	return int(result.RowsAffected()), nil
}
