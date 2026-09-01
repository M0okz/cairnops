package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/M0okz/cairnops/internal/bursts"
	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

type Bursts interface {
	List(context.Context, string, int) ([]bursts.Burst, error)
	Get(context.Context, string) (bursts.Burst, error)
	Acknowledge(context.Context, string, string, string) (bursts.Burst, error)
}

type burstHandler struct {
	bursts Bursts
	logger *slog.Logger
}

func (handler burstHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 500"})
			return
		}
		limit = parsed
	}
	items, err := handler.bursts.List(r.Context(), r.URL.Query().Get("status"), limit)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bursts": items})
}

func (handler burstHandler) get(w http.ResponseWriter, r *http.Request) {
	burstID := r.PathValue("burstID")
	if !validUUID(burstID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid incident burst ID"})
		return
	}
	item, err := handler.bursts.Get(r.Context(), burstID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (handler burstHandler) acknowledge(w http.ResponseWriter, r *http.Request) {
	burstID := r.PathValue("burstID")
	if !validUUID(burstID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid incident burst ID"})
		return
	}
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	item, err := handler.bursts.Acknowledge(r.Context(), burstID, principal.ID, principal.DisplayName)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (handler burstHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bursts.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), bursts.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, bursts.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "incident burst not found"})
	case errors.Is(err, bursts.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": strings.TrimPrefix(err.Error(), bursts.ErrConflict.Error()+": ")})
	default:
		if handler.logger != nil {
			handler.logger.Error("incident burst request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
