package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/indicators"
)

const maximumIndicatorBody = 2 << 20

type Indicators interface {
	Configuration(context.Context, string) (indicators.Configuration, error)
	Preview(context.Context, string) (indicators.Configuration, error)
	Apply(context.Context, string, string, indicators.ApplyInput) (indicators.Configuration, error)
	Overview(context.Context, string) ([]indicators.TargetProjection, error)
	Catalog(context.Context, string) ([]indicators.TargetProjection, error)
	Target(context.Context, string, string, string) (indicators.TargetProjection, error)
	Incident(context.Context, string, string) (indicators.IncidentProjection, error)
	Pins(context.Context, string) ([]indicators.Pin, error)
	SetPins(context.Context, string, indicators.PinInput) ([]indicators.Pin, error)
}

type indicatorHandler struct {
	indicators Indicators
	logger     *slog.Logger
}

func (handler indicatorHandler) configuration(w http.ResponseWriter, r *http.Request) {
	connectorID := r.PathValue("connectorID")
	if !validUUID(connectorID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid connector ID"})
		return
	}
	configuration, err := handler.indicators.Configuration(r.Context(), connectorID)
	if err != nil {
		handler.writeError(w, "read indicator configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (handler indicatorHandler) preview(w http.ResponseWriter, r *http.Request) {
	connectorID := r.PathValue("connectorID")
	if !validUUID(connectorID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid connector ID"})
		return
	}
	configuration, err := handler.indicators.Preview(r.Context(), connectorID)
	if err != nil {
		handler.writeError(w, "preview indicator configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (handler indicatorHandler) apply(w http.ResponseWriter, r *http.Request) {
	connectorID := r.PathValue("connectorID")
	if !validUUID(connectorID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid connector ID"})
		return
	}
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input indicators.ApplyInput
	if err := decodeJSON(w, r, maximumIndicatorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	configuration, err := handler.indicators.Apply(r.Context(), principal.ID, connectorID, input)
	if err != nil {
		handler.writeError(w, "apply indicator configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (handler indicatorHandler) overview(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	projections, err := handler.indicators.Overview(r.Context(), principal.ID)
	if err != nil {
		handler.writeError(w, "read overview indicators", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": projections, "limit": 4})
}

func (handler indicatorHandler) catalog(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	projections, err := handler.indicators.Catalog(r.Context(), principal.ID)
	if err != nil {
		handler.writeError(w, "read indicator catalog", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": projections, "limit": 4})
}

func (handler indicatorHandler) target(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = indicators.WindowDay
	}
	projection, err := handler.indicators.Target(r.Context(), principal.ID, targetID, window)
	if err != nil {
		handler.writeError(w, "read target indicators", err)
		return
	}
	writeJSON(w, http.StatusOK, projection)
}

func (handler indicatorHandler) incident(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	incidentID := r.PathValue("incidentID")
	if !validUUID(incidentID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid incident ID"})
		return
	}
	projection, err := handler.indicators.Incident(r.Context(), principal.ID, incidentID)
	if err != nil {
		handler.writeError(w, "read incident indicators", err)
		return
	}
	writeJSON(w, http.StatusOK, projection)
}

func (handler indicatorHandler) pins(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	pins, err := handler.indicators.Pins(r.Context(), principal.ID)
	if err != nil {
		handler.writeError(w, "read indicator pins", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pins": pins, "limit": 4})
}

func (handler indicatorHandler) setPins(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input indicators.PinInput
	if err := decodeJSON(w, r, 16*1024, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	pins, err := handler.indicators.SetPins(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, "save indicator pins", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pins": pins, "limit": 4})
}

func (handler indicatorHandler) writeError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, indicators.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, indicators.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		handler.logger.Error(operation, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
