package health

import (
	"context"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/alerttext"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store projette l'État de santé de chaque Ressource active. Il rassemble les
// faits ; c'est Evaluate qui conclut, et lui seul.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// States donne l'État de santé de chaque Ressource active, indexé par son
// identifiant. Une Ressource archivée en est absente : elle est sortie de la
// supervision active et de l'État global.
func (store *Store) States(ctx context.Context) (map[string]State, error) {
	facts, err := store.facts(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	states := make(map[string]State, len(facts))
	for id, fact := range facts {
		states[id] = Evaluate(*fact, now)
	}
	return states, nil
}

func (store *Store) facts(ctx context.Context) (map[string]*Facts, error) {
	facts := make(map[string]*Facts)

	// Toute Ressource active figure dans le résultat, même sans Contrôle : son
	// absence de preuve est une conclusion — État inconnu — et non un trou.
	rows, err := store.pool.Query(ctx, `
		SELECT target.id::text
		FROM cairnops_targets target
		WHERE target.archived_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("list resources for health: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan resource for health: %w", err)
		}
		facts[id] = &Facts{TargetID: id}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resources for health: %w", err)
	}
	if len(facts) == 0 {
		return facts, nil
	}

	// Les Contrôles de toutes origines, natifs comme importés : un Contrôle
	// d'Intégration mesure la disponibilité au même titre qu'une sonde locale.
	checkRows, err := store.pool.Query(ctx, `
		SELECT source.target_id::text, source.enabled, source.measures_availability,
		       source.interval_seconds, latest.observed_at, coalesce(latest.outcome, '')
		FROM cairnops_signal_sources source
		JOIN cairnops_targets target ON target.id = source.target_id
		LEFT JOIN LATERAL (
			SELECT observation.outcome, observation.observed_at
			FROM cairnops_observations observation
			WHERE observation.source_id = source.id
			ORDER BY observation.observed_at DESC, observation.id DESC
			LIMIT 1
		) latest ON true
		WHERE target.archived_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("list checks for health: %w", err)
	}
	defer checkRows.Close()
	for checkRows.Next() {
		var targetID string
		var check Check
		var intervalSeconds int
		if err := checkRows.Scan(&targetID, &check.Enabled, &check.MeasuresAvailability,
			&intervalSeconds, &check.LastObservedAt, &check.LatestOutcome); err != nil {
			return nil, fmt.Errorf("scan check for health: %w", err)
		}
		check.Cadence = time.Duration(intervalSeconds) * time.Second
		if fact := facts[targetID]; fact != nil {
			fact.Checks = append(fact.Checks, check)
		}
	}
	if err := checkRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checks for health: %w", err)
	}

	// Seules les Preuves actives des Atteintes actives d'Incidents actifs
	// comptent. Une Preuve invalidée reste lue pour que la règle l'écarte
	// explicitement, plutôt que de disparaître dans une clause SQL.
	conditionRows, err := store.pool.Query(ctx, `
		SELECT evidence.target_id::text,
		       coalesce(evidence.alert_facts->>'kind', ''),
		       evidence.last_seen_at,
		       evidence.invalidated_at IS NOT NULL
		FROM cairnops_incident_evidence evidence
		JOIN cairnops_incident_impacts impact ON impact.id = evidence.impact_id
		JOIN cairnops_incidents incident ON incident.id = evidence.incident_id
		JOIN cairnops_targets target ON target.id = evidence.target_id
		WHERE target.archived_at IS NULL
		  AND evidence.active
		  AND impact.status = 'active'
		  AND incident.status = 'active'
	`)
	if err != nil {
		return nil, fmt.Errorf("list conditions for health: %w", err)
	}
	defer conditionRows.Close()
	for conditionRows.Next() {
		var targetID, kind string
		var condition Condition
		if err := conditionRows.Scan(&targetID, &kind, &condition.LastSeenAt, &condition.Invalidated); err != nil {
			return nil, fmt.Errorf("scan condition for health: %w", err)
		}
		condition.Kind = alerttext.Kind(kind)
		if fact := facts[targetID]; fact != nil {
			fact.Conditions = append(fact.Conditions, condition)
		}
	}
	if err := conditionRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conditions for health: %w", err)
	}
	return facts, nil
}
