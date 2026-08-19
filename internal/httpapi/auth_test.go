package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatorRequiresExactBearerToken(t *testing.T) {
	t.Parallel()

	const token = "correct-token-with-at-least-32-chars"
	auth := newAuthenticator(token)
	handler := auth.require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for name, token := range map[string]string{
		"missing": "",
		"wrong":   "Bearer wrong-token",
		"correct": "Bearer " + token,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			expected := http.StatusUnauthorized
			if name == "correct" {
				expected = http.StatusNoContent
			}
			if response.Code != expected {
				t.Fatalf("expected %d, got %d", expected, response.Code)
			}
		})
	}
}
