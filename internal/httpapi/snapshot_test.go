package httpapi_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/srliao/tt/internal/task"
)

func TestSnapshot_StreamsOpenableCopyWithHeaders(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	if _, err := fx.tasks.Create(ctx, task.CreateInput{Title: "in the snapshot"}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	resp, err := http.Get(fx.server.URL + "/api/v1/snapshot.db")
	if err != nil {
		t.Fatalf("GET /snapshot.db: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/vnd.sqlite3" {
		t.Fatalf("content-type = %q", got)
	}
	if got := resp.Header.Get("X-TT-Timezone"); got != "America/New_York" {
		t.Fatalf("X-TT-Timezone = %q", got)
	}
	if got := resp.Header.Get("X-TT-Version"); got != "test-1.2.3" {
		t.Fatalf("X-TT-Version = %q", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	path := filepath.Join(t.TempDir(), "copy.db")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	copyDB, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open copy: %v", err)
	}
	t.Cleanup(func() { _ = copyDB.Close() })

	var title string
	if err := copyDB.QueryRowContext(ctx, `SELECT title FROM tasks`).Scan(&title); err != nil {
		t.Fatalf("query tasks: %v", err)
	}
	if title != "in the snapshot" {
		t.Fatalf("title = %q", title)
	}

	// The CLI compares this against its own latest migration to detect
	// version_mismatch, so the goose table must survive the copy.
	var gooseVersion int64
	if err := copyDB.QueryRowContext(ctx,
		`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`,
	).Scan(&gooseVersion); err != nil {
		t.Fatalf("query goose version: %v", err)
	}
	if gooseVersion < 4 {
		t.Fatalf("goose version = %d, want at least 4 (task deps migration)", gooseVersion)
	}

	assertDirEmptyEventually(t, fx.snapshotDir)
}

func TestSnapshot_TempFileRemovedOnClientDisconnect(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fx.server.URL+"/api/v1/snapshot.db", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /snapshot.db: %v", err)
	}
	// Hang up after the headers, without reading the body.
	cancel()
	_ = resp.Body.Close()

	assertDirEmptyEventually(t, fx.snapshotDir)
}

// assertDirEmptyEventually polls because the handler's cleanup runs on the
// server goroutine and can finish just after the client sees the body end.
func assertDirEmptyEventually(t *testing.T, dir string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read snapshot dir: %v", err)
		}
		if len(entries) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot dir still holds %d entries, first %q", len(entries), entries[0].Name())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
