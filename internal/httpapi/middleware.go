package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ctxKey is the unexported type used for context keys in this package. Using a
// dedicated type avoids accidental collisions with keys from other packages.
type ctxKey int

const (
	// requestIDKey stores the request id (uuid v4) for downstream loggers.
	requestIDKey ctxKey = iota
)

// requestIDHeader is the HTTP header used to surface the per-request id to
// the client. Clients can echo it back when filing bug reports.
const requestIDHeader = "X-Request-Id"

// RequestIDFromContext returns the request id attached to ctx by the
// requestID middleware, or the empty string when no id is present.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// requestID attaches a uuid v4 to every request, both as a response header
// and on the context so handlers / downstream middleware can log with it.
//
// If the client supplies their own X-Request-Id we trust it (so a reverse
// proxy can correlate requests across services); otherwise we mint a fresh
// uuid.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code for
// logging. Handlers that never call WriteHeader implicitly return 200, so we
// default status to 200 in the recorder.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

// WriteHeader records the status before delegating to the underlying writer.
// Repeated calls beyond the first are ignored by net/http already, but we
// guard the recorded value so log lines report the first (and binding) status.
func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Write delegates to the wrapped writer; if no status has been explicitly set
// we treat it as 200 (the net/http default).
func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status = http.StatusOK
		s.wrote = true
	}
	return s.ResponseWriter.Write(b)
}

// slogLog returns a middleware that writes one log line per request with the
// method, path, status, duration, and request id. The handler-supplied logger
// is captured so callers can configure structured fields globally.
func slogLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Info("http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000.0),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
		})
	}
}

// recoverPanic returns a middleware that catches panics in downstream
// handlers, logs them at error severity (with request id), and emits a
// 500 in the standard error envelope so clients see consistent error shape.
func recoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("http panic",
						slog.Any("err", rec),
						slog.String("request_id", RequestIDFromContext(r.Context())),
					)
					writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
