package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/M0okz/cairnops/internal/connectors"
	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

const maximumConnectorBody = 256 * 1024

type Connectors interface {
	List(context.Context) ([]connectors.Connector, error)
	Suspend(context.Context, string) (connectors.Connector, error)
	Resume(context.Context, string) (connectors.Connector, error)
	Delete(context.Context, string) (connectors.Removal, error)
	PreviewZabbix(context.Context, connectors.ZabbixPreviewInput) (connectors.ZabbixPreview, error)
	ImportZabbix(context.Context, string, connectors.ZabbixImportInput) (connectors.ZabbixImport, error)
	PreviewUptimeKuma(context.Context, connectors.UptimeKumaPreviewInput) (connectors.UptimeKumaPreview, error)
	ImportUptimeKuma(context.Context, string, connectors.UptimeKumaImportInput) (connectors.UptimeKumaImport, error)
	PreviewPatchMon(context.Context, connectors.PatchMonPreviewInput) (connectors.PatchMonPreview, error)
	ImportPatchMon(context.Context, string, connectors.PatchMonImportInput) (connectors.PatchMonImport, error)
}

type connectorHandler struct {
	connectors Connectors
	logger     *slog.Logger
}

func (handler connectorHandler) previewUptimeKuma(w http.ResponseWriter, r *http.Request) {
	var input connectors.UptimeKumaPreviewInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	preview, err := handler.connectors.PreviewUptimeKuma(r.Context(), input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler connectorHandler) importUptimeKuma(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input connectors.UptimeKumaImportInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := handler.connectors.ImportUptimeKuma(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (handler connectorHandler) previewPatchMon(w http.ResponseWriter, r *http.Request) {
	var input connectors.PatchMonPreviewInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	preview, err := handler.connectors.PreviewPatchMon(r.Context(), input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler connectorHandler) importPatchMon(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input connectors.PatchMonImportInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := handler.connectors.ImportPatchMon(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (handler connectorHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := handler.connectors.List(r.Context())
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": items})
}

func (handler connectorHandler) suspend(w http.ResponseWriter, r *http.Request) {
	handler.transition(w, r, handler.connectors.Suspend)
}

func (handler connectorHandler) resume(w http.ResponseWriter, r *http.Request) {
	handler.transition(w, r, handler.connectors.Resume)
}

func (handler connectorHandler) transition(w http.ResponseWriter, r *http.Request, apply func(context.Context, string) (connectors.Connector, error)) {
	connectorID := r.PathValue("connectorID")
	if !validUUID(connectorID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid connector ID"})
		return
	}
	connector, err := apply(r.Context(), connectorID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, connector)
}

func (handler connectorHandler) remove(w http.ResponseWriter, r *http.Request) {
	connectorID := r.PathValue("connectorID")
	if !validUUID(connectorID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid connector ID"})
		return
	}
	removal, err := handler.connectors.Delete(r.Context(), connectorID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, removal)
}

func (handler connectorHandler) previewZabbix(w http.ResponseWriter, r *http.Request) {
	var input connectors.ZabbixPreviewInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	preview, err := handler.connectors.PreviewZabbix(r.Context(), input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler connectorHandler) importZabbix(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input connectors.ZabbixImportInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := handler.connectors.ImportZabbix(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (handler connectorHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, connectors.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), connectors.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, connectors.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connector not found"})
	case errors.Is(err, connectors.ErrPreviewExpired):
		writeJSON(w, http.StatusGone, map[string]string{"error": "connector preview expired; run the verification again"})
	case errors.Is(err, connectors.ErrConnection):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": strings.TrimPrefix(err.Error(), connectors.ErrConnection.Error()+": ")})
	default:
		if handler.logger != nil {
			handler.logger.Error("connector request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
