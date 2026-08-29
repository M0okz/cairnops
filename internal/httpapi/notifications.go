package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/notifications"
)

type Notifications interface {
	List(context.Context) ([]notifications.Channel, error)
	CreateMattermost(context.Context, string, notifications.CreateMattermostInput) (notifications.Channel, error)
	Inbox(ctx context.Context, userID string, limit int) (notifications.Inbox, error)
	MarkRead(ctx context.Context, userID string, ids []int64) (int, error)
	Dismiss(ctx context.Context, userID string) (int, error)
}

type notificationHandler struct {
	notifications Notifications
	logger        *slog.Logger
}

func (handler notificationHandler) list(w http.ResponseWriter, r *http.Request) {
	channels, err := handler.notifications.List(r.Context())
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

func (handler notificationHandler) createMattermost(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input notifications.CreateMattermostInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		return
	}
	channel, err := handler.notifications.CreateMattermost(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, channel)
}

// inbox sert la boîte de la personne connectée. L'identifiant vient de la
// session : cette route n'accepte pas qu'on lui désigne quelqu'un d'autre.
func (handler notificationHandler) inbox(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	inbox, err := handler.notifications.Inbox(r.Context(), principal.ID, notifications.InboxLimit)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inbox)
}

type markReadInput struct {
	// Sans identifiant, toute la boîte est lue : c'est le geste courant.
	IDs []int64 `json:"ids"`
}

func (handler notificationHandler) markRead(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input markReadInput
	if r.ContentLength > 0 {
		if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
			return
		}
	}
	read, err := handler.notifications.MarkRead(r.Context(), principal.ID, input.IDs)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"read": read})
}

func (handler notificationHandler) dismiss(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	dismissed, err := handler.notifications.Dismiss(r.Context(), principal.ID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dismissed": dismissed})
}

func (handler notificationHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, notifications.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), notifications.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, notifications.ErrConnection):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": strings.TrimPrefix(err.Error(), notifications.ErrConnection.Error()+": ")})
	default:
		if handler.logger != nil {
			handler.logger.Error("notification request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
