// Package retention efface ce que CairnOps a décidé de ne pas garder : les
// Observations brutes déjà consolidées, les agrégats horaires trop anciens
// (ADR 0008), les événements temps réel au-delà de leur fenêtre de reprise
// (ADR 0022) et les livraisons de notifications closes depuis longtemps.
//
// Une passe supprime par lots courts, chacun dans sa propre transaction : une
// purge ne verrouille jamais une table assez longtemps pour retarder
// l'ordonnanceur qui, lui, écrit des Observations en continu.
package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// batchSize borne une suppression. Quelques milliers de lignes tiennent
	// dans une transaction brève, même sur un disque modeste.
	batchSize = 5000

	// maximumBatchesPerPass borne une passe pour chaque table. Un rattrapage
	// après une longue interruption se poursuit à la passe suivante plutôt
	// que de monopoliser PostgreSQL.
	maximumBatchesPerPass = 200

	passInterval = 15 * time.Minute
)

// Purger applique la Politique de rétention à intervalle régulier.
type Purger struct {
	pool      *pgxpool.Pool
	policy    Policy
	logger    *slog.Logger
	interval  time.Duration
	batchSize int
}

// Report compte les lignes supprimées par une passe.
type Report struct {
	Observations           int64
	ObservationHours       int64
	Events                 int64
	NotificationDeliveries int64
	PushDeliveries         int64
}

func (report Report) total() int64 {
	return report.Observations + report.ObservationHours + report.Events +
		report.NotificationDeliveries + report.PushDeliveries
}

func NewPurger(pool *pgxpool.Pool, policy Policy, logger *slog.Logger) *Purger {
	if logger == nil {
		logger = slog.Default()
	}
	return &Purger{pool: pool, policy: policy, logger: logger, interval: passInterval, batchSize: batchSize}
}

// Run purge au démarrage puis à intervalle régulier. Une passe en échec
// n'arrête pas la boucle : la suivante reprend là où celle-ci s'est arrêtée.
func (purger *Purger) Run(ctx context.Context) error {
	purger.pass(ctx)
	ticker := time.NewTicker(purger.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			purger.pass(ctx)
		}
	}
}

func (purger *Purger) pass(ctx context.Context) {
	report, err := purger.Purge(ctx, time.Now())
	if err != nil {
		if ctx.Err() == nil {
			purger.logger.Error("retention pass failed", "error", err)
		}
		return
	}
	if report.total() > 0 {
		purger.logger.Info("retention pass completed",
			"observations", report.Observations, "observation_hours", report.ObservationHours,
			"events", report.Events, "notification_deliveries", report.NotificationDeliveries,
			"push_deliveries", report.PushDeliveries)
	}
}

// Purge supprime ce que la Politique n'a plus à conserver à l'instant now.
// Un seul worker purge à la fois ; les autres passent leur tour.
func (purger *Purger) Purge(ctx context.Context, now time.Time) (Report, error) {
	conn, err := purger.pool.Acquire(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("acquire retention connection: %w", err)
	}
	defer conn.Release()

	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('cairnops-retention'))`).Scan(&acquired); err != nil {
		return Report{}, fmt.Errorf("lock retention: %w", err)
	}
	if !acquired {
		return Report{}, nil
	}
	defer func() {
		// Le verrou appartient à la session : une connexion rendue au pool
		// sans l'avoir relâché le garderait pour toujours. Faute de pouvoir le
		// relâcher, la connexion est fermée, ce qui le relâche aussi.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtext('cairnops-retention'))`); err != nil {
			_ = conn.Conn().Close(unlockCtx)
		}
	}()

	now = now.UTC()
	var report Report

	consolidatedThrough, err := readConsolidatedThrough(ctx, conn)
	if err != nil {
		return report, err
	}
	// Une Observation n'est effacée que si son heure est déjà consolidée :
	// sans agrégat, l'effacer reviendrait à perdre la mesure elle-même.
	if consolidatedThrough != nil {
		cutoff := earliest(now.Add(-purger.policy.Observations), *consolidatedThrough)
		if report.Observations, err = purger.deleteInBatches(ctx, conn, deleteObservations, cutoff); err != nil {
			return report, fmt.Errorf("expire observations: %w", err)
		}
	}

	hourCutoff := now.AddDate(0, -purger.policy.ObservationHourMonths, 0).Truncate(time.Hour)
	if report.ObservationHours, err = purger.deleteInBatches(ctx, conn, deleteObservationHours, hourCutoff); err != nil {
		return report, fmt.Errorf("expire observation hours: %w", err)
	}

	if report.Events, err = purger.deleteInBatches(ctx, conn, deleteEvents, now.Add(-purger.policy.Events)); err != nil {
		return report, fmt.Errorf("expire realtime events: %w", err)
	}

	deliveryCutoff := now.Add(-purger.policy.Deliveries)
	if report.NotificationDeliveries, err = purger.deleteInBatches(ctx, conn, deleteNotificationDeliveries, deliveryCutoff); err != nil {
		return report, fmt.Errorf("expire notification deliveries: %w", err)
	}
	if report.PushDeliveries, err = purger.deleteInBatches(ctx, conn, deletePushDeliveries, deliveryCutoff); err != nil {
		return report, fmt.Errorf("expire push deliveries: %w", err)
	}
	return report, nil
}

