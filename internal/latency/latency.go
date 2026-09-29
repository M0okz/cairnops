// Package latency établit le Profil de latence des Contrôles natifs : la
// latence habituelle d'une Source, heure par heure, apprise sur ses propres
// Observations saines.
//
// Le Profil décrit, il ne conclut pas. Il ne produit ni Observation, ni
// Atteinte, ni Incident, ni notification : il se calcule, se persiste et se
// relit tel quel. La règle qui en dérive un seuil appartient au domaine, comme
// celle de la Disponibilité et de la Couverture.
package latency

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// refreshInterval est l'âge au-delà duquel un Profil se recalcule. Une
	// fenêtre de plusieurs semaines ne se déforme pas d'une minute à l'autre.
	refreshInterval = time.Hour

	// maximumSourcesPerPass borne une passe. Le calcul lit les Observations
	// brutes de chaque Source : il ne doit ni monopoliser PostgreSQL, ni
	// retarder l'ordonnanceur qui, lui, observe.
	maximumSourcesPerPass = 25
)

// Builder recalcule les Profils vieillis, passe après passe. Il n'observe rien
// et son échec n'empêche aucune Observation : un Profil en retard rend
// simplement une latence habituelle un peu plus ancienne.
type Builder struct {
	store    *Store
	logger   *slog.Logger
	interval time.Duration
}

func NewBuilder(pool *pgxpool.Pool, logger *slog.Logger) *Builder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Builder{store: NewStore(pool), logger: logger, interval: 5 * time.Minute}
}

// Run calcule au démarrage puis à intervalle régulier. Une passe en échec
// n'arrête pas la boucle : la suivante reprendra les mêmes Sources, l'écriture
// étant idempotente.
func (builder *Builder) Run(ctx context.Context) error {
	if _, err := builder.Build(ctx, time.Now()); err != nil {
		builder.logger.Error("latency profile pass failed", "error", err)
	}
	ticker := time.NewTicker(builder.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			built, err := builder.Build(ctx, time.Now())
			if err != nil {
				builder.logger.Error("latency profile pass failed", "error", err)
				continue
			}
			if built > 0 {
				builder.logger.Info("latency profiles computed", "sources", built)
			}
		}
	}
}

// Build recalcule les Profils dus et rend leur nombre.
//
// L'échec d'une Source n'interrompt pas la passe : les autres Profils n'ont pas
// à attendre qu'une Source cesse de poser problème, et la passe suivante la
// reprendra puisqu'elle restera due.
func (builder *Builder) Build(ctx context.Context, now time.Time) (int, error) {
	sourceIDs, err := builder.store.DueSources(ctx, now.UTC().Add(-refreshInterval), maximumSourcesPerPass)
	if err != nil {
		return 0, err
	}
	built := 0
	for _, sourceID := range sourceIDs {
		if ctx.Err() != nil {
			return built, nil
		}
		rebuilt, err := builder.store.Rebuild(ctx, sourceID, now)
		if err != nil {
			builder.logger.Error("latency profile failed", "source_id", sourceID, "error", err)
			continue
		}
		if rebuilt {
			built++
		}
	}
	return built, nil
}
