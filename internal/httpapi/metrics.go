package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/metrics"
)

type Metrics interface {
	List(context.Context) ([]metrics.TargetMetrics, error)
	Target(context.Context, string) (metrics.TargetDetail, error)
}

type metricsHandler struct {
	metrics             Metrics
	latencyProfiles     LatencyProfiles
	latencyObservations LatencyObservations
	logger              *slog.Logger
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
	profiles := handler.readLatencyProfiles(r.Context(), targetID)
	writeJSON(w, http.StatusOK, targetDetailResponse{
		TargetDetail:    detail,
		LatencyProfiles: profiles.views,
	})
}

// evaluation calcule les candidates uniquement à la demande de l'onglet
// Contrôles. Les mesures générales se rechargent plus souvent que cette lecture
// bornée des Observations brutes.
func (handler metricsHandler) evaluation(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	if _, err := handler.metrics.Target(r.Context(), targetID); errors.Is(err, metrics.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	} else if err != nil {
		handler.logger.Error("read target for latency evaluation", "target_id", targetID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	profiles := handler.readLatencyProfiles(r.Context(), targetID)
	writeJSON(w, http.StatusOK, handler.readLatencyEvaluation(r.Context(), targetID, profiles))
}

type loadedLatencyProfiles struct {
	views     map[string]latencyProfileView
	models    map[string]domain.LatencyProfile
	available bool
}

// readLatencyProfiles joint les Profils de latence aux mesures. Leur lecture ne
// peut pas faire échouer la réponse : un Profil est une habitude apprise, pas
// une mesure, et le détail d'une Cible doit rester consultable sans lui.
func (handler metricsHandler) readLatencyProfiles(ctx context.Context, targetID string) loadedLatencyProfiles {
	loaded := loadedLatencyProfiles{views: map[string]latencyProfileView{}, models: map[string]domain.LatencyProfile{}}
	if handler.latencyProfiles == nil {
		return loaded
	}
	profiles, err := handler.latencyProfiles.Profiles(ctx, targetID)
	if err != nil {
		handler.logger.Error("read target latency profiles", "target_id", targetID, "error", err)
		return loaded
	}
	for sourceID, profile := range profiles {
		loaded.views[sourceID] = newLatencyProfileView(profile)
	}
	loaded.models = profiles
	loaded.available = true
	return loaded
}

func (handler metricsHandler) readLatencyEvaluation(ctx context.Context, targetID string, loaded loadedLatencyProfiles) latencyEvaluationView {
	now := time.Now().UTC()
	view := latencyEvaluationView{WindowStart: now.Add(-domain.LatencyEvaluationWindow), WindowEnd: now, Candidates: []latencyAnomalyView{}}
	models := make(map[string]domain.LatencyProfile, len(loaded.models))
	for sourceID, profile := range loaded.models {
		// Un Profil produit avant cette version pouvait inclure les mesures
		// évaluées. Attendre son prochain recalcul avant de le consulter.
		if profile.WindowEnd.After(view.WindowStart.Add(time.Hour)) {
			continue
		}
		models[sourceID] = profile
		for _, bucket := range profile.Buckets {
			if _, established := bucket.Threshold(); established {
				view.TrainedSources++
				break
			}
		}
	}
	if handler.latencyObservations == nil || !loaded.available {
		return view
	}
	if view.TrainedSources == 0 {
		view.Available = true
		return view
	}
	const maximumObservations = 5000
	observations, truncated, err := handler.latencyObservations.RecentHealthyObservations(ctx, targetID, view.WindowStart, maximumObservations)
	if err != nil {
		handler.logger.Error("read latency evaluation", "target_id", targetID, "error", err)
		return view
	}
	view.Available = true
	view.Truncated = truncated
	view.ScannedObservations = len(observations)
	for _, anomaly := range domain.DetectLatencyAnomalies(observations, models) {
		if len(view.Candidates) == 20 {
			break
		}
		view.Candidates = append(view.Candidates, newLatencyAnomalyView(anomaly))
	}
	return view
}
