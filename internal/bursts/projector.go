package bursts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type candidate struct {
	ID          string
	TargetID    string
	Scope       string
	Namespace   string
	Fingerprint string
	Label       string
	Severity    string
	OpenedAt    time.Time
	Status      string
}

// Project met à jour la projection des Rafales avant que la boîte de sortie ne
// choisisse ses livraisons. L'appelant lui fournit sa transaction : aucun
// deuxième Incident ne peut donc être notifié entre son adhésion et la
// suppression de la livraison redondante.
func Project(ctx context.Context, tx pgx.Tx, now time.Time) error {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('cairnops-incident-bursts'))`); err != nil {
		return fmt.Errorf("lock incident burst projection: %w", err)
	}

	if err := sealExpired(ctx, tx, now); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT incident.id::text, incident.target_id::text,
		       incident.nature_scope, incident.nature_namespace,
		       incident.nature_fingerprint, incident.nature_label,
		       incident.effective_severity, incident.opened_at, incident.status
		FROM cairnops_incidents incident
		WHERE incident.burst_eligible
		  AND NOT EXISTS (
		      SELECT 1 FROM cairnops_incident_burst_members member
		      WHERE member.incident_id = incident.id
		  )
		  AND NOT EXISTS (
		      SELECT 1
		      FROM cairnops_maintenance_targets maintenance_target
		      JOIN cairnops_maintenances maintenance
		        ON maintenance.id = maintenance_target.maintenance_id
		      WHERE maintenance_target.target_id = incident.target_id
		        AND maintenance.cancelled_at IS NULL
		        AND $1 BETWEEN maintenance.starts_at AND maintenance.ends_at
		  )
		ORDER BY incident.opened_at, incident.id
	`, now)
	if err != nil {
		return fmt.Errorf("list incident burst candidates: %w", err)
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(
			&item.ID, &item.TargetID, &item.Scope, &item.Namespace,
			&item.Fingerprint, &item.Label, &item.Severity,
			&item.OpenedAt, &item.Status,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan incident burst candidate: %w", err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate incident burst candidates: %w", err)
	}
	rows.Close()

	for _, item := range candidates {
		var alreadyMember bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM cairnops_incident_burst_members WHERE incident_id = $1::uuid
			)
		`, item.ID).Scan(&alreadyMember); err != nil {
			return fmt.Errorf("check incident burst membership: %w", err)
		}
		if alreadyMember {
			continue
		}
		if err := projectCandidate(ctx, tx, item, now); err != nil {
			return err
		}
	}

	if err := recomputeAll(ctx, tx, now); err != nil {
		return err
	}
	return sealExpired(ctx, tx, now)
}

func projectCandidate(ctx context.Context, tx pgx.Tx, item candidate, now time.Time) error {
	window, err := incidentWindow(ctx, tx, item.ID)
	if err != nil {
		return err
	}
	releasedFromMaintenanceAt, err := maintenanceRelease(ctx, tx, item.TargetID, item.OpenedAt, now)
	if err != nil {
		return err
	}

	var burstID string
	var propagationEndsAt, burstOpenedAt time.Time
	burstRows, err := tx.Query(ctx, `
		SELECT id::text, propagation_ends_at, opened_at
		FROM cairnops_incident_bursts
		WHERE nature_scope = $1 AND nature_namespace = $2 AND nature_fingerprint = $3
		  AND status = 'propagating' AND $4 < propagation_ends_at
		ORDER BY last_joined_at DESC, id
		FOR UPDATE
	`, item.Scope, item.Namespace, item.Fingerprint, now)
	if err != nil {
		return fmt.Errorf("find propagating incident bursts: %w", err)
	}
	type burstMatch struct {
		id       string
		endsAt   time.Time
		openedAt time.Time
	}
	burstMatches := make([]burstMatch, 0)
	for burstRows.Next() {
		var candidateBurstID string
		var candidateEndsAt, candidateOpenedAt time.Time
		if err := burstRows.Scan(&candidateBurstID, &candidateEndsAt, &candidateOpenedAt); err != nil {
			burstRows.Close()
			return fmt.Errorf("scan propagating incident burst: %w", err)
		}
		burstMatches = append(burstMatches, burstMatch{
			id: candidateBurstID, endsAt: candidateEndsAt, openedAt: candidateOpenedAt,
		})
	}
	if err := burstRows.Err(); err != nil {
		burstRows.Close()
		return fmt.Errorf("iterate propagating incident bursts: %w", err)
	}
	burstRows.Close()
	for _, match := range burstMatches {
		compatible := true
		if item.Scope == "canonical" {
			compatible, err = canonicalCompatibleWithBurst(ctx, tx, item.ID, match.id)
			if err != nil {
				return err
			}
		}
		if compatible {
			burstID, propagationEndsAt, burstOpenedAt = match.id, match.endsAt, match.openedAt
			break
		}
	}
	if burstID != "" {
		joinedAt := item.OpenedAt.UTC()
		if now.Sub(joinedAt) > time.Duration(window)*time.Second {
			// Un Incident historique ne peut pas être absorbé au seul motif qu'il
			// vient d'être reçu. La seule exception est sa sortie démontrable d'une
			// maintenance pendant cette Propagation.
			if releasedFromMaintenanceAt == nil || releasedFromMaintenanceAt.Before(burstOpenedAt) {
				return nil
			}
			joinedAt = releasedFromMaintenanceAt.UTC()
		}
		if joinedAt.After(propagationEndsAt) {
			return nil
		}
		return addMember(ctx, tx, burstID, item, joinedAt, window)
	}
	// Une sortie de maintenance ne peut que rejoindre une Propagation déjà
	// ouverte. Sans celle-ci, l'Incident reprend son parcours individuel.
	if releasedFromMaintenanceAt != nil || now.Sub(item.OpenedAt) > time.Duration(window)*time.Second {
		return nil
	}

	var seed candidate
	var seedWindow int
	seedRows, err := tx.Query(ctx, `
		SELECT seed.id::text, seed.target_id::text,
		       seed.nature_scope, seed.nature_namespace, seed.nature_fingerprint,
		       seed.nature_label, seed.effective_severity, seed.opened_at, seed.status,
		       greatest(60, least(300, 2 * coalesce(max(
		           coalesce(source.interval_seconds, connector.sync_interval_seconds)
		       ), 30)))::integer
		FROM cairnops_incidents seed
		LEFT JOIN cairnops_incident_signals signal ON signal.incident_id = seed.id
		LEFT JOIN cairnops_signal_sources source ON source.id = signal.source_id
		LEFT JOIN cairnops_connectors connector ON connector.id = signal.connector_id
		WHERE seed.id <> $1::uuid AND seed.target_id <> $2::uuid
		  AND seed.burst_eligible
		  AND seed.nature_scope = $3 AND seed.nature_namespace = $4
		  AND seed.nature_fingerprint = $5
		  AND seed.opened_at <= $6
		  AND NOT EXISTS (
		      SELECT 1 FROM cairnops_incident_burst_members member
		      WHERE member.incident_id = seed.id
		  )
		  AND NOT EXISTS (
		      SELECT 1
		      FROM cairnops_maintenance_targets maintenance_target
		      JOIN cairnops_maintenances maintenance
		        ON maintenance.id = maintenance_target.maintenance_id
		      WHERE maintenance_target.target_id = seed.target_id
		        AND maintenance.cancelled_at IS NULL
		        AND $7 BETWEEN maintenance.starts_at AND maintenance.ends_at
		  )
		GROUP BY seed.id
		HAVING $6 - seed.opened_at <= make_interval(secs => greatest(
		    $8, greatest(60, least(300, 2 * coalesce(max(
		        coalesce(source.interval_seconds, connector.sync_interval_seconds)
		    ), 30)))::integer
		))
		AND $7 - seed.opened_at <= make_interval(secs => greatest(
		    60, least(300, 2 * coalesce(max(
		        coalesce(source.interval_seconds, connector.sync_interval_seconds)
		    ), 30)))::integer
		)
		ORDER BY seed.opened_at DESC, seed.id DESC
		LIMIT 100
	`, item.ID, item.TargetID, item.Scope, item.Namespace, item.Fingerprint,
		item.OpenedAt, now, window)
	if err != nil {
		return fmt.Errorf("find incident burst seeds: %w", err)
	}
	type seedMatch struct {
		incident candidate
		window   int
	}
	seedMatches := make([]seedMatch, 0)
	for seedRows.Next() {
		var candidateSeed candidate
		var candidateWindow int
		if err := seedRows.Scan(
			&candidateSeed.ID, &candidateSeed.TargetID, &candidateSeed.Scope,
			&candidateSeed.Namespace, &candidateSeed.Fingerprint, &candidateSeed.Label,
			&candidateSeed.Severity, &candidateSeed.OpenedAt, &candidateSeed.Status,
			&candidateWindow,
		); err != nil {
			seedRows.Close()
			return fmt.Errorf("scan incident burst seed: %w", err)
		}
		seedMatches = append(seedMatches, seedMatch{incident: candidateSeed, window: candidateWindow})
	}
	if err := seedRows.Err(); err != nil {
		seedRows.Close()
		return fmt.Errorf("iterate incident burst seeds: %w", err)
	}
	seedRows.Close()
	for _, match := range seedMatches {
		compatible := true
		if item.Scope == "canonical" {
			compatible, err = canonicalIncidentsCompatible(ctx, tx, item.ID, match.incident.ID)
			if err != nil {
				return err
			}
		}
		if compatible {
			seed, seedWindow = match.incident, match.window
			break
		}
	}
	if seed.ID == "" {
		return nil
	}

	if seed.OpenedAt.After(item.OpenedAt) {
		seed, item = item, seed
		seedWindow, window = window, seedWindow
	}
	propagationWindow := max(seedWindow, window)
	lastJoinedAt := item.OpenedAt.UTC()
	if lastJoinedAt.Before(seed.OpenedAt) {
		lastJoinedAt = seed.OpenedAt.UTC()
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO cairnops_incident_bursts (
			anchor_incident_id, nature_scope, nature_namespace, nature_fingerprint,
			nature_label, status, severity, opened_at, last_joined_at,
			propagation_window_seconds, propagation_ends_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5, 'propagating', $6,
			$7::timestamptz, $8::timestamptz,
			$9::integer, $8::timestamptz + make_interval(secs => $9::integer)
		)
		RETURNING id::text
	`, seed.ID, seed.Scope, seed.Namespace, seed.Fingerprint, seed.Label,
		maxSeverity(seed.Severity, item.Severity), seed.OpenedAt, lastJoinedAt,
		propagationWindow).Scan(&burstID)
	if err != nil {
		return fmt.Errorf("form incident burst: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_burst_activity (burst_id, kind, message, data, occurred_at)
		VALUES ($1::uuid, 'formed', 'Rafale formée à partir de deux Cibles distinctes',
		        jsonb_build_object('nature_scope', $2::text, 'nature_namespace', $3::text,
		                           'nature_fingerprint', $4::text), $5::timestamptz)
	`, burstID, seed.Scope, seed.Namespace, seed.Fingerprint, lastJoinedAt); err != nil {
		return fmt.Errorf("record incident burst formation: %w", err)
	}
	if err := insertMember(ctx, tx, burstID, seed, seed.OpenedAt); err != nil {
		return err
	}
	if err := insertMember(ctx, tx, burstID, item, item.OpenedAt); err != nil {
		return err
	}
	return recompute(ctx, tx, burstID, now)
}

func addMember(ctx context.Context, tx pgx.Tx, burstID string, item candidate, joinedAt time.Time, window int) error {
	if err := insertMember(ctx, tx, burstID, item, joinedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_bursts
		SET last_joined_at = greatest(last_joined_at, $2),
		    propagation_window_seconds = greatest(propagation_window_seconds, $3),
		    propagation_ends_at = greatest(last_joined_at, $2)
		        + make_interval(secs => greatest(propagation_window_seconds, $3)),
		    revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid AND status = 'propagating'
	`, burstID, joinedAt, window); err != nil {
		return fmt.Errorf("extend incident burst propagation: %w", err)
	}
	return recompute(ctx, tx, burstID, joinedAt)
}

