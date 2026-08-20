package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/incidents"
)

type Incidents interface {
	List(context.Context, string, int) ([]incidents.Incident, error)
	ListForTarget(context.Context, string, string, int) ([]incidents.Incident, error)
	Get(context.Context, string) (incidents.Incident, error)
	OpenedByDay(context.Context, int) ([]incidents.OpenedDay, error)
	Acknowledge(context.Context, string, string, string) (incidents.Incident, error)
	InvalidateSignal(context.Context, string, string, string, string, string) (incidents.Incident, error)
}

type incidentHandler struct {
	incidents Incidents
	logger    *slog.Logger
}

func (handler incidentHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 500"})
			return
		}
		limit = parsed
	}
	targetID := strings.TrimSpace(r.URL.Query().Get("target_id"))
	if targetID != "" && !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	var items []incidents.Incident
	var err error
	if targetID == "" {
		items, err = handler.incidents.List(r.Context(), r.URL.Query().Get("status"), limit)
	} else {
		items, err = handler.incidents.ListForTarget(r.Context(), r.URL.Query().Get("status"), targetID, limit)
	}
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": items})
}

// history rend le nombre d'Incidents ouverts par jour. La Vue d'ensemble la
// lit sous son compte du moment : le chiffre dit l'instant, la série dit si
// cet instant ressemble aux jours qui le précèdent.
func (handler incidentHandler) history(w http.ResponseWriter, r *http.Request) {
	days := 12
	if rawDays := strings.TrimSpace(r.URL.Query().Get("days")); rawDays != "" {
		parsed, err := strconv.Atoi(rawDays)
		if err != nil || parsed < 1 || parsed > 90 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be between 1 and 90"})
			return
		}
		days = parsed
	}
	series, err := handler.incidents.OpenedByDay(r.Context(), days)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": series})
}

func (handler incidentHandler) get(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("incidentID")
	if !validUUID(incidentID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid incident ID"})
		return
	}
	incident, err := handler.incidents.Get(r.Context(), incidentID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

func (handler incidentHandler) acknowledge(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("incidentID")
	if !validUUID(incidentID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid incident ID"})
		return
	}
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	incident, err := handler.incidents.Acknowledge(r.Context(), incidentID, principal.ID, principal.DisplayName)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

type invalidateSignalInput struct {
	Reason string `json:"reason"`
}

func (handler incidentHandler) invalidateSignal(w http.ResponseWriter, r *http.Request) {
	incidentID, signalID := r.PathValue("incidentID"), r.PathValue("signalID")
	if !validUUID(incidentID) || !validUUID(signalID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid incident or signal ID"})
		return
	}
	var input invalidateSignalInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		return
	}
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	incident, err := handler.incidents.InvalidateSignal(
		r.Context(), incidentID, signalID, principal.ID, principal.DisplayName, input.Reason,
	)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

func (handler incidentHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, incidents.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), incidents.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, incidents.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "incident not found"})
	case errors.Is(err, incidents.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": strings.TrimPrefix(err.Error(), incidents.ErrConflict.Error()+": ")})
	default:
		if handler.logger != nil {
			handler.logger.Error("incident request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