// deleteInBatches répète une suppression bornée jusqu'à épuisement, chaque
// lot formant sa propre transaction.
func (purger *Purger) deleteInBatches(ctx context.Context, conn *pgxpool.Conn, statement string, cutoff time.Time) (int64, error) {
	var deleted int64
	for range maximumBatchesPerPass {
		tag, err := conn.Exec(ctx, statement, cutoff, purger.batchSize)
		if err != nil {
			return deleted, err
		}
		deleted += tag.RowsAffected()
		if tag.RowsAffected() < int64(purger.batchSize) {
			break
		}
	}
	return deleted, nil
}

func readConsolidatedThrough(ctx context.Context, conn *pgxpool.Conn) (*time.Time, error) {
	var consolidatedThrough time.Time
	err := conn.QueryRow(ctx, `SELECT consolidated_through FROM cairnops_observation_rollup_state WHERE id`).Scan(&consolidatedThrough)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read rollup progress: %w", err)
	}
	return &consolidatedThrough, nil
}

func earliest(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

// deleteObservations parcourt les Sources une à une pour profiter de l'index
// (source_id, observed_at) : la table n'en a aucun sur observed_at seul.
//
// La dernière Observation de chaque Source survit à la rétention. Le dernier
// verdict connu d'une Source suspendue ou muette depuis des semaines reste
// ainsi lisible, au lieu de disparaître comme s'il n'avait jamais existé.
const deleteObservations = `
DELETE FROM cairnops_observations
WHERE id IN (
	SELECT expired.id
	FROM cairnops_signal_sources source
	CROSS JOIN LATERAL (
		SELECT max(latest.observed_at) AS observed_at
		FROM cairnops_observations latest
		WHERE latest.source_id = source.id
	) latest
	CROSS JOIN LATERAL (
		SELECT observation.id
		FROM cairnops_observations observation
		WHERE observation.source_id = source.id
		  AND observation.observed_at < least($1::timestamptz, latest.observed_at)
		LIMIT $2
	) expired
	LIMIT $2
)
`

const deleteObservationHours = `
DELETE FROM cairnops_observation_hours
WHERE (source_id, hour) IN (
	SELECT expired.source_id, expired.hour
	FROM cairnops_signal_sources source
	CROSS JOIN LATERAL (
		SELECT bucket.source_id, bucket.hour
		FROM cairnops_observation_hours bucket
		WHERE bucket.source_id = source.id AND bucket.hour < $1
		LIMIT $2
	) expired
	LIMIT $2
)
`

// deleteEvents conserve toujours l'événement le plus récent : il porte la
// version courante, que lit tout client qui se connecte sans curseur, et
// permet de reconnaître un curseur venu d'une autre base.
const deleteEvents = `
DELETE FROM cairnops_events
WHERE version IN (
	SELECT event.version
	FROM cairnops_events event
	WHERE event.occurred_at < $1
	  AND event.version < (SELECT max(newest.version) FROM cairnops_events newest)
	ORDER BY event.occurred_at
	LIMIT $2
)
`

// Les livraisons d'un Incident forment sa mémoire de notification :
// l'ouverture livrée conditionne les révisions et la Résolution, la plus forte
// Gravité livrée conditionne l'escalade, et la plus haute révision évite de
// renvoyer ce qui l'a déjà été. Tant qu'un Incident peut encore évoluer, rien
// de cette mémoire ne s'efface. Seul un Incident résolu depuis plus longtemps
// que la rétention, donc bien au-delà de la fenêtre de Reprise de l'ADR 0054,
// perd ses livraisons closes — livrées ou annulées. Une livraison encore en
// échec reste, puisqu'elle sera retentée.
const deleteNotificationDeliveries = `
DELETE FROM cairnops_notification_outbox
WHERE id IN (
	SELECT delivery.id
	FROM cairnops_notification_outbox delivery
	JOIN cairnops_incidents incident ON incident.id = delivery.incident_id
	WHERE delivery.status IN ('delivered', 'cancelled')
	  AND delivery.updated_at < $1
	  AND incident.status = 'resolved' AND incident.resolved_at < $1
	LIMIT $2
)
`

const deletePushDeliveries = `
DELETE FROM cairnops_push_outbox
WHERE id IN (
	SELECT outgoing.id
	FROM cairnops_push_outbox outgoing
	JOIN cairnops_notification_inbox inbox ON inbox.id = outgoing.inbox_id
	JOIN cairnops_incidents incident ON incident.id = inbox.incident_id
	WHERE outgoing.status IN ('delivered', 'cancelled')
	  AND outgoing.updated_at < $1
	  AND incident.status = 'resolved' AND incident.resolved_at < $1
	LIMIT $2
)
`
