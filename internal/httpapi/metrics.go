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
	metrics         Metrics
	latencyProfiles LatencyProfiles
	logger          *slog.Logger
}

// targetDetailResponse ajoute aux mesures d'une Cible les Profils de latence de
// ses Sources, indexés par Source.
//
// Les Profils restent à côté des mesures et non dedans : une mesure est ce que
// la Cible a fait, un Profil est ce que CairnOps a appris de son habitude.
type targetDetailResponse struct {
	metrics.TargetDetail
	LatencyProfiles map[string]latencyProfileView `json:"latency_profiles"`
}

// list rend les mesures sur 24 heures de toutes les Cibles : une liste de
// Cibles se peuple d'une seule requête, quelle qu'en soit la longueur.
func (handler metricsHandler) list(w http.ResponseWriter, r *http.Request) {
	measured, err := handler.metrics.List(r.Context())
	if err != nil {
		handler.logger.Error("read target measures", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error", "code": "internal_server_error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"window": domain.WindowDay, "targets": measured})
}

// target ouvre les trois fenêtres d'une Cible et la part de chaque Source.
func (handler metricsHandler) target(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID", "code": "invalid_target_id"})
		return
	}
	detail, err := handler.metrics.Target(r.Context(), targetID)
	if errors.Is(err, metrics.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found", "code": "not_found"})
		return
	}
	if err != nil {
		handler.logger.Error("read target measures", "target_id", targetID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error", "code": "internal_server_error"})
		return
	}
	writeJSON(w, http.StatusOK, targetDetailResponse{
		TargetDetail:    detail,
		LatencyProfiles: handler.readLatencyProfiles(r.Context(), targetID),
	})
}

// readLatencyProfiles joint les Profils de latence aux mesures. Leur lecture ne
// peut pas faire échouer la réponse : un Profil est une habitude apprise, pas
// une mesure, et le détail d'une Cible doit rester consultable sans lui.
func (handler metricsHandler) readLatencyProfiles(ctx context.Context, targetID string) map[string]latencyProfileView {
	views := map[string]latencyProfileView{}
	if handler.latencyProfiles == nil {
		return views
	}
	profiles, err := handler.latencyProfiles.Profiles(ctx, targetID)
	if err != nil {
		handler.logger.Error("read target latency profiles", "target_id", targetID, "error", err)
		return views
	}
	for sourceID, profile := range profiles {
		views[sourceID] = newLatencyProfileView(profile)
	}
	return views
}
