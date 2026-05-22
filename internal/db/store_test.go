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