func insertMember(ctx context.Context, tx pgx.Tx, burstID string, item candidate, joinedAt time.Time) error {
	result, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_burst_members (burst_id, incident_id, target_id, joined_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
		ON CONFLICT (incident_id) DO NOTHING
	`, burstID, item.ID, item.TargetID, joinedAt)
	if err != nil {
		return fmt.Errorf("join incident to burst: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_burst_activity (burst_id, kind, message, data, occurred_at)
		VALUES ($1::uuid, 'incident_joined', 'Incident rattaché par Nature et proximité temporelle',
		        jsonb_build_object('incident_id', $2::text, 'target_id', $3::text), $4::timestamptz)
	`, burstID, item.ID, item.TargetID, joinedAt); err != nil {
		return fmt.Errorf("record incident burst membership: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cairnops_incident_activity (incident_id, kind, origin, message, data, occurred_at)
		VALUES ($2::uuid, 'burst_joined', 'cairnops', 'Incident rattaché à une Rafale',
		        jsonb_build_object('burst_id', $1::text), $3::timestamptz)
	`, burstID, item.ID, joinedAt); err != nil {
		return fmt.Errorf("record incident burst membership: %w", err)
	}

	// Un Acquittement de Rafale couvre aussi les futurs membres. La tentative de
	// synchronisation externe reste portée par l'Incident et peut donc échouer
	// indépendamment des autres membres.
	if _, err := tx.Exec(ctx, `
		WITH acknowledged AS (
		UPDATE cairnops_incidents incident
		SET acknowledged_at = burst.acknowledged_at,
		    acknowledged_by = burst.acknowledged_by,
		    acknowledgement_origin = 'user',
		    acknowledgement_sync_status = CASE WHEN EXISTS (
		        SELECT 1 FROM cairnops_incident_signals signal
		        WHERE signal.incident_id = incident.id AND signal.active AND signal.origin = 'zabbix'
		    ) THEN 'pending' ELSE 'not_applicable' END,
		    updated_at = now()
		FROM cairnops_incident_bursts burst
		WHERE incident.id = $2::uuid AND burst.id = $1::uuid
		  AND burst.acknowledged_at IS NOT NULL AND incident.acknowledged_at IS NULL
		RETURNING incident.id, burst.acknowledged_by, burst.acknowledged_at
		)
		INSERT INTO cairnops_incident_activity (
			incident_id, kind, origin, actor_id, message, data, occurred_at
		)
		SELECT id, 'acknowledged', 'user', acknowledged_by,
		       'Incident acquitté par la Rafale', jsonb_build_object('burst_id', $1::text),
		       acknowledged_at
		FROM acknowledged
	`, burstID, item.ID); err != nil {
		return fmt.Errorf("inherit incident burst acknowledgement: %w", err)
	}
	return nil
}

func incidentWindow(ctx context.Context, tx pgx.Tx, incidentID string) (int, error) {
	var seconds int
	if err := tx.QueryRow(ctx, `
		SELECT greatest(60, least(300, 2 * coalesce(max(
		    coalesce(source.interval_seconds, connector.sync_interval_seconds)
		), 30)))::integer
		FROM cairnops_incident_signals signal
		LEFT JOIN cairnops_signal_sources source ON source.id = signal.source_id
		LEFT JOIN cairnops_connectors connector ON connector.id = signal.connector_id
		WHERE signal.incident_id = $1::uuid
	`, incidentID).Scan(&seconds); err != nil {
		return 0, fmt.Errorf("calculate incident burst window: %w", err)
	}
	return seconds, nil
}

func maintenanceRelease(ctx context.Context, tx pgx.Tx, targetID string, openedAt, now time.Time) (*time.Time, error) {
	var releasedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT max(least(maintenance.ends_at, coalesce(maintenance.cancelled_at, maintenance.ends_at)))
		FROM cairnops_maintenance_targets maintenance_target
		JOIN cairnops_maintenances maintenance ON maintenance.id = maintenance_target.maintenance_id
		WHERE maintenance_target.target_id = $1::uuid
		  AND $2 BETWEEN maintenance.starts_at
		             AND least(maintenance.ends_at, coalesce(maintenance.cancelled_at, maintenance.ends_at))
		  AND least(maintenance.ends_at, coalesce(maintenance.cancelled_at, maintenance.ends_at)) <= $3
	`, targetID, openedAt, now).Scan(&releasedAt); err != nil {
		return nil, fmt.Errorf("find incident maintenance release: %w", err)
	}
	return releasedAt, nil
}

func canonicalIncidentsCompatible(ctx context.Context, tx pgx.Tx, leftIncidentID, rightIncidentID string) (bool, error) {
	var compatible bool
	if err := tx.QueryRow(ctx, `
		WITH left_sources AS (
			SELECT DISTINCT origin AS kind,
			       coalesce(connector_id::text, 'cairnops') AS instance
			FROM cairnops_incident_signals WHERE incident_id = $1::uuid
		), right_sources AS (
			SELECT DISTINCT origin AS kind,
			       coalesce(connector_id::text, 'cairnops') AS instance
			FROM cairnops_incident_signals WHERE incident_id = $2::uuid
		), shared_kinds AS (
			SELECT DISTINCT left_source.kind
			FROM left_sources left_source
			JOIN right_sources right_source ON right_source.kind = left_source.kind
		)
		SELECT NOT EXISTS (
			SELECT 1 FROM shared_kinds shared
			WHERE NOT EXISTS (
				SELECT 1
				FROM left_sources left_source
				JOIN right_sources right_source
				  ON right_source.kind = left_source.kind
				 AND right_source.instance = left_source.instance
				WHERE left_source.kind = shared.kind
			)
		)
	`, leftIncidentID, rightIncidentID).Scan(&compatible); err != nil {
		return false, fmt.Errorf("compare canonical incident sources: %w", err)
	}
	return compatible, nil
}

func canonicalCompatibleWithBurst(ctx context.Context, tx pgx.Tx, incidentID, burstID string) (bool, error) {
	var compatible bool
	if err := tx.QueryRow(ctx, `
		WITH candidate_sources AS (
			SELECT DISTINCT origin AS kind,
			       coalesce(connector_id::text, 'cairnops') AS instance
			FROM cairnops_incident_signals WHERE incident_id = $1::uuid
		), burst_sources AS (
			SELECT DISTINCT signal.origin AS kind,
			       coalesce(signal.connector_id::text, 'cairnops') AS instance
			FROM cairnops_incident_burst_members member
			JOIN cairnops_incident_signals signal ON signal.incident_id = member.incident_id
			WHERE member.burst_id = $2::uuid
		), shared_kinds AS (
			SELECT DISTINCT candidate.kind
			FROM candidate_sources candidate
			JOIN burst_sources member ON member.kind = candidate.kind
		)
		SELECT NOT EXISTS (
			SELECT 1 FROM shared_kinds shared
			WHERE NOT EXISTS (
				SELECT 1
				FROM candidate_sources candidate
				JOIN burst_sources member
				  ON member.kind = candidate.kind
				 AND member.instance = candidate.instance
				WHERE candidate.kind = shared.kind
			)
		)
	`, incidentID, burstID).Scan(&compatible); err != nil {
		return false, fmt.Errorf("compare canonical incident and burst sources: %w", err)
	}
	return compatible, nil
}

func recomputeAll(ctx context.Context, tx pgx.Tx, now time.Time) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text FROM cairnops_incident_bursts
		WHERE status <> 'resolved' ORDER BY opened_at, id
	`)
	if err != nil {
		return fmt.Errorf("list incident bursts to recompute: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan incident burst to recompute: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate incident bursts to recompute: %w", err)
	}
	rows.Close()
	for _, id := range ids {
		if err := recompute(ctx, tx, id, now); err != nil {
			return err
		}
	}
	return nil
}

func recompute(ctx context.Context, tx pgx.Tx, burstID string, now time.Time) error {
	type state struct {
		severity                                             string
		extended                                             bool
		activeIncidents, incidents, affectedTargets, targets int
		maxAffected                                          int
	}
	var before state
	if err := tx.QueryRow(ctx, `
		SELECT severity, extended, active_incident_count, incident_count,
		       affected_target_count, target_count, max_affected_targets
		FROM cairnops_incident_bursts WHERE id = $1::uuid FOR UPDATE
	`, burstID).Scan(
		&before.severity, &before.extended, &before.activeIncidents,
		&before.incidents, &before.affectedTargets, &before.targets,
		&before.maxAffected,
	); err != nil {
		return fmt.Errorf("lock incident burst summary: %w", err)
	}

	var after state
	var activeSeverity *string
	if err := tx.QueryRow(ctx, `
		WITH membership AS (
			SELECT incident.target_id, incident.status, incident.effective_severity,
			       NOT EXISTS (
			           SELECT 1
			           FROM cairnops_maintenance_targets maintenance_target
			           JOIN cairnops_maintenances maintenance
			             ON maintenance.id = maintenance_target.maintenance_id
			           WHERE maintenance_target.target_id = incident.target_id
			             AND maintenance.cancelled_at IS NULL
			             AND $2 BETWEEN maintenance.starts_at AND maintenance.ends_at
			       ) AS visible
			FROM cairnops_incident_burst_members member
			JOIN cairnops_incidents incident ON incident.id = member.incident_id
			WHERE member.burst_id = $1::uuid
		)
		SELECT count(*)::integer,
		       count(*) FILTER (WHERE status = 'active' AND visible)::integer,
		       count(DISTINCT target_id)::integer,
		       count(DISTINCT target_id) FILTER (WHERE status = 'active' AND visible)::integer,
		       CASE max(CASE effective_severity
		           WHEN 'information' THEN 1 WHEN 'warning' THEN 2
		           WHEN 'major' THEN 3 WHEN 'critical' THEN 4 END)
		           FILTER (WHERE status = 'active' AND visible)
		           WHEN 1 THEN 'information' WHEN 2 THEN 'warning'
		           WHEN 3 THEN 'major' WHEN 4 THEN 'critical' END
		FROM membership
	`, burstID, now).Scan(
		&after.incidents, &after.activeIncidents, &after.targets,
		&after.affectedTargets, &activeSeverity,
	); err != nil {
		return fmt.Errorf("summarize incident burst: %w", err)
	}
	after.severity = before.severity
	if activeSeverity != nil {
		after.severity = *activeSeverity
	}
	after.maxAffected = max(before.maxAffected, after.affectedTargets)

	var estateTargets int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)::integer
		FROM cairnops_targets target
		WHERE target.archived_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1
		      FROM cairnops_maintenance_targets maintenance_target
		      JOIN cairnops_maintenances maintenance
		        ON maintenance.id = maintenance_target.maintenance_id
		      WHERE maintenance_target.target_id = target.id
		        AND maintenance.cancelled_at IS NULL
		        AND $1 BETWEEN maintenance.starts_at AND maintenance.ends_at
		  )
	`, now).Scan(&estateTargets); err != nil {
		return fmt.Errorf("count active targets for incident burst: %w", err)
	}
	becameExtended := !before.extended && after.affectedTargets >= 5 &&
		(after.affectedTargets >= 20 || after.affectedTargets*5 >= max(estateTargets, 1))
	after.extended = before.extended || becameExtended

	changed := before.severity != after.severity || before.extended != after.extended ||
		before.activeIncidents != after.activeIncidents || before.incidents != after.incidents ||
		before.affectedTargets != after.affectedTargets || before.targets != after.targets ||
		before.maxAffected != after.maxAffected
	if !changed {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_incident_bursts
		SET severity = $2, extended = $3, active_incident_count = $4,
		    incident_count = $5, affected_target_count = $6, target_count = $7,
		    max_affected_targets = $8, revision = revision + 1, updated_at = now()
		WHERE id = $1::uuid
	`, burstID, after.severity, after.extended, after.activeIncidents,
		after.incidents, after.affectedTargets, after.targets, after.maxAffected); err != nil {
		return fmt.Errorf("refresh incident burst summary: %w", err)
	}
	if before.severity != after.severity {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_burst_activity (burst_id, kind, message, data, occurred_at)
			VALUES ($1::uuid, 'severity_changed', 'Gravité effective de la Rafale actualisée',
			        jsonb_build_object('previous', $2::text, 'current', $3::text), $4::timestamptz)
		`, burstID, before.severity, after.severity, now); err != nil {
			return fmt.Errorf("record incident burst severity: %w", err)
		}
	}
	if becameExtended {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_burst_activity (burst_id, kind, message, data, occurred_at)
			VALUES ($1::uuid, 'extended', 'Seuil de Propagation étendue atteint',
			        jsonb_build_object('affected_targets', $2::integer, 'active_targets', $3::integer), $4::timestamptz)
		`, burstID, after.affectedTargets, estateTargets, now); err != nil {
			return fmt.Errorf("record extended incident burst: %w", err)
		}
	}
	return nil
}

func sealExpired(ctx context.Context, tx pgx.Tx, now time.Time) error {
	rows, err := tx.Query(ctx, `
		UPDATE cairnops_incident_bursts
		SET status = 'sealed', sealed_at = propagation_ends_at,
		    revision = revision + 1, updated_at = now()
		WHERE status = 'propagating' AND propagation_ends_at <= $1
		RETURNING id::text, sealed_at
	`, now)
	if err != nil {
		return fmt.Errorf("seal incident bursts: %w", err)
	}
	type sealed struct {
		id string
		at time.Time
	}
	items := make([]sealed, 0)
	for rows.Next() {
		var item sealed
		if err := rows.Scan(&item.id, &item.at); err != nil {
			rows.Close()
			return fmt.Errorf("scan sealed incident burst: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate sealed incident bursts: %w", err)
	}
	rows.Close()
	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_burst_activity (burst_id, kind, message, occurred_at)
			VALUES ($1::uuid, 'sealed', 'Propagation fermée ; le périmètre est désormais immuable', $2)
		`, item.id, item.at); err != nil {
			return fmt.Errorf("record incident burst seal: %w", err)
		}
	}

	resolvedRows, err := tx.Query(ctx, `
		UPDATE cairnops_incident_bursts burst
		SET status = 'resolved', resolved_at = $1,
		    revision = revision + 1, updated_at = now()
		WHERE burst.status = 'sealed'
		  AND NOT EXISTS (
		      SELECT 1
		      FROM cairnops_incident_burst_members member
		      JOIN cairnops_incidents incident ON incident.id = member.incident_id
		      WHERE member.burst_id = burst.id AND incident.status = 'active'
		  )
		RETURNING id::text
	`, now)
	if err != nil {
		return fmt.Errorf("resolve sealed incident bursts: %w", err)
	}
	resolvedIDs := make([]string, 0)
	for resolvedRows.Next() {
		var id string
		if err := resolvedRows.Scan(&id); err != nil {
			resolvedRows.Close()
			return fmt.Errorf("scan resolved incident burst: %w", err)
		}
		resolvedIDs = append(resolvedIDs, id)
	}
	if err := resolvedRows.Err(); err != nil {
		resolvedRows.Close()
		return fmt.Errorf("iterate resolved incident bursts: %w", err)
	}
	resolvedRows.Close()
	for _, id := range resolvedIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_incident_burst_activity (burst_id, kind, message, occurred_at)
			VALUES ($1::uuid, 'resolved', 'Tous les Incidents membres sont résolus', $2)
		`, id, now); err != nil {
			return fmt.Errorf("record incident burst resolution: %w", err)
		}
	}
	return nil
}

func maxSeverity(left, right string) string {
	rank := map[string]int{"information": 1, "warning": 2, "major": 3, "critical": 4}
	if rank[right] > rank[left] {
		return right
	}
	return left
}
