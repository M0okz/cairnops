package incidents

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Runtime fait avancer le temps métier du cycle Incident–Preuve. Il ferme les
// fenêtres de propagation même en l'absence de nouvelle observation et reprend
// les acquittements hérités par de nouvelles Preuves Zabbix.
type Runtime struct {
	store    *PostgresStore
	service  *Service
	logger   *slog.Logger
	interval time.Duration
}

func NewRuntime(store *PostgresStore, service *Service, logger *slog.Logger) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{store: store, service: service, logger: logger, interval: 2 * time.Second}
}

func (runtime *Runtime) Run(ctx context.Context) error {
	ticker := time.NewTicker(runtime.interval)
	defer ticker.Stop()
	for {
		if err := runtime.Process(ctx); err != nil && !errors.Is(err, context.Canceled) {
			runtime.logger.Error("advance incident evidence cycle", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (runtime *Runtime) Process(ctx context.Context) error {
	if runtime == nil || runtime.store == nil {
		return nil
	}
	if err := runtime.store.Advance(ctx, time.Now().UTC()); err != nil {
		return err
	}
	if runtime.service == nil {
		return nil
	}
	rows, err := runtime.store.pool.Query(ctx, `
		SELECT incident.id::text, coalesce(incident.acknowledged_by::text, ''),
		       coalesce(actor.display_name, '')
		FROM cairnops_incidents incident
		LEFT JOIN cairnops_users actor ON actor.id = incident.acknowledged_by
		WHERE incident.status = 'active'
		  AND incident.acknowledged_at IS NOT NULL
		  AND EXISTS (
		      SELECT 1 FROM cairnops_incident_evidence evidence
		      WHERE evidence.incident_id = incident.id
		        AND evidence.active AND evidence.origin = 'zabbix'
		        AND (
		            evidence.acknowledgement_sync_status = 'pending'
		            OR (
		                evidence.acknowledgement_sync_status = 'failed'
		                AND evidence.updated_at <= now() - interval '30 seconds'
		            )
		        )
		  )
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
		if _, err := runtime.service.Acknowledge(ctx, item.incidentID, item.actorID, item.actorName); err != nil {
			runtime.logger.Warn("synchronize inherited incident acknowledgement",
				"incident_id", item.incidentID, "error", err)
		}
	}
	return nil
}
