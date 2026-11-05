package cli_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/srliao/tt/internal/task"
)

func TestSnapshotPull_WritesDBAndSidecar(t *testing.T) {
	f := newFixture(t)
	f.createTask("in the snapshot")
	pulledAt := time.Date(2026, 11, 5, 14, 30, 0, 0, time.UTC)
	f.now = func() time.Time { return pulledAt }
	out := filepath.Join(t.TempDir(), "snap.db")

	var got map[string]any
	f.run("snapshot", "pull", "-o", out).ok(t, &got)

	if got["path"] != out {
		t.Fatalf("output = %v", got)
	}
	raw, err := os.ReadFile(out + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]string
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"timezone": "America/New_York", "server_version": "test-1.2.3", "pulled_at": "2026-11-05T14:30:00Z"}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("sidecar %s = %q, want %q", k, meta[k], v)
		}
	}

	db, err := sql.Open("sqlite", "file:"+out+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var title string
	if err := db.QueryRow(`SELECT title FROM tasks`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "in the snapshot" {
		t.Fatalf("title = %q", title)
	}
}

func TestSnapshotPull_RequiresOutput(t *testing.T) {
	f := newFixture(t)
	f.run("snapshot", "pull").fails(t, 1, "usage")
}

// pullSnapshot saves the fixture server's current DB via the CLI.
func (f *fixture) pullSnapshot() string {
	f.t.Helper()
	out := filepath.Join(f.t.TempDir(), "snap.db")
	f.run("snapshot", "pull", "-o", out).ok(f.t, nil)
	return out
}

func TestSnapshot_OlderGooseVersionIsVersionMismatch(t *testing.T) {
	f := newFixture(t)
	path := f.pullSnapshot()

	// Roll the saved copy back one migration, as an older server would ship.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM goose_db_version WHERE version_id = (SELECT MAX(version_id) FROM goose_db_version)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	e := f.runStdin(`{"tasks": [{"ref": "a", "title": "A"}]}`, "plan", "check", "--snapshot", path).fails(t, 4, "version_mismatch")
	if !strings.Contains(e.Message, "upgrade the tt server") {
		t.Fatalf("message should say which side to upgrade: %q", e.Message)
	}
	if e.Details["snapshot_version"] == nil || e.Details["cli_version"] == nil {
		t.Fatalf("details = %v", e.Details)
	}
}

// The check runs against a local copy: the server is never written.
func TestPlanCheck_RunsAgainstFreshSnapshotWithoutWriting(t *testing.T) {
	f := newFixture(t)
	existing := f.createTask("existing")
	doc := `{"tasks": [{"ref": "a", "title": "A"}], "deps": [{"task": "a", "depends_on": ` + id(existing.ID) + `}]}`

	var got task.PlanResult
	f.runStdin(doc, "plan", "check").ok(t, &got)

	if got.IDs["a"] == 0 || len(got.Tasks) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got.Tasks[0].Title != "A" || len(got.Tasks[0].BlockedBy) != 1 || got.Tasks[0].BlockedBy[0] != existing.ID {
		t.Fatalf("plan task = %+v", got.Tasks[0])
	}
	all, err := f.tasks.List(context.Background(), task.FilterSort{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("server has %d tasks after check, want 1", len(all))
	}
}
