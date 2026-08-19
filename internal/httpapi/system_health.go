package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/M0okz/cairnops/internal/systemhealth"
)

type SystemHealth interface {
	Snapshot(context.Context) (systemhealth.Snapshot, error)
}

type systemHealthHandler struct {
	health SystemHealth
	logger *slog.Logger
}

func (handler systemHealthHandler) snapshot(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	snapshot, err := handler.health.Snapshot(ctx)
	if err != nil {
		handler.logger.Error("read system health", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "system health unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
