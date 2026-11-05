package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/srliao/tt/internal/cli"
	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/drawing"
	"github.com/srliao/tt/internal/httpapi"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

type noopEnqueuer struct{}

func (noopEnqueuer) EnqueueManual(int64, int64) error { return nil }

type fixture struct {
	t      *testing.T
	server *httptest.Server
	tasks  task.Service
	tags   tag.Service
	env    map[string]string
	config string

	mu      sync.Mutex
	headers []http.Header // every request's headers, in order
	// gate, when set, plays Cloudflare Access in front of the server.
	gate http.HandlerFunc
}

// newFixture runs the real httpapi.Server on a temp DB and points the CLI
// at it through TT_URL. No config file exists unless a test writes one.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := dbtest.New(t)
	tasks := task.New(store)
	tags := tag.New(store)
	srv := httpapi.New(
		tasks, tags, script.New(store), drawing.New(store), noopEnqueuer{}, nil,
		httpapi.Options{
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			Version:  "test-1.2.3",
			Timezone: "America/New_York",
		},
	)
	f := &fixture{
		t:      t,
		tasks:  tasks,
		tags:   tags,
		config: filepath.Join(t.TempDir(), "config.toml"),
	}
	routes := srv.Routes()
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.headers = append(f.headers, r.Header.Clone())
		gate := f.gate
		f.mu.Unlock()
		if gate != nil {
			gate(w, r)
			if w.Header().Get("X-Gated") != "" {
				return
			}
		}
		routes.ServeHTTP(w, r)
	}))
	t.Cleanup(f.server.Close)
	f.env = map[string]string{"TT_URL": f.server.URL}
	return f
}

// requireAccess makes the fixture behave like Cloudflare Access: requests
// without the right service-token headers are redirected to a login page.
func (f *fixture) requireAccess(id, secret string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gate = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("CF-Access-Client-Id") == id && r.Header.Get("CF-Access-Client-Secret") == secret {
			return
		}
		w.Header().Set("X-Gated", "1")
		http.Redirect(w, r, "https://tt.example.com/cdn-cgi/access/login", http.StatusFound)
	}
}

// lastHeaders returns the headers of the most recent request.
func (f *fixture) lastHeaders() http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.headers) == 0 {
		f.t.Fatal("no request reached the server")
	}
	return f.headers[len(f.headers)-1]
}

type result struct {
	code   int
	stdout string
	stderr string
}

func (f *fixture) run(args ...string) result {
	return f.runStdin("", args...)
}

func (f *fixture) runStdin(stdin string, args ...string) result {
	f.t.Helper()
	var out, errb bytes.Buffer
	app := cli.App{
		Stdin:      strings.NewReader(stdin),
		Stdout:     &out,
		Stderr:     &errb,
		Getenv:     func(k string) string { return f.env[k] },
		ConfigPath: f.config,
	}
	code := cli.Run(context.Background(), app, args)
	return result{code: code, stdout: out.String(), stderr: errb.String()}
}

func (f *fixture) writeConfig(body string, mode os.FileMode) {
	f.t.Helper()
	if err := os.WriteFile(f.config, []byte(body), mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) createTask(title string, tagNames ...string) task.Task {
	f.t.Helper()
	ctx := context.Background()
	created, err := f.tasks.Create(ctx, task.CreateInput{Title: title})
	if err != nil {
		f.t.Fatal(err)
	}
	if len(tagNames) > 0 {
		ids, err := f.tags.Resolve(ctx, tagNames, true)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := f.tasks.SetTagsByID(ctx, created.ID, ids); err != nil {
			f.t.Fatal(err)
		}
	}
	got, err := f.tasks.Get(ctx, created.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

func (f *fixture) createTag(name string) {
	f.t.Helper()
	if _, err := f.tags.Create(context.Background(), name); err != nil {
		f.t.Fatal(err)
	}
}

// ok asserts exit 0 and decodes stdout into v.
func (r result) ok(t *testing.T, v any) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", r.code, r.stderr)
	}
	if r.stderr != "" {
		t.Fatalf("unexpected stderr: %s", r.stderr)
	}
	if v == nil {
		return
	}
	if err := json.Unmarshal([]byte(r.stdout), v); err != nil {
		t.Fatalf("stdout is not JSON (%v): %q", err, r.stdout)
	}
}

type cliErr struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// fails asserts the exit code, the error code on stderr, and empty stdout.
func (r result) fails(t *testing.T, exit int, code string) cliErr {
	t.Helper()
	if r.code != exit {
		t.Fatalf("exit = %d, want %d; stdout = %s; stderr = %s", r.code, exit, r.stdout, r.stderr)
	}
	if r.stdout != "" {
		t.Fatalf("stdout should be empty on error, got %q", r.stdout)
	}
	var e cliErr
	if err := json.Unmarshal([]byte(r.stderr), &e); err != nil {
		t.Fatalf("stderr is not JSON (%v): %q", err, r.stderr)
	}
	if e.Code != code {
		t.Fatalf("error code = %q, want %q (message %q)", e.Code, code, e.Message)
	}
	if e.Message == "" {
		t.Fatalf("error message is empty")
	}
	return e
}

func TestTaskCreate_ReturnsFullTask(t *testing.T) {
	f := newFixture(t)
	f.createTag("work")

	var got task.Task
	f.run("task", "create", "--title", "Write report", "--notes", "draft", "--due", "2026-11-20", "--tag", "work").ok(t, &got)

	if got.ID == 0 || got.Title != "Write report" || got.Notes != "draft" {
		t.Fatalf("got %+v", got)
	}
	if got.DueDate == nil || *got.DueDate != "2026-11-20" {
		t.Fatalf("due = %v", got.DueDate)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "work" {
		t.Fatalf("tags = %v", got.Tags)
	}
}
