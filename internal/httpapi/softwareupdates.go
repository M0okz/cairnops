package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/softwareupdates"
)

type SoftwareUpdates interface {
	List(context.Context, string) ([]softwareupdates.Service, error)
	Get(context.Context, string) (softwareupdates.Service, error)
	Confirm(context.Context, string, string, softwareupdates.Source) error
	Config(context.Context) (softwareupdates.AIConfig, error)
	SaveConfig(context.Context, softwareupdates.AIConfig) error
}
type softwareHandler struct{ service SoftwareUpdates }

func (h softwareHandler) list(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("target_id")
	if id != "" && !validUUID(id) {
		writeJSON(w, 400, map[string]string{"error": "invalid target ID"})
		return
	}
	data, err := h.service.List(r.Context(), id)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"services": data})
}
func (h softwareHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !validUUID(id) {
		writeJSON(w, 400, map[string]string{"error": "invalid service ID"})
		return
	}
	data, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, 200, data)
}
func (h softwareHandler) confirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !validUUID(id) {
		writeJSON(w, 400, map[string]string{"error": "invalid service ID"})
		return
	}
	var input softwareupdates.Source
	if err := decodeJSON(w, r, 16384, &input, false); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid source"})
		return
	}
	actor, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	if err := h.service.Confirm(r.Context(), id, actor.ID, input); err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"confirmed": true})
}
func (h softwareHandler) config(w http.ResponseWriter, r *http.Request) {
	c, err := h.service.Config(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, 200, c)
}
func (h softwareHandler) saveConfig(w http.ResponseWriter, r *http.Request) {
	var c softwareupdates.AIConfig
	if err := decodeJSON(w, r, 16384, &c, false); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid configuration"})
		return
	}
	if err := h.service.SaveConfig(r.Context(), c); err != nil {
		h.fail(w, err)
		return
	}
	h.config(w, r)
}
func (h softwareHandler) fail(w http.ResponseWriter, err error) {
	status := 500
	message := "software update operation failed"
	if errors.Is(err, softwareupdates.ErrInvalid) {
		status = 400
		message = err.Error()
	}
	if errors.Is(err, softwareupdates.ErrNotFound) {
		status = 404
		message = "software service not found"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
