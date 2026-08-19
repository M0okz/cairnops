package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type authenticator struct {
	digest [sha256.Size]byte
	valid  bool
}

func newAuthenticator(token string) authenticator {
	return authenticator{digest: sha256.Sum256([]byte(token)), valid: len(token) >= 32}
}

func (auth authenticator) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !auth.valid || !strings.HasPrefix(header, prefix) {
			unauthorized(w)
			return
		}
		candidate := sha256.Sum256([]byte(strings.TrimSpace(strings.TrimPrefix(header, prefix))))
		if subtle.ConstantTimeCompare(candidate[:], auth.digest[:]) != 1 {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="cairnops-bootstrap"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
}
