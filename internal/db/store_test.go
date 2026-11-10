package db_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/srliao/tt/internal/db"
	"github.com/srliao/tt/internal/db/migrations"
)

func TestOpenInMemoryAndMigrate(t *testing.T) {
	ctx := context.Background()

	store, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("db.Open(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := store.DB().QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	want := []string{"drawings", "script_logs", "script_runs", "scripts", "tags", "task_deps", "task_tags", "tasks"}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("got %d tables (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("table[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestTagColorHueBackfillFormula pins the SQL the 0002 migration uses to
// spread pre-existing tags across the 12-hue palette. The migration runs
// against a fresh database in tests (zero rows to backfill), so this
// stand-in inserts legacy-shaped rows (color_hue = 0) and re-runs the
// same UPDATE to confirm the formula still produces a spread. Existing
// users who upgrade with N >= 2 tags will see exactly this UPDATE applied
// once at startup.
func TestTagColorHueBackfillFormula(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for i, name := range []string{"a", "b", "c", "d", "e"} {
		_, err := store.DB().ExecContext(ctx,
			`INSERT INTO tags (id, name, color_hue) VALUES (?, ?, 0)`, i+1, name)
		if err != nil {
			t.Fatalf("insert %q: %v", name, err)
		}
	}

	if _, err := store.DB().ExecContext(ctx,
		`UPDATE tags SET color_hue = ((id - 1) % 12) * 30`); err != nil {
		t.Fatalf("backfill update: %v", err)
	}

	rows, err := store.DB().QueryContext(ctx, `SELECT name, color_hue FROM tags ORDER BY id`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer func() { _ = rows.Close() }()

	hues := map[int64]bool{}
	for rows.Next() {
		var name string
		var hue int64
		if err := rows.Scan(&name, &hue); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if hue%30 != 0 || hue < 0 || hue > 330 {
			t.Errorf("tag %q hue = %d, want a multiple of 30 in [0, 330]", name, hue)
		}
		hues[hue] = true
	}
	if len(hues) != 5 {
		t.Errorf("5 backfilled tags produced %d distinct hues, want 5", len(hues))
	}
}

func TestOpenFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.sqlite")

	store, err := db.Open(ctx, path)
	if err != nil {
		t.Fatalf("db.Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Migrations applied on the production (file-backed) path. Use the
	// real tags query (the SelectTagsHealth stub was removed when the
	// full tag service queries landed).
	got, err := store.Queries().ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("fresh database tags = %d, want 0", len(got))
	}

	if err := store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}
}

func TestTaskDepsMigrationUpgradeAndDown(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "up.sqlite")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.FS)
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 3); err != nil {
		t.Fatalf("up to 3: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO tasks (title) VALUES ('a'), ('b')`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := provider.UpTo(ctx, 4); err != nil {
		t.Fatalf("up to 4: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO task_deps (task_id, depends_on_id) VALUES (1, 2)`); err != nil {
		t.Fatalf("insert edge: %v", err)
	}
	if _, err := provider.DownTo(ctx, 3); err != nil {
		t.Fatalf("down to 3: %v", err)
	}
	var n int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='task_deps'`).Scan(&n); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if n != 0 {
		t.Fatalf("task_deps still present after down")
	}
}

func TestTaskProjectMigrationUpgradeAndDown(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "up.sqlite")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.FS)
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 4); err != nil {
		t.Fatalf("up to 4: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO tasks (title) VALUES ('a')`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	if _, err := provider.UpTo(ctx, 5); err != nil {
		t.Fatalf("up to 5: %v", err)
	}
	var project sql.NullString
	if err := sqlDB.QueryRowContext(ctx, `SELECT project FROM tasks`).Scan(&project); err != nil {
		t.Fatalf("select project: %v", err)
	}
	if project.Valid {
		t.Fatalf("existing task project = %q, want NULL", project.String)
	}
	if _, err := provider.DownTo(ctx, 4); err != nil {
		t.Fatalf("down to 4: %v", err)
	}
}

// The server's database runs in WAL mode; its VACUUM INTO copy must still
// load into memory, and writes there must not touch the file it came from.
func TestOpenSnapshot_LoadsCopyOfWALDatabaseInMemory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	src, err := db.Open(ctx, filepath.Join(dir, "tt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })
	if _, err := src.DB().ExecContext(ctx, `INSERT INTO tags (name) VALUES ('work')`); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dir, "copy.db")
	if err := src.VacuumInto(ctx, copyPath); err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := db.OpenSnapshot(ctx, image)
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	t.Cleanup(func() { _ = snap.Close() })
	if _, err := snap.DB().ExecContext(ctx, `INSERT INTO tags (name) VALUES ('home')`); err != nil {
		t.Fatalf("write to snapshot: %v", err)
	}
	var n int
	if err := snap.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tags`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("snapshot tags = %d, want 2", n)
	}
	if err := src.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tags`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("source tags = %d, want 1", n)
	}
}

func openMigrated(t *testing.T, upTo int64) (*sql.DB, *goose.Provider) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.sqlite")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.FS)
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(context.Background(), upTo); err != nil {
		t.Fatalf("up to %d: %v", upTo, err)
	}
	return sqlDB, provider
}

func mustExec(t *testing.T, d *sql.DB, q string) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func count(t *testing.T, d *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := d.QueryRowContext(context.Background(), q).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

func TestTaskAutoincrementMigrationPreservesData(t *testing.T) {
	ctx := context.Background()
	d, provider := openMigrated(t, 5)
	mustExec(t, d, `INSERT INTO tasks (title, staged_order, project) VALUES ('a', 1.5, 'p'), ('b', NULL, NULL), ('c', 2.5, 'q')`)
	mustExec(t, d, `INSERT INTO tags (name) VALUES ('t')`)
	mustExec(t, d, `INSERT INTO task_tags (task_id, tag_id) VALUES (1, 1), (3, 1)`)
	mustExec(t, d, `INSERT INTO task_deps (task_id, depends_on_id) VALUES (1, 2), (3, 1)`)

	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatalf("up to 6: %v", err)
	}

	if n := count(t, d, `SELECT COUNT(*) FROM tasks`); n != 3 {
		t.Fatalf("tasks = %d, want 3", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM task_tags`); n != 2 {
		t.Fatalf("task_tags = %d, want 2", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM task_deps`); n != 2 {
		t.Fatalf("task_deps = %d, want 2", n)
	}
	var order float64
	var project string
	if err := d.QueryRowContext(ctx, `SELECT staged_order, project FROM tasks WHERE id = 3`).Scan(&order, &project); err != nil {
		t.Fatalf("select: %v", err)
	}
	if order != 2.5 || project != "q" {
		t.Fatalf("task 3 = (%v, %q), want (2.5, q)", order, project)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM pragma_foreign_key_check`); n != 0 {
		t.Fatalf("foreign_key_check rows = %d, want 0", n)
	}
	if n := count(t, d, `PRAGMA foreign_keys`); n != 1 {
		t.Fatalf("foreign_keys = %d, want 1", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'tasks_%_idx'`); n != 6 {
		t.Fatalf("task indexes = %d, want 6", n)
	}

	mustExec(t, d, `DELETE FROM tasks WHERE id = 3`)
	if n := count(t, d, `SELECT COUNT(*) FROM task_tags`); n != 1 {
		t.Fatalf("task_tags after delete = %d, want 1", n)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM task_deps`); n != 1 {
		t.Fatalf("task_deps after delete = %d, want 1", n)
	}
}

func TestTaskIDsNotReusedAfterMigration(t *testing.T) {
	ctx := context.Background()
	d, provider := openMigrated(t, 5)
	mustExec(t, d, `INSERT INTO tasks (title) VALUES ('a'), ('b')`)
	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatalf("up to 6: %v", err)
	}
	mustExec(t, d, `DELETE FROM tasks WHERE id = 2`)
	mustExec(t, d, `INSERT INTO tasks (title) VALUES ('c')`)
	if n := count(t, d, `SELECT MAX(id) FROM tasks`); n != 3 {
		t.Fatalf("new id = %d, want 3", n)
	}
}

func TestTaskAutoincrementMigrationDown(t *testing.T) {
	ctx := context.Background()
	d, provider := openMigrated(t, 6)
	mustExec(t, d, `INSERT INTO tasks (title, project) VALUES ('a', 'p')`)
	if _, err := provider.DownTo(ctx, 5); err != nil {
		t.Fatalf("down to 5: %v", err)
	}
	if n := count(t, d, `SELECT COUNT(*) FROM tasks WHERE project = 'p'`); n != 1 {
		t.Fatalf("tasks after down = %d, want 1", n)
	}
}
