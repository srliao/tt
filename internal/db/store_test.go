package db_test

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/srliao/tt/internal/db"
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

	want := []string{"script_logs", "script_runs", "scripts", "tags", "task_tags", "tasks"}
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
