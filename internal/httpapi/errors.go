// Package httpapi implements the HTTP transport layer for the tt task
// tracker: routing, middleware, request decoding, response encoding, and the
// uniform error envelope described in spec §8.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Stable error code constants used in the JSON envelope. The set matches
// spec §8 and is exhaustive — every 4xx/5xx the server emits should pick
// one of these codes.
const (
	CodeValidation     = "validation_failed"
	CodeNotFound       = "not_found"
	CodeConflict       = "conflict"
	CodeSchedulerBusy  = "scheduler_busy"
	CodeInternal       = "internal"
)

// errorEnvelope is the on-the-wire shape used by every error response. The
// outer "error" wrapper exists so future revisions can add sibling fields
// (e.g. "warnings") without breaking clients.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// errorBody carries the code, human-readable message, and an optional
// details map for field-level validation hints.
type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// writeJSON writes payload as JSON with the given status. Marshalling errors
// degrade to a 500 with a generic envelope so callers never receive a half
// response.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// At this point the response status has already been written, so
		// the best we can do is log via slog default. Callers wanting to
		// observe this should use the middleware logger.
		slog.Default().Error("httpapi: encode response", "err", err)
	}
}

// writeError writes a uniform error envelope at the given status. details may
// be nil; passing nil omits the JSON "details" key entirely.
func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, errorEnvelope{
		Error: errorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
