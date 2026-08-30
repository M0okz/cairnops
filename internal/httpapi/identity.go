package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/devices"
	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

const maximumIdentityBody = 16 * 1024

type Identity interface {
	SetupStatus(context.Context) (identitymodel.Status, error)
	RenameInstance(context.Context, string) (identitymodel.Status, error)
	Initialize(context.Context, identitymodel.InitializeInput) (identitymodel.AuthenticatedSession, error)
	Login(context.Context, identitymodel.LoginInput) (identitymodel.AuthenticatedSession, error)
	Authenticate(context.Context, string) (identitymodel.Principal, error)
	Logout(context.Context, string) error
	ChangePassword(ctx context.Context, userID, current, next string) error
	SetPassword(ctx context.Context, userID, next string) (identitymodel.Principal, error)
	RecoverPassword(ctx context.Context, username, next string) (identitymodel.Principal, error)
	CountActiveSessions(ctx context.Context, userID string) (int, error)
	ListAccounts(context.Context) ([]identitymodel.Account, error)
	CreateAccount(context.Context, identitymodel.CreateAccountInput) (identitymodel.Account, error)
	UpdateAccount(ctx context.Context, actorID, userID string, input identitymodel.UpdateAccountInput) (identitymodel.Account, error)
	SetAccountActivation(ctx context.Context, actorID, userID string, active bool) (identitymodel.Account, error)
}

type identityHandler struct {
	identity Identity
	devices  DeviceManager
	security sessionSecurity
	logger   *slog.Logger
}

type sessionSecurity struct {
	cookieName string
	secure     bool
	origin     string
}

type principalContextKey struct{}

type deviceContextKey struct{}

func newSessionSecurity(publicURL string) sessionSecurity {
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}
	parsed, _ := url.Parse(publicURL)
	secure := parsed != nil && parsed.Scheme == "https"
	cookieName := "cairnops_session"
	if secure {
		cookieName = "__Host-cairnops_session"
	}
	return sessionSecurity{cookieName: cookieName, secure: secure, origin: strings.TrimSuffix(publicURL, "/")}
}

