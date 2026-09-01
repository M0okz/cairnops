package bursts

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AcknowledgementSynchronizer reprend les futurs membres d'une Rafale déjà
// acquittée. L'Acquittement local est posé dans la transaction d'adhésion ; ce
// petit cycle confie ensuite à chaque Incident sa propre synchronisation amont.
type AcknowledgementSynchronizer struct {
	pool      *pgxpool.Pool
	incidents IncidentAcknowledger
	logger    *slog.Logger
	interval  time.Duration
}

func NewAcknowledgementSynchronizer(pool *pgxpool.Pool, incidentAcknowledger IncidentAcknowledger, logger *slog.Logger) *AcknowledgementSynchronizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &AcknowledgementSynchronizer{
		pool: pool, incidents: incidentAcknowledger, logger: logger, interval: 2 * time.Second,
	}
}

func (synchronizer *AcknowledgementSynchronizer) Run(ctx context.Context) error {
	ticker := time.NewTicker(synchronizer.interval)
	defer ticker.Stop()
	for {
		if err := synchronizer.Process(ctx); err != nil && !errors.Is(err, context.Canceled) {
			synchronizer.logger.Error("synchronize inherited burst acknowledgements", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (synchronizer *AcknowledgementSynchronizer) Process(ctx context.Context) error {
	if synchronizer == nil || synchronizer.pool == nil || synchronizer.incidents == nil {
		return nil
	}
	rows, err := synchronizer.pool.Query(ctx, `
		SELECT incident.id::text, coalesce(burst.acknowledged_by::text, ''),
		       coalesce(actor.display_name, '')
		FROM cairnops_incidents incident
		JOIN cairnops_incident_burst_members member ON member.incident_id = incident.id
		JOIN cairnops_incident_bursts burst ON burst.id = member.burst_id
		LEFT JOIN cairnops_users actor ON actor.id = burst.acknowledged_by
		WHERE burst.acknowledged_at IS NOT NULL
		  AND incident.status = 'active'
		  AND incident.acknowledgement_sync_status = 'pending'
		ORDER BY incident.updated_at, incident.id
		LIMIT 20
	`)
	if err != nil {
		return err
	}
	type pending struct{ incidentID, actorID, actorName string }
	items := make([]pending, 0)
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.incidentID, &item.actorID, &item.actorName); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range items {
		if _, err := synchronizer.incidents.Acknowledge(
			ctx, item.incidentID, item.actorID, item.actorName,
		); err != nil {
			synchronizer.logger.Warn(
				"synchronize inherited incident acknowledgement",
				"incident_id", item.incidentID, "error", err,
			)
		}
	}
	return nil
}
