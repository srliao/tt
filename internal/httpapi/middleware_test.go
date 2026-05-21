package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRequestID_SetsHeaderAndContext(t *testing.T) {
	t.Parallel()

	var seenID string
	h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	hdr := rec.Header().Get(requestIDHeader)
	if hdr == "" {
		t.Fatalf("response header %q is empty", requestIDHeader)
	}
	if _, err := uuid.Parse(hdr); err != nil {
		t.Fatalf("response header %q = %q, not a valid uuid: %v", requestIDHeader, hdr, err)
	}
	if seenID != hdr {
		t.Fatalf("ctx id %q != header id %q", seenID, hdr)
	}
}

func TestRequestID_TrustsClientSupplied(t *testing.T) {
	t.Parallel()

	const clientID = "client-provided-id"
	h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != clientID {
			t.Fatalf("ctx id = %q, want %q", got, clientID)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	req.Header.Set(requestIDHeader, clientID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != clientID {
		t.Fatalf("response header = %q, want %q", got, clientID)
	}
}

func TestSlogLog_RecordsRequest(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mw := slogLog(logger)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "ok")
	}))

	req := httptest.NewRequest(http.MethodPost, "/things", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey, "rid-123"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rec.Code)
	}

	// Parse the (single) JSON log line.
	line := strings.TrimSpace(buf.String())
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("decode log line %q: %v", line, err)
	}
	if got := m["method"]; got != "POST" {
		t.Fatalf("method = %v", got)
	}
	if got := m["path"]; got != "/things" {
		t.Fatalf("path = %v", got)
	}
	if got, ok := m["status"].(float64); !ok || int(got) != http.StatusTeapot {
		t.Fatalf("status field = %v", m["status"])
	}
	if got := m["request_id"]; got != "rid-123" {
		t.Fatalf("request_id = %v", got)
	}
	if _, ok := m["duration_ms"]; !ok {
		t.Fatalf("duration_ms missing from log line: %s", line)
	}
}

func TestRecoverPanic_Returns500WithEnvelope(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	mw := recoverPanic(logger)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/explode", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content type = %q", got)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, rec.Body.String())
	}
	if env.Error.Code != CodeInternal {
		t.Fatalf("code = %q", env.Error.Code)
	}
	if !strings.Contains(buf.String(), "http panic") {
		t.Fatalf("panic not logged; log = %s", buf.String())
	}
}
