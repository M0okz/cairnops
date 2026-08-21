package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/M0okz/cairnops/internal/devices"
	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

type DeviceManager interface {
	CreatePairing(context.Context, string) (devices.Invitation, error)
	GetPairing(context.Context, string, string) (devices.Pairing, error)
	ClaimPairing(context.Context, string, devices.ClaimInput) (devices.PairingResult, error)
	ConfirmPairing(context.Context, string, string) (devices.Pairing, error)
	PairingResult(context.Context, string) (devices.PairingResult, error)
	CancelPairing(context.Context, string, string) error
	Authenticate(context.Context, string) (devices.AuthenticatedDevice, error)
	List(context.Context, identitymodel.Principal) ([]devices.Device, error)
	Update(context.Context, identitymodel.Principal, string, devices.UpdateInput) (devices.Device, error)
	Revoke(context.Context, identitymodel.Principal, string) error
	RevokeSelf(context.Context, string) error
}

type deviceHandler struct {
	devices DeviceManager
	logger  *slog.Logger
}

func (handler deviceHandler) createPairing(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestPrincipal(r)
	if !ok {
		unauthorizedSession(w)
		return
	}
	invitation, err := handler.devices.CreatePairing(r.Context(), principal.ID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusCreated, invitation)
}

func (handler deviceHandler) getPairing(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestPrincipal(r)
	if !ok {
		unauthorizedSession(w)
		return
	}
	pairingID := r.PathValue("pairingID")
	if !validUUID(pairingID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pairing ID"})
		return
	}
	pairing, err := handler.devices.GetPairing(r.Context(), principal.ID, pairingID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pairing)
}

func (handler deviceHandler) claimPairing(w http.ResponseWriter, r *http.Request) {
	token, ok := pairingBearer(r)
	if !ok {
		unauthorizedPairing(w)
		return
	}
	var input devices.ClaimInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		return
	}
	result, err := handler.devices.ClaimPairing(r.Context(), token, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusAccepted, result)
}

func (handler deviceHandler) confirmPairing(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestPrincipal(r)
	if !ok {
		unauthorizedSession(w)
		return
	}
	pairingID := r.PathValue("pairingID")
	if !validUUID(pairingID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pairing ID"})
		return
	}
	pairing, err := handler.devices.ConfirmPairing(r.Context(), principal.ID, pairingID)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusCreated, pairing)
}

func (handler deviceHandler) pairingResult(w http.ResponseWriter, r *http.Request) {
	token, ok := pairingBearer(r)
	if !ok {
		unauthorizedPairing(w)
		return
	}
	result, err := handler.devices.PairingResult(r.Context(), token)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, result)
}

func (handler deviceHandler) cancelPairing(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestPrincipal(r)
	if !ok {
		unauthorizedSession(w)
		return
	}
	pairingID := r.PathValue("pairingID")
	if !validUUID(pairingID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pairing ID"})
		return
	}
	if err := handler.devices.CancelPairing(r.Context(), principal.ID, pairingID); err != nil {
		handler.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler deviceHandler) list(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestPrincipal(r)
	if !ok {
		unauthorizedSession(w)
		return
	}
	items, err := handler.devices.List(r.Context(), principal)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": items})
}

func (handler deviceHandler) update(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestPrincipal(r)
	if !ok {
		unauthorizedSession(w)
		return
	}
	deviceID := r.PathValue("deviceID")
	if !validUUID(deviceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device ID"})
		return
	}
	if authenticatedDeviceID, mobile := r.Context().Value(deviceContextKey{}).(string); mobile && authenticatedDeviceID != deviceID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "a mobile identity can only change its own device"})
		return
	}
	var input devices.UpdateInput
	if err := decodeJSON(w, r, maximumAdminBody, &input, false); err != nil {
		return
	}
	device, err := handler.devices.Update(r.Context(), principal, deviceID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (handler deviceHandler) revoke(w http.ResponseWriter, r *http.Request) {
	principal, ok := requestPrincipal(r)
	if !ok {
		unauthorizedSession(w)
		return
	}
	deviceID := r.PathValue("deviceID")
	if !validUUID(deviceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device ID"})
		return
	}
	if authenticatedDeviceID, mobile := r.Context().Value(deviceContextKey{}).(string); mobile && authenticatedDeviceID != deviceID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "a mobile identity can only revoke its own device"})
		return
	}
	if err := handler.devices.Revoke(r.Context(), principal, deviceID); err != nil {
		handler.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handler deviceHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, devices.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), devices.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, devices.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device resource not found"})
	case errors.Is(err, devices.ErrPairingExpired), errors.Is(err, devices.ErrCredentialConsumed):
		writeJSON(w, http.StatusGone, map[string]string{"error": err.Error()})
	case errors.Is(err, devices.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": strings.TrimPrefix(err.Error(), devices.ErrConflict.Error()+": ")})
	default:
		if handler.logger != nil {
			handler.logger.Error("device request failed", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

func requestPrincipal(r *http.Request) (identitymodel.Principal, bool) {
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	return principal, ok
}

func pairingBearer(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

func unauthorizedPairing(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="cairnops-device-pairing"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "pairing authentication required"})
}
