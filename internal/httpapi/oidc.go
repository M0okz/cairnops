package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
	"github.com/M0okz/cairnops/internal/oidcauth"
)

type OIDC interface {
	PublicStatus(context.Context) (oidcauth.PublicStatus, error)
	Configurations(context.Context) (oidcauth.ConfigurationSet, error)
	SaveDraft(context.Context, string, oidcauth.ConfigurationInput) (oidcauth.Configuration, error)
	Activate(context.Context) (oidcauth.Configuration, error)
	Begin(context.Context, string, string) (oidcauth.Authorization, error)
	Complete(context.Context, string, string) (oidcauth.Completion, error)
}

const oidcStateCookieLifetime = 10 * time.Minute

type oidcHandler struct {
	oidc     OIDC
	identity identityHandler
}

func (handler oidcHandler) status(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	status, err := handler.oidc.PublicStatus(r.Context())
	if err != nil {
		handler.identity.internalError(w, "read OIDC status", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handler oidcHandler) configurations(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	configurations, err := handler.oidc.Configurations(r.Context())
	if err != nil {
		handler.identity.internalError(w, "read OIDC configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, configurations)
}

func (handler oidcHandler) saveDraft(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input oidcauth.ConfigurationInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	configuration, err := handler.oidc.SaveDraft(r.Context(), principal.ID, input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": configuration})
}

func (handler oidcHandler) activate(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	configuration, err := handler.oidc.Activate(r.Context())
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": configuration})
}

func (handler oidcHandler) login(w http.ResponseWriter, r *http.Request) {
	handler.begin(w, r, "login", r.URL.Query().Get("return_to"))
}

func (handler oidcHandler) test(w http.ResponseWriter, r *http.Request) {
	handler.begin(w, r, "test", "/reglages")
}

func (handler oidcHandler) begin(w http.ResponseWriter, r *http.Request, purpose, returnTo string) {
	noStore(w)
	authorization, err := handler.oidc.Begin(r.Context(), purpose, returnTo)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	handler.setStateCookie(w, purpose, authorization.State, time.Now().Add(oidcStateCookieLifetime))
	http.Redirect(w, r, authorization.URL, http.StatusFound)
}

func (handler oidcHandler) callback(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(handler.stateCookieName())
	purpose, boundState, bindingValid := strings.Cut(cookieValue(cookie), ":")
	if err != nil || !bindingValid || (purpose != "login" && purpose != "test") || state == "" || subtle.ConstantTimeCompare([]byte(boundState), []byte(state)) != 1 {
		http.Redirect(w, r, "/?oidc_error=invalid_flow", http.StatusFound)
		return
	}
	handler.clearStateCookie(w)
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, handler.failureDestination(purpose), http.StatusFound)
		return
	}
	completion, err := handler.oidc.Complete(r.Context(), state, r.URL.Query().Get("code"))
	if err != nil {
		handler.identity.logger.Warn("OIDC callback refused", "error", err)
		http.Redirect(w, r, handler.failureDestination(purpose), http.StatusFound)
		return
	}
	if completion.Purpose != purpose {
		http.Redirect(w, r, "/?oidc_error=invalid_flow", http.StatusFound)
		return
	}
	if completion.Purpose == "test" {
		http.Redirect(w, r, "/reglages?oidc_test=success", http.StatusFound)
		return
	}
	handler.identity.setSessionCookie(w, completion.Session)
	destination := completion.ReturnTo
	if destination == "" {
		destination = "/"
	}
	http.Redirect(w, r, destination, http.StatusFound)
}

func (handler oidcHandler) stateCookieName() string {
	if handler.identity.security.secure {
		return "__Host-cairnops_oidc_state"
	}
	return "cairnops_oidc_state"
}

func (handler oidcHandler) setStateCookie(w http.ResponseWriter, purpose, state string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: handler.stateCookieName(), Value: purpose + ":" + state, Path: "/", MaxAge: int(oidcStateCookieLifetime.Seconds()),
		Expires: expiresAt, HttpOnly: true, Secure: handler.identity.security.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (handler oidcHandler) clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: handler.stateCookieName(), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: handler.identity.security.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (handler oidcHandler) failureDestination(purpose string) string {
	if purpose == "test" {
		return "/reglages?oidc_test=failed"
	}
	return "/?oidc_error=access_refused"
}

func cookieValue(cookie *http.Cookie) string {
	if cookie == nil {
		return ""
	}
	return cookie.Value
}

func (handler oidcHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, oidcauth.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), oidcauth.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, oidcauth.ErrNotConfigured):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "OIDC is not configured"})
	case errors.Is(err, oidcauth.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": strings.TrimPrefix(err.Error(), oidcauth.ErrConflict.Error()+": ")})
	case errors.Is(err, oidcauth.ErrInvalidFlow), errors.Is(err, oidcauth.ErrNotAuthorized):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "OIDC access refused"})
	default:
		handler.identity.internalError(w, "OIDC request failed", err)
	}
}
