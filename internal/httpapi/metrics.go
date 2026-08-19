package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/metrics"
)

type Metrics interface {
	List(context.Context) ([]metrics.TargetMetrics, error)
	Target(context.Context, string) (metrics.TargetDetail, error)
}

type metricsHandler struct {
	metrics Metrics
	logger  *slog.Logger
}

// list rend les mesures sur 24 heures de toutes les Cibles : une liste de
// Cibles se peuple d'une seule requête, quelle qu'en soit la longueur.
func (handler metricsHandler) list(w http.ResponseWriter, r *http.Request) {
	measured, err := handler.metrics.List(r.Context())
	if err != nil {
		handler.logger.Error("read target measures", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"window": domain.WindowDay, "targets": measured})
}

// target ouvre les trois fenêtres d'une Cible et la part de chaque Source.
func (handler metricsHandler) target(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	detail, err := handler.metrics.Target(r.Context(), targetID)
	if errors.Is(err, metrics.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		handler.logger.Error("read target measures", "target_id", targetID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
