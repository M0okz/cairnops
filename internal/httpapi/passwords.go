package httpapi

import (
	"errors"
	"net/http"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

type changePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type setPasswordInput struct {
	NewPassword string `json:"new_password"`
}

type recoverPasswordInput struct {
	Username    string `json:"username"`
	NewPassword string `json:"new_password"`
}

// changeOwnPassword sert la personne connectée. Les sessions du compte sont
// révoquées, y compris celle qui porte cette requête : la réponse repose donc
// un cookie neuf plutôt que de renvoyer l'opérateur à l'écran de connexion
// après un geste qu'il vient de réussir.
func (handler identityHandler) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input changePasswordInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := handler.identity.ChangePassword(r.Context(), principal.ID, input.CurrentPassword, input.NewPassword); err != nil {
		handler.writeError(w, err)
		return
	}
	session, err := handler.identity.Login(r.Context(), identitymodel.LoginInput{
		Username: principal.Username, Password: input.NewPassword,
	})
	if err != nil {
		// Le mot de passe est bien changé ; seule la continuité de session a
		// échoué. On l'admet plutôt que de laisser croire à un échec.
		handler.clearSessionCookie(w)
		writeJSON(w, http.StatusOK, map[string]string{"status": "changed", "session": "ended"})
		return
	}
	handler.setSessionCookie(w, session)
	writeJSON(w, http.StatusOK, map[string]string{"status": "changed", "session": "renewed"})
}

// setUserPassword laisse un Administrateur réinitialiser le compte d'un tiers.
func (handler identityHandler) setUserPassword(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var input setPasswordInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	principal, err := handler.identity.SetPassword(r.Context(), r.PathValue("userID"), input.NewPassword)
	if err != nil {
		if errors.Is(err, identitymodel.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": principal})
}

// recoverPassword est la porte de secours, gardée par le Jeton d'amorçage et
// donc ouverte sans session — c'est tout son intérêt. Un compte introuvable
// répond comme un jeton invalide : cette route ne dit pas qui existe.
func (handler identityHandler) recoverPassword(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var input recoverPasswordInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if _, err := handler.identity.RecoverPassword(r.Context(), input.Username, input.NewPassword); err != nil {
		handler.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recovered"})
}

func (handler identityHandler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: handler.security.cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: handler.security.secure, SameSite: http.SameSiteStrictMode,
	})
}
