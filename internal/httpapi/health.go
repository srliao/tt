package httpapi

import (
	"context"
	"net/http"
)

// healthResponse is the JSON shape returned by GET /health. Two strings
// instead of a single status keep the DB liveness visible separately from
// the binary's own readiness — the UI can use this to distinguish "server
// up, DB down" from "server itself is down".
type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

// handleHealth pings the DB through the injected Pinger and reports overall
// status. A DB failure returns 503 with the standard error envelope (per
// plan §4 and spec §8) so monitoring can distinguish a live-but-broken
// instance from a fully-healthy one. If the Pinger itself is nil we treat
// that as the test/dev wiring and emit a generic ok response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.pinger != nil {
		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()
		if err := s.pinger.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, CodeInternal, "database ping failed", nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", DB: "ok"})
}

// versionResponse is the JSON shape returned by GET /version. built_at is
// included to make it easy to distinguish two builds that share the same
// version tag (e.g. dev rebuilds).
type versionResponse struct {
	Version string `json:"version"`
	BuiltAt string `json:"built_at,omitempty"`
}

// handleVersion returns the version string supplied at server construction.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, versionResponse{
		Version: s.version,
		BuiltAt: s.builtAt,
	})
}
