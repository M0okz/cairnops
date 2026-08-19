package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// SvelteKit's accessibility announcer carries this exact visually-hidden style
// attribute. Allowing its hash keeps style attributes closed for every other
// value without weakening style-src with unsafe-inline.
const svelteAnnouncerStyleHash = "sha256-S8qMpvofolR8Mpjy4kQvEm7m1q8clzU4dfDH0AmvZjo="

func newSPAHandler(directory string) http.Handler {
	root := os.DirFS(directory)
	files := http.FileServer(http.FS(root))
	index, indexErr := fs.ReadFile(root, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested == "." || requested == "" {
			requested = "index.html"
		}
		if requested == "index.html" {
			serveIndex(w, index, indexErr)
			return
		}
		if info, err := fs.Stat(root, requested); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(requested) != "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		serveIndex(w, index, indexErr)
	})
}

func serveIndex(w http.ResponseWriter, index []byte, indexErr error) {
	if indexErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "web application unavailable"})
		return
	}

	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "generate content security nonce"})
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	page := strings.ReplaceAll(string(index), "<script", `<script nonce="`+nonce+`"`)

	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'nonce-"+nonce+"'; style-src 'self'; style-src-attr 'unsafe-hashes' '"+svelteAnnouncerStyleHash+"'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}
