package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/maintenance"
)

type Maintenances interface {
	List(context.Context, int) ([]maintenance.Maintenance, error)
	Create(context.Context, string, maintenance.CreateInput) (maintenance.Maintenance, error)
	Cancel(context.Context, string, string) (maintenance.Maintenance, error)
}

type maintenanceHandler struct {
	maintenances Maintenances
	logger       *slog.Logger
}

func (handler maintenanceHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 200"})
			return
		}
		limit = parsed
	}
	items, err := handler.maintenances.List(r.Context(), limit)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"maintenances": items})
}

func (handler maintenanceHandler) create(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input maintenance.CreateInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		return
	}
	item, err := handler.maintenances.Create(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (handler maintenanceHandler) cancel(w http.ResponseWriter, r *http.Request) {
	maintenanceID := r.PathValue("maintenanceID")
	if !validUUID(maintenanceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid maintenance ID"})
		return
	}
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	item, err := handler.maintenances.Cancel(r.Context(), maintenanceID, principal.ID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (handler maintenanceHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, maintenance.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), maintenance.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, maintenance.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "maintenance not found"})
	case errors.Is(err, maintenance.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": strings.TrimPrefix(err.Error(), maintenance.ErrConflict.Error()+": ")})
	default:
		if handler.logger != nil {
			handler.logger.Error("maintenance request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
