package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/M0okz/cairnops/internal/controlplane"
)

const (
	maximumAdminBody     = 64 * 1024
	maximumHeartbeatBody = 4 * 1024
)

type ControlPlane interface {
	ListTargets(context.Context) ([]controlplane.Target, error)
	CreateTarget(context.Context, controlplane.CreateTargetInput) (controlplane.Target, error)
	UpdateTarget(context.Context, string, controlplane.UpdateTargetInput) (controlplane.Target, error)
	ArchiveTarget(context.Context, string) error
	RestoreTarget(context.Context, string) (controlplane.Target, error)
	CreateSource(context.Context, string, controlplane.CreateSourceInput) (controlplane.CreatedSource, error)
	UpdateSource(context.Context, string, controlplane.UpdateSourceInput) (controlplane.Source, error)
	DeleteSource(context.Context, string) error
	ListObservations(context.Context, string, int) ([]controlplane.Observation, error)
	ReceiveHeartbeat(context.Context, string, controlplane.HeartbeatPayload) (controlplane.Observation, error)
}

type controlPlaneHandler struct {
	controlPlane ControlPlane
	logger       *slog.Logger
}

func (handler controlPlaneHandler) listTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := handler.controlPlane.ListTargets(r.Context())
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

func (handler controlPlaneHandler) createTarget(w http.ResponseWriter, r *http.Request) {
	var input controlplane.CreateTargetInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	target, err := handler.controlPlane.CreateTarget(r.Context(), input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, target)
}

func (handler controlPlaneHandler) updateTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	var input controlplane.UpdateTargetInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	target, err := handler.controlPlane.UpdateTarget(r.Context(), targetID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (handler controlPlaneHandler) archiveTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	if err := handler.controlPlane.ArchiveTarget(r.Context(), targetID); err != nil {
		handler.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler controlPlaneHandler) restoreTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	target, err := handler.controlPlane.RestoreTarget(r.Context(), targetID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (handler controlPlaneHandler) updateSource(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("sourceID")
	if !validUUID(sourceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source ID"})
		return
	}
	var input controlplane.UpdateSourceInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	source, err := handler.controlPlane.UpdateSource(r.Context(), sourceID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, source)
}

func (handler controlPlaneHandler) deleteSource(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("sourceID")
	if !validUUID(sourceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source ID"})
		return
	}
	if err := handler.controlPlane.DeleteSource(r.Context(), sourceID); err != nil {
		handler.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler controlPlaneHandler) createSource(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	var input controlplane.CreateSourceInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := handler.controlPlane.CreateSource(r.Context(), targetID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (handler controlPlaneHandler) listObservations(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("targetID")
	if !validUUID(targetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target ID"})
		return
	}
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 500"})
			return
		}
		limit = parsed
	}
	observations, err := handler.controlPlane.ListObservations(r.Context(), targetID, limit)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"observations": observations})
}

func (handler controlPlaneHandler) receiveHeartbeat(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	decodedToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decodedToken) != 32 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "heartbeat not found"})
		return
	}
	var payload controlplane.HeartbeatPayload
	if err := decodeJSON(w, r, maximumHeartbeatBody, &payload, true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	observation, err := handler.controlPlane.ReceiveHeartbeat(r.Context(), token, payload)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, observation)
}

func (handler controlPlaneHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, controlplane.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), controlplane.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, controlplane.ErrNotFound), errors.Is(err, controlplane.ErrHeartbeatNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, controlplane.ErrIntegrationOwned):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "cette Source appartient à une Intégration : réglez-la dans le produit d'origine",
		})
	case errors.Is(err, controlplane.ErrStructureBusy):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "la structure de cette Cible est verrouillée pendant son rapprochement",
		})
	default:
		if handler.logger != nil {
			handler.logger.Error("control plane request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, maximum int64, value any, optional bool) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return fmt.Errorf("content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximum)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("invalid JSON body")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON body must contain one object")
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}
