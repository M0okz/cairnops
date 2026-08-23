package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/reconciliation"
)

type reconciliationHandler struct {
	service reconciliation.Service
	logger  *slog.Logger
}

func (handler reconciliationHandler) suggestions(w http.ResponseWriter, r *http.Request) {
	items, err := handler.service.ListSuggestions(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": items})
}

type targetPreviewInput struct {
	PrimaryTargetID   string `json:"primary_target_id"`
	SecondaryTargetID string `json:"secondary_target_id"`
}

func (handler reconciliationHandler) previewTargets(w http.ResponseWriter, r *http.Request) {
	var input targetPreviewInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !validUUID(input.PrimaryTargetID) || !validUUID(input.SecondaryTargetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	preview, err := handler.service.PreviewTargets(r.Context(), input.PrimaryTargetID, input.SecondaryTargetID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

type sourcePreviewInput struct {
	SourceID            string `json:"source_id"`
	DestinationTargetID string `json:"destination_target_id"`
}

func (handler reconciliationHandler) previewSource(w http.ResponseWriter, r *http.Request) {
	var input sourcePreviewInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !validUUID(input.SourceID) || !validUUID(input.DestinationTargetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Source or Target ID"})
		return
	}
	preview, err := handler.service.PreviewSourceMove(r.Context(), input.SourceID, input.DestinationTargetID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler reconciliationHandler) operations(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 100"})
			return
		}
		limit = parsed
	}
	items, err := handler.service.ListOperations(r.Context(), limit)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": items})
}

func (handler reconciliationHandler) enqueue(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input reconciliation.EnqueueInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !validUUID(input.PrimaryTargetID) || !validUUID(input.SecondaryTargetID) ||
		(input.SourceID != "" && !validUUID(input.SourceID)) ||
		(input.SuggestionID != "" && !validUUID(input.SuggestionID)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid reconciliation identifier"})
		return
	}
	operation, err := handler.service.Enqueue(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, operation)
}

func (handler reconciliationHandler) reject(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	if !validUUID(r.PathValue("suggestionID")) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid suggestion ID"})
		return
	}
	var input reconciliation.RejectInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	item, err := handler.service.RejectSuggestion(r.Context(), principal.ID, r.PathValue("suggestionID"), input.Reason)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (handler reconciliationHandler) snooze(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	if !validUUID(r.PathValue("suggestionID")) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid suggestion ID"})
		return
	}
	var input reconciliation.SnoozeInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	item, err := handler.service.SnoozeSuggestion(r.Context(), principal.ID, r.PathValue("suggestionID"), input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (handler reconciliationHandler) targetActivity(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	items, err := handler.service.ListTargetActivity(r.Context(), targetID, 100)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": items})
}

func (handler reconciliationHandler) resolveTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	resolvedID, err := handler.service.ResolveTarget(r.Context(), targetID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"target_id": resolvedID})
}

func (handler reconciliationHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reconciliation.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), reconciliation.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, reconciliation.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, reconciliation.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": strings.TrimPrefix(err.Error(), reconciliation.ErrConflict.Error()+": ")})
	default:
		if handler.logger != nil {
			handler.logger.Error("target reconciliation request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
