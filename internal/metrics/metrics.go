// Package metrics lit la Disponibilité, la Couverture et la latence d'une
// Cible sur des agrégats horaires, l'heure en cours venant des Observations
// brutes. La règle de dérivation, elle, appartient au domaine.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

// TargetMetrics porte les mesures d'une Cible pour les listes : une seule
// fenêtre, et la tendance horaire qui l'accompagne.
type TargetMetrics struct {
	TargetID string           `json:"target_id"`
	Measures []domain.Measure `json:"measures"`
	Trend    []float64        `json:"trend"`
	// LatencyTrend porte la latence moyenne heure par heure. La Disponibilité
	// d'une installation saine est plate — elle ne se déforme qu'en cas de
	// défaut ; la latence, elle, respire, et c'est elle que la liste trace.
	LatencyTrend []float64 `json:"latency_trend"`
	// LatestObservedAt est la fraîcheur de la Cible, toutes Sources confondues
	// — une Cible n'ayant que des Sources d'Intégration en a une, elle aussi.
	LatestObservedAt *time.Time `json:"latest_observed_at,omitempty"`
}

// SourceMetrics détaille ce qu'une Source a établi sur chaque fenêtre. Son
// origine distingue un Contrôle natif d'une Source apportée par une Intégration
// — la mesure les traite pareillement, l'écran non.
type SourceMetrics struct {
	SourceID      string          `json:"source_id"`
	Name          string          `json:"name"`
	Kind          string          `json:"kind"`
	Origin        string          `json:"origin"`
	LatestOutcome *domain.Outcome `json:"latest_outcome,omitempty"`
	// LatestObservedAt reste absent tant qu'aucune Observation n'a eu lieu.
	LatestObservedAt *time.Time       `json:"latest_observed_at,omitempty"`
	Measures         []domain.Measure `json:"measures"`
}

// TargetDetail ouvre les trois fenêtres de la Cible et nomme la part de
// chaque Source.
type TargetDetail struct {
	TargetID         string           `json:"target_id"`
	GeneratedAt      time.Time        `json:"generated_at"`
	Measures         []domain.Measure `json:"measures"`
	Trend            []float64        `json:"trend"`
	LatencyTrend     []float64        `json:"latency_trend"`
	LatestObservedAt *time.Time       `json:"latest_observed_at,omitempty"`
	Sources          []SourceMetrics  `json:"sources"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// List rend les mesures sur 24 heures de toutes les Cibles actives, tendance
// comprise, en une seule lecture : une liste de Cibles ne doit pas coûter une
// requête par ligne.
func (store *Store) List(ctx context.Context) ([]TargetMetrics, error) {
	hourly, err := store.hourlyByTarget(ctx, domain.WindowDay, "")
	if err != nil {
		return nil, err
	}

	metrics := make([]TargetMetrics, 0, len(hourly))
	for _, target := range hourly {
		metrics = append(metrics, TargetMetrics{
			TargetID:         target.targetID,
			Measures:         []domain.Measure{target.total.Measure(domain.WindowDay)},
			Trend:            target.trend(),
			LatencyTrend:     target.latencyTrend(),
			LatestObservedAt: target.latestObservedAt,
		})
	}
	return metrics, nil
}

// Target ouvre les trois fenêtres d'une Cible et la part de chacune de ses
// Sources. Le total d'une fenêtre est la somme de ses Sources.
func (store *Store) Target(ctx context.Context, targetID string) (TargetDetail, error) {
	var exists bool
	if err := store.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM cairnops_targets WHERE id = $1::uuid AND archived_at IS NULL)", targetID,
	).Scan(&exists); err != nil {
		return TargetDetail{}, fmt.Errorf("find target: %w", err)
	}
	if !exists {
		return TargetDetail{}, ErrNotFound
	}

	detail := TargetDetail{TargetID: targetID, GeneratedAt: time.Now().UTC()}
	// L'ordre des Sources est celui de la première fenêtre lue, donc le tri par
	// nom appliqué en base ; les fenêtres suivantes le complètent.
	indexes := make(map[string]int)
	for _, window := range domain.MeasureWindows {
		perSource, err := store.bySource(ctx, window, targetID)
		if err != nil {
			return TargetDetail{}, err
		}
		total := domain.Counters{}
		for _, source := range perSource {
			total = total.Add(source.counters)
			index, known := indexes[source.sourceID]
			if !known {
				index = len(detail.Sources)
				indexes[source.sourceID] = index
				detail.Sources = append(detail.Sources, SourceMetrics{
					SourceID: source.sourceID, Name: source.name,
					Kind: source.kind, Origin: source.origin,
					LatestOutcome: source.latestOutcome, LatestObservedAt: source.latestObservedAt,
				})
			}
			detail.Sources[index].Measures = append(detail.Sources[index].Measures, source.counters.Measure(window))
			if source.latestObservedAt != nil &&
				(detail.LatestObservedAt == nil || source.latestObservedAt.After(*detail.LatestObservedAt)) {
				detail.LatestObservedAt = source.latestObservedAt
			}
		}
		detail.Measures = append(detail.Measures, total.Measure(window))
	}
	if detail.Sources == nil {
		detail.Sources = []SourceMetrics{}
	}

	hourly, err := store.hourlyByTarget(ctx, domain.WindowDay, targetID)
	if err != nil {
		return TargetDetail{}, err
	}
	for _, target := range hourly {
		if target.targetID == targetID {
			detail.Trend = target.trend()
			detail.LatencyTrend = target.latencyTrend()
		}
	}
	if detail.Trend == nil {
		detail.Trend = []float64{}
	}
	if detail.LatencyTrend == nil {
		detail.LatencyTrend = []float64{}
	}
	return detail, nil
}
