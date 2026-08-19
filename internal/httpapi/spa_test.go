package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSPAHandlerServesIndexFallback(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<h1>CairnOps</h1><script>start()</script>"), 0o600); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/incidents/42", nil)
	response := httptest.NewRecorder()
	newSPAHandler(directory).ServeHTTP(response, request)

	if response.Code != http.StatusOK || !regexp.MustCompile(`<script nonce="[A-Za-z0-9_-]+">start`).MatchString(response.Body.String()) {
		t.Fatalf("unexpected response: %d %q", response.Code, response.Body.String())
	}
	if !regexp.MustCompile(`script-src 'self' 'nonce-[A-Za-z0-9_-]+'`).MatchString(response.Header().Get("Content-Security-Policy")) {
		t.Fatalf("missing nonce in CSP: %q", response.Header().Get("Content-Security-Policy"))
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "style-src-attr 'unsafe-hashes' '"+svelteAnnouncerStyleHash+"'") {
		t.Fatalf("missing scoped Svelte announcer style hash in CSP: %q", response.Header().Get("Content-Security-Policy"))
	}
}

func TestSPAHandlerRejectsMutatingMethods(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	response := httptest.NewRecorder()
	newSPAHandler(t.TempDir()).ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", response.Code)
	}
}

func TestSPAHandlerDoesNotFallbackForMissingAssets(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("index"), 0o600); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	response := httptest.NewRecorder()
	newSPAHandler(directory).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}
