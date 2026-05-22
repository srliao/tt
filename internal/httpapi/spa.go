package httpapi

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// NewSPAHandler returns an http.Handler that serves the SPA bundle from
// assets, falling back to index.html for client-side routes per spec §7.
//
// In production, assets is the fs.FS returned by web.Dist() — an fs.Sub of
// the embed.FS produced by //go:embed all:dist. In tests, an fstest.MapFS
// rooted the same way (no leading "dist/" prefix) is interchangeable.
//
// Behavior:
//   - GET /assets/<anything> reads the file from assets and serves it with
//     Cache-Control: public, max-age=31536000, immutable (the bundle's
//     content-hashed filenames are safe to cache forever).
//   - GET /index.html or any non-asset path reads index.html and serves it
//     with Cache-Control: no-cache so the browser re-validates on every
//     navigation.
//   - If the requested asset path is missing, returns 404 with the standard
//     error envelope so callers can distinguish "SPA not built" from a real
//     server error.
//   - If assets is nil every request returns 404 with the envelope (used as
//     a safety net; cmd/tt always passes a non-nil fs.FS in production).
func NewSPAHandler(assets fs.FS) http.Handler {
	if assets == nil {
		return http.HandlerFunc(spaNotFound)
	}
	return &spaHandler{fs: assets}
}

type spaHandler struct {
	fs fs.FS
}

// ServeHTTP implements http.Handler. The chi router strips nothing from the
// path (we wire it via r.Handle("/*", ...)), so r.URL.Path is the full path.
func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}

	// Asset path → try to serve verbatim with the long-cache header.
	if strings.HasPrefix(p, "assets/") {
		if h.serveFile(w, r, p, true) {
			return
		}
		// Missing asset → 404 envelope. Falling back to index.html for a
		// .js or .css 404 would corrupt the bundle.
		writeError(w, http.StatusNotFound, CodeNotFound, "not found", nil)
		return
	}

	// Direct request for index.html → serve with no-cache.
	if p == "index.html" {
		if h.serveFile(w, r, "index.html", false) {
			return
		}
		writeError(w, http.StatusNotFound, CodeNotFound, "not found", nil)
		return
	}

	// Anything else: SPA fallback to index.html. The client router takes
	// over from there.
	if h.serveFile(w, r, "index.html", false) {
		return
	}
	writeError(w, http.StatusNotFound, CodeNotFound, "not found", nil)
}

// serveFile reads name from h.fs and writes the response. Returns false when
// the file does not exist so the caller can decide on a fallback. Other I/O
// failures degrade to 500 with the standard envelope.
func (h *spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string, longCache bool) bool {
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	f, err := h.fs.Open(clean)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	if longCache {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if ctype := contentTypeFor(clean); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		// Headers + status already written; nothing more we can do.
		return true
	}
	return true
}

// contentTypeFor returns a content type for a filename based on extension.
// Returns the empty string when unknown so net/http's auto-detection still
// gets a chance.
func contentTypeFor(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".woff2":
		return "font/woff2"
	}
	return ""
}
