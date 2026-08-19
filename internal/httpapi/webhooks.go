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

const maximumWebhookBody = 64 * 1024

type Webhooks interface {
	Create(context.Context, string, connectors.CreateGenericWebhookInput) (connectors.GenericWebhookCreated, error)
	Receive(context.Context, string, string, connectors.GenericWebhookEvent) (connectors.WebhookReceipt, error)
	Quarantine(context.Context, string) ([]connectors.WebhookQuarantine, error)
	Approve(context.Context, string, string, string, connectors.ApproveWebhookIdentityInput) (connectors.WebhookApproval, error)
}

type webhookHandler struct {
	webhooks Webhooks
	logger   *slog.Logger
}

func (handler webhookHandler) create(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input connectors.CreateGenericWebhookInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := handler.webhooks.Create(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusCreated, created)
}

func (handler webhookHandler) receive(w http.ResponseWriter, r *http.Request) {
	var event connectors.GenericWebhookEvent
	if err := decodeJSON(w, r, maximumWebhookBody, &event, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	receipt, err := handler.webhooks.Receive(r.Context(), r.PathValue("publicID"), r.Header.Get("Authorization"), event)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusAccepted, receipt)
}

func (handler webhookHandler) quarantine(w http.ResponseWriter, r *http.Request) {
	items, err := handler.webhooks.Quarantine(r.Context(), r.PathValue("connectorID"))
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quarantine": items})
}

func (handler webhookHandler) approve(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input connectors.ApproveWebhookIdentityInput
	if err := decodeJSON(w, r, maximumConnectorBody, &input, true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	approval, err := handler.webhooks.Approve(
		r.Context(), principal.ID, r.PathValue("connectorID"), r.PathValue("quarantineID"), input,
	)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, approval)
}

func (handler webhookHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, connectors.ErrWebhookUnauthorized):
		noStore(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "webhook authentication failed"})
	case errors.Is(err, connectors.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), connectors.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, connectors.ErrWebhookNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "webhook resource not found"})
	case errors.Is(err, connectors.ErrWebhookConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "webhook identity is already bound to another target"})
	default:
		if handler.logger != nil {
			handler.logger.Error("webhook request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
