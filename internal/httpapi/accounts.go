package httpapi

import (
	"errors"
	"net/http"
	"strings"

	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

// Les gestes qui portent sur les comptes d'autrui. Ils sont tous réservés aux
// Administrateurs par le routeur, et tous réversibles : un compte se désactive,
// il ne s'efface pas.

// listAccounts alimente l'écran d'administration des comptes.
func (handler identityHandler) listAccounts(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	accounts, err := handler.identity.ListAccounts(r.Context())
	if err != nil {
		handler.internalError(w, "list accounts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": accounts})
}

// createAccount ouvre un compte avec son premier mot de passe. CairnOps n'envoie
// pas de courrier en V1 : l'Administrateur le choisit et le transmet lui-même.
func (handler identityHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var input identitymodel.CreateAccountInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	account, err := handler.identity.CreateAccount(r.Context(), input)
	if err != nil {
		handler.writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": account})
}

// updateAccount corrige un nom d'affichage ou change un rôle. L'identifiant ne
// bouge pas : il désigne la personne dans le Journal d'activité.
func (handler identityHandler) updateAccount(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	var input identitymodel.UpdateAccountInput
	if err := decodeJSON(w, r, maximumIdentityBody, &input, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	account, err := handler.identity.UpdateAccount(r.Context(), principal.ID, r.PathValue("userID"), input)
	if err != nil {
		handler.writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": account})
}

// deactivateAccount retire un compte du service ; reactivateAccount l'y remet.
// Les deux passent par la même route, comme la suspension d'un Connecteur.
func (handler identityHandler) deactivateAccount(w http.ResponseWriter, r *http.Request) {
	handler.setAccountActivation(w, r, false)
}

func (handler identityHandler) reactivateAccount(w http.ResponseWriter, r *http.Request) {
	handler.setAccountActivation(w, r, true)
}

func (handler identityHandler) setAccountActivation(w http.ResponseWriter, r *http.Request, active bool) {
	noStore(w)
	principal, ok := r.Context().Value(principalContextKey{}).(identitymodel.Principal)
	if !ok {
		unauthorizedSession(w)
		return
	}
	account, err := handler.identity.SetAccountActivation(r.Context(), principal.ID, r.PathValue("userID"), active)
	if err != nil {
		handler.writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": account})
}

func (handler identityHandler) writeAccountError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identitymodel.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
	case errors.Is(err, identitymodel.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": strings.TrimPrefix(err.Error(), identitymodel.ErrConflict.Error()+": "),
		})
	default:
		handler.writeError(w, err)
	}
}
