package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/httpapi"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

// noopEnqueuer is a ManualRunEnqueuer that records calls. The default value
// returns nil from EnqueueManual.
type noopEnqueuer struct {
	lastScript int64
	lastRun    int64
	err        error
}

func (n *noopEnqueuer) EnqueueManual(scriptID, runID int64) error {
	n.lastScript, n.lastRun = scriptID, runID
	return n.err
}

// okPinger is a Pinger that always succeeds.
type okPinger struct{}

func (okPinger) Ping(_ context.Context) error { return nil }

// failPinger is a Pinger that always fails. Used to exercise the degraded
// health branch.
type failPinger struct{}

func (failPinger) Ping(_ context.Context) error { return errors.New("db down") }

// newTestServer assembles a Server with real services bound to an in-memory
// store. The returned httptest.Server is auto-closed by t.Cleanup. opts
// allows individual tests to override options (e.g. pass a failing Pinger).
type serverFixture struct {
	server *httptest.Server
	tasks  task.Service
	tags   tag.Service
	scripts script.Service
	enq    *noopEnqueuer
}

func newTestServer(t *testing.T, pinger httpapi.Pinger) *serverFixture {
	t.Helper()

	store := dbtest.New(t)
	tasks := task.New(store)
	tags := tag.New(store)
	scripts := script.New(store)
	enq := &noopEnqueuer{}

	if pinger == nil {
		pinger = okPinger{}
	}

	srv := httpapi.New(
		tasks, tags, scripts, enq, pinger,
		httpapi.Options{
			Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
			Version: "test-1.2.3",
			BuiltAt: "2026-05-21T00:00:00Z",
		},
	)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	return &serverFixture{
		server:  ts,
		tasks:   tasks,
		tags:    tags,
		scripts: scripts,
		enq:     enq,
	}
}

func TestHealth_OK(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, okPinger{})
	resp, err := http.Get(fx.server.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
		DB     string `json:"db"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" || body.DB != "ok" {
		t.Fatalf("body = %+v", body)
	}
	if got := resp.Header.Get("X-Request-Id"); got == "" {
		t.Fatalf("missing request id header")
	}
}

func TestHealth_DBDown(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, failPinger{})
	resp, err := http.Get(fx.server.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code == "" || env.Error.Message == "" {
		t.Fatalf("expected error envelope, got %+v", env)
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp, err := http.Get(fx.server.URL + "/api/v1/version")
	if err != nil {
		t.Fatalf("GET /version: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Version string `json:"version"`
		BuiltAt string `json:"built_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != "test-1.2.3" {
		t.Fatalf("version = %q", body.Version)
	}
	if body.BuiltAt != "2026-05-21T00:00:00Z" {
		t.Fatalf("built_at = %q", body.BuiltAt)
	}
}

func TestSPAFallback_DefaultIs404Envelope(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp, err := http.Get(fx.server.URL + "/unknown-route")
	if err != nil {
		t.Fatalf("GET /unknown-route: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