func (handler identityHandler) setupStatus(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	status, err := handler.identity.SetupStatus(r.Context())
	if err != nil {
		handler.internalError(w, "read setup status", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// renameInstance corrige le nom que porte l'instance. Il ne touche à aucune
// donnée opérationnelle : c'est l'étiquette que le rail, l'onglet et la porte
// d'entrée lisent pour dire laquelle des instances on regarde.
func (handler identityHandler) renameInstance(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	status, err := handler.identity.RenameInstance(r.Context(), input.Name)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handler identityHandler) initialize(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var input identitymodel.InitializeInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	session, err := handler.identity.Initialize(r.Context(), input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	handler.setSessionCookie(w, session)
	writeJSON(w, http.StatusCreated, session)
}

func (handler identityHandler) login(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var input identitymodel.LoginInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	session, err := handler.identity.Login(r.Context(), input)
	if err != nil {
		handler.writeError(w, err)
		return
	}
	handler.setSessionCookie(w, session)
	writeJSON(w, http.StatusOK, session)
}

func (handler identityHandler) currentSession(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	// Le nombre de sessions accompagne l'identité : les Réglages disent à chacun
	// combien de fois son compte est ouvert ailleurs, avant de proposer de tout
	// refermer. Ne pas le savoir n'empêche pas de se reconnaître.
	sessions, err := handler.identity.CountActiveSessions(r.Context(), principal.ID)
	if err != nil {
		handler.internalError(w, "count active sessions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": principal, "active_sessions": sessions})
}

func (handler identityHandler) logout(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	cookie, err := r.Cookie(handler.security.cookieName)
	if err == nil {
		if err := handler.identity.Logout(r.Context(), cookie.Value); err != nil && !errors.Is(err, identitymodel.ErrInvalidSession) {
			handler.internalError(w, "revoke session", err)
			return
		}
	}
	if deviceID, ok := r.Context().Value(deviceContextKey{}).(string); ok && deviceID != "" && handler.devices != nil {
		if err := handler.devices.RevokeSelf(r.Context(), deviceID); err != nil && !errors.Is(err, devices.ErrNotFound) {
			handler.internalError(w, "revoke current device", err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: handler.security.cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: handler.security.secure, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (handler identityHandler) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(handler.security.cookieName)
		if err == nil {
			principal, authErr := handler.identity.Authenticate(r.Context(), cookie.Value)
			if authErr != nil {
				if !errors.Is(authErr, identitymodel.ErrInvalidSession) {
					handler.logger.Error("authenticate session", "error", authErr)
				}
				unauthorizedSession(w)
				return
			}
			ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if handler.devices != nil {
			if token, ok := pairingBearer(r); ok {
				authenticated, authErr := handler.devices.Authenticate(r.Context(), token)
				if authErr == nil {
					ctx := context.WithValue(r.Context(), principalContextKey{}, authenticated.Principal)
					ctx = context.WithValue(ctx, deviceContextKey{}, authenticated.DeviceID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				if errors.Is(authErr, devices.ErrInvalidDevice) {
					unauthorizedSession(w)
					return
				}
				handler.internalError(w, "authenticate device", authErr)
				return
			}
		}
		unauthorizedSession(w)
	})
}

func (handler identityHandler) requireBrowserSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(handler.security.cookieName)
		if err != nil {
			unauthorizedSession(w)
			return
		}
		principal, err := handler.identity.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			if !errors.Is(err, identitymodel.ErrInvalidSession) {
				handler.logger.Error("authenticate browser session", "error", err)
			}
			unauthorizedSession(w)
			return
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (handler identityHandler) requireRole(role string, next http.Handler) http.Handler {
	return handler.requireAnyRole([]string{role}, next)
}

// La configuration de l'autorité externe reste sous le contrôle d'un accès
// qui ne dépend pas d'elle. Cela évite qu'un rôle reçu par OIDC puisse changer
// sa propre source d'autorisation ou activer un brouillon déjà testé.
func (handler identityHandler) requireLocalAdministrator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
		if !ok {
			unauthorizedSession(w)
			return
		}
		if principal.Role != "administrator" || principal.AuthorizationRegime != "local" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "local administrator required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (handler identityHandler) requireAnyRole(roles []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
		if !ok {
			unauthorizedSession(w)
			return
		}
		allowed := false
		for _, role := range roles {
			if principal.Role == role {
				allowed = true
				break
			}
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (handler identityHandler) requireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSuffix(r.Header.Get("Origin"), "/")
		if origin != "" && !strings.EqualFold(origin, handler.security.origin) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (handler identityHandler) setSessionCookie(w http.ResponseWriter, session identitymodel.AuthenticatedSession) {
	maxAge := max(1, int(time.Until(session.ExpiresAt).Seconds()))
	http.SetCookie(w, &http.Cookie{
		Name: handler.security.cookieName, Value: session.Token, Path: "/", MaxAge: maxAge,
		Expires: session.ExpiresAt, HttpOnly: true, Secure: handler.security.secure, SameSite: http.SameSiteStrictMode,
	})
}

func (handler identityHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identitymodel.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), identitymodel.ErrInvalidInput.Error()+": ")})
	case errors.Is(err, identitymodel.ErrAlreadyInitialized):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "installation already initialized"})
	case errors.Is(err, identitymodel.ErrNotInitialized):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "installation not initialized"})
	case errors.Is(err, identitymodel.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	default:
		handler.internalError(w, "identity request failed", err)
	}
}

func (handler identityHandler) internalError(w http.ResponseWriter, message string, err error) {
	if handler.logger != nil {
		handler.logger.Error(message, "error", err)
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

func unauthorizedSession(w http.ResponseWriter) {
	noStore(w)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}
